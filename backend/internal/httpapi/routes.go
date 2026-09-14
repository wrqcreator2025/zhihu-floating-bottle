package httpapi

import (
	"context"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"driftbottle/internal/auth"
	"driftbottle/internal/domain"
	"driftbottle/internal/service"
	"driftbottle/internal/zhihu"
)

type Options struct {
	Auth          auth.Auth
	Origin        string
	Origins       string
	Cipher        cipher.AEAD
	SecureCookies bool
}
type oauthAttempt struct {
	User, Nonce string
	Login       bool
	Expires     time.Time
}
type api struct {
	s     *service.Service
	o     Options
	mu    sync.Mutex
	oauth map[string]oauthAttempt
}

func New(s *service.Service, o Options) *gin.Engine {
	a := &api{s: s, o: o, oauth: map[string]oauthAttempt{}}
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(a.middleware())
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if s.Store.DB.PingContext(ctx) != nil {
			respond(c, nil, domain.Fail(503, "DATABASE_UNAVAILABLE", "数据库暂时不可用"), 200)
			return
		}
		c.JSON(200, gin.H{"data": gin.H{"status": "ok"}})
	})
	r.GET("/api/v1/auth/zhihu", a.oauthLogin)
	r.GET("/api/v1/integrations/zhihu/callback", a.oauthCallback)
	g := r.Group("/api/v1", a.authenticate)
	g.GET("/auth/session", a.session)
	g.POST("/auth/logout", a.logout)
	g.GET("/home", func(c *gin.Context) { v, e := s.Home(ctx(c), user(c)); respond(c, v, e, 200) })
	g.POST("/bottles", func(c *gin.Context) {
		var in service.BottleCreate
		if !bind(c, &in) || !text(c, &in.EpisodeText, 4000, true) || !text(c, &in.TargetHint, 4000, false) {
			return
		}
		v, e := s.CreateBottle(ctx(c), user(c), in)
		respond(c, v, e, 201)
	})
	g.PATCH("/bottles/:id", func(c *gin.Context) {
		var in service.BottlePatch
		if !bind(c, &in) || !bottlePatch(c, in) {
			return
		}
		id, ok := pathID(c, "btl")
		if !ok {
			return
		}
		v, e := s.EditBottle(ctx(c), user(c), id, in)
		respond(c, v, e, 200)
	})
	g.GET("/bottles/:id", func(c *gin.Context) {
		id, ok := pathID(c, "btl")
		if !ok {
			return
		}
		v, e := s.BottleDetail(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	for _, action := range []string{"launch", "pause", "resume", "retry"} {
		g.POST("/bottles/:id/"+action, func(c *gin.Context) {
			id, ok := pathID(c, "btl")
			if !ok {
				return
			}
			var in struct {
				Target *service.TargetPatch `json:"target"`
			}
			if action == "retry" {
				if !bindOptional(c, &in) {
					return
				}
				if in.Target != nil && !targetPatch(c, in.Target) {
					return
				}
			}
			v, e := s.BottleAction(ctx(c), user(c), id, action, in.Target)
			respond(c, v, e, 200)
		})
	}
	g.POST("/bottles/:id/appeal", func(c *gin.Context) {
		id, ok := pathID(c, "btl")
		if !ok {
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !bind(c, &in) || !text(c, &in.Reason, 2000, true) {
			return
		}
		v, e := s.AppealBottle(ctx(c), user(c), id, in.Reason)
		respond(c, v, e, 200)
	})

	g.GET("/bottles/:id/search-status", func(c *gin.Context) {
		id, ok := pathID(c, "btl")
		if !ok {
			return
		}
		v, e := s.SearchStatus(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	for _, kind := range []string{"episode", "target"} {
		g.POST("/ai/"+kind+"-drafts", func(c *gin.Context) {
			var in struct {
				ID      string `json:"bottleId"`
				Version int    `json:"contentVersion"`
			}
			if !bind(c, &in) {
				return
			}
			id, ok := parseID(c, in.ID, "btl")
			if !ok {
				return
			}
			if in.Version < 1 {
				invalid(c, "contentVersion 必须为正整数")
				return
			}
			v, e := s.Draft(ctx(c), user(c), id, kind, in.Version)
			respond(c, v, e, 200)
		})
	}
	g.GET("/invitations/next", func(c *gin.Context) {
		v, e := s.NextInvitation(ctx(c), user(c))
		if v == nil && e == nil {
			c.Status(204)
			return
		}
		respond(c, v, e, 200)
	})
	g.POST("/invitations/:id/decision", func(c *gin.Context) {
		id, ok := pathID(c, "inv")
		if !ok {
			return
		}
		var in struct {
			Decision string `json:"decision"`
		}
		if !bind(c, &in) || !enum(c, in.Decision, "accept", "not_now", "not_mine") {
			return
		}
		v, e := s.Decide(ctx(c), user(c), id, in.Decision)
		respond(c, v, e, 200)
	})
	g.GET("/connections/:id", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		v, e := s.ConnectionDetail(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	for _, route := range []struct{ Path, Kind string }{{"messages", "reply"}, {"chat/messages", "chat"}} {
		g.POST("/connections/:id/"+route.Path, func(c *gin.Context) {
			id, ok := pathID(c, "con")
			if !ok {
				return
			}
			var in struct {
				Body string `json:"body"`
			}
			if !bind(c, &in) || !text(c, &in.Body, 8000, true) {
				return
			}
			key := c.GetHeader("Idempotency-Key")
			if len(key) > 128 {
				invalid(c, "Idempotency-Key 过长")
				return
			}
			for _, r := range key {
				if r < 33 || r > 126 {
					invalid(c, "Idempotency-Key 只接受可打印 ASCII 字符")
					return
				}
			}
			v, e := s.SendMessage(ctx(c), user(c), id, in.Body, key, route.Kind)
			respond(c, v, e, 202)
		})
	}
	g.GET("/connections/:id/messages", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		cursor, limit, ok := page(c, "msg")
		if !ok {
			return
		}
		v, next, e := s.Messages(ctx(c), user(c), id, cursor, limit)
		list(c, v, next, e)
	})
	g.PATCH("/messages/:id", func(c *gin.Context) {
		id, ok := pathID(c, "msg")
		if !ok {
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if !bind(c, &in) || !text(c, &in.Body, 8000, true) {
			return
		}
		v, e := s.EditMessage(ctx(c), user(c), id, &in.Body, "")
		respond(c, v, e, 202)
	})
	g.POST("/messages/:id/retry", func(c *gin.Context) {
		id, ok := pathID(c, "msg")
		if !ok {
			return
		}
		v, e := s.EditMessage(ctx(c), user(c), id, nil, "")
		respond(c, v, e, 202)
	})
	g.POST("/messages/:id/appeal", func(c *gin.Context) {
		id, ok := pathID(c, "msg")
		if !ok {
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !bind(c, &in) || !text(c, &in.Reason, 2000, true) {
			return
		}
		v, e := s.EditMessage(ctx(c), user(c), id, nil, in.Reason)
		respond(c, v, e, 202)
	})
	g.POST("/connections/:id/close", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		v, e := s.CloseConnection(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.POST("/connections/:id/feedback", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		var in struct {
			Result string `json:"result"`
		}
		if !bind(c, &in) || !enum(c, in.Result, "felt_understood", "similar_but_missed", "wrong_experience") {
			return
		}
		v, e := s.Feedback(ctx(c), user(c), id, in.Result)
		respond(c, v, e, 200)
	})
	g.POST("/connections/:id/chat-invitations", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		v, e := s.InviteChat(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.GET("/connections/:id/chat", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		v, e := s.ChatStatus(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.POST("/chat-invitations/:id/decision", func(c *gin.Context) {
		id, ok := pathID(c, "chi")
		if !ok {
			return
		}
		var in struct {
			Decision string `json:"decision"`
		}
		if !bind(c, &in) || !enum(c, in.Decision, "accept", "decline") {
			return
		}
		v, e := s.DecideChat(ctx(c), user(c), id, in.Decision)
		respond(c, v, e, 200)
	})
	g.POST("/connections/:id/reports", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		var in struct {
			Reason    string `json:"reason"`
			MessageID string `json:"messageId"`
		}
		if !bind(c, &in) || !text(c, &in.Reason, 2000, true) {
			return
		}
		mid := ""
		if in.MessageID != "" {
			mid, ok = parseID(c, in.MessageID, "msg")
			if !ok {
				return
			}
		}
		v, e := s.Report(ctx(c), user(c), id, in.Reason, mid)
		respond(c, v, e, 201)
	})
	g.POST("/connections/:id/block", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		respond(c, gin.H{"blocked": true}, s.Block(ctx(c), user(c), id), 200)
	})
	g.GET("/cabinet", func(c *gin.Context) {
		direction := c.Query("direction")
		if !enum(c, direction, "sent", "received") {
			return
		}
		cursor, limit, ok := page(c, "cab_"+direction)
		if !ok {
			return
		}
		v, next, e := s.Cabinet(ctx(c), user(c), direction, cursor, limit)
		list(c, v, next, e)
	})
	g.GET("/cabinet/:id", func(c *gin.Context) { v, e := s.CabinetDetail(ctx(c), user(c), c.Param("id")); respond(c, v, e, 200) })
	g.GET("/experiences", func(c *gin.Context) { v, e := s.Experiences(ctx(c), user(c)); respond(c, v, e, 200) })
	g.POST("/experiences", func(c *gin.Context) {
		var in service.ExperienceInput
		if !bind(c, &in) || !experienceInput(c, in, true) {
			return
		}
		v, e := s.SaveExperience(ctx(c), user(c), "", in)
		respond(c, v, e, 201)
	})
	g.PATCH("/experiences/:id", func(c *gin.Context) {
		id, ok := pathID(c, "exp")
		if !ok {
			return
		}
		var in service.ExperienceInput
		if !bind(c, &in) || !experienceInput(c, in, false) {
			return
		}
		v, e := s.SaveExperience(ctx(c), user(c), id, in)
		respond(c, v, e, 200)
	})
	g.DELETE("/experiences/:id", func(c *gin.Context) {
		id, ok := pathID(c, "exp")
		if !ok {
			return
		}
		respond(c, nil, s.DeleteExperience(ctx(c), user(c), id), 204)
	})
	g.POST("/ai/experience-suggestions", func(c *gin.Context) {
		var in struct {
			Query string `json:"query"`
		}
		if !bind(c, &in) || !text(c, &in.Query, 4000, true) {
			return
		}
		respond(c, nil, domain.Fail(503, "ZHIHU_AUTHOR_FILTER_UNAVAILABLE", "知乎搜索暂不支持可靠限定本人的公开表达，请手动填写经历"), 202)
	})
	g.GET("/ai/experience-suggestions/:id", func(c *gin.Context) {
		id, ok := pathID(c, "job")
		if !ok {
			return
		}
		v, e := s.Suggestion(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.GET("/notifications", func(c *gin.Context) {
		cursor, limit, ok := page(c, "ntf")
		if !ok {
			return
		}
		unread := c.Query("unreadOnly")
		if unread != "" && !enum(c, unread, "true", "false") {
			return
		}
		v, next, e := s.Notifications(ctx(c), user(c), cursor, limit, unread == "true")
		list(c, v, next, e)
	})
	g.POST("/notifications/:id/read", func(c *gin.Context) {
		id, ok := pathID(c, "ntf")
		if !ok {
			return
		}
		v, e := s.ReadNotification(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.POST("/connections/:id/slice-drafts", func(c *gin.Context) {
		id, ok := pathID(c, "con")
		if !ok {
			return
		}
		var in struct {
			Consent bool `json:"consent"`
		}
		if !bind(c, &in) {
			return
		}
		if !in.Consent {
			invalid(c, "需要作者主动同意生成草稿")
			return
		}
		v, e := s.CreateSlice(ctx(c), user(c), id)
		respond(c, v, e, 202)
	})
	g.GET("/slice-drafts/:id", func(c *gin.Context) {
		id, ok := pathID(c, "slc")
		if !ok {
			return
		}
		v, e := s.Slice(ctx(c), user(c), id)
		respond(c, v, e, 200)
	})
	g.PATCH("/slice-drafts/:id", func(c *gin.Context) {
		id, ok := pathID(c, "slc")
		if !ok {
			return
		}
		var in struct {
			Title *string `json:"title"`
			Body  *string `json:"body"`
		}
		if !bind(c, &in) {
			return
		}
		if in.Title == nil && in.Body == nil {
			invalid(c, "需要修改标题或正文")
			return
		}
		if in.Title != nil && !text(c, in.Title, 80, true) || in.Body != nil && !text(c, in.Body, 8000, true) {
			return
		}
		v, e := s.EditSlice(ctx(c), user(c), id, in.Title, in.Body, false)
		respond(c, v, e, 200)
	})
	g.POST("/slice-drafts/:id/publish", func(c *gin.Context) {
		id, ok := pathID(c, "slc")
		if !ok {
			return
		}
		v, e := s.EditSlice(ctx(c), user(c), id, nil, nil, true)
		respond(c, v, e, 200)
	})
	g.GET("/integrations/zhihu/status", func(c *gin.Context) { v, e := s.IntegrationStatus(ctx(c), user(c)); respond(c, v, e, 200) })
	g.POST("/integrations/zhihu/authorize", a.oauthStart)
	g.DELETE("/integrations/zhihu", func(c *gin.Context) { respond(c, nil, s.DisconnectZhihu(ctx(c), user(c)), 204) })
	r.NoRoute(func(c *gin.Context) { respond(c, nil, domain.NotFound, 404) })
	return r
}
func ctx(c *gin.Context) context.Context { return c.Request.Context() }
func user(c *gin.Context) string         { return c.GetString("user") }
func respond(c *gin.Context, v any, err error, status int) {
	if err != nil {
		var e *domain.Error
		if !errors.As(err, &e) {
			slog.Error("request failed", "request_id", c.GetString("requestID"))
			e = &domain.Error{Status: 500, Code: "INTERNAL_ERROR", Message: "服务暂时出错"}
		}
		c.AbortWithStatusJSON(e.Status, gin.H{"error": e})
		return
	}
	if status == 204 {
		c.Status(status)
		return
	}
	c.JSON(status, gin.H{"data": v})
}
func list(c *gin.Context, v any, next *string, e error) {
	if e != nil {
		respond(c, nil, e, 200)
		return
	}
	c.JSON(200, gin.H{"data": v, "nextCursor": next})
}
func invalid(c *gin.Context, msg string) {
	respond(c, nil, domain.Fail(400, "INVALID_REQUEST", msg), 400)
}
func bind(c *gin.Context, v any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	err := d.Decode(v)
	if err == nil {
		var extra any
		if e := d.Decode(&extra); e != io.EOF {
			err = errors.New("trailing JSON")
		}
	}
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			respond(c, nil, domain.Fail(413, "BODY_TOO_LARGE", "请求正文过大"), 413)
		} else {
			invalid(c, "请求 JSON 格式或字段无效")
		}
		return false
	}
	return true
}
func bindOptional(c *gin.Context, v any) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	return bind(c, v)
}
func text(c *gin.Context, s *string, max int, required bool) bool {
	*s = strings.TrimSpace(*s)
	if required && *s == "" || utf8.RuneCountInString(*s) > max {
		invalid(c, "文字为空或超过长度限制")
		return false
	}
	return true
}
func enum(c *gin.Context, v string, values ...string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	invalid(c, "不支持的选项")
	return false
}
func parseID(c *gin.Context, v, prefix string) (string, bool) {
	s, ok := strings.CutPrefix(v, prefix+"_")
	if !ok || len(s) != 26 {
		invalid(c, "资源 ID 无效")
		return "", false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", r) {
			invalid(c, "资源 ID 无效")
			return "", false
		}
	}
	return s, true
}
func pathID(c *gin.Context, prefix string) (string, bool) { return parseID(c, c.Param("id"), prefix) }
func page(c *gin.Context, prefix string) (string, int, bool) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 50 {
			invalid(c, "limit 必须为 1–50")
			return "", 0, false
		}
		limit = n
	}
	cursor := c.Query("cursor")
	if cursor != "" {
		var ok bool
		cursor, ok = parseID(c, cursor, prefix)
		if !ok {
			return "", 0, false
		}
	}
	return cursor, limit, true
}
func targetPatch(c *gin.Context, p *service.TargetPatch) bool {
	if p.Hint != nil && !text(c, p.Hint, 4000, true) {
		return false
	}
	for _, a := range []*[]string{p.Required, p.Preferred, p.Viewpoints} {
		if a != nil {
			if len(*a) > 20 {
				invalid(c, "目标条件最多 20 条")
				return false
			}
			for i := range *a {
				if !text(c, &(*a)[i], 4000, true) {
					return false
				}
			}
		}
	}
	return true
}
func bottlePatch(c *gin.Context, in service.BottlePatch) bool {
	if in.Episode == nil && in.Target == nil {
		invalid(c, "缺少修改字段")
		return false
	}
	if in.Episode != nil {
		p := in.Episode
		if p.Raw != nil && !text(c, p.Raw, 4000, true) || p.Title != nil && !text(c, p.Title, 80, true) {
			return false
		}
	}
	return in.Target == nil || targetPatch(c, in.Target)
}
func experienceInput(c *gin.Context, in service.ExperienceInput, create bool) bool {
	if create && (in.Title == nil || in.Body == nil || in.Confirmed == nil || !*in.Confirmed) {
		invalid(c, "需要标题、正文和本人确认")
		return false
	}
	if in.Title != nil && !text(c, in.Title, 80, true) || in.Body != nil && !text(c, in.Body, 8000, true) {
		return false
	}
	return true
}
func (a *api) authenticate(c *gin.Context) {
	header := c.GetHeader("Authorization")
	token := ""
	if strings.HasPrefix(header, "Bearer ") {
		token = strings.TrimPrefix(header, "Bearer ")
	} else {
		token, _ = c.Cookie("drift_session")
	}
	if token == "" {
		respond(c, nil, domain.Fail(401, "UNAUTHORIZED", "请先登录"), 401)
		return
	}
	subject, err := a.o.Auth.Verify(token)
	if err != nil {
		respond(c, nil, domain.Fail(401, "UNAUTHORIZED", "登录已失效"), 401)
		return
	}
	u, err := a.s.User(ctx(c), subject)
	if err != nil {
		respond(c, nil, err, 500)
		return
	}
	c.Set("user", u)
	c.Next()
}
func (a *api) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		rid := domain.ID()
		c.Set("requestID", rid)
		c.Header("X-Request-ID", rid)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		requestCtx, cancel := context.WithTimeout(c.Request.Context(), 55*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(requestCtx)
		defer func() {
			if recover() != nil {
				respond(c, nil, domain.Fail(500, "INTERNAL_ERROR", "服务暂时出错"), 500)
			}
			slog.Info("http", "request_id", rid, "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
		}()
		if origin := c.GetHeader("Origin"); origin != "" {
			if !a.originAllowed(origin) {
				respond(c, nil, domain.Fail(403, "ORIGIN_NOT_ALLOWED", "请求来源不被允许"), 403)
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(204)
			c.Abort()
			return
		}
		c.Next()
	}
}
func (a *api) originAllowed(origin string) bool {
	for _, configured := range append(strings.Split(a.o.Origin, ","), strings.Split(a.o.Origins, ",")...) {
		if strings.TrimSpace(configured) == origin {
			return true
		}
	}
	return false
}
func (a *api) oauthStart(c *gin.Context) {
	if a.o.Cipher == nil || a.s.Zhihu.AppID == "" || a.s.Zhihu.AppKey == "" || a.s.Zhihu.Redirect == "" {
		respond(c, nil, domain.Fail(503, "ZHIHU_OAUTH_UNAVAILABLE", "知乎授权尚未配置"), 503)
		return
	}
	state, nonce := domain.ID()+domain.ID(), domain.ID()+domain.ID()
	a.mu.Lock()
	for k, v := range a.oauth {
		if time.Now().After(v.Expires) {
			delete(a.oauth, k)
		}
	}
	a.oauth[state] = oauthAttempt{User: user(c), Nonce: nonce, Expires: time.Now().Add(10 * time.Minute)}
	a.mu.Unlock()
	http.SetCookie(c.Writer, &http.Cookie{Name: "zhihu_oauth", Value: nonce, Path: "/api/v1/integrations/zhihu/callback", HttpOnly: true, Secure: a.o.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	respond(c, gin.H{"authorizationUrl": a.s.Zhihu.Authorize(state)}, nil, 200)
}

func (a *api) oauthLogin(c *gin.Context) {
	if a.o.Cipher == nil || a.s.Zhihu.AppID == "" || a.s.Zhihu.AppKey == "" || a.s.Zhihu.Redirect == "" {
		respond(c, nil, domain.Fail(503, "ZHIHU_OAUTH_UNAVAILABLE", "知乎登录尚未配置"), 503)
		return
	}
	state, nonce := domain.ID()+domain.ID(), domain.ID()+domain.ID()
	a.mu.Lock()
	for key, attempt := range a.oauth {
		if time.Now().After(attempt.Expires) {
			delete(a.oauth, key)
		}
	}
	a.oauth[state] = oauthAttempt{Nonce: nonce, Login: true, Expires: time.Now().Add(10 * time.Minute)}
	a.mu.Unlock()
	http.SetCookie(c.Writer, a.oauthCookie(nonce, 600))
	c.Redirect(http.StatusFound, a.s.Zhihu.Authorize(state))
}

func (a *api) oauthCookie(value string, age int) *http.Cookie {
	return &http.Cookie{Name: "zhihu_oauth", Value: value, Path: "/api/v1/integrations/zhihu/callback", HttpOnly: true, Secure: a.o.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: age}
}
func (a *api) sessionCookie(value string, age int) *http.Cookie {
	mode := http.SameSiteLaxMode
	if a.o.SecureCookies {
		mode = http.SameSiteNoneMode
	}
	return &http.Cookie{Name: "drift_session", Value: value, Path: "/", HttpOnly: true, Secure: a.o.SecureCookies, SameSite: mode, MaxAge: age}
}
func (a *api) session(c *gin.Context) {
	profile, err := a.s.Profile(ctx(c), user(c))
	respond(c, profile, err, http.StatusOK)
}
func (a *api) logout(c *gin.Context) {
	http.SetCookie(c.Writer, a.sessionCookie("", -1))
	c.Status(http.StatusNoContent)
}

func (a *api) oauthCallback(c *gin.Context) {
	state := c.Query("state")
	cookie, _ := c.Cookie("zhihu_oauth")
	attempt, ok := a.takeOAuthAttempt(state, cookie)
	if !ok {
		respond(c, nil, domain.Fail(400, "OAUTH_STATE_INVALID", "授权回调无法与本次操作绑定，请重新授权"), 400)
		return
	}
	code := c.Query("authorization_code")
	if code == "" {
		code = c.Query("code")
	}
	token, err := a.s.Zhihu.Exchange(ctx(c), code)
	if err == nil && attempt.Login {
		var upstream zhihu.User
		upstream, err = a.s.Zhihu.User(ctx(c), token.Value)
		if err == nil {
			var subject, session string
			subject, err = a.s.LoginZhihu(ctx(c), upstream.ID, upstream.Name, upstream.Avatar, token, a.o.Cipher)
			if err == nil {
				session, err = a.o.Auth.Issue(subject, 30*24*time.Hour)
			}
			if err == nil {
				http.SetCookie(c.Writer, a.sessionCookie(session, 30*24*60*60))
			}
		}
	} else if err == nil {
		err = a.s.ConnectZhihu(ctx(c), attempt.User, token, a.o.Cipher)
	}
	http.SetCookie(c.Writer, a.oauthCookie("", -1))
	if err == nil && attempt.Login {
		c.Redirect(http.StatusFound, strings.TrimRight(a.o.Origin, "/")+"/?login=success")
		return
	}
	respond(c, gin.H{"status": "connected"}, err, 200)
}

func (a *api) takeOAuthAttempt(state, cookie string) (oauthAttempt, bool) {
	if cookie == "" {
		return oauthAttempt{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for key, attempt := range a.oauth {
		if !attempt.Expires.After(now) {
			delete(a.oauth, key)
		}
	}
	if state != "" {
		attempt, ok := a.oauth[state]
		if !ok || subtle.ConstantTimeCompare([]byte(cookie), []byte(attempt.Nonce)) != 1 {
			return oauthAttempt{}, false
		}
		delete(a.oauth, state)
		return attempt, true
	}
	// The hackathon OAuth callback may omit state. The random HttpOnly cookie
	// still binds the callback to the browser that initiated this one-time flow.
	var matchedKey string
	var matched oauthAttempt
	for key, attempt := range a.oauth {
		if subtle.ConstantTimeCompare([]byte(cookie), []byte(attempt.Nonce)) == 1 {
			if matchedKey != "" {
				return oauthAttempt{}, false
			}
			matchedKey, matched = key, attempt
		}
	}
	if matchedKey == "" {
		return oauthAttempt{}, false
	}
	delete(a.oauth, matchedKey)
	return matched, true
}
