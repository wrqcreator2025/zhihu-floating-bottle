package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseFormatsAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content any
		finish  string
		valid   bool
	}{
		{"markdown", "```json\n{\"allowed\":false}\n```", "stop", true},
		{"explanation", "整理如下：\n{\"allowed\":false}\n以上是结果。", "stop", true},
		{"reasoning", "<think>考虑 {上下文}</think>{\"allowed\":false}", "stop", true},
		{"blocks", []map[string]string{{"type": "text", "text": "{\"allowed\":false}"}}, "stop", true},
		{"empty", "", "stop", false},
		{"null", "null", "stop", false},
		{"multiple", "{\"allowed\":true}{\"allowed\":false}", "stop", false},
		{"truncated", "{\"allowed\":true}", "length", false},
		{"wrong_type", "{\"allowed\":\"true\"}", "stop", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": tc.content}, "finish_reason": tc.finish}}})
			}))
			defer server.Close()
			var out Review
			err := New(server.URL, "key", "model").Run(context.Background(), "moderation", "text", &out)
			if tc.valid {
				if err != nil || out.Allowed == nil || *out.Allowed || calls != 1 {
					t.Fatalf("out=%+v err=%v calls=%d", out, err, calls)
				}
			} else if err == nil || out.Allowed != nil || calls != 2 {
				t.Fatalf("invalid output accepted or retries incorrect: %+v %v %d", out, err, calls)
			}
		})
	}
}

func TestInvalidResponseRetriesAndRecovers(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		content := "not JSON"
		if calls == 2 {
			content = `{"allowed":false,"reason":"harassment"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	var out Review
	if err := New(server.URL, "key", "model").Run(context.Background(), "moderation", "text", &out); err != nil || calls != 2 || out.Allowed == nil || *out.Allowed {
		t.Fatalf("retry did not recover: %+v %v calls=%d", out, err, calls)
	}
}

func TestStructuredResponseAndProtocolFailure(t *testing.T) {
	for i, body := range []string{`{"choices":[{"message":{"content":"{\"allowed\":false,\"reason\":\"harassment\"}"}}]}`, `{"choices":[]}`, `{"choices":[{"message":{"content":"not JSON"}}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("X-Request-Timestamp") == "" {
				t.Error("model request mismatch")
			}
			fmt.Fprint(w, body)
		}))
		c := New(server.URL, "key", "model")
		var review Review
		err := c.Run(context.Background(), "moderation", map[string]string{"content": "test"}, &review)
		if i == 0 {
			if err != nil || review.Allowed == nil || *review.Allowed {
				t.Fatalf("valid rejection was not decoded: %v", err)
			}
		} else if err == nil {
			t.Fatal("invalid model response accepted")
		}

		server.Close()
	}
}
func TestUnconfiguredProviderFailsExplicitly(t *testing.T) {
	var result Review
	if err := New("", "", "").Run(context.Background(), "moderation", "text", &result); err == nil {
		t.Fatal("unconfigured provider must not allow delivery")
	}
}
