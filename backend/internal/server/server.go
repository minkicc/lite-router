package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/minkicc/mkswitch/backend/internal/config"
	"github.com/minkicc/mkswitch/backend/internal/engine"
	"github.com/minkicc/mkswitch/backend/internal/routing"
)

const maxRequestBody = 64 << 20

type Server struct {
	engine  *engine.Engine
	cfgPath string
	ui      http.Handler
	logger  *log.Logger
	saveMu  sync.Mutex
	usage   *usageStore
}

type usageInfo struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

func New(eng *engine.Engine, cfgPath string, uiFS fs.FS, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	usageLimit := 500
	if cfg := eng.Config(); cfg != nil && cfg.UsageMaxRecords > 0 {
		usageLimit = cfg.UsageMaxRecords
	}
	s := &Server{
		engine:  eng,
		cfgPath: cfgPath,
		ui:      http.FileServerFS(uiFS),
		logger:  logger,
		usage:   newUsageStore(cfgPath, usageLimit),
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleProxy)
	mux.HandleFunc("POST /v1/responses", s.handleProxy)
	mux.HandleFunc("POST /v1/images/generations", s.handleProxy)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("GET /api/usage", s.handleUsage)
	mux.HandleFunc("GET /api/usage/events", s.handleUsageEvents)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("POST /api/probe_models", s.handleProbeModels)
	mux.HandleFunc("GET /api/tokens", s.handleListTokens)
	mux.HandleFunc("POST /api/tokens", s.handleCreateToken)
	mux.HandleFunc("PUT /api/tokens/{id}", s.handleUpdateToken)
	mux.HandleFunc("DELETE /api/tokens/{id}", s.handleDeleteToken)
	mux.HandleFunc("POST /api/reload", s.handleReload)
	mux.HandleFunc("POST /api/check", s.handleCheckAll)
	mux.HandleFunc("POST /api/check/{id}", s.handleCheckOne)
	mux.Handle("/", s.ui)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	states := s.engine.State()
	unhealthy := 0
	for _, st := range states {
		if st.Status == engine.StatusUnhealthy {
			unhealthy++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"channels":           len(states),
		"unhealthy_channels": unhealthy,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticate(r); !ok {
		writeAuthError(w)
		return
	}
	models := s.engine.AvailableModels()
	data := make([]map[string]any, 0, len(models))
	now := time.Now().Unix()
	for _, model := range models {
		data = append(data, map[string]any{
			"id":       model,
			"object":   "model",
			"created":  now,
			"owned_by": "mkswitch",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (s *Server) authenticate(r *http.Request) (string, bool) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	auth = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	t, ok := s.engine.Authorize(auth)
	if !ok {
		return "", false
	}
	return t.ID, true
}

func writeAuthError(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]any{
		"error": map[string]any{
			"message": "invalid api key",
			"type":    "invalid_request_error",
			"code":    "invalid_api_key",
		},
	})
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := s.authenticate(r)
	if !ok {
		writeAuthError(w)
		return
	}
	s.recordRequest(tokenID)

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		writeProxyError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "")
		return
	}
	requestedModel, err := requestModel(body)
	if err != nil || strings.TrimSpace(requestedModel) == "" {
		writeProxyError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "")
		return
	}

	start := time.Now()
	excluded := map[string]bool{}
	var lastErr error
	var lastForwardErr error
	var lastSelection *engine.Selection
	for {
		if err := r.Context().Err(); err != nil {
			lastErr = err
			break
		}
		selection, selectErr := s.engine.Select(requestedModel, excluded)
		if selectErr != nil {
			lastErr = selectErr
			break
		}
		lastSelection = selection
		maxRetries := selection.Channel.MaxRetries
		if maxRetries < 1 {
			// A transient upstream failure gets one automatic retry even when
			// the channel has no explicit retry setting.
			maxRetries = 1
		}
		attempts := 0
		for {
			if err := r.Context().Err(); err != nil {
				lastErr = err
				break
			}
			retry, usage, forwardErr := s.forward(w, r, selection, body)
			if forwardErr != nil {
				lastErr = forwardErr
				lastForwardErr = forwardErr
			}
			if r.Context().Err() != nil {
				lastErr = r.Context().Err()
				break
			}
			if !retry {
				if forwardErr != nil {
					s.recordUsage(r, tokenID, requestedModel, selection, usageInfo{}, start, false, errorText(forwardErr))
					return
				}
				s.recordTokens(tokenID, usage)
				s.recordUsage(r, tokenID, requestedModel, selection, usage, start, true, "")
				return
			}
			attempts++
			if !retryOnSameChannel(forwardErr) || attempts > maxRetries {
				excluded[selection.Channel.ID] = true
				s.engine.Cooldown(selection.Channel.ID)
				break
			}
		}
	}
	errMsg := errorText(lastForwardErr)
	if errMsg == "" {
		errMsg = errorText(lastErr)
	}
	s.recordUsage(r, tokenID, requestedModel, lastSelection, usageInfo{}, start, false, errMsg)
	writeProxyError(w, http.StatusBadGateway, "all channels failed", "upstream_unavailable", errorText(lastErr))
}

