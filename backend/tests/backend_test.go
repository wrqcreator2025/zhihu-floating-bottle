package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"driftbottle/internal/auth"
	"driftbottle/internal/domain"
	"driftbottle/internal/httpapi"
	"driftbottle/internal/jobs"
	db "driftbottle/internal/repository/mysql"
	"driftbottle/internal/service"
	"driftbottle/internal/zhihu"
)

type fakeAI struct{}

func (fakeAI) Run(_ context.Context, task string, input, out any) error {
	raw := domain.JSON(input)
	var v any
	switch task {
	case "moderation":
		if strings.Contains(raw, "provider-offline") {
			return domain.Fail(503, "AI_UNAVAILABLE", "test outage")
		}
		v = map[string]any{"allowed": !strings.Contains(raw, "reject-content"), "reason": "test_review"}
	case "match":
		if strings.Contains(raw, "provider-offline") {
			return domain.Fail(503, "AI_UNAVAILABLE", "test outage")
		}
		var in struct {
			Candidates []struct {
				ID string `json:"experienceId"`
			}
		}
		json.Unmarshal([]byte(raw), &in)
		items := []any{}
		for _, c := range in.Candidates {
			items = append(items, map[string]any{"experienceId": c.ID, "eligible": true, "score": 0.9})
		}
		v = map[string]any{"items": items}
	case "slice":
		v = map[string]any{"title": "我的经历", "body": "我尝试了新的方法。"}
	case "episode":
		v = map[string]any{"title": "实习求职", "summary": "正在尝试"}
	case "target":
		v = map[string]any{"requiredExperiences": []string{"经历过求职"}}
	case "profile":
		v = map[string]any{"topics": []string{"求职"}}
	default:
		return fmt.Errorf("unexpected AI task %s", task)
	}
	return json.Unmarshal([]byte(domain.JSON(v)), out)
}

