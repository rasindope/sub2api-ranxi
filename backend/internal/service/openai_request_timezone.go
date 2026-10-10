package service

// 请求时区清洗（移植自 MACOS-DO/sub4api 的 openai_request_locale / openai_request_timezone）。
//
// OpenAI 账号可在 extra.openai_request_timezone 设置目标 IANA 时区（建议与该账号代理出口所在地一致）。
// 设置后转发前：
//   - 把 user 消息里 <environment_context> 已有的 <timezone> 值替换为目标时区；
//   - 把 web_search* 工具已有的 user_location.timezone 替换为目标时区；
//   - 把已有的 Accept-Language 请求头统一为 en-US（账号级请求头覆写仍优先）。
//
// 未设置时完全不改动请求（与 sub4api 默认新加坡不同，这里是显式开启）。
// 只改写已有字段，不新增字段；普通对话文本、current_date 和服务器时区都不动。

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	openAIRequestTimezoneExtraKey = "openai_request_timezone"
	openAIUpstreamAcceptLanguage  = "en-US,en;q=0.9"
)

// 中国大陆及港澳台时区不可作为目标时区。
var openAIRequestTimezoneBlocked = map[string]struct{}{
	"Asia/Shanghai": {}, "Asia/Chongqing": {}, "Asia/Chungking": {}, "Asia/Harbin": {},
	"Asia/Urumqi": {}, "Asia/Kashgar": {}, "PRC": {},
	"Asia/Hong_Kong": {}, "Hongkong": {},
	"Asia/Macau": {}, "Asia/Macao": {},
	"Asia/Taipei": {}, "ROC": {},
}

func isAllowedOpenAIRequestTimezone(name string) bool {
	if name == "" || name == "Local" || !strings.Contains(name, "/") {
		return false
	}
	if _, blocked := openAIRequestTimezoneBlocked[name]; blocked {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// ValidateOpenAIRequestTimezoneExtra 校验账号 extra 中的请求时区；空字符串表示关闭。
func ValidateOpenAIRequestTimezoneExtra(platform string, extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, exists := extra[openAIRequestTimezoneExtraKey]
	if !exists || raw == nil {
		return nil
	}
	name, ok := raw.(string)
	if !ok || (name != "" && (platform != PlatformOpenAI || !isAllowedOpenAIRequestTimezone(name))) {
		return infraerrors.New(http.StatusBadRequest, "INVALID_OPENAI_REQUEST_TIMEZONE",
			"openai_request_timezone must be an IANA timezone (excluding mainland China, Hong Kong, Macau and Taiwan) on an OpenAI account")
	}
	return nil
}

// OpenAIRequestTimezone 返回账号配置的目标时区；未配置或无效时返回空字符串（不清洗）。
func (a *Account) OpenAIRequestTimezone() string {
	if a == nil || !a.IsOpenAI() {
		return ""
	}
	if name := strings.TrimSpace(a.getExtraString(openAIRequestTimezoneExtraKey)); isAllowedOpenAIRequestTimezone(name) {
		return name
	}
	return ""
}

// normalizeOpenAIUpstreamAcceptLanguage 在账号开启请求时区时统一已有的 Accept-Language。
// 必须在 ApplyHeaderOverrides 之前调用，使账号级显式覆写仍然生效。
func normalizeOpenAIUpstreamAcceptLanguage(account *Account, header http.Header) {
	if header == nil || account.OpenAIRequestTimezone() == "" {
		return
	}
	for key := range header {
		if strings.EqualFold(key, "Accept-Language") {
			delete(header, key)
			header.Set("Accept-Language", openAIUpstreamAcceptLanguage)
			return
		}
	}
}

// Only parse field boundaries. Environment text is XML-like, but fields such as
// subagents and network can contain unescaped text and are opaque to this rewrite.
var openAIEnvironmentFieldStart = regexp.MustCompile(`^<([A-Za-z_][A-Za-z0-9_.:-]*)(?:[ \t\r\n]+(?:[^<>"']|"[^"]*"|'[^']*')*)?[ \t\r\n]*/?>`)

// Closing tags may contain whitespace before >. Match only the field name;
// nested fields and unescaped characters in the body remain opaque.
func findOpenAIEnvironmentFieldEnd(text, name string) (start, end int) {
	prefix := "</" + name
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], prefix)
		if index < 0 {
			break
		}
		start = offset + index
		offset = start + len(prefix)
		rest := strings.TrimLeft(text[offset:], " \t\r\n")
		if strings.HasPrefix(rest, ">") {
			return start, len(text) - len(rest) + 1
		}
	}
	return -1, -1
}