type retryDecisionError struct {
	err  error
	same bool
}

func (e *retryDecisionError) Error() string { return e.err.Error() }
func (e *retryDecisionError) Unwrap() error { return e.err }

func retryOnSameChannel(err error) bool {
	var decision *retryDecisionError
	return errors.As(err, &decision) && decision.same
}

func (s *Server) recordUsage(r *http.Request, tokenID, model string, selection *engine.Selection, usage usageInfo, start time.Time, success bool, errText string) {
	if s.usage == nil {
		return
	}
	rec := usageRecord{
		Time:      time.Now().Unix(),
		TokenID:   tokenID,
		TokenName: s.engine.TokenName(tokenID),
		Model:     model,
		ElapsedMS: time.Since(start).Milliseconds(),
		Success:   success,
		Endpoint:  r.URL.Path,
		Error:     errText,
	}
	if selection != nil {
		rec.UpstreamModel = selection.UpstreamModel
		rec.ChannelID = selection.Channel.ID
		rec.ChannelName = selection.Channel.Name
	}
	if success {
		rec.PromptTokens = usage.PromptTokens
		rec.CompletionTokens = usage.CompletionTokens
	}
	s.usage.add(rec)
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, selection *engine.Selection, body []byte) (bool, usageInfo, error) {
	ch := selection.Channel
	rewritten, stream, err := rewriteRequestBody(body, selection.UpstreamModel)
	if err != nil {
		return true, usageInfo{}, &retryDecisionError{err: err, same: false}
	}
	target := engine.BuildURL(ch.BaseURL, r.URL.Path)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(rewritten))
	if err != nil {
		return true, usageInfo{}, &retryDecisionError{err: err, same: false}
	}
	copyRequestHeaders(req.Header, r.Header)
	if strings.TrimSpace(ch.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	}
	for k, v := range ch.Headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := s.engine.ProxyClient().Do(req)
	if err != nil {
		s.engine.RecordAttempt(ch.ID, 0, err, time.Since(start))
		if r.Context().Err() != nil || errors.Is(err, context.Canceled) {
			return true, usageInfo{}, &retryDecisionError{err: err, same: false}
		}
		return true, usageInfo{}, &retryDecisionError{err: err, same: true}
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	if routing.IsRetryableError(resp.StatusCode, nil) {
		retryErr := &retryDecisionError{
			err:  fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			same: routing.IsRetryableOnSameRoute(resp.StatusCode),
		}
		// Do not consume a potentially large error response before switching.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		s.engine.RecordAttempt(ch.ID, resp.StatusCode, nil, latency)
		return true, usageInfo{}, retryErr
	}

	if stream {
		copyResponseHeaders(w.Header(), resp.Header)
		flusher, _ := w.(http.Flusher)
		var captured bytes.Buffer
		tee := io.TeeReader(resp.Body, &captured)
		buf := make([]byte, 32<<10)
		wrote := false
		for {
			n, readErr := tee.Read(buf)
			if n > 0 {
				if !wrote {
					if routing.ResponseFailed(captured.Bytes()) {
						s.engine.RecordAttempt(ch.ID, resp.StatusCode, nil, time.Since(start))
						return true, usageInfo{}, &retryDecisionError{err: errors.New("upstream response failed"), same: true}
					}
					w.WriteHeader(resp.StatusCode)
				}
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return false, parseSSEUsageBody(captured.Bytes()), writeErr
				}
				wrote = true
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					if !wrote {
						w.WriteHeader(resp.StatusCode)
					}
					s.engine.RecordAttempt(ch.ID, resp.StatusCode, nil, time.Since(start))
					return false, parseSSEUsageBody(captured.Bytes()), nil
				}
				s.engine.RecordAttempt(ch.ID, resp.StatusCode, readErr, time.Since(start))
				if !wrote {
					return true, usageInfo{}, &retryDecisionError{err: readErr, same: true}
				}
				return false, parseSSEUsageBody(captured.Bytes()), readErr
			}
		}
	}

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		s.engine.RecordAttempt(ch.ID, resp.StatusCode, readErr, latency)
		return true, usageInfo{}, &retryDecisionError{err: readErr, same: true}
	}

	if routing.IsRetryableError(resp.StatusCode, data) {
		s.engine.RecordAttempt(ch.ID, resp.StatusCode, nil, latency)
		return true, usageInfo{}, &retryDecisionError{
			err:  errors.New(http.StatusText(resp.StatusCode)),
			same: routing.IsRetryableOnSameRoute(resp.StatusCode),
		}
	}

	s.engine.RecordAttempt(ch.ID, resp.StatusCode, nil, latency)
	usage := parseUsage(data)
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, writeErr := w.Write(data); writeErr != nil {
		return false, usage, writeErr
	}
	return false, usage, nil
}