type fixture struct {
	t        *testing.T
	s        *service.Service
	r        http.Handler
	auth     auth.Auth
	subjects map[string]string
	tokens   map[string]string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN or run python3 scripts/test_backend.py")
	}
	store, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	if err = store.CheckSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := auth.Auth{Key: []byte("test-signing-key-32-bytes-or-longer"), Issuer: "tests", Audience: "web"}
	s := &service.Service{Store: store, AI: fakeAI{}, Zhihu: zhihu.New("", "", "", ""), AttemptLimit: 5, SearchBudget: 5, ActiveBottleLimit: 3}
	f := &fixture{t: t, s: s, auth: a, subjects: map[string]string{}, tokens: map[string]string{}}
	for _, name := range []string{"sender", "a", "b", "outsider"} {
		sub := "test-" + domain.ID() + name
		u, e := s.User(context.Background(), sub)
		if e != nil {
			t.Fatal(e)
		}
		f.subjects[name] = u
		f.tokens[name], _ = a.Issue(sub, time.Hour)
	}
	f.r = httpapi.New(s, httpapi.Options{Auth: a, Origin: "http://localhost:5173"})
	t.Cleanup(func() {
		for _, u := range f.subjects {
			store.DB.Exec(`UPDATE experiences SET receive_open=0 WHERE owner_id=?`, u)
		}
	})
	return f
}
func (f *fixture) request(who, method, path string, body any, want int) map[string]any {
	f.t.Helper()
	return f.keyRequest(who, method, path, body, "", want)
}
func (f *fixture) keyRequest(who, method, path string, body any, key string, want int) map[string]any {
	f.t.Helper()
	var b []byte
	if body != nil {
		b = []byte(domain.JSON(body))
	}
	r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+f.tokens[who])
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, r)
	if w.Code != want {
		f.t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	if want == 204 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		f.t.Fatal(err)
	}
	return out
}
func data(v map[string]any) map[string]any { return v["data"].(map[string]any) }
func (f *fixture) bottle(text string) string {
	v := data(f.request("sender", "POST", "/bottles", map[string]any{"episodeText": text}, 201))
	id := v["id"].(string)
	f.request("sender", "PATCH", "/bottles/"+id, map[string]any{"episode": map[string]any{"title": "实习求职", "confirmed": true}, "target": map[string]any{"requiredExperiences": []string{"实习求职"}}}, 200)
	return id
}
func (f *fixture) experience(who string) string {
	v := data(f.request(who, "POST", "/experiences", map[string]any{"title": "实习求职", "body": "经历过实习求职", "confirmedByUser": true, "receiveOpen": true, "disclosure": map[string]any{"summary": true}}, 201))
	return v["id"].(string)
}
func (f *fixture) match(bid string) {
	f.t.Helper()
	b, err := db.Bottle(context.Background(), f.s.Store.DB, strings.TrimPrefix(bid, "btl_"), false)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.s.MatchBottle(context.Background(), domain.JobPayload{ID: b.ID, Round: b.Round}); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) accept(who string) string {
	f.t.Helper()
	inv := data(f.request(who, "GET", "/invitations/next", nil, 200))["id"].(string)
	v := data(f.request(who, "POST", "/invitations/"+inv+"/decision", map[string]any{"decision": "accept"}, 200))
	again := data(f.request(who, "POST", "/invitations/"+inv+"/decision", map[string]any{"decision": "accept"}, 200))
	if again["connectionId"] != v["connectionId"] {
		f.t.Fatal("duplicate connection")
	}
	f.request(who, "POST", "/invitations/"+inv+"/decision", map[string]any{"decision": "not_now"}, 409)
	return v["connectionId"].(string)
}
func (f *fixture) moderate(mid string) {
	f.t.Helper()
	if err := f.s.ModerateMessage(context.Background(), domain.JobPayload{ID: strings.TrimPrefix(mid, "msg_"), Version: 1}); err != nil {
		f.t.Fatal(err)
	}
}
func TestBackendFlow(t *testing.T) {
	f := setup(t)
	f.experience("a")
	expB := f.experience("b")
	if len(f.request("a", "GET", "/experiences", nil, 200)["data"].([]any)) != 1 {
		t.Fatal("experience list is not connected")
	}
	bid := f.bottle("第一次求职很迷茫")
	f.request("outsider", "GET", "/bottles/"+bid, nil, 404)
	f.request("sender", "POST", "/bottles/"+bid+"/launch", nil, 200)
	f.request("sender", "POST", "/bottles/"+bid+"/launch", nil, 200)
	var expSource string
	var expOpen bool
	if err := f.s.Store.DB.QueryRow(`SELECT source,receive_open FROM experiences WHERE id=? AND owner_id=?`, strings.TrimPrefix(bid, "btl_"), f.subjects["sender"]).Scan(&expSource, &expOpen); err != nil || expSource != "bottle" || expOpen {
		t.Fatalf("launched bottle was not saved as a private experience: source=%q open=%v err=%v", expSource, expOpen, err)
	}
	other := f.bottle("另一个独立的瓶子")
	f.request("sender", "POST", "/bottles/"+other+"/launch", nil, 200)
	f.match(bid)
	ca := f.accept("a")
	cb := f.accept("b")
	f.request("a", "GET", "/connections/"+cb, nil, 404)
	f.request("b", "GET", "/connections/"+ca, nil, 404)
	f.request("b", "DELETE", "/experiences/"+expB, nil, 204)
	f.request("b", "GET", "/connections/"+cb, nil, 200)
	v := data(f.keyRequest("a", "POST", "/connections/"+ca+"/messages", map[string]any{"body": "我也经历过，后来继续尝试。"}, "reply-one", 202))
	mid := v["message"].(map[string]any)["id"].(string)
	again := data(f.keyRequest("a", "POST", "/connections/"+ca+"/messages", map[string]any{"body": "我也经历过，后来继续尝试。"}, "reply-one", 202))
	if again["message"].(map[string]any)["id"] != mid {
		t.Fatal("idempotency failed")
	}
	f.keyRequest("a", "POST", "/connections/"+ca+"/messages", map[string]any{"body": "different"}, "reply-one", 409)
	messages := f.request("sender", "GET", "/connections/"+ca+"/messages", nil, 200)["data"].([]any)
	if len(messages) != 0 {
		t.Fatal("unmoderated message leaked")
	}
	f.moderate(mid)
	f.moderate(mid)
	messages = f.request("sender", "GET", "/connections/"+ca+"/messages", nil, 200)["data"].([]any)
	if len(messages) != 1 {
		t.Fatal("delivery or deduplication failed")
	}
	notifications := f.request("sender", "GET", "/notifications?unreadOnly=true", nil, 200)["data"].([]any)
	if len(notifications) == 0 {
		t.Fatal("delivered reply did not create a notification")
	}
	notificationID := notifications[0].(map[string]any)["id"].(string)
	f.request("sender", "POST", "/notifications/"+notificationID+"/read", nil, 200)
	f.request("sender", "POST", "/notifications/"+notificationID+"/read", nil, 200)
	f.request("sender", "POST", "/connections/"+ca+"/feedback", map[string]any{"result": "felt_understood"}, 200)
	f.request("sender", "POST", "/connections/"+ca+"/feedback", map[string]any{"result": "felt_understood"}, 200)
	f.request("sender", "POST", "/connections/"+ca+"/chat/messages", map[string]any{"body": "想继续聊聊"}, 409)
	ci := data(f.request("sender", "POST", "/connections/"+ca+"/chat-invitations", nil, 200))["id"].(string)
	f.request("sender", "POST", "/chat-invitations/"+ci+"/decision", map[string]any{"decision": "accept"}, 404)
	f.request("a", "POST", "/chat-invitations/"+ci+"/decision", map[string]any{"decision": "accept"}, 200)
	f.request("a", "POST", "/chat-invitations/"+ci+"/decision", map[string]any{"decision": "accept"}, 200)
	chat := data(f.request("sender", "POST", "/connections/"+ca+"/chat/messages", map[string]any{"body": "谢谢你的回信"}, 202))["message"].(map[string]any)["id"].(string)
	f.moderate(chat)
	f.request("b", "GET", "/connections/"+ca+"/messages", nil, 404)
	cab := f.request("sender", "GET", "/cabinet?direction=sent&limit=1", nil, 200)
	if len(cab["data"].([]any)) != 1 {
		t.Fatal("cabinet")
	}
	sentRecord := cab["data"].([]any)[0].(map[string]any)["id"].(string)
	f.request("sender", "GET", "/cabinet/"+sentRecord, nil, 200)
	received := f.request("a", "GET", "/cabinet?direction=received", nil, 200)["data"].([]any)
	if len(received) != 1 {
		t.Fatal("received cabinet")
	}
	f.request("a", "GET", "/cabinet/"+received[0].(map[string]any)["id"].(string), nil, 200)
	f.request("sender", "GET", "/home", nil, 200)
	f.match(bid)
	status := data(f.request("sender", "GET", "/bottles/"+bid+"/search-status", nil, 200))["status"]
	if status != "completed" {
		t.Fatalf("status %v", status)
	}
	f.request("sender", "POST", "/connections/"+ca+"/close", nil, 200)
	f.request("sender", "POST", "/connections/"+ca+"/close", nil, 200)
	f.request("sender", "POST", "/connections/"+ca+"/chat/messages", map[string]any{"body": "closed"}, 409)
	slice := data(f.request("a", "POST", "/connections/"+ca+"/slice-drafts", map[string]any{"consent": true}, 202))["id"].(string)
	if err := f.s.GenerateSlice(context.Background(), domain.JobPayload{ID: strings.TrimPrefix(slice, "slc_")}); err != nil {
		t.Fatal(err)
	}
	f.request("sender", "GET", "/slice-drafts/"+slice, nil, 404)
	f.request("a", "PATCH", "/slice-drafts/"+slice, map[string]any{"title": "我的经历"}, 200)
	f.request("a", "POST", "/slice-drafts/"+slice+"/publish", nil, 200)
	f.request("a", "POST", "/slice-drafts/"+slice+"/publish", nil, 200)
}
func TestConcurrentLaunchAndPause(t *testing.T) {
	f := setup(t)
	bottles := []string{f.bottle("a"), f.bottle("b"), f.bottle("c"), f.bottle("d")}
	var wg sync.WaitGroup
	results := make(chan error, len(bottles))
	for _, id := range bottles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.BottleAction(context.Background(), f.subjects["sender"], strings.TrimPrefix(id, "btl_"), "launch", nil)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 3 {
		t.Fatalf("wanted three successful launches, got %d", success)
	}
	var active string
	f.s.Store.DB.QueryRow(`SELECT bottle_id FROM active_search_slots WHERE user_id=? ORDER BY bottle_id LIMIT 1`, f.subjects["sender"]).Scan(&active)
	f.request("sender", "POST", "/bottles/btl_"+active+"/pause", nil, 200)
	f.request("sender", "POST", "/bottles/btl_"+active+"/pause", nil, 200)
	failed := ""
	for _, id := range bottles {
		b, _ := db.Bottle(context.Background(), f.s.Store.DB, strings.TrimPrefix(id, "btl_"), false)
		if b.Status == "draft" {
			failed = id
		}
	}
	f.request("sender", "POST", "/bottles/"+failed+"/launch", nil, 200)
	f.request("sender", "POST", "/bottles/btl_"+active+"/resume", nil, 409)
	home := data(f.request("sender", "GET", "/home", nil, 200))
	if len(home["activeBottles"].([]any)) != 3 || home["activeBottleLimit"] != float64(3) {
		t.Fatal(home)
	}
}
func TestModerationRejectRetryAndMatchFallback(t *testing.T) {
	f := setup(t)
	f.experience("a")
	bid := f.bottle("普通求助")
	f.request("sender", "POST", "/bottles/"+bid+"/launch", nil, 200)
	f.match(bid)
	cid := f.accept("a")
	mid := data(f.request("a", "POST", "/connections/"+cid+"/messages", map[string]any{"body": "加我微信 abc123"}, 202))["message"].(map[string]any)["id"].(string)
	f.moderate(mid)
	if len(f.request("sender", "GET", "/connections/"+cid+"/messages", nil, 200)["data"].([]any)) != 0 {
		t.Fatal("rejected content leaked")
	}
	f.request("a", "PATCH", "/messages/"+mid, map[string]any{"body": "修改后的正常回信"}, 202)
	if e := f.s.ModerateMessage(context.Background(), domain.JobPayload{ID: strings.TrimPrefix(mid, "msg_"), Version: 1}); e != nil {
		t.Fatal(e)
	}
	if e := f.s.ModerateMessage(context.Background(), domain.JobPayload{ID: strings.TrimPrefix(mid, "msg_"), Version: 2}); e != nil {
		t.Fatal(e)
	}
	// Matching AI failures should degrade to broad matching instead of failing the bottle.
	f.request("sender", "POST", "/bottles/"+bid+"/pause", nil, 200)
	failed := f.bottle("provider-offline")
	f.request("sender", "POST", "/bottles/"+failed+"/launch", nil, 200)
	bare := strings.TrimPrefix(failed, "btl_")
	_, e := f.s.Store.DB.Exec(`UPDATE outbox_jobs SET attempts=4,available_at='2000-01-01' WHERE type='match_bottle' AND JSON_UNQUOTE(JSON_EXTRACT(payload,'$.id'))=?`, bare)
	if e != nil {
		t.Fatal(e)
	}
	w := jobs.Worker{Service: f.s}
	worked, e := w.Once(context.Background())
	if e != nil || !worked {
		t.Fatalf("worker: %v", e)
	}
	search := data(f.request("sender", "GET", "/bottles/"+failed+"/search-status", nil, 200))
	if search["status"] != "searching" || int(search["attemptedCount"].(float64)) == 0 {
		t.Fatalf("matching did not fall back: %v", search)
	}
}
func TestValidationAndUnavailableCapabilities(t *testing.T) {
	f := setup(t)
	f.request("sender", "POST", "/bottles", map[string]any{"episodeText": "  "}, 400)
	f.request("sender", "POST", "/bottles", map[string]any{"episodeText": "hello", "userId": "forged"}, 400)
	f.request("sender", "GET", "/notifications?limit=51", nil, 400)
	f.request("sender", "POST", "/ai/experience-suggestions", map[string]any{"query": "求职"}, 503)
	f.request("sender", "POST", "/integrations/zhihu/authorize", nil, 503)
	f.request("sender", "GET", "/integrations/zhihu/status", nil, 200)
	bid := f.bottle("问题")
	f.request("sender", "POST", "/ai/episode-drafts", map[string]any{"bottleId": bid, "contentVersion": 1}, 409)
	f.request("sender", "POST", "/ai/episode-drafts", map[string]any{"bottleId": bid, "contentVersion": 2}, 200)
}

