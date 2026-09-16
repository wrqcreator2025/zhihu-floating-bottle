package jobs

import "testing"

func TestRetryPlanKeepsRateLimitJobsPendingLonger(t *testing.T) {
	retry, delay := retryPlan("AI_HTTP_429", 5)
	if !retry {
		t.Fatal("rate limited jobs should keep retrying after five attempts")
	}
	if delay < 960 || delay > 989 {
		t.Fatalf("unexpected rate limit delay: %d", delay)
	}
	retry, _ = retryPlan("AI_HTTP_429", 12)
	if retry {
		t.Fatal("rate limited jobs should eventually stop retrying")
	}
}

func TestRetryPlanKeepsDefaultAttemptLimit(t *testing.T) {
	retry, _ := retryPlan("AI_PROTOCOL_ERROR", 5)
	if retry {
		t.Fatal("default transient failures should stop after five attempts")
	}
}
