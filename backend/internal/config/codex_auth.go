package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	ChannelAuthAPIKey = "api_key"
	ChannelAuthCodex  = "codex"
	CodexBaseURL      = "https://chatgpt.com/backend-api/codex"
	CodexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
)

type CodexAuth struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	UpdatedAt    int64  `json:"updated_at,omitempty"`
}

func (a *CodexAuth) Normalize() {
	if a == nil {
		return
	}
	a.AccessToken = strings.TrimSpace(a.AccessToken)
	a.RefreshToken = strings.TrimSpace(a.RefreshToken)
	a.IDToken = strings.TrimSpace(a.IDToken)
	a.AccountID = strings.TrimSpace(a.AccountID)
	a.UserID = strings.TrimSpace(a.UserID)
	a.Email = strings.TrimSpace(a.Email)
	a.ClientID = strings.TrimSpace(a.ClientID)
	a.ExpiresAt = strings.TrimSpace(a.ExpiresAt)
	if a.ClientID == "" {
		a.ClientID = CodexClientID
	}
	a.enrichFromJWT(a.IDToken, false)
	a.enrichFromJWT(a.AccessToken, true)
}

func (a *CodexAuth) ExpiresAtTime() (time.Time, bool) {
	if a == nil {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(a.ExpiresAt)
	if raw == "" {
		return time.Time{}, false
	}
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if unix > 1_000_000_000_000 {
			unix /= 1000
		}
		return time.Unix(unix, 0).UTC(), true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func ParseCodexAuthJSON(content string) (*CodexAuth, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("Codex auth.json cannot be empty")
	}
	var raw map[string]any
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid Codex auth.json: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("Codex auth.json cannot be empty")
	}

	updatedAt, hasUpdatedAt := firstInt64(raw,
		[]string{"updated_at"},
		[]string{"updatedAt"},
	)
	auth := &CodexAuth{
		AccessToken: firstString(raw,
			[]string{"tokens", "access_token"},
			[]string{"tokens", "accessToken"},
			[]string{"access_token"},
			[]string{"accessToken"},
		),
		RefreshToken: firstString(raw,
			[]string{"tokens", "refresh_token"},
			[]string{"tokens", "refreshToken"},
			[]string{"refresh_token"},
			[]string{"refreshToken"},
		),
		IDToken: firstString(raw,
			[]string{"tokens", "id_token"},
			[]string{"tokens", "idToken"},
			[]string{"id_token"},
			[]string{"idToken"},
		),
		AccountID: firstString(raw,
			[]string{"tokens", "account_id"},
			[]string{"tokens", "accountId"},
			[]string{"chatgpt_account_id"},
			[]string{"chatgptAccountId"},
			[]string{"account_id"},
			[]string{"accountId"},
			[]string{"account", "id"},
		),
		UserID: firstString(raw,
			[]string{"chatgpt_user_id"},
			[]string{"chatgptUserId"},
			[]string{"user_id"},
			[]string{"userId"},
			[]string{"user", "id"},
		),
		Email: firstString(raw,
			[]string{"email"},
			[]string{"user", "email"},
		),
		ClientID: firstString(raw,
			[]string{"client_id"},
			[]string{"clientId"},
		),
		ExpiresAt: firstString(raw,
			[]string{"tokens", "expires_at"},
			[]string{"tokens", "expiresAt"},
			[]string{"expires_at"},
			[]string{"expiresAt"},
		),
		UpdatedAt: updatedAt,
	}
	if !hasUpdatedAt {
		auth.UpdatedAt = time.Now().UnixMilli()
	}
	auth.Normalize()
	if auth.AccessToken == "" && auth.RefreshToken == "" {
		return nil, errors.New("Codex auth.json is missing access_token and refresh_token")
	}
	return auth, nil
}

type codexJWTClaims struct {
	Sub        string                `json:"sub"`
	Email      string                `json:"email"`
	Exp        int64                 `json:"exp"`
	OpenAIAuth *codexJWTOpenAIClaims `json:"https://api.openai.com/auth,omitempty"`
}

type codexJWTOpenAIClaims struct {
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	ChatGPTUserID    string `json:"chatgpt_user_id"`
	UserID           string `json:"user_id"`
}

func (a *CodexAuth) enrichFromJWT(token string, includeExpiry bool) {
	claims, err := decodeCodexJWTClaims(token)
	if err != nil {
		return
	}
	if a.Email == "" {
		a.Email = strings.TrimSpace(claims.Email)
	}
	if claims.OpenAIAuth != nil {
		if a.AccountID == "" {
			a.AccountID = strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
		}
		if a.UserID == "" {
			a.UserID = strings.TrimSpace(claims.OpenAIAuth.ChatGPTUserID)
			if a.UserID == "" {
				a.UserID = strings.TrimSpace(claims.OpenAIAuth.UserID)
			}
		}
	}
	if a.UserID == "" {
		a.UserID = strings.TrimSpace(claims.Sub)
	}
	if includeExpiry && a.ExpiresAt == "" && claims.Exp > 0 {
		a.ExpiresAt = time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339)
	}
}

func decodeCodexJWTClaims(token string) (*codexJWTClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	payload, err := decodeCodexJWTSegment(parts[1])
	if err != nil {
		return nil, err
	}
	var claims codexJWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func decodeCodexJWTSegment(segment string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	padded := segment
	if remainder := len(padded) % 4; remainder > 0 {
		padded += strings.Repeat("=", 4-remainder)
	}
	if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(padded)
}

func firstString(raw map[string]any, paths ...[]string) string {
	for _, path := range paths {
		var current any = raw
		ok := true
		for _, key := range path {
			object, isObject := current.(map[string]any)
			if !isObject {
				ok = false
				break
			}
			current, ok = object[key]
			if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		switch value := current.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case json.Number:
			return value.String()
		case float64:
			return strconv.FormatInt(int64(value), 10)
		}
	}
	return ""
}

func firstInt64(raw map[string]any, paths ...[]string) (int64, bool) {
	for _, path := range paths {
		var current any = raw
		ok := true
		for _, key := range path {
			object, isObject := current.(map[string]any)
			if !isObject {
				ok = false
				break
			}
			current, ok = object[key]
			if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		switch value := current.(type) {
		case json.Number:
			parsed, err := value.Int64()
			return parsed, err == nil
		case float64:
			return int64(value), true
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			return parsed, err == nil
		}
	}
	return 0, false
}
