package sessiondistill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type coreV1 struct{}

type normalizedTurn struct {
	inputEventID string
	eventID      string
	ordinal      uint32
	role         Role
	text         string
	redaction    []redactionSpan
}

type visibleSlice struct {
	turnIndex int
	index     int
	start     int
	end       int
	text      string
	reported  bool
}

type classifiedSlice struct {
	slice     visibleSlice
	itemID    string
	itemKind  Kind
	statement string
}

func (core *coreV1) Distill(ctx context.Context, request Request) (Bundle, error) {
	if err := canceled(ctx); err != nil {
		return Bundle{}, err
	}
	turns, redactedCount, err := normalizeRequest(request)
	if err != nil {
		return Bundle{}, err
	}
	if err := canceled(ctx); err != nil {
		return Bundle{}, err
	}
	slices, err := sliceTurns(turns)
	if err != nil {
		return Bundle{}, err
	}
	publicSessionID := publicSessionID(request.SessionID, turns)
	result := Distillation{
		SchemaVersion:           SchemaVersion,
		PolicyVersion:           PolicyVersion,
		SessionID:               publicSessionID,
		InputDigest:             digestInput(publicSessionID, turns),
		UntrustedVisibleContent: true,
		RedactedCount:           redactedCount,
		UserItems:               make([]Item, 0),
		AssistantContext:        make([]Item, 0),
		Relations:               make([]Relation, 0),
	}
	classified := make([]classifiedSlice, 0, len(slices))
	userIndex := make(map[string]int)
	assistantIndex := make(map[string]int)
	for _, current := range slices {
		if err := canceled(ctx); err != nil {
			return Bundle{}, err
		}
		turn := turns[current.turnIndex]
		entry := classifiedSlice{slice: current}
		if current.reported || intersectsRedaction(turn.redaction, current.start, current.end) {
			classified = append(classified, entry)
			continue
		}
		kind, ruleID, ok := classify(current.text)
		if !ok {
			classified = append(classified, entry)
			continue
		}
		source := Source{
			EventID: turn.eventID,
			Ordinal: turn.ordinal,
			Role:    turn.role,
			Basis:   "canonical_redacted_utf8_v1",
			Start:   current.start,
			End:     current.end,
			Hash: digest(
				"mindweaver/sessiondistill/span/v1",
				[]byte(turn.eventID),
				[]byte(fmt.Sprintf("%d", current.start)),
				[]byte(fmt.Sprintf("%d", current.end)),
				[]byte(turn.text[current.start:current.end]),
			),
		}
		key := string(kind) + "\x00" + string(turn.role) + "\x00" + current.text
		items, indexes := &result.UserItems, userIndex
		if turn.role == RoleAssistant {
			items, indexes = &result.AssistantContext, assistantIndex
		}
		if position, exists := indexes[key]; exists {
			(*items)[position].Sources = append((*items)[position].Sources, source)
			entry.itemID = (*items)[position].ID
			entry.itemKind = (*items)[position].Kind
			entry.statement = (*items)[position].Statement
		} else {
			item := Item{
				ID:          digest("mindweaver/sessiondistill/item/v1", []byte(publicSessionID), []byte(key)),
				Kind:        kind,
				Attribution: turn.role,
				Statement:   current.text,
				RuleID:      ruleID,
				Sources:     []Source{source},
			}
			*items = append(*items, item)
			position := len(*items) - 1
			indexes[key] = position
			entry.itemID = item.ID
			entry.itemKind = item.Kind
			entry.statement = item.Statement
		}
		classified = append(classified, entry)
	}
	result.Relations = buildRelations(turns, classified)
	if err := verifyResult(result, turns, request.SessionID); err != nil {
		return Bundle{}, err
	}
	jsonOutput, err := renderJSON(result)
	if err != nil {
		return Bundle{}, err
	}
	markdownOutput, err := renderMarkdown(result)
	if err != nil {
		return Bundle{}, err
	}
	if len(jsonOutput) > maxRenderedBytes || len(markdownOutput) > maxRenderedBytes {
		return Bundle{}, newError(CodeOutputLimitExceeded, fmt.Errorf("render limit"))
	}
	return sealBundle(result, jsonOutput, markdownOutput), nil
}

