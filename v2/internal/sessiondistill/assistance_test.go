package sessiondistill

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestAttachModelAssistanceIsDeterministicAndResealsWithoutChangingBase(t *testing.T) {
	base := assistanceBaseBundle(t, 3)
	baseResult := base.Result()
	baseJSON := base.JSON()
	baseMarkdown := base.Markdown()
	input := ModelAssistanceInput{
		ProviderConfigDigest: assistanceOpaqueID(900),
		RankedItemIDs:        []string{baseResult.UserItems[2].ID, baseResult.UserItems[0].ID},
	}

	first, err := AttachModelAssistance(base, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AttachModelAssistance(base, input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.JSON(), second.JSON()) || !bytes.Equal(first.Markdown(), second.Markdown()) {
		t.Fatal("identical assistance inputs produced different sealed output")
	}
	if !validBundle(first) || !validBundle(second) {
		t.Fatal("attached bundle is not validly sealed")
	}
	if !bytes.Equal(base.JSON(), baseJSON) || !bytes.Equal(base.Markdown(), baseMarkdown) || base.Result().ModelAssistance != nil {
		t.Fatal("attaching assistance mutated the base bundle")
	}

	result := first.Result()
	assistance := result.ModelAssistance
	if assistance == nil || assistance.SchemaVersion != 1 || assistance.Provider != "ollama" ||
		assistance.Purpose != "rank_existing_user_candidates" || assistance.Status != "completed" ||
		assistance.ProviderConfigDigest != input.ProviderConfigDigest || assistance.InputDigest != result.InputDigest ||
		!reflect.DeepEqual(assistance.RankedItemIDs, input.RankedItemIDs) {
		t.Fatalf("assistance=%+v", assistance)
	}
	withoutAssistance := result
	withoutAssistance.ModelAssistance = nil
	if !reflect.DeepEqual(withoutAssistance, baseResult) {
		t.Fatalf("base evidence changed while attaching assistance:\nbase=%+v\nattached=%+v", baseResult, withoutAssistance)
	}
	if !bytes.Contains(first.JSON(), []byte(`"model_assistance"`)) ||
		!bytes.Contains(first.Markdown(), []byte("Optional local-model priority (non-authoritative)")) {
		t.Fatalf("assistance missing from rendered output:\n%s\n%s", first.JSON(), first.Markdown())
	}
	if _, err := AttachModelAssistance(first, input); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("reattach error=%v code=%s", err, CodeOf(err))
	}
}

func TestAttachModelAssistanceDeepCopiesInputAndAccessors(t *testing.T) {
	base := assistanceBaseBundle(t, 2)
	baseResult := base.Result()
	ids := []string{baseResult.UserItems[1].ID, baseResult.UserItems[0].ID}
	wantIDs := append([]string(nil), ids...)
	bundle, err := AttachModelAssistance(base, ModelAssistanceInput{
		ProviderConfigDigest: assistanceOpaqueID(901),
		RankedItemIDs:        ids,
	})
	if err != nil {
		t.Fatal(err)
	}

	ids[0] = assistanceOpaqueID(902)
	result := bundle.Result()
	if !reflect.DeepEqual(result.ModelAssistance.RankedItemIDs, wantIDs) {
		t.Fatalf("input slice aliases sealed bundle: %+v", result.ModelAssistance.RankedItemIDs)
	}
	result.ModelAssistance.Provider = "mutated"
	result.ModelAssistance.RankedItemIDs[0] = assistanceOpaqueID(903)
	result.UserItems[0].Statement = "mutated"
	result.UserItems[0].Sources[0].Basis = "mutated"
	jsonCopy := bundle.JSON()
	markdownCopy := bundle.Markdown()
	jsonCopy[0] ^= 0xff
	markdownCopy[0] ^= 0xff

	fresh := bundle.Result()
	if fresh.ModelAssistance.Provider != "ollama" || !reflect.DeepEqual(fresh.ModelAssistance.RankedItemIDs, wantIDs) ||
		fresh.UserItems[0].Statement == "mutated" || fresh.UserItems[0].Sources[0].Basis == "mutated" ||
		bytes.Equal(bundle.JSON(), jsonCopy) || bytes.Equal(bundle.Markdown(), markdownCopy) || !validBundle(bundle) {
		t.Fatalf("bundle accessor leaked mutable state: %+v", fresh)
	}
}

