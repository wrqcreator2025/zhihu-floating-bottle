// Package moderation provides a fast local gate before contextual AI review.
package moderation

import (
	"regexp"
	"strings"
)

type Action uint8

const (
	Allow Action = iota
	Reject
	NeedsAI
)

type Result struct {
	Action Action
	Reason string
}

var (
	urlPattern   = regexp.MustCompile(`(?i)(https?://|www\.|(?:[a-z0-9-]+\.)+(?:com|cn|net|org|io|me)(?:[/\s]|$))`)
	emailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	phonePattern = regexp.MustCompile(`(?:^|\D)(?:\+?86[- ]?)?1[3-9]\d{9}(?:\D|$)`)
	qqPattern    = regexp.MustCompile(`(?i)(?:qq|q q|扣扣|企鹅)\s*(?:号|号码|：|:|是|加)?\s*[1-9]\d{4,11}`)
)

// Direct phrases are safe to reject without interpreting conversational context.
var directPhrases = map[string][]string{
	"off_platform_contact": {
		"加我微信", "加下微信", "微信联系", "留个微信", "交换微信", "vx联系", "v信联系",
		"加我qq", "加下qq", "qq联系", "留个qq", "私下联系", "线下联系", "站外联系",
		"加我好友", "扫码加我", "扫二维码", "联系方式发我", "留个联系方式",
	},
	"illegal_guidance": {
		"代开发票", "买卖银行卡", "出售银行卡", "收购银行卡", "提供银行卡跑分", "兼职跑分",
		"刷单返利", "裸聊敲诈", "代办假证", "出售假证", "购买假证", "博彩代理", "网赌代理",
	},
}

// Contextual terms may be an attack, or may describe something the author experienced.
// Only messages containing them need semantic review.
var contextualTerms = []string{
	"傻逼", "煞笔", "沙比", "蠢货", "废物", "垃圾人", "脑残", "弱智", "智障", "贱人", "婊子",
	"去死", "弄死你", "杀了你", "打死你", "废了你", "人肉你", "曝光你", "跟踪你",
	"操你", "草你", "妈的", "滚蛋", "滚开", "恶心死了",
	"骗钱", "转账", "汇款", "保证金", "押金", "返利", "下注", "博彩", "毒品", "迷药",
}

func Check(text string) Result {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), ""))
	if urlPattern.MatchString(text) || emailPattern.MatchString(text) || phonePattern.MatchString(text) || qqPattern.MatchString(text) {
		return Result{Reject, "off_platform_contact"}
	}
	for reason, phrases := range directPhrases {
		for _, phrase := range phrases {
			if strings.Contains(normalized, phrase) {
				return Result{Reject, reason}
			}
		}
	}
	for _, term := range contextualTerms {
		if strings.Contains(normalized, term) {
			return Result{NeedsAI, "contextual_risk"}
		}
	}
	return Result{Allow, "local_rules_pass"}
}