func parseUsage(data []byte) usageInfo {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return usageInfo{}
	}
	usage, _ := payload["usage"].(map[string]any)
	if usage == nil {
		if response, ok := payload["response"].(map[string]any); ok {
			usage, _ = response["usage"].(map[string]any)
		}
	}
	if usage == nil {
		return usageInfo{}
	}
	return usageInfo{
		PromptTokens:     usageTokenCount(usage["prompt_tokens"], usage["input_tokens"]),
		CompletionTokens: usageTokenCount(usage["completion_tokens"], usage["output_tokens"]),
	}
}

func usageTokenCount(values ...any) int64 {
	for _, value := range values {
		switch number := value.(type) {
		case float64:
			return int64(number)
		case int64:
			return number
		case int:
			return int64(number)
		case json.Number:
			parsed, _ := number.Int64()
			return parsed
		}
	}
	return 0
}

func parseSSEUsage(line []byte) (usageInfo, bool) {
	lineText := strings.TrimSpace(string(line))
	if !strings.HasPrefix(lineText, "data:") {
		return usageInfo{}, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(lineText, "data:"))
	if payload == "" || payload == "[DONE]" {
		return usageInfo{}, false
	}
	usage := parseUsage([]byte(payload))
	return usage, usage != (usageInfo{})
}

func parseSSEUsageBody(data []byte) usageInfo {
	var usage usageInfo
	for _, line := range strings.Split(string(data), "\n") {
		if u, ok := parseSSEUsage([]byte(line)); ok {
			usage = u
		}
	}
	return usage
}

func (s *Server) recordRequest(tokenID string) {
	if tokenID == "" {
		return
	}
	if s.engine.RecordTokenRequest(tokenID) {
		s.persistConfig()
	}
}

func (s *Server) recordTokens(tokenID string, usage usageInfo) {
	if tokenID == "" {
		return
	}
	if s.engine.RecordTokenTokens(tokenID, usage.PromptTokens, usage.CompletionTokens) {
		s.persistConfig()
	}
}

func (s *Server) persistConfig() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if err := s.engine.Config().Save(s.cfgPath); err != nil {
		s.logger.Printf("persist config: %v", err)
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	cfg := s.engine.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"listen_addr":     cfg.ListenAddr,
		"allow_lan":       cfg.AllowLAN,
		"lan_addrs":       lanAddrs(),
		"channels":        s.engine.State(),
		"model_mappings":  cfg.ModelMappings,
		"fallback_models": cfg.FallbackModels,
	})
}