func normalizeRequest(request Request) ([]normalizedTurn, int, error) {
	if request.SchemaVersion != SchemaVersion {
		return nil, 0, newError(CodeUnsupportedSchema, fmt.Errorf("schema version"))
	}
	if !validOpaqueID(request.SessionID) || len(request.Turns) == 0 {
		return nil, 0, invalid()
	}
	if len(request.Turns) > maxTurns {
		return nil, 0, newError(CodeInputLimitExceeded, fmt.Errorf("turn limit"))
	}
	seen := make(map[string]struct{}, len(request.Turns))
	turns := make([]normalizedTurn, 0, len(request.Turns))
	total, safeTotal, redactedCount := 0, 0, 0
	for index, turn := range request.Turns {
		if !validOpaqueID(turn.EventID) || turn.Ordinal != uint32(index+1) || turn.Ordinal > maxOrdinal {
			return nil, 0, invalid()
		}
		if _, exists := seen[turn.EventID]; exists {
			return nil, 0, invalid()
		}
		seen[turn.EventID] = struct{}{}
		if turn.Role != RoleUser && turn.Role != RoleAssistant {
			return nil, 0, invalid()
		}
		if !utf8.ValidString(turn.Text) || len(turn.Text) == 0 || len(turn.Text) > maxTurnBytes {
			return nil, 0, newError(CodeInputLimitExceeded, fmt.Errorf("turn size"))
		}
		if hasForbiddenControl(turn.Text) {
			return nil, 0, invalid()
		}
		total += len(turn.Text)
		if total > maxInputBytes {
			return nil, 0, newError(CodeInputLimitExceeded, fmt.Errorf("input size"))
		}
		normalized := strings.ReplaceAll(strings.ReplaceAll(turn.Text, "\r\n", "\n"), "\r", "\n")
		normalized, spans, count := redact(normalized)
		if !utf8.ValidString(normalized) || len(normalized) == 0 || len(normalized) > maxTurnBytes || hasForbiddenControl(normalized) {
			return nil, 0, newError(CodeInputLimitExceeded, fmt.Errorf("normalized turn size"))
		}
		safeTotal += len(normalized)
		if safeTotal > maxInputBytes {
			return nil, 0, newError(CodeInputLimitExceeded, fmt.Errorf("safe input size"))
		}
		publicEventID := digest(
			"mindweaver/sessiondistill/public-event/v1",
			[]byte(turn.EventID),
			[]byte(fmt.Sprintf("%d", turn.Ordinal)),
			[]byte(turn.Role),
			[]byte(normalized),
		)
		turns = append(turns, normalizedTurn{
			inputEventID: turn.EventID,
			eventID:      publicEventID,
			ordinal:      turn.Ordinal,
			role:         turn.Role,
			text:         normalized,
			redaction:    spans,
		})
		redactedCount += count
	}
	return turns, redactedCount, nil
}

func validOpaqueID(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return false
			}
		}
	}
	return true
}

func hasForbiddenControl(value string) bool {
	for _, char := range value {
		if char == '\n' || char == '\r' || char == '\t' {
			continue
		}
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func canceled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return newError(CodeCanceled, ctx.Err())
	default:
		return nil
	}
}

