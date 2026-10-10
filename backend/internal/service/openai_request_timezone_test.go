package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func timezoneTestAccount(tz string) *Account {
	return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAIRequestTimezoneExtraKey: tz}}
}

func TestNormalizeOpenAIRequestLocaleRewritesEnvironmentAndWebSearch(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[` +
		`{"role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/tmp</cwd>\n  <current_date>2026-10-09</current_date>\n  <timezone>Asia/Shanghai</timezone>\n</environment_context>"}]},` +
		`{"role":"user","content":"<environment_context><timezone> Asia/Shanghai </timezone></environment_context>"},` +
		`{"role":"user","content":[{"type":"input_text","text":"my timezone is <timezone>Asia/Shanghai</timezone>"}]},` +
		`{"role":"assistant","content":[{"type":"output_text","text":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}` +
		`],"tools":[{"type":"web_search","user_location":{"type":"approximate","timezone":"Asia/Shanghai"}},{"type":"function","name":"f"}]}`)

	out := normalizeOpenAIRequestLocale(context.Background(), timezoneTestAccount("America/New_York"), body, "http")

	require.Equal(t, "<environment_context>\n  <cwd>/tmp</cwd>\n  <current_date>2026-10-09</current_date>\n  <timezone>America/New_York</timezone>\n</environment_context>",
		gjson.GetBytes(out, "input.0.content.0.text").String())
	require.Equal(t, "<environment_context><timezone> America/New_York </timezone></environment_context>", gjson.GetBytes(out, "input.1.content").String())
	// 普通对话文本和 assistant 消息不改
	require.Equal(t, "my timezone is <timezone>Asia/Shanghai</timezone>", gjson.GetBytes(out, "input.2.content.0.text").String())
	require.Contains(t, gjson.GetBytes(out, "input.3.content.0.text").String(), "Asia/Shanghai")
	require.Equal(t, "America/New_York", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
}

func TestNormalizeOpenAIRequestLocaleDisabledOrNonOpenAI(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`)
	require.Equal(t, body, normalizeOpenAIRequestLocale(context.Background(), timezoneTestAccount(""), body, "http"))
	require.Equal(t, body, normalizeOpenAIRequestLocale(context.Background(), &Account{Platform: PlatformOpenAI}, body, "http"))
	other := &Account{Platform: "deepseek", Extra: map[string]any{openAIRequestTimezoneExtraKey: "Asia/Tokyo"}}
	require.Equal(t, body, normalizeOpenAIRequestLocale(context.Background(), other, body, "http"))
}

func TestNormalizeOpenAIUpstreamAcceptLanguage(t *testing.T) {
	h := http.Header{}
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	normalizeOpenAIUpstreamAcceptLanguage(timezoneTestAccount("Asia/Tokyo"), h)
	require.Equal(t, openAIUpstreamAcceptLanguage, h.Get("Accept-Language"))

	ws := http.Header{"accept-language": []string{"zh-CN"}}
	normalizeOpenAIUpstreamAcceptLanguage(timezoneTestAccount("Asia/Tokyo"), ws)
	require.Equal(t, []string{openAIUpstreamAcceptLanguage}, ws.Values("Accept-Language"))
	require.Len(t, ws, 1)

	absent := http.Header{}
	normalizeOpenAIUpstreamAcceptLanguage(timezoneTestAccount("Asia/Tokyo"), absent)
	require.Empty(t, absent.Get("Accept-Language"))

	off := http.Header{}
	off.Set("Accept-Language", "zh-CN")
	normalizeOpenAIUpstreamAcceptLanguage(timezoneTestAccount(""), off)
	require.Equal(t, "zh-CN", off.Get("Accept-Language"))
}

func TestValidateOpenAIRequestTimezoneExtra(t *testing.T) {
	require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, nil))
	require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: ""}))
	require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: "America/Los_Angeles"}))
	for _, bad := range []any{"Asia/Shanghai", "Asia/Hong_Kong", "Asia/Taipei", "PRC", "UTC", "Mars/Base", 8} {
		require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: bad}), bad)
	}
	require.Error(t, ValidateOpenAIRequestTimezoneExtra("deepseek", map[string]any{openAIRequestTimezoneExtraKey: "Asia/Tokyo"}))
}
