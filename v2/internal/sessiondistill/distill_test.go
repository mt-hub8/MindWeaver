package sessiondistill

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDistillBilingualIdeasAttributionSourcesAndRelations(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{
			{
				EventID: testID('b'), Ordinal: 1, Role: RoleUser,
				Text: "我的想法是先做一个可用的 CLI。\r\n因为这样更多人可以使用。\r\n下一步补充规则测试。",
			},
			{
				EventID: testID('c'), Ordinal: 2, Role: RoleUser,
				Text: "是否需要立即引入强化学习？",
			},
			{
				EventID: testID('d'), Ordinal: 3, Role: RoleAssistant,
				Text: "我认为先用确定性规则更稳妥。",
			},
		},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if result.SchemaVersion != 1 || result.PolicyVersion != PolicyVersion || !result.UntrustedVisibleContent {
		t.Fatalf("unexpected metadata: %+v", result)
	}
	wantKinds := []Kind{KindIdea, KindRationale, KindNextAction, KindQuestion}
	if len(result.UserItems) != len(wantKinds) {
		t.Fatalf("user items = %#v", result.UserItems)
	}
	for index, want := range wantKinds {
		item := result.UserItems[index]
		if item.Kind != want || item.Attribution != RoleUser || len(item.Sources) != 1 {
			t.Fatalf("item %d = %+v", index, item)
		}
		source := item.Sources[0]
		if source.Role != RoleUser || !validOpaqueID(source.EventID) || source.EventID == request.Turns[int(source.Ordinal)-1].EventID || !validOpaqueID(source.Hash) {
			t.Fatalf("source %d = %+v", index, source)
		}
	}
	if len(result.AssistantContext) != 1 || result.AssistantContext[0].Attribution != RoleAssistant {
		t.Fatalf("assistant context = %#v", result.AssistantContext)
	}
	relationKinds := make(map[RelationKind]bool)
	for _, relation := range result.Relations {
		relationKinds[relation.Kind] = true
	}
	for _, want := range []RelationKind{RelationSupports, RelationLeadsTo, RelationRespondsTo} {
		if !relationKinds[want] {
			t.Fatalf("missing relation %s in %#v", want, result.Relations)
		}
	}
	for _, relation := range result.Relations {
		if relation.Kind == RelationRespondsTo && relation.Derivation != "structural" || relation.Kind != RelationRespondsTo && relation.Derivation != "rule_derived_candidate" {
			t.Fatalf("relation authority is unclear: %+v", relation)
		}
	}
	if !bytes.Contains(bundle.Markdown(), []byte("Assistant context (kept separate)")) {
		t.Fatalf("assistant boundary absent: %s", bundle.Markdown())
	}
	var decoded Distillation
	if err := json.Unmarshal(bundle.JSON(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.InputDigest != result.InputDigest {
		t.Fatalf("JSON result drift")
	}
}

func TestDistillCommonGoalForms(t *testing.T) {
	for index, text := range []string{"我的目标是完成 Codex CLI。", "My goal is to ship a useful CLI."} {
		request := Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{
			EventID: testID(byte('b' + index)), Ordinal: 1, Role: RoleUser, Text: text,
		}}}
		bundle, err := NewV1().Distill(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		items := bundle.Result().UserItems
		if len(items) != 1 || items[0].Kind != KindGoal || items[0].Statement != text {
			t.Fatalf("text=%q items=%#v", text, items)
		}
	}
}

func TestDistillEveryTaxonomyKindHasChineseAndEnglishGoldens(t *testing.T) {
	testCases := []struct {
		name   string
		text   string
		kind   Kind
		ruleID string
	}{
		{name: "idea_zh", text: "我的想法是先做本地 CLI。", kind: KindIdea, ruleID: "idea-explicit-v1"},
		{name: "idea_en", text: "My idea is a local CLI.", kind: KindIdea, ruleID: "idea-explicit-v1"},
		{name: "goal_zh", text: "我的目标是完成可用的 CLI。", kind: KindGoal, ruleID: "goal-explicit-v1"},
		{name: "goal_en", text: "My goal is to ship a useful CLI.", kind: KindGoal, ruleID: "goal-explicit-v1"},
		{name: "constraint_zh", text: "必须保留来源证据。", kind: KindConstraint, ruleID: "constraint-explicit-v1"},
		{name: "constraint_en", text: "Must preserve source evidence.", kind: KindConstraint, ruleID: "constraint-explicit-v1"},
		{name: "question_zh", text: "是否需要保留来源证据？", kind: KindQuestion, ruleID: "question-explicit-v1"},
		{name: "question_en", text: "Should we preserve source evidence?", kind: KindQuestion, ruleID: "question-explicit-v1"},
		{name: "assumption_zh", text: "假设仓库保持在本地。", kind: KindAssumption, ruleID: "assumption-explicit-v1"},
		{name: "assumption_en", text: "Assume the repository stays local.", kind: KindAssumption, ruleID: "assumption-explicit-v1"},
		{name: "rationale_zh", text: "因为可追溯来源便于复审。", kind: KindRationale, ruleID: "rationale-explicit-v1"},
		{name: "rationale_en", text: "Because provenance improves review.", kind: KindRationale, ruleID: "rationale-explicit-v1"},
		{name: "evidence_zh", text: "证据是测试通过。", kind: KindEvidence, ruleID: "evidence-explicit-v1"},
		{name: "evidence_en", text: "Evidence: the test passed.", kind: KindEvidence, ruleID: "evidence-explicit-v1"},
		{name: "decision_zh", text: "我决定使用 Go。", kind: KindDecision, ruleID: "decision-explicit-v1"},
		{name: "decision_en", text: "I decided to use Go.", kind: KindDecision, ruleID: "decision-explicit-v1"},
		{name: "learning_zh", text: "我学到明确范围可以减少错误。", kind: KindLearning, ruleID: "learning-explicit-v1"},
		{name: "learning_en", text: "I learned that explicit scope prevents mistakes.", kind: KindLearning, ruleID: "learning-explicit-v1"},
		{name: "concern_zh", text: "我担心同用户篡改。", kind: KindConcern, ruleID: "concern-explicit-v1"},
		{name: "concern_en", text: "Concern: same-user tampering remains possible.", kind: KindConcern, ruleID: "concern-explicit-v1"},
		{name: "open_item_zh", text: "尚未完成独立账户资格。", kind: KindOpenItem, ruleID: "open-item-explicit-v1"},
		{name: "open_item_en", text: "Open item: isolated-profile qualification.", kind: KindOpenItem, ruleID: "open-item-explicit-v1"},
		{name: "next_action_zh", text: "下一步串行运行门禁。", kind: KindNextAction, ruleID: "next-action-explicit-v1"},
		{name: "next_action_en", text: "Next: run the serial gates.", kind: KindNextAction, ruleID: "next-action-explicit-v1"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{
				EventID: digest("mindweaver/sessiondistill/taxonomy-golden-event/v1", []byte(testCase.name)), Ordinal: 1, Role: RoleUser, Text: testCase.text,
			}}}
			bundle, err := NewV1().Distill(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			items := bundle.Result().UserItems
			if len(items) != 1 || items[0].Kind != testCase.kind || items[0].RuleID != testCase.ruleID ||
				items[0].Attribution != RoleUser || items[0].Statement != testCase.text || len(items[0].Sources) != 1 {
				t.Fatalf("text=%q items=%#v", testCase.text, items)
			}
		})
	}
}

