package service

// 自定义补丁（rasindope）：请求时区绑定开启时，同时统一 OpenAI 上游请求已有的 Accept-Language。
// 上游的「请求时区绑定」只改写环境时区，保留客户端语言偏好；这里额外把 zh-CN 等语言头
// 统一为英文，避免与代理出口和目标时区不一致。只替换已有的头，不新增；账号级请求头覆写仍优先。

import (
	"context"
	"net/http"
	"strings"
)

const openAIUpstreamAcceptLanguage = "en-US,en;q=0.9"

// normalizeUpstreamAcceptLanguage 必须在 account.ApplyHeaderOverrides 之前调用。
func (s *OpenAIGatewayService) normalizeUpstreamAcceptLanguage(ctx context.Context, account *Account, header http.Header) {
	if header == nil || account == nil || !account.IsOpenAI() || s == nil || s.settingService == nil ||
		!s.settingService.GetOpenAIRequestTimezoneEnabled(ctx, false) {
		return
	}
	normalizeOpenAIUpstreamAcceptLanguage(header)
}

func normalizeOpenAIUpstreamAcceptLanguage(header http.Header) {
	for key := range header {
		if strings.EqualFold(key, "Accept-Language") {
			delete(header, key)
			header.Set("Accept-Language", openAIUpstreamAcceptLanguage)
			return
		}
	}
}
