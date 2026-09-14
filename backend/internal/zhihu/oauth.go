package zhihu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"driftbottle/internal/domain"
)

type Token struct {
	Value   string
	Expires time.Time
}
type User struct{ ID, Name, Avatar string }

func (c *Client) Authorize(state string) string {
	return c.OAuthBase + "/authorize?" + url.Values{"app_id": {c.AppID}, "redirect_uri": {c.Redirect}, "response_type": {"code"}, "state": {state}}.Encode()
}
func (c *Client) Exchange(ctx context.Context, code string) (Token, error) {
	var t Token
	if code == "" {
		return t, domain.Fail(400, "OAUTH_CODE_REQUIRED", "缺少授权码")
	}
	form := url.Values{"app_id": {c.AppID}, "app_key": {c.AppKey}, "grant_type": {"authorization_code"}, "redirect_uri": {c.Redirect}, "code": {code}}
	req, err := http.NewRequestWithContext(ctx, "POST", c.OAuthBase+"/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return t, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return t, domain.Fail(503, "OAUTH_EXCHANGE_FAILED", "授权交换失败，请重新授权")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return t, domain.Fail(503, "OAUTH_EXCHANGE_FAILED", "授权交换失败，请重新授权")
	}
	type value struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	var v struct {
		value
		Data value `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v); err != nil {
		return t, domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "授权响应格式无效")
	}
	x := v.value
	if x.AccessToken == "" {
		x = v.Data
	}
	if x.AccessToken == "" || x.ExpiresIn <= 0 {
		return t, domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "授权响应缺少有效期或 Token")
	}
	return Token{x.AccessToken, time.Now().Add(time.Duration(x.ExpiresIn) * time.Second)}, nil
}

func (c *Client) User(ctx context.Context, token string) (User, error) {
	var out User
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.OAuthBase+"/user", nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.HTTP.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		if res != nil {
			res.Body.Close()
		}
		return out, domain.Fail(503, "ZHIHU_USER_FAILED", "无法读取知乎用户信息")
	}
	defer res.Body.Close()
	var raw map[string]any
	decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
	decoder.UseNumber()
	if decoder.Decode(&raw) != nil {
		return out, domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "知乎用户响应格式无效")
	}
	profile, ok := zhihuUserObject(raw)
	if !ok {
		return User{}, domain.Fail(503, "ZHIHU_SUBJECT_MISSING", "知乎未返回稳定用户标识")
	}
	value := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := profile[key].(string); ok && v != "" {
				return v
			}
			if v, ok := profile[key].(json.Number); ok && v.String() != "" {
				return v.String()
			}
		}
		return ""
	}
	out = User{ID: value("id", "user_id", "uid", "url_token"), Name: value("name", "fullname", "full_name"), Avatar: value("avatar_url", "avatarUrl", "avatar")}
	if len(out.ID) > 180 || len(out.Name) > 128 || len(out.Avatar) > 2048 {
		return User{}, domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "知乎用户响应字段超限")
	}
	return out, nil
}

func zhihuUserObject(raw map[string]any) (map[string]any, bool) {
	queue := []map[string]any{raw}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, key := range []string{"id", "user_id", "uid", "url_token"} {
			switch value := current[key].(type) {
			case string:
				if value != "" {
					return current, true
				}
			case json.Number:
				if value.String() != "" {
					return current, true
				}
			}
		}
		for _, key := range []string{"data", "user", "user_info", "profile", "member"} {
			switch nested := current[key].(type) {
			case map[string]any:
				queue = append(queue, nested)
			case []any:
				for _, item := range nested {
					if object, ok := item.(map[string]any); ok {
						queue = append(queue, object)
					}
				}
			}
		}
	}
	return nil, false
}
func Crypt(key string) (cipher.AEAD, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("ZHIHU_TOKEN_ENCRYPTION_KEY must be base64 encoded 32 bytes")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func Encrypt(a cipher.AEAD, user, token string) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, []byte(token), []byte(user)), nil
}
func Decrypt(a cipher.AEAD, user string, raw []byte) (string, error) {
	if len(raw) < a.NonceSize() {
		return "", errors.New("invalid encrypted token")
	}
	b, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(user))
	return string(b), err
}