func TestDistillRedactsBeforeHashSliceAndRender(t *testing.T) {
	secret := "z9"
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{{
			EventID: testID('b'), Ordinal: 1, Role: RoleUser,
			Text: "我的想法 password=" + secret + " 应该进入报告。\n下一步只保留安全内容。",
		}},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if result.RedactedCount != 1 {
		t.Fatalf("redacted count = %d", result.RedactedCount)
	}
	for _, data := range [][]byte{bundle.JSON(), bundle.Markdown()} {
		if bytes.Contains(data, []byte(secret)) || bytes.Contains(data, []byte("password=")) {
			t.Fatalf("secret context leaked: %s", data)
		}
	}
	if len(result.UserItems) != 1 || result.UserItems[0].Kind != KindNextAction {
		t.Fatalf("redaction-overlap slice was admitted: %#v", result.UserItems)
	}
}

func TestDistillKeepsSafeTextOnEitherSideOfRedaction(t *testing.T) {
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{{
			EventID: testID('b'), Ordinal: 1, Role: RoleUser,
			Text: "我的想法是先做 CLI。 password=hidden-value 下一步写测试。",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.UserItems) != 2 || result.UserItems[0].Kind != KindIdea || result.UserItems[1].Kind != KindNextAction {
		t.Fatalf("safe context was discarded: %#v", result.UserItems)
	}
	if bytes.Contains(bundle.JSON(), []byte("hidden-value")) || bytes.Contains(bundle.Markdown(), []byte("hidden-value")) {
		t.Fatalf("secret leaked")
	}
}

func TestContentAddressedAdapterRoundTripIsByteIdentical(t *testing.T) {
	request, err := NewContentAddressedRequest([]InputTurn{{Role: RoleUser, Text: "我的想法 refresh_token=rt-private-value 应该被隐藏。\n下一步输出报告。"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped, err := DecodeRequest(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewV1().Distill(context.Background(), roundTripped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.JSON(), second.JSON()) || !bytes.Equal(first.Markdown(), second.Markdown()) || first.Result().RedactedCount != 1 {
		t.Fatalf("adapter round trip drifted: first=%s second=%s", first.JSON(), second.JSON())
	}
}

func TestDistillQuestionNeverBecomesClaimAndAssistantNeverBecomesUser(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{
			{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Redis 是必需的吗？"},
			{EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我的想法是可以先不用 Redis。"},
		},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.UserItems) != 1 || result.UserItems[0].Kind != KindQuestion {
		t.Fatalf("question changed meaning: %#v", result.UserItems)
	}
	if len(result.AssistantContext) != 1 || result.AssistantContext[0].Attribution != RoleAssistant {
		t.Fatalf("assistant attribution changed: %#v", result.AssistantContext)
	}
}

func TestDistillChineseQuestionAndUndecidedStatementKeepTheirMeaning(t *testing.T) {
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{
			{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "必须用 Redis 吗"},
			{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "我尚未决定使用 Redis。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.UserItems) != 2 || result.UserItems[0].Kind != KindQuestion || result.UserItems[1].Kind != KindOpenItem {
		t.Fatalf("meaning changed: %#v", result.UserItems)
	}
}

func TestDistillUnclassifiedOrRedactedSliceBreaksRelationAdjacency(t *testing.T) {
	tests := []string{
		"我的想法是先做 CLI。\n这是一句没有显式分类的中间文本。\n因为这样容易使用。",
		"我的想法是先做 CLI。\npassword=hidden-value\n因为这样容易使用。",
	}
	for _, text := range tests {
		bundle, err := NewV1().Distill(context.Background(), Request{
			SchemaVersion: SchemaVersion,
			SessionID:     testID('a'),
			Turns:         []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: text}},
		})
		if err != nil {
			t.Fatal(err)
		}
		result := bundle.Result()
		for _, relation := range result.Relations {
			if relation.Kind == RelationSupports {
				t.Fatalf("relation crossed an ineligible slice: %#v", result.Relations)
			}
		}
	}
}

func TestDistillRejectsReportedOrNegatedDecisionsAndNegativeSupport(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{
			{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "不要把这当成决定：使用 Redis。"},
			{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Agent 回答：\"我们决定使用 Redis。\""},
			{EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "我的想法是采用 SQLite。\n因为这并不支持前述想法。"},
		},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	for _, item := range result.UserItems {
		if item.Kind == KindDecision {
			t.Fatalf("reported or negated decision was promoted: %#v", result.UserItems)
		}
	}
	for _, relation := range result.Relations {
		if relation.Kind == RelationSupports {
			t.Fatalf("negative text created support: %#v", result.Relations)
		}
	}
}

func TestDistillKnownJSONURIAndEncryptedPEMCredentialsNeverReachArtifacts(t *testing.T) {
	secrets := []string{
		`{"password":"json-private-value"}`,
		`amqp://worker:uri-private-value@localhost/queue`,
		"-----BEGIN ENCRYPTED PRIVATE KEY-----\nencrypted-private-value\n-----END ENCRYPTED PRIVATE KEY-----",
		"refresh_token=refresh-private-value",
		"--token cli-private-value",
		"-----BEGIN PGP PRIVATE KEY BLOCK-----\npgp-private-value\n-----END PGP PRIVATE KEY BLOCK-----",
	}
	idChars := []byte("bcdef1")
	turns := make([]VisibleTurn, 0, len(secrets)+1)
	for index, secret := range secrets {
		turns = append(turns, VisibleTurn{
			EventID: testID(idChars[index]), Ordinal: uint32(index + 1), Role: RoleUser,
			Text: "我的想法是凭据 " + secret + " 不应进入报告。",
		})
	}
	turns = append(turns, VisibleTurn{EventID: testID('2'), Ordinal: uint32(len(turns) + 1), Role: RoleUser, Text: "下一步只输出安全条目。"})
	bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: 1, SessionID: testID('a'), Turns: turns})
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if result.RedactedCount != len(secrets) || len(result.UserItems) != 1 {
		t.Fatalf("result=%+v", result)
	}
	for _, secret := range []string{"json-private-value", "uri-private-value", "encrypted-private-value", "refresh-private-value", "cli-private-value", "pgp-private-value"} {
		if bytes.Contains(bundle.JSON(), []byte(secret)) || bytes.Contains(bundle.Markdown(), []byte(secret)) {
			t.Fatalf("secret leaked: %q", secret)
		}
	}
}

func TestDistillSourceSpanUsesNormalizedRedactedUTF8Bytes(t *testing.T) {
	text := "password=hidden-value\r\n  我的想法是保留中文和 emoji 🧠。  \r\n"
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns:         []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: text}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.UserItems) != 1 {
		t.Fatalf("items = %#v", result.UserItems)
	}
	normalized, _, _ := redact(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	item := result.UserItems[0]
	source := item.Sources[0]
	if source.Start < 0 || source.End > len(normalized) || normalized[source.Start:source.End] != item.Statement {
		t.Fatalf("span %d:%d does not select %q from %q", source.Start, source.End, item.Statement, normalized)
	}
	if source.Basis != "canonical_redacted_utf8_v1" || !validOpaqueID(source.EventID) || !validOpaqueID(source.Hash) {
		t.Fatalf("source basis is not explicit: %+v", source)
	}
}

func TestDistillIsByteDeterministic(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{
			{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "目标是输出稳定结果。\n因为审计依赖稳定哈希。"},
			{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "下一步重复运行一百次。"},
		},
	}
	first, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 100; run++ {
		current, err := NewV1().Distill(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.JSON(), current.JSON()) || !bytes.Equal(first.Markdown(), current.Markdown()) {
			t.Fatalf("output changed on run %d", run)
		}
	}
}

func TestDistillRejectsInvalidShapeBoundsAndCancellationWithZeroBundle(t *testing.T) {
	tests := []Request{
		{SchemaVersion: 2, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法。"}}},
		{SchemaVersion: 1, SessionID: "bad", Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法。"}}},
		{SchemaVersion: 1, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 2, Role: RoleUser, Text: "我的想法。"}}},
		{SchemaVersion: 1, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: Role("system"), Text: "我的想法。"}}},
		{SchemaVersion: 1, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "bad\x00text"}}},
	}
	for index, request := range tests {
		bundle, err := NewV1().Distill(context.Background(), request)
		if err == nil || len(bundle.JSON()) != 0 || len(bundle.Markdown()) != 0 {
			t.Fatalf("case %d returned bundle=%+v err=%v", index, bundle, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bundle, err := NewV1().Distill(ctx, Request{})
	if err == nil || CodeOf(err) != CodeCanceled || len(bundle.JSON()) != 0 || len(bundle.Markdown()) != 0 {
		t.Fatalf("canceled bundle=%+v err=%v", bundle, err)
	}
}

func TestMarkdownTreatsVisibleTextAsData(t *testing.T) {
	attack := "我的想法是 <script>alert(1)</script>，并且 ``` 不是指令。"
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns:         []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: attack}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle.Markdown(), []byte("<script>")) || !bytes.Contains(bundle.Markdown(), []byte("&lt;script&gt;")) {
		t.Fatalf("unsafe Markdown: %s", bundle.Markdown())
	}
	if !bytes.Contains(bundle.JSON(), []byte("<script>")) {
		t.Fatalf("JSON did not preserve data encoding: %s", bundle.JSON())
	}
}