func TestInvitationExpiryPauseRetryAndCabinetPagination(t *testing.T) {
	f := setup(t)
	f.experience("a")
	bid := f.bottle("第一次求职")
	f.request("sender", "POST", "/bottles/"+bid+"/launch", nil, 200)
	f.match(bid)
	inv := data(f.request("a", "GET", "/invitations/next", nil, 200))["id"].(string)
	f.request("sender", "POST", "/bottles/"+bid+"/pause", nil, 200)
	v := data(f.request("a", "POST", "/invitations/"+inv+"/decision", map[string]any{"decision": "not_now"}, 200))
	if v["bottleContinuesMatching"] != false {
		t.Fatal("paused bottle resumed by decision")
	}
	f.request("sender", "POST", "/bottles/"+bid+"/resume", nil, 200)
	f.match(bid)
	f.request("sender", "POST", "/bottles/"+bid+"/retry", nil, 200)
	f.request("sender", "POST", "/bottles/"+bid+"/retry", nil, 200)
	b, err := db.Bottle(context.Background(), f.s.Store.DB, strings.TrimPrefix(bid, "btl_"), false)
	if err != nil || b.Round != 2 {
		t.Fatalf("retry round %d %v", b.Round, err)
	}
	f.match(bid)
	f.request("a", "GET", "/invitations/next", nil, 204)
	// A distinct bottle can be sent after the previous search releases its slot.
	second := f.bottle("新的处境")
	f.request("sender", "POST", "/bottles/"+second+"/launch", nil, 200)
	f.match(second)
	inv = data(f.request("a", "GET", "/invitations/next", nil, 200))["id"].(string)
	_, err = f.s.Store.DB.Exec(`UPDATE match_invitations SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 SECOND WHERE id=?`, strings.TrimPrefix(inv, "inv_"))
	if err != nil {
		t.Fatal(err)
	}
	f.request("a", "POST", "/invitations/"+inv+"/decision", map[string]any{"decision": "accept"}, 409)
	w := jobs.Worker{Service: f.s}
	if err = w.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	f.s.Store.DB.QueryRow(`SELECT status FROM match_invitations WHERE id=?`, strings.TrimPrefix(inv, "inv_")).Scan(&status)
	if status != "expired" {
		t.Fatal(status)
	}
	first := f.request("sender", "GET", "/cabinet?direction=sent&limit=1", nil, 200)
	cursor := first["nextCursor"].(string)
	next := f.request("sender", "GET", "/cabinet?direction=sent&limit=1&cursor="+cursor, nil, 200)
	if len(next["data"].([]any)) != 1 || next["nextCursor"] != nil {
		t.Fatal("pagination did not reach distinct final record")
	}
	if first["data"].([]any)[0].(map[string]any)["id"] == next["data"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("pagination duplicated record")
	}
}

