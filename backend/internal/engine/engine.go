package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minkicc/lite-router/backend/internal/config"
	"github.com/minkicc/lite-router/backend/internal/routing"
)

type HealthStatus string

const (
	StatusUnknown   HealthStatus = "unknown"
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusDisabled  HealthStatus = "disabled"
)

type ChannelState struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	BaseURL              string            `json:"base_url"`
	Models               []string          `json:"models"`
	ModelMappings        map[string]string `json:"model_mappings,omitempty"`
	Priority             int               `json:"priority"`
	Group                string            `json:"group"`
	Weight               int               `json:"weight"`
	Price                float64           `json:"price"`
	Enabled              bool              `json:"enabled"`
	APIKeySet            bool              `json:"api_key_set"`
	Status               HealthStatus      `json:"status"`
	ResponseTimeMS       int64             `json:"response_time_ms"`
	ConsecutiveFailures  int               `json:"consecutive_failures"`
	ConsecutiveSuccesses int               `json:"consecutive_successes"`
	LastStatusCode       int               `json:"last_status_code"`
	LastChecked          int64             `json:"last_checked"`
	LastError            string            `json:"last_error,omitempty"`
}

type Selection struct {
	Channel       config.Channel `json:"channel"`
	UpstreamModel string         `json:"upstream_model"`
}

type healthState struct {
	status               HealthStatus
	responseTime         time.Duration
	consecutiveFailures  int
	consecutiveSuccesses int
	lastStatusCode       int
	lastChecked          time.Time
	lastError            string
	nextCheck            time.Time
}

type channelRuntime struct {
	channel config.Channel
	health  healthState
}

type Engine struct {
	mu           sync.RWMutex
	cfg          config.Config
	runtimes     map[string]*channelRuntime
	checkMu      sync.Mutex
	checking     map[string]bool
	proxyClient  *http.Client
	healthClient *http.Client
}

func (e *Engine) ProxyClient() *http.Client {
	return e.proxyClient
}

func New(cfg *config.Config) (*Engine, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	proxyTransport := http.DefaultTransport.(*http.Transport).Clone()
	proxyTransport.ResponseHeaderTimeout = 30 * time.Second
	e := &Engine{
		runtimes:     map[string]*channelRuntime{},
		checking:     map[string]bool{},
		proxyClient:  &http.Client{Transport: proxyTransport},
		healthClient: &http.Client{},
	}
	e.ReplaceConfig(cfg)
	return e, nil
}

func (e *Engine) ReplaceConfig(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	next := map[string]*channelRuntime{}
	e.mu.Lock()
	old := e.runtimes
	for i := range cfg.Channels {
		ch := cfg.Channels[i]
		rt := &channelRuntime{channel: ch}
		if old != nil {
			if prev, ok := old[ch.ID]; ok {
				rt.health = prev.health
				rt.health.nextCheck = time.Now()
			}
		}
		if !ch.IsEnabled() {
			rt.health.status = StatusDisabled
		} else if rt.health.status == "" {
			rt.health.status = StatusUnknown
		}
		if rt.health.nextCheck.IsZero() {
			rt.health.nextCheck = time.Now()
		}
		next[ch.ID] = rt
	}
	e.runtimes = next
	e.cfg = *cfg.Clone()
	e.mu.Unlock()
	return nil
}

func (e *Engine) Config() *config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg.Clone()
}

func (e *Engine) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.checkDue()
			}
		}
	}()
}

func (e *Engine) checkDue() {
	now := time.Now()
	e.mu.RLock()
	due := make([]string, 0)
	for id, rt := range e.runtimes {
		if !rt.channel.IsEnabled() || rt.health.status == StatusDisabled {
			continue
		}
		if !rt.health.nextCheck.After(now) {
			due = append(due, id)
		}
	}
	e.mu.RUnlock()
	for _, id := range due {
		go e.CheckNow(id)
	}
}

func (e *Engine) CheckAll() {
	e.mu.RLock()
	ids := make([]string, 0, len(e.runtimes))
	for id := range e.runtimes {
		ids = append(ids, id)
	}
	e.mu.RUnlock()
	for _, id := range ids {
		e.CheckNow(id)
	}
}

