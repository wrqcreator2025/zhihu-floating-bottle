package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"driftbottle/internal/service"
)

func TestOAuthAttemptAcceptsMissingStateWithBoundCookie(t *testing.T) {
	a := &api{oauth: map[string]oauthAttempt{
		"expected-state": {Nonce: "browser-nonce", Login: true, Expires: time.Now().Add(time.Minute)},
	}}
	attempt, ok := a.takeOAuthAttempt("", "browser-nonce")
	if !ok || !attempt.Login || len(a.oauth) != 0 {
		t.Fatalf("attempt=%+v ok=%v remaining=%d", attempt, ok, len(a.oauth))
	}
}

func TestOAuthAttemptRejectsWrongStateOrCookie(t *testing.T) {
	for _, tc := range []struct{ state, cookie string }{
		{"wrong-state", "browser-nonce"},
		{"expected-state", "wrong-nonce"},
		{"", "wrong-nonce"},
	} {
		a := &api{oauth: map[string]oauthAttempt{
			"expected-state": {Nonce: "browser-nonce", Expires: time.Now().Add(time.Minute)},
		}}
		if _, ok := a.takeOAuthAttempt(tc.state, tc.cookie); ok {
			t.Fatalf("accepted state=%q cookie=%q", tc.state, tc.cookie)
		}
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