func digest(domain string, values ...[]byte) string {
	hash := sha256.New()
	hash.Write([]byte(domain))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		hash.Write(length[:])
		hash.Write(value)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestInput(sessionID string, turns []normalizedTurn) string {
	values := make([][]byte, 0, 1+len(turns)*4)
	values = append(values, []byte(sessionID))
	var ordinal [4]byte
	for _, turn := range turns {
		binary.BigEndian.PutUint32(ordinal[:], turn.ordinal)
		ordinalCopy := append([]byte(nil), ordinal[:]...)
		values = append(values, []byte(turn.eventID), ordinalCopy, []byte(turn.role), []byte(turn.text))
	}
	return digest("mindweaver/sessiondistill/input/v1", values...)
}

func publicSessionID(inputID string, turns []normalizedTurn) string {
	values := make([][]byte, 0, 1+len(turns))
	values = append(values, []byte(inputID))
	for _, turn := range turns {
		values = append(values, []byte(turn.eventID))
	}
	return digest("mindweaver/sessiondistill/public-session/v1", values...)
}

func sliceTurns(turns []normalizedTurn) ([]visibleSlice, error) {
	result := make([]visibleSlice, 0, len(turns)*2)
	for turnIndex, turn := range turns {
		boundaries := sentenceBoundaries(turn.text)
		start := 0
		for _, end := range boundaries {
			appendBoundedSlices(&result, turnIndex, &start, end, turn.text)
			if len(result) > maxSlices {
				return nil, newError(CodeInputLimitExceeded, fmt.Errorf("slice limit"))
			}
		}
		appendBoundedSlices(&result, turnIndex, &start, len(turn.text), turn.text)
		if len(result) > maxSlices {
			return nil, newError(CodeInputLimitExceeded, fmt.Errorf("slice limit"))
		}
	}
	result = splitAtRedactions(result, turns)
	if len(result) > maxSlices {
		return nil, newError(CodeInputLimitExceeded, fmt.Errorf("slice limit"))
	}
	markReportedBlocks(result)
	for index := range result {
		result[index].index = index
	}
	return result, nil
}

// markReportedBlocks keeps a speaker label and the remainder of that visible
// turn on the reported-content side of the attribution boundary. Sentence
// slicing deliberately happens first, so a label-only line such as
// "Assistant:\n" cannot be detached from the quoted block that follows it.
// The block lasts only to the end of the same visible turn; ambiguous mixed
// content is ignored rather than promoted as the user's own viewpoint.
func markReportedBlocks(slices []visibleSlice) {
	turnIndex := -1
	reported := false
	for index := range slices {
		if slices[index].turnIndex != turnIndex {
			turnIndex = slices[index].turnIndex
			reported = false
		}
		label := strings.ToLower(strings.TrimSpace(slices[index].text))
		if reportedSpeakerBlockLabel(label) || reportedSpeakerLabel(label) {
			reported = true
		}
		slices[index].reported = reported
	}
}

func splitAtRedactions(input []visibleSlice, turns []normalizedTurn) []visibleSlice {
	result := make([]visibleSlice, 0, len(input))
	for _, current := range input {
		cursor := current.start
		for _, span := range turns[current.turnIndex].redaction {
			if span.end <= current.start || span.start >= current.end {
				continue
			}
			if cursor < span.start {
				start, end := trimByteRange(turns[current.turnIndex].text, cursor, min(span.start, current.end))
				if start < end {
					result = append(result, visibleSlice{turnIndex: current.turnIndex, start: start, end: end, text: turns[current.turnIndex].text[start:end]})
				}
			}
			start, end := max(span.start, current.start), min(span.end, current.end)
			if start < end {
				result = append(result, visibleSlice{turnIndex: current.turnIndex, start: start, end: end, text: turns[current.turnIndex].text[start:end]})
			}
			cursor = max(cursor, end)
		}
		if cursor < current.end {
			start, end := trimByteRange(turns[current.turnIndex].text, cursor, current.end)
			if start < end {
				result = append(result, visibleSlice{turnIndex: current.turnIndex, start: start, end: end, text: turns[current.turnIndex].text[start:end]})
			}
		}
	}
	return result
}

func sentenceBoundaries(value string) []int {
	boundaries := make([]int, 0, 8)
	for index, char := range value {
		next := index + utf8.RuneLen(char)
		switch char {
		case '\n', '。', '！', '？', '；':
			boundaries = append(boundaries, next)
		case '.', '!', '?', ';':
			if next == len(value) {
				boundaries = append(boundaries, next)
				continue
			}
			nextRune, _ := utf8.DecodeRuneInString(value[next:])
			if unicode.IsSpace(nextRune) {
				boundaries = append(boundaries, next)
			}
		}
	}
	return boundaries
}

func appendBoundedSlices(result *[]visibleSlice, turnIndex int, cursor *int, end int, value string) {
	start := *cursor
	*cursor = end
	for start < end {
		chunkEnd := end
		if chunkEnd-start > maxSliceBytes {
			chunkEnd = safeChunkEnd(value, start, start+maxSliceBytes)
		}
		trimmedStart, trimmedEnd := trimByteRange(value, start, chunkEnd)
		if trimmedStart < trimmedEnd {
			*result = append(*result, visibleSlice{
				turnIndex: turnIndex,
				start:     trimmedStart,
				end:       trimmedEnd,
				text:      value[trimmedStart:trimmedEnd],
			})
		}
		start = chunkEnd
	}
}

func safeChunkEnd(value string, start, limit int) int {
	if limit >= len(value) {
		return len(value)
	}
	for limit > start && !utf8.RuneStart(value[limit]) {
		limit--
	}
	lastSpace := -1
	for index, char := range value[start:limit] {
		if unicode.IsSpace(char) {
			lastSpace = start + index + utf8.RuneLen(char)
		}
	}
	if lastSpace > start {
		return lastSpace
	}
	return limit
}

func trimByteRange(value string, start, end int) (int, int) {
	for start < end {
		char, size := utf8.DecodeRuneInString(value[start:end])
		if !unicode.IsSpace(char) {
			break
		}
		start += size
	}
	for start < end {
		char, size := utf8.DecodeLastRuneInString(value[start:end])
		if !unicode.IsSpace(char) {
			break
		}
		end -= size
	}
	return start, end
}

func classify(statement string) (Kind, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(statement))
	if lower == "" {
		return "", "", false
	}
	if incompleteCredentialContext(lower) {
		return "", "", false
	}
	// Reported or quoted assistant/agent/document content is context, not a
	// first-person user belief. Keep this conservative gate ahead of every
	// taxonomy rule so a quoted question, constraint, or open item cannot be
	// promoted merely because its words match a later classifier.
	if reportedSpeech(lower) {
		return "", "", false
	}
	// An explicit first-person revision is itself an idea even when the old
	// proposition contains words such as "must" or ends as a quoted question.
	// Keep this ahead of the ordinary taxonomy while still behind the
	// reported-speech gate so an Agent's revision is never attributed to user.
	if explicitEvolutionCue(lower) {
		return KindIdea, "idea-evolution-explicit-v1", true
	}
	questionTail := strings.TrimRight(lower, "。.!！;； ")
	if strings.HasSuffix(lower, "?") || strings.HasSuffix(lower, "？") ||
		strings.HasSuffix(questionTail, "吗") || strings.HasSuffix(questionTail, "么") || strings.HasSuffix(questionTail, "呢") ||
		startsAny(lower,
			"为什么", "怎么", "如何", "是否", "能否", "可不可以", "what ", "why ", "how ", "should ", "can ", "could ") {
		return KindQuestion, "question-explicit-v1", true
	}
	if containsAny(lower, "尚未决定", "还没决定", "未决定", "没有决定", "尚未", "未完成", "待办", "开放项", "remaining ", "open item", "todo:", "not yet") {
		return KindOpenItem, "open-item-explicit-v1", true
	}
	if containsAny(lower, "必须", "不能", "不要", "只允许", "只需要", "约束", "前提是", "must ", "must not", "cannot", "do not", "constraint:", "only ") {
		return KindConstraint, "constraint-explicit-v1", true
	}
	if containsAny(lower, "不是决定", "并非决定", "not a decision", "not decided") {
		return "", "", false
	}
	if startsAny(lower, "我决定", "我们决定", "决定：", "decision:", "i decided", "we decided", "结论是", "选择采用", "确定采用") {
		return KindDecision, "decision-explicit-v1", true
	}
	if startsAny(lower, "目标是", "我的目标是", "我们的目标是", "目标：", "我希望", "我们希望", "需要实现", "goal:", "my goal is", "our goal", "i want", "we want") {
		return KindGoal, "goal-explicit-v1", true
	}
	if startsAny(lower, "下一步", "接下来", "随后", "然后", "next ", "next:", "then ") || containsAny(lower, "下一项是", "后续要") {
		return KindNextAction, "next-action-explicit-v1", true
	}
	if startsAny(lower, "假设", "假定", "前提是", "assuming ", "assume ", "assumption:") {
		return KindAssumption, "assumption-explicit-v1", true
	}
	if startsAny(lower, "因为", "原因是", "理由是", "because ", "since ", "rationale:") {
		return KindRationale, "rationale-explicit-v1", true
	}
	if containsAny(lower, "测试通过", "测试失败", "数据显示", "证据是", "观察到", "test passed", "test failed", "evidence:", "observed ") {
		return KindEvidence, "evidence-explicit-v1", true
	}
	if containsAny(lower, "我学到", "我们学到", "经验是", "发现了", "learned ", "learning:", "we found") {
		return KindLearning, "learning-explicit-v1", true
	}
	if containsAny(lower, "担心", "风险是", "顾虑", "concern:", "risk:", "i worry", "we worry") {
		return KindConcern, "concern-explicit-v1", true
	}
	if containsAny(lower, "我的想法", "我认为", "我觉得", "可以考虑", "想法是", "idea:", "my idea", "i think", "we think", "consider ") {
		return KindIdea, "idea-explicit-v1", true
	}
	return "", "", false
}

