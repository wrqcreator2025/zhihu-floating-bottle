package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStructuredResponseAndProtocolFailure(t *testing.T) {
	for i, body := range []string{`{"choices":[{"message":{"content":"{\"allowed\":false,\"reason\":\"harassment\"}"}}]}`, `{"choices":[]}`, `{"choices":[{"message":{"content":"not JSON"}}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer key" {
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