func TestSharedSearchBudgetAndCache(t *testing.T) {
	f := setup(t)
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		fmt.Fprint(w, `{"Code":0,"Data":{"Items":[{"ContentID":"cached-id","Title":"摘要"}],"HasMore":false}}`)
	}))
	defer server.Close()
	f.s.Zhihu = zhihu.New("test-secret-"+domain.ID(), "", "", "")
	f.s.Zhihu.Base = server.URL
	query := "query-" + domain.ID()
	a, err := f.s.Search(context.Background(), f.subjects["sender"], query)
	if err != nil || len(a.Items) != 1 {
		t.Fatal(err)
	}
	b, err := f.s.Search(context.Background(), f.subjects["a"], query)
	if err != nil || len(b.Items) != 1 || b.Items[0].ContentID != "cached-id" {
		t.Fatal("cache response lost", err)
	}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.s.Search(context.Background(), f.subjects["b"], fmt.Sprintf("%s-%d", query, i))
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calls != 5 {
		t.Fatalf("shared budget allowed %d calls, want 5", calls)
	}
	status := data(f.request("sender", "GET", "/integrations/zhihu/status", nil, 200))
	if status["usedToday"] != float64(5) || status["searchAvailable"] != false {
		t.Fatal(status)
	}
}

func TestBottleSkipsModerationAndPreservesBlocks(t *testing.T) {
	f := setup(t)
	f.experience("a")
	bid := f.bottle("reject-content")
	f.request("sender", "POST", "/bottles/"+bid+"/launch", nil, 200)
	f.match(bid)
	b, err := db.Bottle(context.Background(), f.s.Store.DB, strings.TrimPrefix(bid, "btl_"), false)
	if err != nil || b.Status != "searching" {
		t.Fatal("bottle did not proceed directly to matching")
	}
	var reviews int
	if err := f.s.Store.DB.QueryRow(`SELECT COUNT(*) FROM moderation_reviews WHERE resource_type='bottle' AND resource_id=?`, b.ID).Scan(&reviews); err != nil || reviews != 0 {
		t.Fatalf("unexpected bottle moderation: count=%d err=%v", reviews, err)
	}
	cid := f.accept("a")
	f.request("sender", "POST", "/connections/"+cid+"/block", nil, 200)
	f.request("a", "POST", "/connections/"+cid+"/messages", map[string]any{"body": "blocked"}, 409)
	f.request("sender", "POST", "/connections/"+cid+"/reports", map[string]any{"reason": "不希望继续联系"}, 201)
}