func (e *Engine) CheckNow(id string) {
	e.mu.RLock()
	rt, ok := e.runtimes[id]
	e.mu.RUnlock()
	if !ok || !rt.channel.IsEnabled() {
		return
	}
	if !e.beginCheck(id) {
		return
	}
	defer e.endCheck(id)

	health := rt.channel.EffectiveHealth(e.currentGlobalHealth())
	path := strings.TrimSpace(health.Path)
	if path == "" {
		path = "/v1/models"
	}
	status, latency, err := e.probe(rt.channel, path, health.TimeoutSeconds)
	if err != nil {
		e.recordFailure(id, status, latency, err)
		return
	}
	if status >= 200 && status < 300 {
		e.recordSuccess(id, status, latency)
		return
	}
	if status == http.StatusNotFound && (path == "/v1/models" || path == "/models") {
		alt := "/models"
		if path == "/models" {
			alt = "/v1/models"
		}
		altStatus, altLatency, altErr := e.probe(rt.channel, alt, health.TimeoutSeconds)
		if altErr == nil && altStatus >= 200 && altStatus < 300 {
			e.recordSuccess(id, altStatus, altLatency)
			return
		}
	}
	e.recordFailure(id, status, latency, nil)
}

func (e *Engine) currentGlobalHealth() config.HealthCheck {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg.HealthCheck
}

func (e *Engine) beginCheck(id string) bool {
	e.checkMu.Lock()
	defer e.checkMu.Unlock()
	if e.checking[id] {
		return false
	}
	e.checking[id] = true
	return true
}

func (e *Engine) endCheck(id string) {
	e.checkMu.Lock()
	defer e.checkMu.Unlock()
	delete(e.checking, id)
}

func (e *Engine) probe(ch config.Channel, path string, timeoutSeconds int) (int, time.Duration, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	target := BuildURL(ch.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, 0, err
	}
	if strings.TrimSpace(ch.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	}
	for k, v := range ch.Headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := e.healthClient.Do(req)
	if err != nil {
		return 0, time.Since(start), err
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	_ = drainBody(resp)
	return status, time.Since(start), nil
}

func drainBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	_, err := io.Copy(io.Discard, resp.Body)
	return err
}

func (e *Engine) recordSuccess(id string, status int, latency time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rt, ok := e.runtimes[id]
	if !ok {
		return
	}
	cfg := e.cfg
	global := cfg.HealthCheck
	health := rt.channel.EffectiveHealth(global)
	rt.health.consecutiveSuccesses++
	rt.health.consecutiveFailures = 0
	rt.health.lastStatusCode = status
	rt.health.lastChecked = time.Now()
	rt.health.lastError = ""
	rt.health.responseTime = latency
	threshold := health.SuccessThreshold
	if threshold <= 0 {
		threshold = 1
	}
	if rt.health.consecutiveSuccesses >= threshold {
		rt.health.status = StatusHealthy
	}
	rt.health.nextCheck = time.Now().Add(time.Duration(health.IntervalSeconds) * time.Second)
}

func (e *Engine) recordFailure(id string, status int, latency time.Duration, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rt, ok := e.runtimes[id]
	if !ok {
		return
	}
	cfg := e.cfg
	global := cfg.HealthCheck
	health := rt.channel.EffectiveHealth(global)
	rt.health.consecutiveFailures++
	rt.health.consecutiveSuccesses = 0
	rt.health.lastStatusCode = status
	rt.health.lastChecked = time.Now()
	rt.health.responseTime = latency
	if err != nil {
		rt.health.lastError = err.Error()
	} else {
		rt.health.lastError = fmt.Sprintf("status %d", status)
	}
	threshold := health.FailureThreshold
	if threshold <= 0 {
		threshold = 1
	}
	if rt.health.consecutiveFailures >= threshold {
		rt.health.status = StatusUnhealthy
	}
	rt.health.nextCheck = time.Now().Add(time.Duration(health.IntervalSeconds) * time.Second)
}

func (e *Engine) RecordAttempt(id string, status int, err error, latency time.Duration) {
	if err != nil {
		e.recordFailure(id, status, latency, err)
		return
	}
	if status >= 200 && status < 300 {
		e.recordSuccess(id, status, latency)
		return
	}
	if routing.IsRetryableStatus(status) {
		e.recordFailure(id, status, latency, nil)
	}
}

