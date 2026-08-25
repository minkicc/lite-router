package config

import (
	"net/http"
	"testing"
)

func TestNormalizeLegacyAPIKeyChannelKeepsBearerAuthorization(t *testing.T) {
	cfg := Default()
	cfg.Channels = []Channel{{
		ID:      "#1",
		BaseURL: "https://example.com",
		APIKey:  "secret",
	}}
	cfg.Normalize()

	channel := cfg.Channels[0]
	if channel.AuthType != ChannelAuthAPIKey {
		t.Fatalf("auth type = %q", channel.AuthType)
	}
	if channel.APIKeyPlacement != APIKeyPlacementBearer || channel.APIKeyPrefix != "Bearer" {
		t.Fatalf("legacy API key defaults = %+v", channel)
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	channel.ApplyAPIKeyAuthorization(req)
	if got := req.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestApplyAPIKeyAuthorizationPlacements(t *testing.T) {
	tests := []struct {
		name       string
		channel    Channel
		wantHeader string
		wantValue  string
		wantQuery  string
	}{
		{
			name: "raw authorization",
			channel: Channel{
				APIKey:          "secret",
				APIKeyPlacement: APIKeyPlacementAuthorization,
			},
			wantHeader: "Authorization",
			wantValue:  "secret",
		},
		{
			name: "custom header with prefix",
			channel: Channel{
				APIKey:          "secret",
				APIKeyPlacement: APIKeyPlacementHeader,
				APIKeyHeader:    "X-Token",
				APIKeyPrefix:    "Token",
			},
			wantHeader: "X-Token",
			wantValue:  "Token secret",
		},
		{
			name: "query parameter",
			channel: Channel{
				APIKey:          "secret",
				APIKeyPlacement: APIKeyPlacementQuery,
				APIKeyQuery:     "key",
			},
			wantQuery: "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/models", nil)
			if err != nil {
				t.Fatal(err)
			}
			tt.channel.ApplyAPIKeyAuthorization(req)
			if tt.wantHeader != "" && req.Header.Get(tt.wantHeader) != tt.wantValue {
				t.Fatalf("%s = %q", tt.wantHeader, req.Header.Get(tt.wantHeader))
			}
			if tt.wantQuery != "" && req.URL.Query().Get(tt.channel.APIKeyQuery) != tt.wantQuery {
				t.Fatalf("query = %q", req.URL.RawQuery)
			}
		})
	}
}

func TestNoAuthDoesNotApplyConfiguredLegacyKey(t *testing.T) {
	channel := Channel{
		AuthType: ChannelAuthNone,
		APIKey:   "must-not-leak",
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	channel.ApplyAPIKeyAuthorization(req)
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q", got)
	}
}
