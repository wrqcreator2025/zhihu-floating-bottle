package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTokenVerification(t *testing.T) {
	a := Auth{Key: []byte(strings.Repeat("x", 32)), Issuer: "issuer", Audience: "app"}
	token, _ := a.Issue("subject", time.Hour)
	if sub, err := a.Verify(token); err != nil || sub != "subject" {
		t.Fatal("valid token rejected")
	}
	if _, err := a.Verify(token + "bad"); err == nil {
		t.Fatal("tampered token accepted")
	}
	a.Audience = "other"
	if _, err := a.Verify(token); err == nil {
		t.Fatal("wrong audience accepted")
	}
	a.Audience = "app"
	token, _ = a.Issue("subject", -time.Hour)
	if _, err := a.Verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
}
