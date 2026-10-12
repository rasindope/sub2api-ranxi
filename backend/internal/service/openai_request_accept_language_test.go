package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIUpstreamAcceptLanguage(t *testing.T) {
	h := http.Header{}
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	normalizeOpenAIUpstreamAcceptLanguage(h)
	require.Equal(t, openAIUpstreamAcceptLanguage, h.Get("Accept-Language"))

	ws := http.Header{"accept-language": []string{"zh-CN"}}
	normalizeOpenAIUpstreamAcceptLanguage(ws)
	require.Equal(t, []string{openAIUpstreamAcceptLanguage}, ws.Values("Accept-Language"))
	require.Len(t, ws, 1)

	absent := http.Header{}
	normalizeOpenAIUpstreamAcceptLanguage(absent)
	require.Empty(t, absent.Get("Accept-Language"))
}

func TestNormalizeUpstreamAcceptLanguageRequiresSetting(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	h := http.Header{}
	h.Set("Accept-Language", "zh-CN")
	// 无 settingService（等同开关关闭）时不改动
	(&OpenAIGatewayService{}).normalizeUpstreamAcceptLanguage(context.Background(), account, h)
	require.Equal(t, "zh-CN", h.Get("Accept-Language"))
}
