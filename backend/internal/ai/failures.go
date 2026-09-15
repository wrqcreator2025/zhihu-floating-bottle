package ai

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"

	"driftbottle/internal/domain"
)

// Never log transport error strings or upstream bodies: they may contain URLs,
// credentials, echoed prompts or personal content. Codes survive in outbox_jobs.
func requestFailure(ctx context.Context, task, code string, status int) error {
	slog.WarnContext(ctx, "AI request failed", "task", task, "code", code, "http_status", status)
	return domain.Fail(503, code, "内容处理服务暂时不可用，请稍后重试")
}

func transportCode(err error) string {
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "AI_CANCELED"
	case errors.As(err, &dns):
		return "AI_DNS_ERROR"
	case errors.As(err, &cert):
		return "AI_TLS_ERROR"
	case errors.As(err, &network) && network.Timeout():
		return "AI_TIMEOUT"
	default:
		return "AI_NETWORK_ERROR"
	}
}