func TestDistillRejectsReportedOrQuotedContentBeforeAllTaxonomy(t *testing.T) {
	reported := []string{
		"Agent said: Must use Redis.",
		"Assistant said: Should we use Redis?",
		"According to Assistant, must use Redis.",
		"According to Agent, must use Redis.",
		"As Assistant said, must use Redis.",
		"模型说：尚未决定 Redis。",
		"根据 Agent 回答，必须使用 Redis。",
		"根据 Codex 回答，必须使用 Redis。",
		"根据助手回答，必须使用 Redis。",
		"文档写着：“尚未决定 Redis。”",
		"文档中说：必须使用 Redis。",
		"“必须使用 Redis”是 Assistant 的建议。",
		"“必须使用 Redis。”",
		"Codex said: Must use Redis.",
		"According to Codex, must use Redis.",
		"Agent 建议必须使用 Redis。",
		"文档建议必须使用 Redis。",
		"正如 Agent 所说，必须使用 Redis。",
		"正如智能体所说，必须使用 Redis。",
		"Agent asked: Must we use Redis?",
		"Agent 问：必须使用 Redis 吗？",
		"Assistant: my idea is Redis.",
		"Agent：我的想法是使用 Redis。",
		"Assistant - my idea is Redis.",
		"Assistant- my idea is Redis.",
		"助手 – 我的想法是使用 Redis。",
		"AI Assistant: my idea is Redis.",
		"Assistant's answer: Must use Redis.",
		"助手的回答：必须使用 Redis。",
		"Assistant:\nMust use Redis.",
		"助手：\n必须使用 Redis。",
		"Assistant: preliminary context.\nMust use Redis.",
	}
	for _, text := range reported {
		t.Run(text, func(t *testing.T) {
			bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: text}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.Result().UserItems) != 0 || len(bundle.Result().Relations) != 0 {
				t.Fatalf("reported content was promoted: %#v", bundle.Result())
			}
		})
	}
	controls := []struct {
		text string
		kind Kind
	}{
		{text: "我认为 Agent 的回答不可靠。", kind: KindIdea},
		{text: "必须验证 Agent 的回答。", kind: KindConstraint},
		{text: "Agent-based constraint: must remain local.", kind: KindConstraint},
	}
	for _, control := range controls {
		bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: control.text}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.Result().UserItems) != 1 || bundle.Result().UserItems[0].Kind != control.kind {
			t.Fatalf("direct user statement was suppressed: text=%q items=%#v", control.text, bundle.Result().UserItems)
		}
	}
}