var evolutionCuePrefixes = []string{
	"我改主意了", "我改变了想法", "我不再认为",
	"i changed my mind", "i no longer think",
}

var evolutionRelationPrefixes = []string{
	"关于我刚才的想法，我改主意了", "关于上一条想法，我改主意了", "对于我刚才的想法，我改变了想法", "修正上一条想法：",
	"regarding my previous idea, i changed my mind", "about my previous idea, i changed my mind", "revising my previous idea:",
}

func explicitEvolutionCue(value string) bool {
	if explicitEvolutionRelationCue(value) {
		return true
	}
	for _, prefix := range evolutionCuePrefixes {
		remainder, ok := consumeReportedToken(value, prefix)
		if !ok {
			continue
		}
		remainder = strings.TrimSpace(strings.TrimLeft(remainder, ":：,，-—"))
		if utf8.RuneCountInString(strings.Trim(remainder, "。.!！?？;； ")) >= 2 {
			return true
		}
	}
	return false
}

func explicitEvolutionRelationCue(value string) bool {
	for _, prefix := range evolutionRelationPrefixes {
		remainder, ok := consumeEvolutionRelationPrefix(value, prefix)
		if !ok {
			continue
		}
		remainder = strings.TrimSpace(strings.TrimLeft(remainder, ":：,，-—"))
		if utf8.RuneCountInString(strings.Trim(remainder, "。.!！?？;； ")) >= 2 && !negatesEvolutionCue(remainder) {
			return true
		}
	}
	return false
}

