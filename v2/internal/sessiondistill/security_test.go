package sessiondistill

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRedactionCredentialClassesAndPreRedactedTextAreStable(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		secret string
	}{
		{name: "basic", input: "Authorization: Basic dTpw", want: "Authorization: Basic [redacted:basic_auth]", secret: "dTpw"},
		{name: "bearer", input: "Authorization: Bearer abc123", want: "Authorization: Bearer [redacted:bearer_token]", secret: "abc123"},
		{name: "cookie outer precedence", input: "Cookie: theme=dark; sessionid=abc", want: "Cookie: [redacted:cookie]", secret: "abc"},
		{name: "session cookie", input: "sessionid=abc", want: "sessionid=[redacted:session_cookie]", secret: "abc"},
		{name: "github pat", input: "github_pat_abcdefghijklmnopqrstuvwx", want: "[redacted:github_token]", secret: "abcdefghijklmnopqrstuvwx"},
		{name: "aws secret", input: "AWS_SECRET_ACCESS_KEY=z9", want: "AWS_SECRET_ACCESS_KEY=[redacted:aws_secret_key]", secret: "z9"},
		{name: "database uri", input: "postgres://user:p@localhost/db", want: "postgres://user:[redacted:database_password]@localhost/db", secret: ":p@"},
		{name: "pre redacted", input: "before [redacted:credential] after", want: "before [redacted:credential] after", secret: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			safe, spans, count := redact(test.input)
			if safe != test.want || count != 1 || len(spans) != 1 || !validRedactions(normalizedTurn{text: safe, redaction: spans}) {
				t.Fatalf("safe=%q spans=%+v count=%d", safe, spans, count)
			}
			if test.secret != "" && strings.Contains(safe, test.secret) {
				t.Fatalf("secret fragment survived: %q", safe)
			}
		})
	}
}

func TestDistillBindsPublicProvenanceToRedactedContentAndCallerIdentity(t *testing.T) {
	base := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns:         []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是方案 A。"}},
	}
	first, err := NewV1().Distill(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	changedText := base
	changedText.Turns = append([]VisibleTurn(nil), base.Turns...)
	changedText.Turns[0].Text = "我的想法是方案 B。"
	second, err := NewV1().Distill(context.Background(), changedText)
	if err != nil {
		t.Fatal(err)
	}
	changedCallerID := base
	changedCallerID.Turns = append([]VisibleTurn(nil), base.Turns...)
	changedCallerID.Turns[0].EventID = testID('c')
	third, err := NewV1().Distill(context.Background(), changedCallerID)
	if err != nil {
		t.Fatal(err)
	}
	firstResult, secondResult, thirdResult := first.Result(), second.Result(), third.Result()
	firstSource := firstResult.UserItems[0].Sources[0]
	if firstSource.EventID == base.Turns[0].EventID || firstSource.EventID == secondResult.UserItems[0].Sources[0].EventID ||
		firstSource.EventID == thirdResult.UserItems[0].Sources[0].EventID || firstResult.SessionID == secondResult.SessionID ||
		firstResult.SessionID == thirdResult.SessionID || firstResult.InputDigest == secondResult.InputDigest {
		t.Fatalf("public provenance was not bound to both safe content and caller identity")
	}
}

func TestDistillBoundsRedactionSpansAndPostRedactionTotal(t *testing.T) {
	fallback, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{{
			EventID: testID('b'), Ordinal: 1, Role: RoleUser,
			Text: strings.Repeat("password=x ", maxRedactionSpans+1),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result := fallback.Result(); result.RedactedCount != 1 || len(result.UserItems) != 0 {
		t.Fatalf("redaction overflow did not fail safe: %+v", result)
	}

	turns := make([]VisibleTurn, 800)
	for index := range turns {
		turns[index] = VisibleTurn{
			EventID: digest("mindweaver/sessiondistill/test-event/v1", []byte(fmt.Sprintf("%d", index))),
			Ordinal: uint32(index + 1),
			Role:    RoleUser,
			Text:    strings.Repeat("password=x ", maxRedactionSpans),
		}
	}
	bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: turns})
	if CodeOf(err) != CodeInputLimitExceeded || len(bundle.JSON()) != 0 || len(bundle.Markdown()) != 0 {
		t.Fatalf("post-redaction expansion was accepted: err=%v code=%s", err, CodeOf(err))
	}
}

