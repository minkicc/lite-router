package server

import (
	"path/filepath"
	"testing"
	"time"
)

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