func consumeEvolutionRelationPrefix(value, prefix string) (string, bool) {
	remainder, ok := consumeReportedToken(value, prefix)
	if !ok || remainder == "" {
		return "", false
	}
	if strings.HasSuffix(prefix, ":") || strings.HasSuffix(prefix, "：") {
		return remainder, true
	}
	separator, _ := utf8.DecodeRuneInString(remainder)
	if !unicode.IsSpace(separator) && !strings.ContainsRune(",，:：-—;；。.!！?？", separator) {
		return "", false
	}
	return remainder, true
}

func negatesEvolutionCue(value string) bool {
	return containsAny(value,
		"不是修正", "并非修正", "不算修正", "只是示例", "只是引用", "不要把", "不能视为修正",
		"not a revision", "isn't a revision", "is not a revision", "not revising", "just an example", "only an example",
		"do not treat", "don't treat", "must not treat",
	)
}

func incompleteCredentialContext(value string) bool {
	trimmed := strings.TrimSpace(value)
	return containsAny(trimmed, "凭据", "credential") ||
		strings.HasSuffix(trimmed, "=") || strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "{") ||
		strings.HasSuffix(trimmed, "[") || strings.HasSuffix(trimmed, "--token") || strings.HasSuffix(trimmed, "password") ||
		trimmed == "我的想法" || trimmed == "我的想法是" || trimmed == "my idea" || trimmed == "my idea is"
}

