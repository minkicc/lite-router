package engine

import (
	"testing"
	"time"

	"lite-router/internal/config"
	"lite-router/internal/routing"
)

func boolPtr(v bool) *bool { return &v }

func testConfig(channels ...config.Channel) *config.Config {
	cfg := config.Default()
	cfg.Channels = channels
	cfg.Normalize()
	return cfg
}

func TestSelectPicksHighestPriority(t *testing.T) {
	cfg := testConfig(
		config.Channel{ID: "low", BaseURL: "https://a.example.com", Models: []string{"gpt-4"}, Priority: 1},
		config.Channel{ID: "high", BaseURL: "https://b.example.com", Models: []string{"gpt-4"}, Priority: 10},
	)
	eng, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.Select("gpt-4", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel.ID != "high" {
		t.Fatalf("picked %q, want high", got.Channel.ID)
	}
}

func TestSelectPrefersHealthyOverUnknown(t *testing.T) {
	cfg := testConfig(
		config.Channel{ID: "unknown", BaseURL: "https://a.example.com", Models: []string{"gpt-4"}, Priority: 10},
		config.Channel{ID: "healthy", BaseURL: "https://b.example.com", Models: []string{"gpt-4"}, Priority: 0},
	)
	eng, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	eng.RecordAttempt("healthy", 200, nil, 10*time.Millisecond)
	got, err := eng.Select("gpt-4", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel.ID != "healthy" {
		t.Fatalf("picked %q, want healthy", got.Channel.ID)
	}
}

func TestSelectExcludesFailedChannel(t *testing.T) {
	cfg := testConfig(
		config.Channel{ID: "a", BaseURL: "https://a.example.com", Models: []string{"gpt-4"}},
	)
	eng, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Select("gpt-4", map[string]bool{"a": true}); err == nil {
		t.Fatal("expected error when all channels are excluded")
	}
}

func TestResolveChannelModelMapping(t *testing.T) {
	ch := config.Channel{
		ID:            "deepseek",
		BaseURL:       "https://api.deepseek.com",
		Models:        []string{"deepseek-chat"},
		ModelMappings: map[string]string{"codex-default": "deepseek-chat"},
	}
	route := routing.Route{
		ID:            ch.ID,
		Models:        ch.Models,
		ModelMappings: ch.ModelMappings,
	}
	got, ok := routing.ResolveUpstreamModel(&route, "codex-default", nil)
	if !ok || got != "deepseek-chat" {
		t.Fatalf("resolve = %q, %v; want deepseek-chat, true", got, ok)
	}
}

func TestResolveGlobalMappingOnlyForSupportingChannel(t *testing.T) {
	ch := config.Channel{
		ID:     "ds",
		Models: []string{"deepseek-chat"},
	}
	mappings := []routing.ModelMapping{
		{
			PlatformModel: "codex-default",
			UpstreamModel: "deepseek-chat",
			Enabled:       boolPtr(true),
		},
	}
	route := routing.Route{ID: ch.ID, Models: ch.Models}
	got, ok := routing.ResolveUpstreamModel(&route, "codex-default", mappings)
	if !ok || got != "deepseek-chat" {
		t.Fatalf("resolve = %q, %v; want deepseek-chat, true", got, ok)
	}
	unsupported := routing.Route{ID: "other", Models: []string{"gpt-5.6-sol"}}
	if got, ok := routing.ResolveUpstreamModel(&unsupported, "codex-default", mappings); ok {
		t.Fatalf("unsupported channel resolved mapping as %q", got)
	}
}

func TestBuildURL(t *testing.T) {
	cases := []struct {
		base string
		path string
		want string
	}{
		{"https://api.deepseek.com", "/v1/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1", "/v1/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1/", "/v1/models", "https://api.deepseek.com/v1/models"},
	}
	for _, c := range cases {
		got := BuildURL(c.base, c.path)
		if got != c.want {
			t.Errorf("BuildURL(%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}