func TestVerifyResultRejectsIdentitySourceAndRelationMutations(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{{
			EventID: testID('b'), Ordinal: 1, Role: RoleUser,
			Text: "我的想法是先做 CLI。\n因为这样更容易审计。",
		}},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	turns, _, err := normalizeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*Distillation)
	}{
		{name: "session identity", mutate: func(result *Distillation) { result.SessionID = testID('f') }},
		{name: "input digest", mutate: func(result *Distillation) { result.InputDigest = testID('f') }},
		{name: "redaction count", mutate: func(result *Distillation) { result.RedactedCount++ }},
		{name: "item rule", mutate: func(result *Distillation) { result.UserItems[0].RuleID = "forged" }},
		{name: "source event", mutate: func(result *Distillation) { result.UserItems[0].Sources[0].EventID = testID('f') }},
		{name: "source role", mutate: func(result *Distillation) { result.UserItems[0].Sources[0].Role = RoleAssistant }},
		{name: "rune boundary", mutate: func(result *Distillation) { result.UserItems[0].Sources[0].Start++ }},
		{name: "span hash", mutate: func(result *Distillation) { result.UserItems[0].Sources[0].Hash = testID('f') }},
		{name: "relation endpoint", mutate: func(result *Distillation) { result.Relations[0].ToID = result.Relations[0].FromID }},
		{name: "relation rule", mutate: func(result *Distillation) { result.Relations[0].RuleID = "forged" }},
		{name: "relation derivation", mutate: func(result *Distillation) { result.Relations[0].Derivation = "structural" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			result := bundle.Result()
			mutation.mutate(&result)
			if err := verifyResult(result, turns, request.SessionID); CodeOf(err) != CodeOutputFailed {
				t.Fatalf("mutation passed verification: err=%v code=%s", err, CodeOf(err))
			}
		})
	}
}

func TestVerifyResultRejectsSourceOverlappingRedaction(t *testing.T) {
	request := Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns: []VisibleTurn{{
			EventID: testID('b'), Ordinal: 1, Role: RoleUser,
			Text: "我的想法是 A password=hidden-value",
		}},
	}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	turns, _, err := normalizeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	item := &result.UserItems[0]
	item.Statement = turns[0].text
	item.ID = digest("mindweaver/sessiondistill/item/v1", []byte(result.SessionID), []byte(string(item.Kind)+"\x00"+string(item.Attribution)+"\x00"+item.Statement))
	item.Sources[0].Start = 0
	item.Sources[0].End = len(turns[0].text)
	item.Sources[0].Hash = digest(
		"mindweaver/sessiondistill/span/v1",
		[]byte(turns[0].eventID),
		[]byte("0"),
		[]byte(fmt.Sprintf("%d", len(turns[0].text))),
		[]byte(item.Statement),
	)
	if err := verifyResult(result, turns, request.SessionID); CodeOf(err) != CodeOutputFailed {
		t.Fatalf("redaction-overlapping source passed verification: err=%v code=%s", err, CodeOf(err))
	}
}

func TestDistillDoesNotPromoteDecisionMentionsOrNegations(t *testing.T) {
	statements := []string{
		"我担心这个决定有风险。",
		"不要把问题改写成决定。",
		"文档中写着‘我决定使用 Redis’。",
		"Agent said: I decided to use Redis.",
		"This is not a decision: use Redis.",
	}
	turns := make([]VisibleTurn, len(statements))
	for index, statement := range statements {
		turns[index] = VisibleTurn{EventID: digest("mindweaver/sessiondistill/negative-event/v1", []byte(fmt.Sprintf("%d", index))), Ordinal: uint32(index + 1), Role: RoleUser, Text: statement}
	}
	bundle, err := NewV1().Distill(context.Background(), Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: turns})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range bundle.Result().UserItems {
		if item.Kind == KindDecision {
			t.Fatalf("decision mention was promoted: %+v", item)
		}
	}
	for _, artifact := range [][]byte{bundle.JSON(), bundle.Markdown()} {
		if bytes.Contains(artifact, []byte(`"kind":"decision"`)) || bytes.Contains(artifact, []byte("`decision`")) {
			t.Fatalf("decision mention reached artifacts as a decision: %s", artifact)
		}
	}
}