func TestAttachModelAssistanceSealsEveryContentFreeFallback(t *testing.T) {
	limitations := []string{
		"NO_USER_CANDIDATES",
		"MODEL_INPUT_LIMIT",
		"MODEL_TIMEOUT",
		"MODEL_UNAVAILABLE",
		"MODEL_RESPONSE_INVALID",
		"OUTCOME_UNCERTAIN",
	}
	for _, limitation := range limitations {
		t.Run(limitation, func(t *testing.T) {
			userItemCount := 1
			if limitation == "NO_USER_CANDIDATES" {
				userItemCount = 0
			}
			base := assistanceBaseBundle(t, userItemCount)
			bundle, err := AttachModelAssistance(base, ModelAssistanceInput{
				ProviderConfigDigest: assistanceOpaqueID(910),
				LimitationCode:       limitation,
				RankedItemIDs:        []string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			result := bundle.Result()
			assistance := result.ModelAssistance
			if assistance == nil || assistance.Status != "fallback" || assistance.LimitationCode != limitation ||
				assistance.RankedItemIDs == nil || len(assistance.RankedItemIDs) != 0 || !validBundle(bundle) {
				t.Fatalf("fallback assistance=%+v", assistance)
			}
			if !bytes.Contains(bundle.JSON(), []byte(`"ranked_item_ids":[]`)) ||
				!bytes.Contains(bundle.Markdown(), []byte("- limitation `"+limitation+"`")) {
				t.Fatalf("fallback missing from rendered output:\n%s\n%s", bundle.JSON(), bundle.Markdown())
			}
		})
	}
}

func TestAttachModelAssistanceRejectsIllegalOrNonUserIDs(t *testing.T) {
	base := assistanceBaseBundle(t, 2)
	result := base.Result()
	userID := result.UserItems[0].ID
	assistantID := result.AssistantContext[0].ID
	digest := assistanceOpaqueID(920)
	tests := []struct {
		name  string
		input ModelAssistanceInput
	}{
		{name: "nil completed ranking", input: ModelAssistanceInput{ProviderConfigDigest: digest}},
		{name: "empty completed ranking", input: ModelAssistanceInput{ProviderConfigDigest: digest, RankedItemIDs: []string{}}},
		{name: "malformed provider digest", input: ModelAssistanceInput{ProviderConfigDigest: "digest", RankedItemIDs: []string{userID}}},
		{name: "uppercase provider digest", input: ModelAssistanceInput{ProviderConfigDigest: "sha256:" + strings.Repeat("A", 64), RankedItemIDs: []string{userID}}},
		{name: "foreign ID", input: ModelAssistanceInput{ProviderConfigDigest: digest, RankedItemIDs: []string{assistanceOpaqueID(999)}}},
		{name: "assistant ID", input: ModelAssistanceInput{ProviderConfigDigest: digest, RankedItemIDs: []string{assistantID}}},
		{name: "duplicate user ID", input: ModelAssistanceInput{ProviderConfigDigest: digest, RankedItemIDs: []string{userID, userID}}},
		{name: "unknown limitation", input: ModelAssistanceInput{ProviderConfigDigest: digest, LimitationCode: "MODEL_OTHER", RankedItemIDs: []string{}}},
		{name: "fallback carrying an ID", input: ModelAssistanceInput{ProviderConfigDigest: digest, LimitationCode: "MODEL_TIMEOUT", RankedItemIDs: []string{userID}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AttachModelAssistance(base, test.input); CodeOf(err) != CodeInvalidArgument {
				t.Fatalf("error=%v code=%s", err, CodeOf(err))
			}
		})
	}

	largeBase := assistanceBaseBundle(t, maxModelRankedItems+1)
	largeResult := largeBase.Result()
	tooMany := make([]string, len(largeResult.UserItems))
	for index := range largeResult.UserItems {
		tooMany[index] = largeResult.UserItems[index].ID
	}
	if _, err := AttachModelAssistance(largeBase, ModelAssistanceInput{
		ProviderConfigDigest: digest,
		RankedItemIDs:        tooMany,
	}); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("over-limit ranking error=%v code=%s", err, CodeOf(err))
	}
}

func TestAttachModelAssistanceRejectsUnsealedOrForgedBundle(t *testing.T) {
	base := assistanceBaseBundle(t, 1)
	input := ModelAssistanceInput{
		ProviderConfigDigest: assistanceOpaqueID(930),
		RankedItemIDs:        []string{base.Result().UserItems[0].ID},
	}

	forgedJSON := base
	forgedJSON.json = base.JSON()
	forgedJSON.json[len(forgedJSON.json)-2] ^= 1
	forgedResult := base
	forgedResult.result = cloneDistillation(base.result)
	forgedResult.result.UserItems[0].Statement = "forged"
	forgedProof := base
	forgedProof.proof[0] ^= 1
	tests := []struct {
		name   string
		bundle Bundle
	}{
		{name: "zero value", bundle: Bundle{}},
		{name: "forged JSON", bundle: forgedJSON},
		{name: "forged result", bundle: forgedResult},
		{name: "forged proof", bundle: forgedProof},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AttachModelAssistance(test.bundle, input); CodeOf(err) != CodeInvalidArgument {
				t.Fatalf("error=%v code=%s", err, CodeOf(err))
			}
		})
	}
	if !validBundle(base) {
		t.Fatal("forgery checks mutated the original bundle")
	}
}

func assistanceBaseBundle(t *testing.T, userItemCount int) Bundle {
	t.Helper()
	turns := make([]VisibleTurn, 0, userItemCount+1)
	for index := 0; index < userItemCount; index++ {
		turns = append(turns, VisibleTurn{
			EventID: assistanceOpaqueID(index + 1),
			Ordinal: uint32(index + 1),
			Role:    RoleUser,
			Text:    fmt.Sprintf("我的想法是候选方案 %d。", index+1),
		})
	}
	turns = append(turns, VisibleTurn{
		EventID: assistanceOpaqueID(userItemCount + 100),
		Ordinal: uint32(userItemCount + 1),
		Role:    RoleAssistant,
		Text:    "我的想法是助手建议。",
	})
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     assistanceOpaqueID(800),
		Turns:         turns,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.Result()
	if len(result.UserItems) != userItemCount || len(result.AssistantContext) != 1 || !validBundle(bundle) {
		t.Fatalf("unexpected base result: %+v", result)
	}
	return bundle
}

func assistanceOpaqueID(value int) string {
	return fmt.Sprintf("sha256:%064x", value)
}
