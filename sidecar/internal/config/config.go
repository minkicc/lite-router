package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type HealthCheck struct {
	IntervalSeconds  int    `json:"interval_seconds"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	Path             string `json:"path"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
}

type Channel struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key,omitempty"`
	Models        []string          `json:"models,omitempty"`
	ModelMappings map[string]string `json:"model_mappings,omitempty"`
	Priority      int               `json:"priority,omitempty"`
	Group         string            `json:"group,omitempty"`
	MaxRetries    int               `json:"max_retries,omitempty"`
	Weight        int               `json:"weight,omitempty"`
	Price         float64           `json:"price,omitempty"`
	Enabled       *bool             `json:"enabled,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HealthCheck   *HealthCheck      `json:"health_check,omitempty"`
}

type ModelMapping struct {
	PlatformModel string `json:"platform_model"`
	UpstreamModel string `json:"upstream_model"`
	ChannelID     string `json:"channel_id,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

type Token struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Token            string `json:"token,omitempty"`
	Enabled          *bool  `json:"enabled,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	LastUsedAt       int64  `json:"last_used_at,omitempty"`
	RequestCount     int64  `json:"request_count,omitempty"`
	PromptTokens     int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens int64  `json:"completion_tokens,omitempty"`
}

type Group struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
}

func (g *Group) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		g.Name = name
		g.Priority = 0
		return nil
	}
	type groupAlias Group
	var alias groupAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*g = Group(alias)
	return nil
}

type Config struct {
	ListenAddr      string            `json:"listen_addr"`
	AuthToken       string            `json:"auth_token,omitempty"`
	NoAuth          bool              `json:"no_auth,omitempty"`
	HealthCheck     HealthCheck       `json:"health_check"`
	Channels        []Channel         `json:"channels"`
	ModelMappings   []ModelMapping    `json:"model_mappings,omitempty"`
	FallbackModels  map[string]string `json:"fallback_models,omitempty"`
	Tokens          []Token           `json:"tokens,omitempty"`
	Groups          []Group           `json:"groups,omitempty"`
	AllowLAN        bool              `json:"allow_lan,omitempty"`
	UsageMaxRecords int               `json:"usage_max_records,omitempty"`
}

func (c *Config) Clone() *Config {
	data, err := json.Marshal(c)
	if err != nil {
		out := Default()
		return out
	}
	var out Config
	if err := json.Unmarshal(data, &out); err != nil {
		return Default()
	}
	return &out
}

func Default() *Config {
	return &Config{
		ListenAddr: "127.0.0.1:8787",
		HealthCheck: HealthCheck{
			IntervalSeconds:  30,
			TimeoutSeconds:   5,
			Path:             "/v1/models",
			FailureThreshold: 2,
			SuccessThreshold: 1,
		},
		Channels:       []Channel{},
		ModelMappings:  []ModelMapping{},
		FallbackModels: map[string]string{},
		Tokens:         []Token{},
		Groups:         []Group{{Name: "default"}},
	}
}

func DefaultPath() string {
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		current := filepath.Join(dir, "lite-router", "config.json")
		legacy := filepath.Join(dir, "local-router", "config.json")
		if _, err := os.Stat(current); errors.Is(err, os.ErrNotExist) {
			if _, legacyErr := os.Stat(legacy); legacyErr == nil {
				return legacy
			}
		}
		return current
	}
	return "lite-router.json"
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.Normalize()
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	c.Normalize()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Config) Normalize() {
	if strings.TrimSpace(c.ListenAddr) == "" {
		c.ListenAddr = "127.0.0.1:8787"
	}
	if c.HealthCheck.IntervalSeconds <= 0 {
		c.HealthCheck.IntervalSeconds = 30
	}
	if c.HealthCheck.TimeoutSeconds <= 0 {
		c.HealthCheck.TimeoutSeconds = 5
	}
	if strings.TrimSpace(c.HealthCheck.Path) == "" {
		c.HealthCheck.Path = "/v1/models"
	}
	if c.HealthCheck.FailureThreshold <= 0 {
		c.HealthCheck.FailureThreshold = 2
	}
	if c.HealthCheck.SuccessThreshold <= 0 {
		c.HealthCheck.SuccessThreshold = 1
	}
	used := map[string]bool{}
	for i := range c.Channels {
		if id := strings.TrimSpace(c.Channels[i].ID); id != "" && !isLegacyAutoID(id) {
			used[id] = true
		}
	}
	nextID := nextChannelNumber(used)
	for i := range c.Channels {
		ch := &c.Channels[i]
		id := strings.TrimSpace(ch.ID)
		if id == "" || isLegacyAutoID(id) {
			for {
				candidate := fmt.Sprintf("#%d", nextID)
				nextID++
				if !used[candidate] {
					ch.ID = candidate
					break
				}
			}
		}
		ch.ID = strings.TrimSpace(ch.ID)
		used[ch.ID] = true
		if ch.Enabled == nil {
			enabled := true
			ch.Enabled = &enabled
		}
		if ch.Headers == nil {
			ch.Headers = map[string]string{}
		}
		if ch.ModelMappings == nil {
			ch.ModelMappings = map[string]string{}
		}
		if ch.HealthCheck != nil {
			if ch.HealthCheck.IntervalSeconds <= 0 {
				ch.HealthCheck.IntervalSeconds = c.HealthCheck.IntervalSeconds
			}
			if ch.HealthCheck.TimeoutSeconds <= 0 {
				ch.HealthCheck.TimeoutSeconds = c.HealthCheck.TimeoutSeconds
			}
			if strings.TrimSpace(ch.HealthCheck.Path) == "" {
				ch.HealthCheck.Path = c.HealthCheck.Path
			}
			if ch.HealthCheck.FailureThreshold <= 0 {
				ch.HealthCheck.FailureThreshold = c.HealthCheck.FailureThreshold
			}
			if ch.HealthCheck.SuccessThreshold <= 0 {
				ch.HealthCheck.SuccessThreshold = c.HealthCheck.SuccessThreshold
			}
		}
	}
	if c.FallbackModels == nil {
		c.FallbackModels = map[string]string{}
	}
	if c.UsageMaxRecords <= 0 {
		c.UsageMaxRecords = 500
	}
	if c.ModelMappings == nil {
		c.ModelMappings = []ModelMapping{}
	}
	for i := range c.ModelMappings {
		m := &c.ModelMappings[i]
		m.PlatformModel = strings.TrimSpace(m.PlatformModel)
		m.UpstreamModel = strings.TrimSpace(m.UpstreamModel)
		m.ChannelID = strings.TrimSpace(m.ChannelID)
	}
	if c.Tokens == nil {
		c.Tokens = []Token{}
	}
	for i := range c.Tokens {
		t := &c.Tokens[i]
		t.ID = strings.TrimSpace(t.ID)
		t.Name = strings.TrimSpace(t.Name)
		if t.ID == "" {
			t.ID = t.Name
		}
		if t.Enabled == nil {
			enabled := true
			t.Enabled = &enabled
		}
	}
	seenGroup := map[string]bool{}
	groups := make([]Group, 0, len(c.Groups))
	for _, g := range c.Groups {
		g.Name = strings.TrimSpace(g.Name)
		if g.Name == "" || seenGroup[g.Name] {
			continue
		}
		seenGroup[g.Name] = true
		groups = append(groups, g)
	}
	if !seenGroup["default"] {
		groups = append([]Group{{Name: "default"}}, groups...)
	}
	c.Groups = groups
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return errors.New("listen_addr cannot be empty")
	}
	seen := map[string]bool{}
	for _, ch := range c.Channels {
		if strings.TrimSpace(ch.ID) == "" {
			return errors.New("every channel needs an id")
		}
		if strings.TrimSpace(ch.BaseURL) == "" {
			return errors.New("channel " + ch.ID + " needs base_url")
		}
		if seen[ch.ID] {
			return errors.New("duplicate channel id: " + ch.ID)
		}
		seen[ch.ID] = true
	}
	for i, m := range c.ModelMappings {
		if m.PlatformModel == "" || m.UpstreamModel == "" {
			return errors.New("model mapping requires platform_model and upstream_model")
		}
		_ = i
	}
	return nil
}

func (c *Channel) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func (t *Token) IsEnabled() bool {
	return t.Enabled == nil || *t.Enabled
}

func (c *Channel) EffectiveHealth(global HealthCheck) HealthCheck {
	if c.HealthCheck == nil {
		return global
	}
	return *c.HealthCheck
}

func nextChannelNumber(existing map[string]bool) int {
	max := 0
	for id := range existing {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "#") {
			if n, err := strconv.Atoi(strings.TrimPrefix(id, "#")); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}

func isLegacyAutoID(id string) bool {
	if !strings.HasPrefix(id, "ch_") {
		return false
	}
	hexPart := strings.TrimPrefix(id, "ch_")
	if len(hexPart) != 8 {
		return false
	}
	for _, r := range hexPart {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