func rewriteOpenAIRequestEnvironment(text, timezone string) (rewritten, previous, reason string) {
	trimmed := strings.TrimSpace(text)
	root := openAIEnvironmentFieldStart.FindStringSubmatchIndex(trimmed)
	if root == nil || trimmed[root[2]:root[3]] != "environment_context" {
		return text, "", "no_environment_context"
	}
	rootCloseStart, rootCloseEnd := findOpenAIEnvironmentFieldEnd(trimmed[root[1]:], "environment_context")
	if strings.HasSuffix(trimmed[:root[1]], "/>") || rootCloseEnd != len(trimmed)-root[1] {
		return text, "", "invalid_environment_context"
	}
	content := trimmed[root[1] : root[1]+rootCloseStart]
	valueStart, valueEnd := -1, -1
	for offset := 0; offset < len(content); {
		rest := strings.TrimLeftFunc(content[offset:], unicode.IsSpace)
		offset = len(content) - len(rest)
		if rest == "" {
			break
		}
		field := openAIEnvironmentFieldStart.FindStringSubmatchIndex(rest)
		if field == nil {
			return text, "", "invalid_environment_context"
		}
		name := rest[field[2]:field[3]]
		if name == "environment_context" {
			return text, "", "invalid_environment_context"
		}
		selfClosing := strings.HasSuffix(rest[:field[1]], "/>")
		if name == "timezone" && (valueStart >= 0 || selfClosing) {
			return text, "", "invalid_environment_context"
		}
		offset += field[1]
		if selfClosing {
			continue
		}
		end, closeEnd := findOpenAIEnvironmentFieldEnd(content[offset:], name)
		if end < 0 {
			return text, "", "invalid_environment_context"
		}
		if name == "timezone" {
			value := content[offset : offset+end]
			previous = strings.TrimSpace(value)
			if previous == "" || strings.Contains(value, "<") {
				return text, "", "invalid_environment_context"
			}
			valueStart = offset + len(value) - len(strings.TrimLeftFunc(value, unicode.IsSpace))
			valueEnd = offset + len(strings.TrimRightFunc(value, unicode.IsSpace))
		}
		offset += closeEnd
	}
	if valueStart < 0 {
		return text, "", "no_timezone"
	}
	if previous == timezone {
		return text, previous, "already_target"
	}
	// Patch only the timezone value, preserving all original tags and whitespace.
	base := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace)) + root[1]
	return text[:base+valueStart] + timezone + text[base+valueEnd:], previous, "replaced"
}

// normalizeOpenAIRequestLocale 按账号目标时区清洗 Responses 请求体（HTTP 与 WS response.create 共用）。
func normalizeOpenAIRequestLocale(ctx context.Context, account *Account, body []byte, transport string) []byte {
	timezone := account.OpenAIRequestTimezone()
	if timezone == "" || len(body) == 0 {
		return body
	}
	replaced := 0
	var before []string
	rewriteText := func(path string, text gjson.Result) {
		if text.Type != gjson.String {
			return
		}
		next, previous, reason := rewriteOpenAIRequestEnvironment(text.String(), timezone)
		if reason != "replaced" {
			return
		}
		if updated, err := sjson.SetBytes(body, path, next); err == nil {
			body = updated
			replaced++
			before = append(before, truncateOpenAIRequestTimezoneLog(previous))
		}
	}
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			content := item.Get("content")
			path := "input." + strconv.Itoa(inputIndex) + ".content"
			if content.Type == gjson.String {
				rewriteText(path, content)
			} else if content.IsArray() {
				for contentIndex, part := range content.Array() {
					if part.Get("type").String() == "input_text" {
						rewriteText(path+"."+strconv.Itoa(contentIndex)+".text", part.Get("text"))
					}
				}
			}
		}
	}
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		for index, tool := range tools.Array() {
			kind := tool.Get("type").String()
			if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
				continue
			}
			value := tool.Get("user_location.timezone")
			if value.Type != gjson.String || value.String() == timezone {
				continue
			}
			if updated, err := sjson.SetBytes(body, "tools."+strconv.Itoa(index)+".user_location.timezone", timezone); err == nil {
				body = updated
				replaced++
				before = append(before, truncateOpenAIRequestTimezoneLog(value.String()))
			}
		}
	}
	if replaced > 0 {
		logger.FromContext(ctx).Debug("openai request timezone normalized",
			zap.Int64("account_id", account.ID), zap.String("transport", transport),
			zap.String("target_timezone", timezone), zap.Strings("timezone_before", before),
			zap.Int("replaced_count", replaced))
	}
	return body
}

func truncateOpenAIRequestTimezoneLog(value string) string {
	if len(value) > 128 {
		return value[:128]
	}
	return value
}
