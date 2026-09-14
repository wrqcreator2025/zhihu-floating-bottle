package zhihu

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOfficialRequestContracts(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Request-Timestamp") == "" {
			t.Error("missing upstream headers")
		}
		switch r.URL.Path {
		case "/api/v1/user/contents":
			if r.Header.Get("X-OAuth-Token") != "user-token" || r.URL.Query().Get("ContentType") != "all" || r.URL.Query().Get("SortField") != "ts" || r.URL.Query().Get("Offset") != "0" {
				t.Error("user contract mismatch")
			}
			fmt.Fprint(w, `{"Code":0,"Data":{"Items":[],"Paging":{"IsEnd":true}}}`)
		case "/api/v1/content/zhihu_search":
			if r.Header.Get("X-OAuth-Token") != "" || r.URL.Query().Get("Query") != "中文 搜索" || r.URL.Query().Get("Count") != "10" {
				t.Error("search contract mismatch")
			}
			fmt.Fprint(w, `{"Code":0,"Data":{"Items":[{"ContentID":"123","Title":"test"}],"HasMore":false}}`)
		case "/api/v1/quota":
			if r.Header.Get("X-OAuth-Token") != "" || r.URL.Query().Get("APIIDs") != "user_data,zhihu_search" {
				t.Error("quota contract mismatch")
			}
			fmt.Fprint(w, `{"Code":0,"Data":[]}`)
		}
	}))
	defer server.Close()
	c := New("secret", "", "", "")
	c.Base = server.URL
	if _, err := c.Contents(context.Background(), "", "0"); err == nil {
		t.Fatal("missing token must not fall back to developer")
	}
	if calls != 0 {
		t.Fatal("unexpected request")
	}
	if _, err := c.Contents(context.Background(), "user-token", "0"); err != nil {
		t.Fatal(err)
	}
	out, err := c.Search(context.Background(), "中文 搜索")
	if err != nil || len(out.Items) != 1 {
		t.Fatalf("search %v", err)
	}
	if _, err = c.Quotas(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestOAuthCode20000AndEncryption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer oauth-token" {
				t.Error("wrong user request")
			}
			fmt.Fprint(w, `{"code":20000,"data":{"id":"stable-user-id","name":"知乎用户","avatar_url":"https://example.test/avatar.png"}}`)
			return
		}
		r.ParseForm()
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Form.Get("code") != "one-time-code" || r.Form.Get("authorization_code") != "" || r.Form.Get("grant_type") != "authorization_code" {
			t.Error("wrong token form")
		}
		fmt.Fprint(w, `{"code":20000,"access_token":"oauth-token","expires_in":3600}`)
	}))
	defer server.Close()
	c := New("secret", "app", "key", "https://example.test/callback")
	c.OAuthBase = server.URL
	tok, err := c.Exchange(context.Background(), "one-time-code")
	if err != nil || tok.Value != "oauth-token" || tok.Expires.Before(time.Now()) {
		t.Fatalf("exchange %v", err)
	}
	user, err := c.User(context.Background(), tok.Value)
	if err != nil || user.ID != "stable-user-id" || user.Name != "知乎用户" {
		t.Fatalf("user profile %#v %v", user, err)
	}
	a, err := Crypt(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := Encrypt(a, "user-a", tok.Value)
	if _, err = Decrypt(a, "user-b", raw); err == nil {
		t.Fatal("token ciphertext must bind owner")
	}
	v, err := Decrypt(a, "user-a", raw)
	if err != nil || v != tok.Value {
		t.Fatal("encryption round trip")
	}
}
func TestErrorsAndPaging(t *testing.T) {
	for _, body := range []string{`{"Data":{}}`, `{"Code":20001,"Data":{}}`, `{"Code":30002,"Data":{}}`, `not json`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c := New("secret", "", "", "")
		c.Base = server.URL
		if _, err := c.Search(context.Background(), "test"); err == nil {
			t.Fatalf("accepted %s", body)
		}
		server.Close()
	}
	if _, err := Next(Paging{NextOffset: "1.5"}); err == nil {
		t.Fatal("invalid paging accepted")
	}
}
