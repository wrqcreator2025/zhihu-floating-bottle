package zhihu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"driftbottle/internal/domain"
)

type Client struct {
	Base, OAuthBase, Secret, AppID, AppKey, Redirect string
	HTTP                                             *http.Client
}

func New(secret, appID, appKey, redirect string) *Client {
	return &Client{Base: "https://developer.zhihu.com", OAuthBase: "https://openapi.zhihu.com", Secret: secret, AppID: appID, AppKey: appKey, Redirect: redirect, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type Content struct {
	Title       string
	Summary     string
	Url         string
	ContentType string
	CreatedAt   int64
}
type Followee struct{ Headline string }
type Paging struct {
	IsEnd      bool
	NextOffset string
}
type Contents struct {
	Items  []Content
	Paging Paging
}
type Followees struct {
	Items  []Followee
	Paging Paging
}
type SearchItem struct {
	ContentID, ContentType, Title, ContentText, Url string
	EditTime                                        int64
}
type SearchResult struct {
	Items   []SearchItem
	HasMore bool
}
type Quota struct {
	APIID                                 string
	TotalQuota, TotalUsed, RemainingQuota int64
}

// Get performs one actual call. The service owns cached budgets, including retry accounting.
func (c *Client) Get(ctx context.Context, path string, q url.Values, token string, out any) error {
	if c.Secret == "" {
		return domain.Fail(503, "ZHIHU_UNAVAILABLE", "知乎接口尚未配置")
	}
	if strings.HasPrefix(path, "/api/v1/user/") && token == "" {
		return domain.Fail(503, "ZHIHU_REAUTHORIZATION_REQUIRED", "请先授权知乎数据")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.Base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("X-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-OAuth-Token", token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return domain.Fail(503, "ZHIHU_UNAVAILABLE", "知乎暂时不可用")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return domain.Fail(503, "ZHIHU_REAUTHORIZATION_REQUIRED", "知乎授权已失效")
	}
	if res.StatusCode == 429 {
		return domain.Fail(429, "ZHIHU_RATE_LIMIT", "知乎接口额度或频率受限")
	}
	if res.StatusCode != 200 {
		return domain.Fail(503, "ZHIHU_UNAVAILABLE", "知乎暂时不可用")
	}
	var envelope struct {
		Code *int
		Data json.RawMessage
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&envelope); err != nil || envelope.Code == nil || len(envelope.Data) == 0 {
		return domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "知乎响应格式无效")
	}
	switch *envelope.Code {
	case 0:
	case 20001:
		return domain.Fail(503, "ZHIHU_REAUTHORIZATION_REQUIRED", "知乎授权已失效")
	case 30001, 30002:
		return domain.Fail(429, "ZHIHU_RATE_LIMIT", "知乎接口额度或频率受限")
	case 10001:
		return domain.Fail(503, "ZHIHU_INVALID_REQUEST", "知乎请求参数不被支持")
	default:
		return domain.Fail(503, "ZHIHU_UNAVAILABLE", "知乎暂时不可用")
	}
	if err = json.Unmarshal(envelope.Data, out); err != nil {
		return domain.Fail(503, "ZHIHU_PROTOCOL_ERROR", "知乎响应格式无效")
	}
	return nil
}
func (c *Client) Contents(ctx context.Context, token, offset string) (Contents, error) {
	var out Contents
	err := c.Get(ctx, "/api/v1/user/contents", url.Values{"ContentType": {"all"}, "SortField": {"ts"}, "SortOrder": {"desc"}, "Offset": {offset}, "Limit": {"20"}}, token, &out)
	return out, err
}
func (c *Client) Followees(ctx context.Context, token, offset string) (Followees, error) {
	var out Followees
	err := c.Get(ctx, "/api/v1/user/followees", url.Values{"Offset": {offset}, "Limit": {"20"}}, token, &out)
	return out, err
}
func (c *Client) Search(ctx context.Context, query string) (SearchResult, error) {
	var out SearchResult
	err := c.Get(ctx, "/api/v1/content/zhihu_search", url.Values{"Query": {query}, "Count": {"10"}}, "", &out)
	return out, err
}
func (c *Client) Quotas(ctx context.Context) ([]Quota, error) {
	var out []Quota
	err := c.Get(ctx, "/api/v1/quota", url.Values{"APIIDs": {"user_data,zhihu_search"}}, "", &out)
	return out, err
}
func Next(p Paging) (string, error) {
	if p.IsEnd {
		return "", nil
	}
	n, err := strconv.ParseInt(p.NextOffset, 10, 64)
	if err != nil || n < 0 {
		return "", fmt.Errorf("invalid paging offset")
	}
	return p.NextOffset, nil
}
