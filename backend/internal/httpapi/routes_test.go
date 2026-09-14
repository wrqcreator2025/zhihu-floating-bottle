package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"driftbottle/internal/auth"
	"driftbottle/internal/service"
)

func oauthTestAPI() *api {
	return &api{o: Options{Auth: auth.Auth{Key: []byte(strings.Repeat("k", 32)), Issuer: "test", Audience: "test"}}}
}

func TestOAuthAttemptAcceptsSignedCookieWithoutReturnedState(t *testing.T) {
	a := oauthTestAPI()
	cookie, err := a.oauthAttemptCookie("expected-state", "", true)
	if err != nil {
		t.Fatal(err)
	}
	attempt, reason := a.takeOAuthAttempt("", cookie.Value)
	if reason != "" || !attempt.Login {
		t.Fatalf("attempt=%+v reason=%q", attempt, reason)
	}
}

func TestOAuthAttemptRejectsWrongStateOrCookie(t *testing.T) {
	a := oauthTestAPI()
	cookie, err := a.oauthAttemptCookie("expected-state", "user-1", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ state, cookie string }{
		{"wrong-state", cookie.Value},
		{"expected-state", cookie.Value + "tampered"},
		{"", ""},
	} {
		if _, reason := a.takeOAuthAttempt(tc.state, tc.cookie); reason == "" {
			t.Fatalf("accepted state=%q cookie length=%d", tc.state, len(tc.cookie))
		}
	}
}

func TestOAuthCookieAllowsProductionCrossSiteCallback(t *testing.T) {
	production := (&api{o: Options{SecureCookies: true}}).oauthCookie("nonce", 600)
	if !production.Secure || production.HttpOnly == false || production.SameSite != http.SameSiteNoneMode {
		t.Fatalf("production cookie=%+v", production)
	}
	local := (&api{}).oauthCookie("nonce", 600)
	if local.Secure || local.SameSite != http.SameSiteLaxMode {
		t.Fatalf("local cookie=%+v", local)
	}
}

func TestPublicRouteInventory(t *testing.T) {
	router := New(&service.Service{}, Options{})
	want := map[string]bool{
		"GET /healthz":                                  true,
		"GET /api/v1/auth/zhihu":                        true,
		"GET /api/v1/auth/session":                      true,
		"POST /api/v1/auth/logout":                      true,
		"GET /api/v1/home":                              true,
		"POST /api/v1/bottles":                          true,
		"PATCH /api/v1/bottles/:id":                     true,
		"GET /api/v1/bottles/:id":                       true,
		"POST /api/v1/bottles/:id/launch":               true,
		"POST /api/v1/bottles/:id/pause":                true,
		"POST /api/v1/bottles/:id/resume":               true,
		"POST /api/v1/bottles/:id/retry":                true,
		"POST /api/v1/bottles/:id/appeal":               true,
		"GET /api/v1/bottles/:id/search-status":         true,
		"POST /api/v1/ai/episode-drafts":                true,
		"POST /api/v1/ai/target-drafts":                 true,
		"POST /api/v1/ai/experience-suggestions":        true,
		"GET /api/v1/ai/experience-suggestions/:id":     true,
		"GET /api/v1/invitations/next":                  true,
		"POST /api/v1/invitations/:id/decision":         true,
		"GET /api/v1/connections/:id":                   true,
		"GET /api/v1/connections/:id/messages":          true,
		"POST /api/v1/connections/:id/messages":         true,
		"POST /api/v1/connections/:id/chat/messages":    true,
		"POST /api/v1/connections/:id/chat-invitations": true,
		"GET /api/v1/connections/:id/chat":              true,
		"POST /api/v1/connections/:id/close":            true,
		"POST /api/v1/connections/:id/feedback":         true,
		"POST /api/v1/connections/:id/reports":          true,
		"POST /api/v1/connections/:id/block":            true,
		"PATCH /api/v1/messages/:id":                    true,
		"POST /api/v1/messages/:id/retry":               true,
		"POST /api/v1/messages/:id/appeal":              true,
		"POST /api/v1/chat-invitations/:id/decision":    true,
		"GET /api/v1/cabinet":                           true,
		"GET /api/v1/cabinet/:id":                       true,
		"GET /api/v1/experiences":                       true,
		"POST /api/v1/experiences":                      true,
		"PATCH /api/v1/experiences/:id":                 true,
		"DELETE /api/v1/experiences/:id":                true,
		"GET /api/v1/notifications":                     true,
		"POST /api/v1/notifications/:id/read":           true,
		"POST /api/v1/connections/:id/slice-drafts":     true,
		"GET /api/v1/slice-drafts/:id":                  true,
		"PATCH /api/v1/slice-drafts/:id":                true,
		"POST /api/v1/slice-drafts/:id/publish":         true,
		"GET /api/v1/integrations/zhihu/status":         true,
		"POST /api/v1/integrations/zhihu/authorize":     true,
		"GET /api/v1/integrations/zhihu/callback":       true,
		"DELETE /api/v1/integrations/zhihu":             true,
	}
	for _, route := range router.Routes() {
		delete(want, route.Method+" "+route.Path)
	}
	for missing := range want {
		t.Errorf("missing route %s", missing)
	}
}

func TestCORSAllowsConfiguredVercelOrigin(t *testing.T) {
	router := New(&service.Service{}, Options{Origin: "https://demo.vercel.app", Origins: "https://preview.vercel.app, https://demo.example.com"})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/home", nil)
	req.Header.Set("Origin", "https://demo.example.com")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || res.Header().Get("Access-Control-Allow-Origin") != "https://demo.example.com" {
		t.Fatalf("status=%d allow-origin=%q", res.Code, res.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSRejectsUnconfiguredOrigin(t *testing.T) {
	router := New(&service.Service{}, Options{Origin: "https://demo.vercel.app"})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/home", nil)
	req.Header.Set("Origin", "https://attacker.example")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestProductionSessionCookieSupportsCrossOriginFrontend(t *testing.T) {
	a := &api{o: Options{SecureCookies: true}}
	cookie := a.sessionCookie("session", 60)
	if !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode {
		t.Fatalf("cookie must be Secure and SameSite=None: %+v", cookie)
	}
}