func lanAddrs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil && ip4.IsPrivate() {
			out = append(out, ip4.String())
		}
	}
	return out
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.Config())
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	records := []usageRecord{}
	summary := map[string]any{}
	if s.usage != nil {
		records = s.usage.list()
		summary = s.usage.summary()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records": records,
		"summary": summary,
	})
}

func (s *Server) handleUsageEvents(w http.ResponseWriter, r *http.Request) {
	if s.usage == nil {
		http.Error(w, "usage store unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events, cancel := s.usage.subscribe()
	defer cancel()
	_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-events:
			if !ok {
				return
			}
			if _, err := fmt.Fprint(w, "event: usage\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// Tokens are managed through dedicated endpoints; keep the live set on save.
	current := s.engine.Config()
	cfg.Tokens = current.Tokens
	cfg.AuthToken = current.AuthToken
	if err := s.engine.ReplaceConfig(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.engine.Config().Save(s.cfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.cfgPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.engine.ReplaceConfig(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCheckAll(w http.ResponseWriter, r *http.Request) {
	s.engine.CheckAll()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) handleCheckOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.engine.FindChannel(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}
	s.engine.CheckNow(id)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens := s.engine.Tokens()
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id":                t.ID,
			"name":              t.Name,
			"token":             t.Token,
			"enabled":           t.IsEnabled(),
			"created_at":        t.CreatedAt,
			"last_used_at":      t.LastUsedAt,
			"request_count":     t.RequestCount,
			"prompt_tokens":     t.PromptTokens,
			"completion_tokens": t.CompletionTokens,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	t, err := s.engine.AddToken(req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.engine.Config().Save(s.cfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": t})
}

func (s *Server) handleUpdateToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	t, ok := s.engine.UpdateToken(id, req.Name, req.Enabled)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "token not found"})
		return
	}
	if err := s.engine.Config().Save(s.cfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": t})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.engine.RemoveToken(id) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "token not found"})
		return
	}
	if err := s.engine.Config().Save(s.cfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleProbeModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "base_url is required"})
		return
	}
	models, err := s.fetchUpstreamModels(req.BaseURL, strings.TrimSpace(req.APIKey))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) fetchUpstreamModels(baseURL, apiKey string) ([]string, error) {
	paths := []string{"/v1/models", "/models"}
	var lastErr error
	for _, path := range paths {
		target := engine.BuildURL(baseURL, path)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := s.engine.ProxyClient().Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		cancel()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("upstream returned 404 for %s", path)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("upstream returned status %d", resp.StatusCode)
			continue
		}
		models := parseModelList(body)
		if len(models) > 0 {
			return models, nil
		}
		lastErr = errors.New("upstream returned an empty model list")
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("unable to fetch models")
}

func parseModelList(body []byte) []string {
	var raw struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	out := make([]string, 0, len(raw.Data))
	for _, m := range raw.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func requestModel(body []byte) (string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", err
	}
	model, _ := raw["model"].(string)
	return strings.TrimSpace(model), nil
}

func rewriteRequestBody(body []byte, upstreamModel string) ([]byte, bool, error) {
	raw := map[string]any{}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, false, err
		}
	}
	raw["model"] = upstreamModel
	out, err := json.Marshal(raw)
	if err != nil {
		return nil, false, err
	}
	stream, _ := raw["stream"].(bool)
	return out, stream, nil
}

func buildFallbackChain(requested string, fallbacks map[string]string) []string {
	chain := []string{requested}
	seen := map[string]bool{requested: true}
	current := requested
	for {
		next := strings.TrimSpace(fallbacks[current])
		if next == "" || seen[next] {
			break
		}
		seen[next] = true
		chain = append(chain, next)
		current = next
	}
	return chain
}

func copyRequestHeaders(dst, src http.Header) {
	for _, key := range []string{"Accept", "Content-Type", "User-Agent", "X-Request-Id"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		// Content-Length must not be forwarded verbatim. The router may stream
		// the body (or the upstream length may be wrong), so Go recomputes it
		// from what is actually written, or switches to chunked encoding.
		if isHopByHopHeader(key) || strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func writeProxyError(w http.ResponseWriter, status int, message, typ, code string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    typ,
			"code":    code,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
