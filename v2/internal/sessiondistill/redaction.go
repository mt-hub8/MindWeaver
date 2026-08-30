package sessiondistill

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type redactionSpan struct {
	start int
	end   int
	class string
}

type secretPattern struct {
	class string
	re    *regexp.Regexp
	group int
}

const maxRedactionSpans = 64

var secretPatterns = []secretPattern{
	{class: "private_key", re: regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----`)},
	{class: "private_key", re: regexp.MustCompile(`(?s)-----BEGIN PGP PRIVATE KEY BLOCK-----.*?-----END PGP PRIVATE KEY BLOCK-----`)},
	{class: "openai_token", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`)},
	{class: "github_token", re: regexp.MustCompile(`\b(?:gh[opusr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)},
	{class: "aws_access_key", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{class: "bearer_token", re: regexp.MustCompile(`(?i)\bBearer[ \t]+([^\s,;]+)`), group: 1},
	{class: "basic_auth", re: regexp.MustCompile(`(?i)\bAuthorization[ \t]*:[ \t]*Basic[ \t]+([^\s,;]+)`), group: 1},
	{class: "cookie", re: regexp.MustCompile(`(?i)\b(?:Set-Cookie|Cookie)[ \t]*:[ \t]*([^ \t\r\n][^\r\n]*)`), group: 1},
	{class: "session_cookie", re: regexp.MustCompile(`(?i)(?:\b(?:sessionid|session_id|jsessionid|connect\.sid|__Host-session|__Secure-session)\b|["'](?:sessionid|session_id|jsessionid|connect\.sid|__Host-session|__Secure-session)["'])[ \t]*[:=][ \t]*(?:"([^"\r\n]+)"|'([^'\r\n]+)'|([^;\s'\"{}\[\]]+))`), group: 1},
	{class: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)},
	{class: "canary", re: regexp.MustCompile(`(?i)\b(?:mw_)?secret_canary_[A-Za-z0-9_-]{8,}\b`)},
	{class: "aws_secret_key", re: regexp.MustCompile(`(?i)(?:\bAWS_SECRET_ACCESS_KEY\b|["']AWS_SECRET_ACCESS_KEY["'])[ \t]*[:=][ \t]*(?:"([^"\r\n]+)"|'([^'\r\n]+)'|([^\s,;'\"{}\[\]]+))`), group: 1},
	{class: "database_password", re: regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|redis)://([^/\s:@]+):([^@\s/]+)@`), group: 2},
	{class: "uri_password", re: regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]{1,31}://([^/\s:@]+):([^@\s/]+)@`), group: 2},
	{class: "credential", re: regexp.MustCompile(`(?i)(?:\b(?:api[_-]?key|access[_-]?token|auth[_-]?token|refresh[_-]?token|session[_-]?token|client[_-]?secret|private[_-]?key|password|passwd|token|secret)\b|["'](?:api[_-]?key|access[_-]?token|auth[_-]?token|refresh[_-]?token|session[_-]?token|client[_-]?secret|private[_-]?key|password|passwd|token|secret)["'])[ \t]*[:=][ \t]*(?:"[^"\r\n]+"|'[^'\r\n]+'|[^\s,;'\"{}\[\]]+)`)},
	{class: "credential", re: regexp.MustCompile(`(?i)(?:--(?:api[_-]?key|access[_-]?token|auth[_-]?token|refresh[_-]?token|session[_-]?token|client[_-]?secret|password|token|secret))[ \t]+[^\s]+`)},
}

var preRedacted = regexp.MustCompile(`\[redacted:[a-z_]+\]`)

// RedactVisibleText applies the same canonical line and secret policy used by
// Distill. Callers that persist visible text can therefore redact before the
// first durable write without gaining access to the package's span internals.
func RedactVisibleText(value string) (string, int, error) {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxTurnBytes || hasForbiddenControl(value) {
		return "", 0, newError(CodeInvalidArgument, fmt.Errorf("visible text"))
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	safe, _, count := redact(normalized)
	if !utf8.ValidString(safe) || len(safe) == 0 || len(safe) > maxTurnBytes || hasForbiddenControl(safe) {
		return "", 0, newError(CodeInputLimitExceeded, fmt.Errorf("safe visible text"))
	}
	return safe, count, nil
}

func redact(value string) (string, []redactionSpan, int) {
	raw := make([]redactionSpan, 0, 4)
	for _, pattern := range secretPatterns {
		for _, match := range pattern.re.FindAllStringSubmatchIndex(value, maxRedactionSpans+1) {
			start, end := match[0], match[1]
			if pattern.group > 0 {
				start, end = -1, -1
				for group := pattern.group; group*2+1 < len(match); group++ {
					if match[group*2] >= 0 {
						start, end = match[group*2], match[group*2+1]
						break
					}
				}
				if start < 0 {
					continue
				}
			}
			raw = append(raw, redactionSpan{start: start, end: end, class: pattern.class})
			if len(raw) > maxRedactionSpans {
				placeholder := "[redacted:secret]"
				return placeholder, []redactionSpan{{start: 0, end: len(placeholder), class: "secret"}}, 1
			}
		}
	}
	for _, match := range preRedacted.FindAllStringIndex(value, maxRedactionSpans+1) {
		raw = append(raw, redactionSpan{start: match[0], end: match[1], class: "pre_redacted"})
		if len(raw) > maxRedactionSpans {
			placeholder := "[redacted:secret]"
			return placeholder, []redactionSpan{{start: 0, end: len(placeholder), class: "secret"}}, 1
		}
	}
	if len(raw) == 0 && hasUnlocalizedSecretSignal(value) {
		placeholder := "[redacted:secret]"
		return placeholder, []redactionSpan{{start: 0, end: len(placeholder), class: "secret"}}, 1
	}
	if len(raw) == 0 {
		return value, nil, 0
	}
	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].start != raw[j].start {
			return raw[i].start < raw[j].start
		}
		return raw[i].end > raw[j].end
	})
	merged := raw[:0]
	for _, span := range raw {
		if span.start < 0 || span.end <= span.start || span.end > len(value) {
			continue
		}
		if len(merged) > 0 && span.start < merged[len(merged)-1].end {
			if span.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = span.end
				merged[len(merged)-1].class = "secret"
			}
			continue
		}
		merged = append(merged, span)
	}
	var output strings.Builder
	output.Grow(len(value))
	result := make([]redactionSpan, 0, len(merged))
	position, count := 0, 0
	for _, span := range merged {
		output.WriteString(value[position:span.start])
		start := output.Len()
		if span.class == "pre_redacted" {
			output.WriteString(value[span.start:span.end])
			count++
		} else {
			output.WriteString("[redacted:" + span.class + "]")
			count++
		}
		result = append(result, redactionSpan{start: start, end: output.Len(), class: span.class})
		position = span.end
	}
	output.WriteString(value[position:])
	return output.String(), result, count
}

func hasUnlocalizedSecretSignal(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "begin private key") ||
		strings.Contains(lower, "begin openssh private key") ||
		strings.Contains(lower, "begin pgp private key block") ||
		strings.Contains(lower, "private key material")
}

func intersectsRedaction(spans []redactionSpan, start, end int) bool {
	for _, span := range spans {
		if start < span.end && end > span.start {
			return true
		}
	}
	return false
}