func TestVerifyResultRejectsForgedUnclassifiedItem(t *testing.T) {
	request := Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{
		EventID: testID('b'), Ordinal: 1, Role: RoleUser,
		Text: "我的想法是采用 SQLite。\n这只是一句普通背景。",
	}}}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	turns, _, err := normalizeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	item := &result.UserItems[0]
	statement := "这只是一句普通背景。"
	start := strings.Index(turns[0].text, statement)
	item.Statement = statement
	item.ID = digest("mindweaver/sessiondistill/item/v1", []byte(result.SessionID), []byte(string(item.Kind)+"\x00"+string(item.Attribution)+"\x00"+statement))
	item.Sources[0].Start = start
	item.Sources[0].End = start + len(statement)
	item.Sources[0].Hash = digest(
		"mindweaver/sessiondistill/span/v1",
		[]byte(turns[0].eventID),
		[]byte(fmt.Sprintf("%d", item.Sources[0].Start)),
		[]byte(fmt.Sprintf("%d", item.Sources[0].End)),
		[]byte(statement),
	)
	result.Relations = nil
	if err := verifyResult(result, turns, request.SessionID); CodeOf(err) != CodeOutputFailed {
		t.Fatalf("unclassified forged item passed verification: err=%v code=%s", err, CodeOf(err))
	}
}

func TestVerifyResultRejectsForgedRelationAcrossIneligibleGap(t *testing.T) {
	request := Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{
		EventID: testID('b'), Ordinal: 1, Role: RoleUser,
		Text: "我的想法是先做 CLI。\n这只是一句普通背景。\n因为便于使用。",
	}}}
	bundle, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	turns, _, err := normalizeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.Relations) != 0 || len(result.UserItems) != 2 {
		t.Fatalf("unexpected baseline: %+v", result)
	}
	var fromID, toID string
	for _, item := range result.UserItems {
		if item.Kind == KindRationale {
			fromID = item.ID
		}
		if item.Kind == KindIdea {
			toID = item.ID
		}
	}
	key := string(RelationSupports) + "\x00" + fromID + "\x00" + toID
	result.Relations = []Relation{{
		ID:         digest("mindweaver/sessiondistill/relation/v1", []byte(key)),
		Kind:       RelationSupports,
		FromID:     fromID,
		ToID:       toID,
		RuleID:     "relation-because-v1",
		Derivation: "rule_derived_candidate",
	}}
	if err := verifyResult(result, turns, request.SessionID); CodeOf(err) != CodeOutputFailed {
		t.Fatalf("gap-crossing forged relation passed verification: err=%v code=%s", err, CodeOf(err))
	}
}

func TestDistillSourceUsesSingleEventIdentity(t *testing.T) {
	request := Request{SchemaVersion: SchemaVersion, SessionID: testID('a'), Turns: []VisibleTurn{{
		EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是先做 CLI。",
	}}}
	first, err := NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result := first.Result()
	source := result.UserItems[0].Sources[0]
	wantSpanHash := digest(
		"mindweaver/sessiondistill/span/v1",
		[]byte(source.EventID),
		[]byte(fmt.Sprintf("%d", source.Start)),
		[]byte(fmt.Sprintf("%d", source.End)),
		[]byte(result.UserItems[0].Statement),
	)
	if !validOpaqueID(source.EventID) || source.EventID == request.Turns[0].EventID || source.Hash != wantSpanHash || bytes.Contains(first.JSON(), []byte("event_hash")) || bytes.Contains(first.Markdown(), []byte("event hash")) {
		t.Fatalf("source identity contract drifted: source=%+v json=%s markdown=%s", source, first.JSON(), first.Markdown())
	}
	changed := request
	changed.Turns = append([]VisibleTurn(nil), request.Turns...)
	changed.Turns[0].Text = "我的想法是先做 TUI。"
	second, err := NewV1().Distill(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	secondResult := second.Result()
	secondSource := secondResult.UserItems[0].Sources[0]
	if source.EventID == secondSource.EventID || source.Hash == secondSource.Hash || result.SessionID == secondResult.SessionID || result.InputDigest == secondResult.InputDigest {
		t.Fatalf("safe input change did not change identities: first=%+v second=%+v", result, secondResult)
	}
}