func TestZhihuAuthorizationBindingAndProfile(t *testing.T) {
	f := setup(t)
	exchanges := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access_token":
			exchanges++
			fmt.Fprint(w, `{"code":20000,"access_token":"authorized-user-token","expires_in":3600}`)
		case "/api/v1/user/contents":
			if r.Header.Get("X-OAuth-Token") != "authorized-user-token" {
				t.Error("wrong OAuth user")
			}
			fmt.Fprint(w, `{"Code":0,"Data":{"Items":[{"Title":"实习经历","Summary":"我的求职尝试"}],"Paging":{"IsEnd":true}}}`)
		case "/api/v1/user/followees":
			fmt.Fprint(w, `{"Code":0,"Data":{"Items":[{"Headline":"产品工作"}],"Paging":{"IsEnd":true}}}`)
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	f.s.Zhihu = zhihu.New("access-secret", "app-id", "app-key", "https://app.test/api/v1/integrations/zhihu/callback")
	f.s.Zhihu.Base = upstream.URL
	f.s.Zhihu.OAuthBase = upstream.URL
	crypt, err := zhihu.Crypt("eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHg=")
	if err != nil {
		t.Fatal(err)
	}
	f.r = httpapi.New(f.s, httpapi.Options{Auth: f.auth, Cipher: crypt, Origin: "http://localhost:5173"})
	request := httptest.NewRequest("POST", "/api/v1/integrations/zhihu/authorize", nil)
	request.Header.Set("Authorization", "Bearer "+f.tokens["sender"])
	response := httptest.NewRecorder()
	f.r.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var envelope map[string]any
	json.Unmarshal(response.Body.Bytes(), &envelope)
	authorizationURL := data(envelope)["authorizationUrl"].(string)
	u, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	cookie := response.Result().Cookies()[0]
	callback := func(query string, includeCookie bool, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/integrations/zhihu/callback?"+query, nil)
		if includeCookie {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		f.r.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("callback %d: %s", w.Code, w.Body.String())
		}
	}
	callback("authorization_code=code", true, 400)
	callback("state="+state+"&authorization_code=code", false, 400)
	if exchanges != 0 {
		t.Fatal("unbound code exchanged")
	}
	callback("state="+state+"&authorization_code=code", true, 200)
	callback("state="+state+"&authorization_code=code", true, 400)
	if exchanges != 1 {
		t.Fatal("OAuth callback replayed")
	}
	var raw []byte
	f.s.Store.DB.QueryRow(`SELECT oauth_token_ciphertext FROM zhihu_integrations WHERE user_id=?`, f.subjects["sender"]).Scan(&raw)
	if bytes.Contains(raw, []byte("authorized-user-token")) {
		t.Fatal("plaintext token persisted")
	}
	if err = f.s.RefreshActivity(context.Background(), f.subjects["sender"], crypt); err != nil {
		t.Fatal(err)
	}
	status := data(f.request("sender", "GET", "/integrations/zhihu/status", nil, 200))
	if status["activityProfileAvailable"] != true || status["followFeedAvailable"] != false {
		t.Fatal(status)
	}
	f.request("sender", "DELETE", "/integrations/zhihu", nil, 204)
	status = data(f.request("sender", "GET", "/integrations/zhihu/status", nil, 200))
	if status["activityProfileAvailable"] != false {
		t.Fatal("profile retained after disconnect")
	}
}