var reportedSpeechSources = []string{
	"ai assistant", "assistant", "coding agent", "agent", "codex", "model", "ai",
	"documentation", "document", "docs",
	"ai助手", "大模型", "模型", "助手", "智能体", "说明文档", "文档",
	"system", "user", "he", "she", "you", "系统", "用户", "他", "她", "你",
}

var reportedSpeechVerbs = []string{
	"answered", "replied", "said", "says", "asked", "asks",
	"suggested", "suggests", "stated", "states", "wrote", "writes",
	"中写着", "中说", "回答", "表示", "指出", "建议", "询问", "提到", "写着", "写道", "所说", "问", "说",
}

func reportedSpeech(value string) bool {
	trimmed := strings.TrimSpace(value)
	if startsAny(trimmed, "\"", "'", "“", "‘", "「", "『", "> ") {
		return true
	}
	if reportedSpeakerLabel(trimmed) {
		return true
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "according to"); ok {
		_, ok = consumeReportedSource(remainder)
		return ok
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "根据"); ok {
		_, ok = consumeReportedSource(remainder)
		return ok
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "as"); ok {
		return reportedSourceAndVerb(remainder)
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "正如"); ok {
		return reportedSourceAndVerb(remainder)
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "quoting"); ok {
		_, ok = consumeReportedSource(remainder)
		return ok
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "引用"); ok {
		_, ok = consumeReportedSource(remainder)
		return ok
	}
	if remainder, ok := consumeReportedPrefix(trimmed, "转述"); ok {
		_, ok = consumeReportedSource(remainder)
		return ok
	}
	return reportedSourceAndVerb(trimmed)
}

func reportedSpeakerLabel(value string) bool {
	return reportedSpeakerLabelForm(value, true)
}

func reportedSpeakerBlockLabel(value string) bool {
	return reportedSpeakerLabelForm(value, false)
}

func reportedSpeakerLabelForm(value string, requirePayload bool) bool {
	value = strings.TrimLeft(value, " \t")
	if remainder, ok := consumeReportedToken(value, "the"); ok {
		value = strings.TrimLeft(remainder, " \t")
	}
	for _, source := range reportedSpeechSources {
		remainder, ok := consumeReportedToken(value, source)
		if !ok {
			continue
		}
		if reportedLabelSuffix(remainder, requirePayload) {
			return true
		}
		remainder = strings.TrimLeft(remainder, " \t")
		for _, label := range []string{"'s answer", "’s answer", "'s reply", "’s reply", "的回答", "的回复", "的观点", "的建议"} {
			labelRemainder, matched := consumeReportedToken(remainder, label)
			if matched && reportedLabelSuffix(labelRemainder, requirePayload) {
				return true
			}
		}
	}
	return false
}

func reportedLabelSuffix(value string, requirePayload bool) bool {
	trimmed := strings.TrimLeft(value, " \t")
	for _, separator := range []string{":", "：", "—", "–"} {
		if strings.HasPrefix(trimmed, separator) {
			return (len(strings.TrimSpace(trimmed[len(separator):])) != 0) == requirePayload
		}
	}
	// Accept both "Speaker - payload" and "Speaker- payload" without treating
	// words such as "agent-based" as speaker labels. A label-only dash is also
	// accepted for a following reported block.
	if !strings.HasPrefix(trimmed, "-") || len(trimmed) > 1 && trimmed[1] != ' ' && trimmed[1] != '\t' {
		return false
	}
	return (len(strings.TrimSpace(trimmed[1:])) != 0) == requirePayload
}

