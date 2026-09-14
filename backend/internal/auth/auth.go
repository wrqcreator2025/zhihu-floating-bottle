package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Tokens are issued by the trusted host or the local development CLI, never by a public user-ID endpoint.
type Claims struct {
	Subject  string `json:"sub"`
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
}
type Auth struct {
	Key              []byte
	Issuer, Audience string
}

func (a Auth) Issue(subject string, ttl time.Duration) (string, error) {
	body, err := json.Marshal(Claims{subject, a.Issuer, a.Audience, time.Now().Add(ttl).Unix()})
	if err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(body)
	return s + "." + a.sign(s), nil
}
func (a Auth) sign(s string) string {
	h := hmac.New(sha256.New, a.Key)
	h.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (a Auth) Verify(token string) (string, error) {
	bad := errors.New("invalid session")
	p := strings.Split(token, ".")
	if len(p) != 3 {
		return "", bad
	}
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil {
		return "", bad
	}
	expected, _ := base64.RawURLEncoding.DecodeString(a.sign(p[0] + "." + p[1]))
	if !hmac.Equal(sig, expected) {
		return "", bad
	}
	var header struct{ Alg string }
	h, err := base64.RawURLEncoding.DecodeString(p[0])
	if err != nil || json.Unmarshal(h, &header) != nil || header.Alg != "HS256" {
		return "", bad
	}
	b, err := base64.RawURLEncoding.DecodeString(p[1])
	var c Claims
	if err != nil || json.Unmarshal(b, &c) != nil || c.Subject == "" || len(c.Subject) > 191 || c.Issuer != a.Issuer || c.Audience != a.Audience || c.Expires <= time.Now().Unix() {
		return "", bad
	}
	return c.Subject, nil
}
