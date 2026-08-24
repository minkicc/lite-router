package config

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestParseCodexAuthJSONNestedTokensAndJWTClaims(t *testing.T) {
	token := testCodexJWT(t, map[string]any{
		"sub":   "user-1",
		"email": "codex@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-1",
		},
	})
	raw, err := json.Marshal(map[string]any{
		"tokens": map[string]any{
			"access_token":  token,
			"refresh_token": "refresh-1",
			"id_token":      "id-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	auth, err := ParseCodexAuthJSON(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != token || auth.RefreshToken != "refresh-1" {
		t.Fatalf("tokens not parsed: %+v", auth)
	}
	if auth.AccountID != "account-1" || auth.UserID != "user-1" || auth.Email != "codex@example.com" {
		t.Fatalf("JWT claims not enriched: %+v", auth)
	}
	if auth.ClientID != CodexClientID {
		t.Fatalf("client id = %q", auth.ClientID)
	}
	if auth.UpdatedAt <= 0 {
		t.Fatalf("updated_at = %d", auth.UpdatedAt)
	}
	if _, ok := auth.ExpiresAtTime(); !ok {
		t.Fatalf("expiry not parsed: %+v", auth)
	}
}

func TestParseCodexAuthJSONPreservesCredentialRevision(t *testing.T) {
	auth, err := ParseCodexAuthJSON(`{"access_token":"access","updated_at":42}`)
	if err != nil {
		t.Fatal(err)
	}
	if auth.UpdatedAt != 42 {
		t.Fatalf("updated_at = %d, want 42", auth.UpdatedAt)
	}
}

func TestParseCodexAuthJSONAcceptsRefreshOnly(t *testing.T) {
	auth, err := ParseCodexAuthJSON(`{"refresh_token":"refresh-only"}`)
	if err != nil {
		t.Fatal(err)
	}
	if auth.RefreshToken != "refresh-only" {
		t.Fatalf("refresh token = %q", auth.RefreshToken)
	}
}

func TestParseCodexAuthJSONRejectsMissingTokens(t *testing.T) {
	if _, err := ParseCodexAuthJSON(`{"email":"codex@example.com"}`); err == nil {
		t.Fatal("expected missing token error")
	}
}

func testCodexJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