func TestDistillAllRelationKindsReachExactRules(t *testing.T) {
	tests := []struct {
		name       string
		turns      []VisibleTurn
		kind       RelationKind
		rule       string
		derivation string
		fromKind   Kind
		toKind     Kind
	}{
		{name: "supports", turns: oneUserTurn("我的想法是先做 CLI。\n因为这样更容易使用。"), kind: RelationSupports, rule: "relation-because-v1", derivation: "rule_derived_candidate", fromKind: KindRationale, toKind: KindIdea},
		{name: "constrains", turns: oneUserTurn("目标是交付 CLI。\n必须保持本地优先。"), kind: RelationConstrains, rule: "relation-constraint-v1", derivation: "rule_derived_candidate", fromKind: KindConstraint, toKind: KindGoal},
		{name: "depends", turns: oneUserTurn("目标是交付 CLI。\ndepends on constraint: Windows support."), kind: RelationDependsOn, rule: "relation-dependency-v1", derivation: "rule_derived_candidate", fromKind: KindConstraint, toKind: KindGoal},
		{name: "addresses", turns: oneUserTurn("尚未决定如何保存记录。\n为了解决保存问题，我的想法是先输出 JSON。"), kind: RelationAddresses, rule: "relation-address-v1", derivation: "rule_derived_candidate", fromKind: KindIdea, toKind: KindOpenItem},
		{name: "leads", turns: oneUserTurn("我的想法是先做 CLI。\n下一步补充测试。"), kind: RelationLeadsTo, rule: "relation-next-v1", derivation: "rule_derived_candidate", fromKind: KindIdea, toKind: KindNextAction},
		{name: "contrasts", turns: oneUserTurn("我的想法是使用 Redis。\n但是我认为 SQLite 更简单。"), kind: RelationContrastsWith, rule: "relation-contrast-v1", derivation: "rule_derived_candidate", fromKind: KindIdea, toKind: KindIdea},
		{name: "evolves", turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以比较两种方案。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}}, kind: RelationEvolvesFrom, rule: "relation-explicit-evolution-v1", derivation: "rule_derived_candidate", fromKind: KindIdea, toKind: KindIdea},
		{name: "responds", turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "是否先做 CLI？"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以先做 CLI。"}}, kind: RelationRespondsTo, rule: "relation-adjacent-response-v1", derivation: "structural", fromKind: KindIdea, toKind: KindQuestion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: test.turns})
			if err != nil {
				t.Fatal(err)
			}
			result := bundle.Result()
			items := make(map[string]Item)
			for _, item := range append(append([]Item(nil), result.UserItems...), result.AssistantContext...) {
				items[item.ID] = item
			}
			found := false
			for _, relation := range result.Relations {
				if relation.Kind == test.kind {
					found = true
					if relation.RuleID != test.rule || relation.Derivation != test.derivation || items[relation.FromID].Kind != test.fromKind || items[relation.ToID].Kind != test.toKind {
						t.Fatalf("relation=%+v from=%+v to=%+v", relation, items[relation.FromID], items[relation.ToID])
					}
				}
			}
			if !found {
				t.Fatalf("missing %s in %#v", test.kind, result.Relations)
			}
		})
	}
}