func reportedSourceAndVerb(value string) bool {
	remainder, ok := consumeReportedSource(value)
	if !ok {
		return false
	}
	_, ok = consumeReportedVerb(remainder)
	return ok
}

func consumeReportedSource(value string) (string, bool) {
	value = trimReportedSeparator(value)
	if remainder, ok := consumeReportedToken(value, "the"); ok {
		value = trimReportedSeparator(remainder)
	}
	for _, source := range reportedSpeechSources {
		if remainder, ok := consumeReportedToken(value, source); ok {
			return trimReportedSeparator(remainder), true
		}
	}
	return "", false
}

func consumeReportedVerb(value string) (string, bool) {
	value = trimReportedSeparator(value)
	for _, verb := range reportedSpeechVerbs {
		if remainder, ok := consumeReportedToken(value, verb); ok {
			return trimReportedSeparator(remainder), true
		}
	}
	return "", false
}

func consumeReportedPrefix(value, prefix string) (string, bool) {
	remainder, ok := consumeReportedToken(value, prefix)
	if !ok {
		return "", false
	}
	return trimReportedSeparator(remainder), true
}

func consumeReportedToken(value, token string) (string, bool) {
	if !strings.HasPrefix(value, token) {
		return "", false
	}
	remainder := value[len(token):]
	if remainder != "" && isASCIIWordByte(token[len(token)-1]) && isASCIIWordByte(remainder[0]) {
		return "", false
	}
	return remainder, true
}

func trimReportedSeparator(value string) string {
	return strings.TrimLeft(value, " \t:：,，-—")
}

func isASCIIWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_'
}

func containsAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func startsAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}