func IsRetryableStatus(status int) bool {
	return routing.IsRetryableStatus(status)
}

func (e *Engine) State() []ChannelState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ids := make([]string, 0, len(e.runtimes))
	for id := range e.runtimes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ChannelState, 0, len(ids))
	for _, id := range ids {
		rt := e.runtimes[id]
		ch := rt.channel
		st := ChannelState{
			ID:                   ch.ID,
			Name:                 ch.Name,
			BaseURL:              ch.BaseURL,
			Models:               append([]string(nil), ch.Models...),
			ModelMappings:        cloneStringMap(ch.ModelMappings),
			Priority:             ch.Priority,
			Group:                ch.Group,
			Weight:               ch.Weight,
			Price:                ch.Price,
			Enabled:              ch.IsEnabled(),
			APIKeySet:            strings.TrimSpace(ch.APIKey) != "",
			Status:               rt.health.status,
			ResponseTimeMS:       rt.health.responseTime.Milliseconds(),
			ConsecutiveFailures:  rt.health.consecutiveFailures,
			ConsecutiveSuccesses: rt.health.consecutiveSuccesses,
			LastStatusCode:       rt.health.lastStatusCode,
			LastChecked:          rt.health.lastChecked.Unix(),
			LastError:            rt.health.lastError,
		}
		out = append(out, st)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (e *Engine) Select(requestedModel string, excluded map[string]bool) (*Selection, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	e.mu.RLock()
	cfg := e.cfg.Clone()
	routes := make([]routing.Route, 0, len(e.runtimes))
	for _, rt := range e.runtimes {
		routes = append(routes, routeFromChannel(rt.channel, rt.health.status, rt.health.responseTime))
	}
	e.mu.RUnlock()

	mappings := make([]routing.ModelMapping, 0, len(cfg.ModelMappings))
	for _, m := range cfg.ModelMappings {
		mappings = append(mappings, routing.ModelMapping{
			PlatformModel: m.PlatformModel,
			UpstreamModel: m.UpstreamModel,
			ChannelID:     m.ChannelID,
			Enabled:       m.Enabled,
		})
	}
	groups := make([]routing.Group, 0, len(cfg.Groups))
	for _, g := range cfg.Groups {
		groups = append(groups, routing.Group{Name: g.Name, Priority: g.Priority})
	}

	sel, err := routing.Select(requestedModel, routes, mappings, groups, excluded)
	if err != nil {
		return nil, err
	}
	channel, ok := e.FindChannel(sel.Route.ID)
	if !ok {
		return nil, errors.New("selected channel not found: " + sel.Route.ID)
	}
	return &Selection{Channel: channel, UpstreamModel: sel.UpstreamModel}, nil
}

func routeFromChannel(ch config.Channel, status HealthStatus, latency time.Duration) routing.Route {
	return routing.Route{
		ID:            ch.ID,
		Name:          ch.Name,
		Models:        append([]string(nil), ch.Models...),
		ModelMappings: cloneStringMap(ch.ModelMappings),
		Priority:      ch.Priority,
		Group:         ch.Group,
		Weight:        ch.Weight,
		Price:         ch.Price,
		Enabled:       ch.IsEnabled(),
		MaxRetries:    ch.MaxRetries,
		Status:        routingStatus(status),
		ResponseTime:  latency,
	}
}

func routingStatus(status HealthStatus) routing.Status {
	switch status {
	case StatusHealthy:
		return routing.StatusHealthy
	case StatusUnhealthy:
		return routing.StatusUnhealthy
	case StatusDisabled:
		return routing.StatusDisabled
	default:
		return routing.StatusUnknown
	}
}

func mappingEnabled(m config.ModelMapping) bool {
	return m.Enabled == nil || *m.Enabled
}

func (e *Engine) AvailableModels() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	seen := map[string]bool{}
	for _, rt := range e.runtimes {
		if !rt.channel.IsEnabled() || rt.health.status == StatusDisabled || rt.health.status == StatusUnhealthy {
			continue
		}
		for _, model := range rt.channel.Models {
			model = strings.TrimSpace(model)
			if model == "" || model == "*" {
				continue
			}
			seen[model] = true
		}
		for platform := range rt.channel.ModelMappings {
			if platform = strings.TrimSpace(platform); platform != "" {
				seen[platform] = true
			}
		}
	}
	for _, m := range e.cfg.ModelMappings {
		if mappingEnabled(m) {
			seen[strings.TrimSpace(m.PlatformModel)] = true
		}
	}
	for platform := range e.cfg.FallbackModels {
		if platform = strings.TrimSpace(platform); platform != "" {
			seen[platform] = true
		}
	}
	out := make([]string, 0, len(seen))
	for model := range seen {
		if model != "" {
			out = append(out, model)
		}
	}
	sort.Strings(out)
	return out
}