func TestValidRelationEndpointsExactClosedMatrix(t *testing.T) {
	allKinds := []Kind{KindIdea, KindGoal, KindConstraint, KindQuestion, KindAssumption, KindRationale, KindEvidence, KindDecision, KindLearning, KindConcern, KindOpenItem, KindNextAction}
	relationKinds := []RelationKind{RelationSupports, RelationConstrains, RelationDependsOn, RelationAddresses, RelationLeadsTo, RelationContrastsWith, RelationEvolvesFrom}
	allowed := func(relation RelationKind, from, to Kind) bool {
		switch relation {
		case RelationSupports:
			return oneOfKind(from, KindRationale, KindEvidence, KindLearning) && oneOfKind(to, KindIdea, KindGoal, KindDecision, KindConstraint, KindConcern, KindOpenItem, KindNextAction)
		case RelationConstrains:
			return from == KindConstraint && oneOfKind(to, KindIdea, KindGoal, KindDecision, KindNextAction)
		case RelationDependsOn:
			return oneOfKind(from, KindAssumption, KindConstraint, KindOpenItem, KindNextAction) && oneOfKind(to, KindGoal, KindDecision, KindIdea, KindNextAction, KindOpenItem)
		case RelationAddresses:
			return oneOfKind(from, KindIdea, KindDecision, KindNextAction, KindLearning) && oneOfKind(to, KindQuestion, KindConcern, KindOpenItem)
		case RelationLeadsTo:
			return oneOfKind(from, KindIdea, KindRationale, KindEvidence, KindDecision, KindGoal, KindLearning) && oneOfKind(to, KindNextAction, KindDecision, KindGoal, KindIdea, KindLearning, KindOpenItem)
		case RelationContrastsWith:
			return oneOfKind(from, KindIdea, KindDecision, KindConstraint, KindConcern, KindRationale) && oneOfKind(to, KindIdea, KindDecision, KindConstraint, KindConcern, KindRationale)
		case RelationEvolvesFrom:
			return from == KindIdea && oneOfKind(to, KindIdea, KindDecision, KindAssumption)
		default:
			return false
		}
	}
	for _, relation := range relationKinds {
		for _, from := range allKinds {
			for _, to := range allKinds {
				got := validRelationEndpoints(relation, Item{Kind: from, Attribution: RoleUser}, Item{Kind: to, Attribution: RoleUser})
				if got != allowed(relation, from, to) {
					t.Fatalf("relation=%s from=%s to=%s got=%t", relation, from, to, got)
				}
			}
		}
	}
	if !validRelationEndpoints(RelationRespondsTo, Item{Kind: KindIdea, Attribution: RoleAssistant}, Item{Kind: KindQuestion, Attribution: RoleUser}) ||
		validRelationEndpoints(RelationRespondsTo, Item{Kind: KindIdea, Attribution: RoleUser}, Item{Kind: KindQuestion, Attribution: RoleUser}) ||
		validRelationEndpoints(RelationRespondsTo, Item{Kind: KindIdea, Attribution: RoleAssistant}, Item{Kind: KindGoal, Attribution: RoleUser}) {
		t.Fatal("responds_to role/kind boundary drifted")
	}
	if validRelationEndpoints(RelationEvolvesFrom, Item{Kind: KindIdea, Attribution: RoleAssistant}, Item{Kind: KindIdea, Attribution: RoleUser}) ||
		validRelationEndpoints(RelationEvolvesFrom, Item{Kind: KindIdea, Attribution: RoleUser}, Item{Kind: KindIdea, Attribution: RoleAssistant}) {
		t.Fatal("evolves_from attribution boundary drifted")
	}
}

