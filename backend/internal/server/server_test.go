package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/minkicc/mkswitch/backend/internal/config"
	"github.com/minkicc/mkswitch/backend/internal/engine"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

type cancelingBody struct {
	data   []byte
	read   bool
	cancel context.CancelFunc
}

func (b *cancelingBody) Read(p []byte) (int, error) {
	if !b.read {
		b.read = true
		return copy(p, b.data), nil
	}
	b.cancel()
	return 0, context.Canceled
}

func (b *cancelingBody) Close() error { return nil }

func TestForwardStreamsResponseImmediately(t *testing.T) {
	eng, err := engine.New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	eng.ProxyClient().Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(bytes.NewBufferString(
				"event: response.completed\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n",
			)),
		}, nil
	})
	srv := &Server{engine: eng}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	recorder := httptest.NewRecorder()
	selection := &engine.Selection{Channel: config.Channel{ID: "#1", BaseURL: "https://example.com"}, UpstreamModel: "gpt-test"}

	retry, usage, err := srv.forward(recorder, req, selection, []byte(`{"model":"gpt-test","stream":true}`))
	if err != nil || retry {
		t.Fatalf("retry=%v err=%v", retry, err)
	}
	if usage != (usageInfo{PromptTokens: 3, CompletionTokens: 2}) {
		t.Fatalf("usage = %+v", usage)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("stream body was not forwarded")
	}
}

func TestForwardStreamDoesNotMarkChannelFailedOnClientCancel(t *testing.T) {
	cfg := config.Default()
	cfg.Channels = []config.Channel{{ID: "#1", BaseURL: "https://example.com", Models: []string{"gpt-4"}}}
	eng, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng.ProxyClient().Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       &cancelingBody{data: []byte("data: {\"type\":\"response.created\"}\n\n"), cancel: cancel},
		}, nil
	})
	srv := &Server{engine: eng}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	selection := &engine.Selection{Channel: config.Channel{ID: "#1", BaseURL: "https://example.com"}, UpstreamModel: "gpt-test"}

	retry, _, err := srv.forward(httptest.NewRecorder(), req, selection, []byte(`{"model":"gpt-test","stream":true}`))
	if retry {
		t.Fatal("client cancel must not trigger failover")
	}
	if err == nil {
		t.Fatal("client cancel must be reported as a cancellation, not success")
	}
	for _, st := range eng.State() {
		if st.ID == "#1" && st.ConsecutiveFailures != 0 {
			t.Fatalf("client cancel must not mark the channel failed, got %d failures", st.ConsecutiveFailures)
		}
	}
}

func TestForwardDoesNotRetryCanceledRequest(t *testing.T) {
	eng, err := engine.New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	eng.ProxyClient().Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})
	srv := &Server{engine: eng}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	selection := &engine.Selection{Channel: config.Channel{ID: "#1", BaseURL: "https://example.com"}, UpstreamModel: "gpt-test"}

	retry, _, err := srv.forward(httptest.NewRecorder(), req, selection, []byte(`{"model":"gpt-test"}`))
	if !retry || retryOnSameChannel(err) {
		t.Fatalf("retry=%v same-channel=%v err=%v", retry, retryOnSameChannel(err), err)
	}
}

func TestHandleProxyCoolsDownAfterRetriesExhausted(t *testing.T) {
	cfg := config.Default()
	cfg.NoAuth = true
	cfg.Channels = []config.Channel{
		{ID: "#1", BaseURL: "https://example.com", Models: []string{"gpt-4"}, MaxRetries: 2},
	}
	eng, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	eng.ProxyClient().Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"down"}}`)),
		}, nil
	})
	srv := &Server{engine: eng}

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-4"}`))
	srv.handleProxy(httptest.NewRecorder(), req)

	states := eng.State()
	if len(states) != 1 {
		t.Fatalf("states = %d, want 1", len(states))
	}
	if states[0].CooldownUntil == 0 {
		t.Fatal("expected channel to be in cooldown after retries exhausted")
	}
}

func TestRetryOnSameChannel(t *testing.T) {
	if retryOnSameChannel(errors.New("network failure")) {
		t.Fatal("plain errors must not be treated as retry decisions")
	}
	if !retryOnSameChannel(&retryDecisionError{err: errors.New("timeout"), same: true}) {
		t.Fatal("same-channel retry decision was not recognized")
	}
	if retryOnSameChannel(&retryDecisionError{err: errors.New("unauthorized"), same: false}) {
		t.Fatal("route-switch decision must not retry the same channel")
	}
}

func TestBuildFallbackChain(t *testing.T) {
	fallbacks := map[string]string{
		"gpt-5.6":     "deepseek-v4",
		"deepseek-v4": "deepseek-chat",
	}
	got := buildFallbackChain("gpt-5.6", fallbacks)
	want := []string{"gpt-5.6", "deepseek-v4", "deepseek-chat"}
	if len(got) != len(want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chain = %v, want %v", got, want)
		}
	}
}

func TestRewriteRequestBody(t *testing.T) {
	out, stream, err := rewriteRequestBody([]byte(`{"model":"codex-default","stream":true}`), "deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	if !stream {
		t.Fatal("stream should be true")
	}
	if string(out) != `{"model":"deepseek-chat","stream":true}` {
		t.Fatalf("body = %s", out)
	}
}

func TestCopyResponseHeadersStripsContentLength(t *testing.T) {
	src := http.Header{
		"Content-Type":   []string{"text/event-stream"},
		"Content-Length": []string{"10"},
		"Connection":     []string{"keep-alive"},
	}
	dst := http.Header{}
	copyResponseHeaders(dst, src)
	if got := dst.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length should be stripped, got %q", got)
	}
	if got := dst.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type was not copied: %q", got)
	}
	if got := dst.Get("Connection"); got != "" {
		t.Fatalf("hop-by-hop Connection should be stripped, got %q", got)
	}
}

func TestParseUsage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want usageInfo
	}{
		{
			name: "chat completions",
			body: `{"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
			want: usageInfo{PromptTokens: 11, CompletionTokens: 7},
		},
		{
			name: "responses",
			body: `{"type":"response.completed","response":{"usage":{"input_tokens":13,"output_tokens":5,"total_tokens":18}}}`,
			want: usageInfo{PromptTokens: 13, CompletionTokens: 5},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseUsage([]byte(test.body)); got != test.want {
				t.Fatalf("usage = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseSSEUsageBody(t *testing.T) {
	body := []byte("event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":21,\"output_tokens\":8}}}\n\n")
	want := usageInfo{PromptTokens: 21, CompletionTokens: 8}
	if got := parseSSEUsageBody(body); got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
}

func TestUsageStoreNotifiesSubscribers(t *testing.T) {
	store := newUsageStore(filepath.Join(t.TempDir(), "config.json"), 10)
	events, cancel := store.subscribe()
	defer cancel()

	store.add(usageRecord{Model: "gpt-5.6-sol", Success: true})
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("usage subscriber was not notified")
	}
}
