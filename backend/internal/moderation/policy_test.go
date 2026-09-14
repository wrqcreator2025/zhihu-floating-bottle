package moderation

import "testing"

func TestPolicyLayers(t *testing.T) {
	tests := []struct {
		text   string
		action Action
		reason string
	}{
		{"谢谢你的回信，我也会慢慢试试看。", Allow, "local_rules_pass"},
		{"加我微信 abc123", Reject, "off_platform_contact"},
		{"电话是 13800138000", Reject, "off_platform_contact"},
		{"发邮件到 hello@example.com", Reject, "off_platform_contact"},
		{"看看 https://example.com", Reject, "off_platform_contact"},
		{"可以一起兼职跑分", Reject, "illegal_guidance"},
		{"以前有人骂我是废物，我难受了很久。", NeedsAI, "contextual_risk"},
	}
	for _, test := range tests {
		got := Check(test.text)
		if got.Action != test.action || got.Reason != test.reason {
			t.Errorf("Check(%q) = %#v", test.text, got)
		}
	}
}