func TestDistillExplicitUserViewpointEvolutionBilingual(t *testing.T) {
	tests := []struct {
		name        string
		turns       []VisibleTurn
		priorKind   Kind
		currentText string
	}{
		{
			name: "Chinese revision of decision",
			turns: []VisibleTurn{
				{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我决定先使用 Redis。"},
				{EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以先验证约束。"},
				{EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"},
			},
			priorKind: KindDecision, currentText: "关于我刚才的想法，我改主意了，SQLite 更合适。",
		},
		{
			name: "English revision of assumption",
			turns: []VisibleTurn{
				{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Assumption: Redis is required."},
				{EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "I think both options can be tested."},
				{EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: SQLite is sufficient."},
			},
			priorKind: KindAssumption, currentText: "Regarding my previous idea, I changed my mind: SQLite is sufficient.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: test.turns})
			if err != nil {
				t.Fatal(err)
			}
			result := bundle.Result()
			items := make(map[string]Item)
			for _, item := range append(append([]Item(nil), result.UserItems...), result.AssistantContext...) {
				items[item.ID] = item
			}
			found := 0
			for _, relation := range result.Relations {
				if relation.Kind != RelationEvolvesFrom {
					continue
				}
				found++
				from, to := items[relation.FromID], items[relation.ToID]
				if relation.RuleID != "relation-explicit-evolution-v1" || relation.Derivation != "rule_derived_candidate" ||
					from.Kind != KindIdea || from.Attribution != RoleUser || from.RuleID != "idea-evolution-explicit-v1" ||
					from.Statement != test.currentText || to.Kind != test.priorKind || to.Attribution != RoleUser {
					t.Fatalf("relation=%+v from=%+v to=%+v", relation, from, to)
				}
			}
			if found != 1 {
				t.Fatalf("evolution count=%d result=%+v", found, result)
			}
		})
	}
}

func TestEveryEvolutionRelationPrefixIsReachableAndBoundaryChecked(t *testing.T) {
	for index, prefix := range evolutionRelationPrefixes {
		suffix := " SQLite replaces Redis."
		if strings.HasSuffix(prefix, "：") || strings.HasSuffix(prefix, ":") {
			suffix = "SQLite replaces Redis."
		}
		bundle, err := NewV1().Distill(context.Background(), Request{
			SchemaVersion: SchemaVersion,
			SessionID:     testID('a'),
			Turns: []VisibleTurn{
				{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."},
				{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: prefix + suffix},
			},
		})
		if err != nil {
			t.Fatalf("prefix %d %q: %v", index, prefix, err)
		}
		count := 0
		for _, relation := range bundle.Result().Relations {
			if relation.Kind == RelationEvolvesFrom {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("prefix %d %q relation count=%d result=%+v", index, prefix, count, bundle.Result())
		}
		if strings.ContainsAny(prefix, "我想法条正修关对") && !strings.HasSuffix(prefix, "：") {
			for _, continuation := range []string{"吗，答案是否定的。", "不起，这只是相邻文字。"} {
				invalid, invalidErr := NewV1().Distill(context.Background(), Request{
					SchemaVersion: SchemaVersion,
					SessionID:     testID('a'),
					Turns: []VisibleTurn{
						{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."},
						{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: prefix + continuation},
					},
				})
				if invalidErr != nil {
					t.Fatalf("prefix %d %q continuation %q: %v", index, prefix, continuation, invalidErr)
				}
				for _, relation := range invalid.Result().Relations {
					if relation.Kind == RelationEvolvesFrom {
						t.Fatalf("prefix %d %q accepted missing boundary %q: %+v", index, prefix, continuation, relation)
					}
				}
			}
		}
	}
}

func TestDistillViewpointEvolutionNeverInfersOrMisattributes(t *testing.T) {
	tests := []struct {
		name  string
		turns []VisibleTurn
	}{
		{
			name:  "assistant revision",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "I changed my mind: use SQLite."}},
		},
		{
			name:  "reported agent revision",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "According to Agent, I changed my mind: use SQLite."}},
		},
		{
			name:  "intervening user context",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "这里还有普通背景。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}},
		},
		{
			name:  "no prior viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我改变了想法，现在使用 SQLite。"}},
		},
		{
			name:  "prior goal is not a viewpoint endpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "目标是完成 CLI。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以先验证。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}},
		},
		{
			name:  "multiple prior stance items are ambiguous",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。\n假设数据必须共享。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以验证。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}},
		},
		{
			name:  "deduplicated prior has multiple sources",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}},
		},
		{
			name:  "speaker label is not a prior user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Assistant: my idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "dash speaker label is not a prior user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Assistant - my idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "composite speaker label is not a prior user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "AI Assistant: my idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "multiline speaker block is not a prior user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Assistant:\nMy idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "multiline speaker block is not a current user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Assistant:\nRegarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "multiline speaker block with payload is not a prior user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "Assistant: preliminary context.\nMy idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Regarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "multiline speaker block with payload is not a current user viewpoint",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Assistant: preliminary context.\nRegarding my previous idea, I changed my mind: use SQLite."}},
		},
		{
			name:  "unanchored strong cue is not linked",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "I think both can be tested."}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "I changed my mind: use SQLite."}},
		},
		{
			name:  "weak now cue is not linked",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "I think both can be tested."}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "I now think SQLite is better."}},
		},
		{
			name:  "Chinese weak now cue is not linked",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以测试。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "我现在认为 SQLite 更合适。"}},
		},
		{
			name:  "mindset prefix is not a revision token",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "I think both can be tested."}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "Regarding my previous idea, I changed my mindset entirely."}},
		},
		{
			name:  "Chinese denied revision",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "修正上一条想法：这不是修正，只是示例。"}},
		},
		{
			name:  "English denied revision",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "My idea is Redis."}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "Revising my previous idea: do not treat this example as a revision."}},
		},
		{
			name:  "multiple current items are ambiguous",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleAssistant, Text: "我认为可以测试。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。\n下一步运行测试。"}},
		},
		{
			name:  "deduplicated current has multiple sources",
			turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是使用 Redis。"}, {EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}, {EventID: testID('d'), Ordinal: 3, Role: RoleUser, Text: "关于我刚才的想法，我改主意了，SQLite 更合适。"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: test.turns})
			if err != nil {
				t.Fatal(err)
			}
			for _, relation := range bundle.Result().Relations {
				if relation.Kind == RelationEvolvesFrom {
					t.Fatalf("inferred or misattributed evolution: %+v", relation)
				}
			}
		})
	}
	for _, text := range []string{"我不再认为必须使用 Redis。", "I changed my mind: must use SQLite."} {
		bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: text}}})
		if err != nil {
			t.Fatal(err)
		}
		items := bundle.Result().UserItems
		if len(items) != 1 || items[0].Kind != KindIdea || items[0].RuleID != "idea-evolution-explicit-v1" {
			t.Fatalf("strong revision was shadowed by another taxonomy: text=%q items=%#v", text, items)
		}
	}
}