func buildRelations(turns []normalizedTurn, classified []classifiedSlice) []Relation {
	relations := make([]Relation, 0)
	seen := make(map[string]struct{})
	items := make(map[string]Item)
	for _, entry := range classified {
		if entry.itemID == "" {
			continue
		}
		items[entry.itemID] = Item{ID: entry.itemID, Kind: entry.itemKind, Attribution: turns[entry.slice.turnIndex].role}
	}
	appendRelation := func(kind RelationKind, fromID, toID, ruleID string) {
		if fromID == "" || toID == "" || fromID == toID {
			return
		}
		from, fromExists := items[fromID]
		to, toExists := items[toID]
		if !fromExists || !toExists || !validRelationEndpoints(kind, from, to) {
			return
		}
		key := string(kind) + "\x00" + fromID + "\x00" + toID
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		derivation := "rule_derived_candidate"
		if kind == RelationRespondsTo {
			derivation = "structural"
		}
		relations = append(relations, Relation{
			ID:         digest("mindweaver/sessiondistill/relation/v1", []byte(key)),
			Kind:       kind,
			FromID:     fromID,
			ToID:       toID,
			RuleID:     ruleID,
			Derivation: derivation,
		})
	}
	occurrences := make(map[string]int)
	userByTurn := make([][]*classifiedSlice, len(turns))
	for index := range classified {
		entry := &classified[index]
		if entry.itemID == "" {
			continue
		}
		occurrences[entry.itemID]++
		if turns[entry.slice.turnIndex].role == RoleUser {
			userByTurn[entry.slice.turnIndex] = append(userByTurn[entry.slice.turnIndex], entry)
		}
	}
	previousUserTurn := -1
	for turnIndex := range turns {
		if turns[turnIndex].role != RoleUser {
			continue
		}
		currentEntries := userByTurn[turnIndex]
		if previousUserTurn >= 0 {
			previousEntries := userByTurn[previousUserTurn]
			if len(currentEntries) == 1 && len(previousEntries) == 1 {
				current, previousUser := currentEntries[0], previousEntries[0]
				lower := strings.ToLower(strings.TrimSpace(current.statement))
				if explicitEvolutionRelationCue(lower) && occurrences[current.itemID] == 1 && occurrences[previousUser.itemID] == 1 {
					appendRelation(RelationEvolvesFrom, current.itemID, previousUser.itemID, "relation-explicit-evolution-v1")
				}
			}
		}
		previousUserTurn = turnIndex
	}

	var previous *classifiedSlice
	for index := range classified {
		current := &classified[index]
		if current.itemID == "" {
			previous = nil
			continue
		}
		if previous != nil && previous.itemID != "" && previous.slice.turnIndex == current.slice.turnIndex {
			lower := strings.ToLower(strings.TrimSpace(current.statement))
			if negatesRelationCue(lower) {
				previous = current
				continue
			}
			switch {
			case startsAny(lower, "因为", "原因是", "because ", "since "):
				appendRelation(RelationSupports, current.itemID, previous.itemID, "relation-because-v1")
			case startsAny(lower, "所以", "因此", "由此", "therefore ", "thus "):
				appendRelation(RelationLeadsTo, previous.itemID, current.itemID, "relation-therefore-v1")
			case startsAny(lower, "下一步", "接下来", "随后", "然后", "next ", "then "):
				appendRelation(RelationLeadsTo, previous.itemID, current.itemID, "relation-next-v1")
			case startsAny(lower, "但是", "然而", "不过", "but ", "however "):
				appendRelation(RelationContrastsWith, current.itemID, previous.itemID, "relation-contrast-v1")
			case startsAny(lower, "取决于", "依赖于", "depends on "):
				appendRelation(RelationDependsOn, current.itemID, previous.itemID, "relation-dependency-v1")
			case startsAny(lower, "为了解决", "用于解决", "to address "):
				appendRelation(RelationAddresses, current.itemID, previous.itemID, "relation-address-v1")
			case current.itemKind == KindConstraint && startsAny(lower, "前提是", "必须", "only ", "provided "):
				appendRelation(RelationConstrains, current.itemID, previous.itemID, "relation-constraint-v1")
			}
		}
		previous = current
	}
	for turnIndex := 1; turnIndex < len(turns); turnIndex++ {
		if turns[turnIndex-1].role != RoleUser || turns[turnIndex].role != RoleAssistant {
			continue
		}
		var questionID, responseID string
		for index := range classified {
			entry := &classified[index]
			if entry.itemID == "" {
				continue
			}
			if entry.slice.turnIndex == turnIndex-1 && entry.itemKind == KindQuestion {
				questionID = entry.itemID
			}
			if entry.slice.turnIndex == turnIndex && responseID == "" {
				responseID = entry.itemID
			}
		}
		appendRelation(RelationRespondsTo, responseID, questionID, "relation-adjacent-response-v1")
	}
	sort.SliceStable(relations, func(i, j int) bool {
		if relations[i].FromID != relations[j].FromID {
			return relations[i].FromID < relations[j].FromID
		}
		if relations[i].ToID != relations[j].ToID {
			return relations[i].ToID < relations[j].ToID
		}
		return relations[i].Kind < relations[j].Kind
	})
	return relations
}

func negatesRelationCue(value string) bool {
	return containsAny(value,
		"不支持", "不能支持", "并不支持", "does not support", "doesn't support", "not support",
		"不是对前项的约束", "并非约束", "不构成约束", "not a constraint", "does not constrain",
		"不取决于", "并不取决于", "depends on nothing", "does not depend", "doesn't depend",
		"不处理前述", "并不处理前述", "只是引用", "does not address", "doesn't address",
		"不是由前项导致", "并非由前项导致", "not caused by", "does not lead", "doesn't lead",
		"不构成对比", "并不与前项构成对比", "not a contrast", "does not contrast", "doesn't contrast",
	)
}

func renderJSON(result Distillation) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return nil, newError(CodeOutputFailed, err)
	}
	return output.Bytes(), nil
}