func (e *Engine) FindChannel(id string) (config.Channel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rt, ok := e.runtimes[id]
	if !ok {
		return config.Channel{}, false
	}
	return rt.channel, true
}

func (e *Engine) Tokens() []config.Token {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]config.Token, len(e.cfg.Tokens))
	copy(out, e.cfg.Tokens)
	return out
}

func (e *Engine) TokenName(id string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, t := range e.cfg.Tokens {
		if t.ID == id {
			return strings.TrimSpace(t.Name)
		}
	}
	return ""
}

func (e *Engine) Authorize(bearer string) (config.Token, bool) {
	e.mu.RLock()
	cfg := e.cfg
	e.mu.RUnlock()

	if cfg.NoAuth {
		return config.Token{}, true
	}

	bearer = strings.TrimSpace(bearer)
	legacy := strings.TrimSpace(cfg.AuthToken)
	if bearer == "" {
		return config.Token{}, false
	}
	if legacy != "" && bearer == legacy {
		return config.Token{ID: "legacy", Name: "legacy"}, true
	}
	for _, t := range cfg.Tokens {
		if t.IsEnabled() && strings.TrimSpace(t.Token) != "" && t.Token == bearer {
			return t, true
		}
	}
	return config.Token{}, false
}

func (e *Engine) AddToken(name string) (config.Token, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "token"
	}
	id, err := randomHex(8)
	if err != nil {
		return config.Token{}, err
	}
	secret, err := randomHex(24)
	if err != nil {
		return config.Token{}, err
	}
	enabled := true
	t := config.Token{
		ID:        "tok_" + id,
		Name:      name,
		Token:     "lr-" + secret,
		Enabled:   &enabled,
		CreatedAt: time.Now().Unix(),
	}
	e.mu.Lock()
	e.cfg.Tokens = append(e.cfg.Tokens, t)
	e.mu.Unlock()
	return t, nil
}

func (e *Engine) RemoveToken(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.cfg.Tokens {
		if e.cfg.Tokens[i].ID == id {
			e.cfg.Tokens = append(e.cfg.Tokens[:i], e.cfg.Tokens[i+1:]...)
			return true
		}
	}
	return false
}

func (e *Engine) UpdateToken(id string, name *string, enabled *bool) (config.Token, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.cfg.Tokens {
		if e.cfg.Tokens[i].ID == id {
			if name != nil {
				n := strings.TrimSpace(*name)
				if n != "" {
					e.cfg.Tokens[i].Name = n
				}
			}
			if enabled != nil {
				v := *enabled
				e.cfg.Tokens[i].Enabled = &v
			}
			return e.cfg.Tokens[i], true
		}
	}
	return config.Token{}, false
}

func (e *Engine) RecordTokenRequest(id string) bool {
	if id == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.cfg.Tokens {
		if e.cfg.Tokens[i].ID == id {
			e.cfg.Tokens[i].RequestCount++
			e.cfg.Tokens[i].LastUsedAt = time.Now().Unix()
			return true
		}
	}
	return false
}

func (e *Engine) RecordTokenTokens(id string, prompt, completion int64) bool {
	if id == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.cfg.Tokens {
		if e.cfg.Tokens[i].ID == id {
			e.cfg.Tokens[i].PromptTokens += prompt
			e.cfg.Tokens[i].CompletionTokens += completion
			return true
		}
	}
	return false
}

func hasEnabledToken(tokens []config.Token) bool {
	for _, t := range tokens {
		if t.IsEnabled() && strings.TrimSpace(t.Token) != "" {
			return true
		}
	}
	return false
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (e *Engine) MarshalConfig() ([]byte, error) {
	return json.MarshalIndent(e.Config(), "", "  ")
}