func TestDistillNegatedRelationCuesDoNotCreateCandidates(t *testing.T) {
	tests := []struct {
		kind RelationKind
		text string
	}{
		{kind: RelationSupports, text: "我的想法是采用 SQLite。\n因为这并不支持前述想法。"},
		{kind: RelationConstrains, text: "目标是交付 CLI。\n必须说明：这不是对前项的约束。"},
		{kind: RelationDependsOn, text: "目标是交付 CLI。\ndepends on nothing from the prior goal, constraint: stay local."},
		{kind: RelationAddresses, text: "尚未决定如何保存记录。\n为了解决只是引用，我认为这并不处理前述问题。"},
		{kind: RelationLeadsTo, text: "我的想法是采用 SQLite。\n下一步不是由前项导致的，我决定独立验证。"},
		{kind: RelationContrastsWith, text: "我的想法是采用 SQLite。\n但是这并不与前项构成对比，我认为仍采用 SQLite。"},
	}
	for _, test := range tests {
		bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: oneUserTurn(test.text)})
		if err != nil {
			t.Fatal(err)
		}
		for _, relation := range bundle.Result().Relations {
			if relation.Kind == test.kind {
				t.Fatalf("negated cue created %s: text=%q relation=%+v", test.kind, test.text, relation)
			}
		}
	}
}

