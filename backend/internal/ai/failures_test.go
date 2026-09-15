package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"driftbottle/internal/domain"
)

func TestHTTPFailuresRetainStatus(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 502, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"private prompt and credentials"}}`)
			}))
			defer server.Close()
			var out Review
			err := New(server.URL, "key", "model").Run(context.Background(), "moderation", "text", &out)
			var failure *domain.Error
			if !errors.As(err, &failure) || failure.Code != fmt.Sprintf("AI_HTTP_%d", status) || failure.Status != 503 || out.Allowed != nil {
				t.Fatalf("unexpected failure: %v", err)
			}
		})
	}
}

func TestTransportFailuresAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{context.DeadlineExceeded, "AI_TIMEOUT"},
		{context.Canceled, "AI_CANCELED"},
		{&net.DNSError{Err: "private hostname"}, "AI_DNS_ERROR"},
		{errors.New("private URL"), "AI_NETWORK_ERROR"},
	} {
		if got := transportCode(tc.err); got != tc.code {
			t.Fatalf("got %s, want %s", got, tc.code)
		}
	}
}