func TestDistillIneligibleGapBreaksEveryRelation(t *testing.T) {
	tests := []struct {
		kind RelationKind
		text string
	}{
		{RelationSupports, "我的想法是先做 CLI。\n普通背景句。\n因为这样更容易使用。"},
		{RelationConstrains, "目标是交付 CLI。\n普通背景句。\n必须保持本地优先。"},
		{RelationDependsOn, "目标是交付 CLI。\n普通背景句。\ndepends on constraint: Windows support."},
		{RelationAddresses, "尚未决定如何保存记录。\n普通背景句。\n为了解决保存问题，我的想法是先输出 JSON。"},
		{RelationLeadsTo, "我的想法是先做 CLI。\n普通背景句。\n下一步补充测试。"},
		{RelationContrastsWith, "我的想法是使用 Redis。\n普通背景句。\n但是我认为 SQLite 更简单。"},
	}
	for _, test := range tests {
		bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: oneUserTurn(test.text)})
		if err != nil {
			t.Fatal(err)
		}
		for _, relation := range bundle.Result().Relations {
			if relation.Kind == test.kind {
				t.Fatalf("relation crossed gap: kind=%s relation=%+v", test.kind, relation)
			}
		}
	}
	responseBundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{
		{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "是否先做 CLI？"},
		{EventID: testID('c'), Ordinal: 2, Role: RoleUser, Text: "普通背景句。"},
		{EventID: testID('d'), Ordinal: 3, Role: RoleAssistant, Text: "我认为可以先做 CLI。"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range responseBundle.Result().Relations {
		if relation.Kind == RelationRespondsTo {
			t.Fatalf("responds_to crossed turn gap: %+v", relation)
		}
	}
}

func oneUserTurn(text string) []VisibleTurn {
	return []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: text}}
}

func testID(char byte) string {
	return "sha256:" + strings.Repeat(string(char), 64)
}
