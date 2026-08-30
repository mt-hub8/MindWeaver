package sessiondistill

import (
	"fmt"
	"reflect"
	"unicode/utf8"
)

func verifyResult(result Distillation, turns []normalizedTurn, inputSessionID string) error {
	expectedSessionID := publicSessionID(inputSessionID, turns)
	if result.SchemaVersion != SchemaVersion || result.PolicyVersion != PolicyVersion ||
		result.SessionID != expectedSessionID || result.InputDigest != digestInput(expectedSessionID, turns) ||
		!result.UntrustedVisibleContent || result.RedactedCount < 0 {
		return newError(CodeOutputFailed, fmt.Errorf("result metadata"))
	}
	turnByEvent := make(map[string]normalizedTurn, len(turns))
	redactedCount := 0
	for index, turn := range turns {
		if !validOpaqueID(turn.inputEventID) || turn.ordinal != uint32(index+1) ||
			turn.role != RoleUser && turn.role != RoleAssistant || !utf8.ValidString(turn.text) || len(turn.text) == 0 || len(turn.text) > maxTurnBytes || hasForbiddenControl(turn.text) {
			return newError(CodeOutputFailed, fmt.Errorf("result event shape"))
		}
		wantEventID := digest(
			"mindweaver/sessiondistill/public-event/v1",
			[]byte(turn.inputEventID),
			[]byte(fmt.Sprintf("%d", turn.ordinal)),
			[]byte(turn.role),
			[]byte(turn.text),
		)
		if turn.eventID != wantEventID || !validRedactions(turn) {
			return newError(CodeOutputFailed, fmt.Errorf("result event identity"))
		}
		if _, exists := turnByEvent[turn.eventID]; exists {
			return newError(CodeOutputFailed, fmt.Errorf("duplicate result event"))
		}
		turnByEvent[turn.eventID] = turn
		redactedCount += len(turn.redaction)
	}
	if result.RedactedCount != redactedCount {
		return newError(CodeOutputFailed, fmt.Errorf("result redaction count"))
	}
	items := make(map[string]Item, len(result.UserItems)+len(result.AssistantContext))
	verifyItems := func(entries []Item, wantRole Role) error {
		for _, item := range entries {
			classifiedKind, classifiedRule, classified := classify(item.Statement)
			if !validKind(item.Kind) || item.Attribution != wantRole || !utf8.ValidString(item.Statement) || item.Statement == "" || len(item.Statement) > maxSliceBytes ||
				len(item.Sources) == 0 || !validItemRule(item.Kind, item.RuleID) || !classified || classifiedKind != item.Kind || classifiedRule != item.RuleID {
				return fmt.Errorf("result item shape")
			}
			key := string(item.Kind) + "\x00" + string(item.Attribution) + "\x00" + item.Statement
			if item.ID != digest("mindweaver/sessiondistill/item/v1", []byte(result.SessionID), []byte(key)) {
				return fmt.Errorf("result item identity")
			}
			if _, exists := items[item.ID]; exists {
				return fmt.Errorf("duplicate result item")
			}
			previousOrdinal, previousStart := uint32(0), -1
			for _, source := range item.Sources {
				turn, exists := turnByEvent[source.EventID]
				if !exists || source.Ordinal != turn.ordinal || source.Role != turn.role || source.Role != wantRole || source.Basis != "canonical_redacted_utf8_v1" ||
					source.Start < 0 || source.End <= source.Start || source.End > len(turn.text) || !utf8.RuneStart(turn.text[source.Start]) || source.End < len(turn.text) && !utf8.RuneStart(turn.text[source.End]) ||
					turn.text[source.Start:source.End] != item.Statement || intersectsRedaction(turn.redaction, source.Start, source.End) {
					return fmt.Errorf("result source")
				}
				wantHash := digest(
					"mindweaver/sessiondistill/span/v1",
					[]byte(turn.eventID),
					[]byte(fmt.Sprintf("%d", source.Start)),
					[]byte(fmt.Sprintf("%d", source.End)),
					[]byte(item.Statement),
				)
				if source.Hash != wantHash || source.Ordinal < previousOrdinal || source.Ordinal == previousOrdinal && source.Start <= previousStart {
					return fmt.Errorf("result source identity")
				}
				previousOrdinal, previousStart = source.Ordinal, source.Start
			}
			items[item.ID] = item
		}
		return nil
	}
	if err := verifyItems(result.UserItems, RoleUser); err != nil {
		return newError(CodeOutputFailed, err)
	}
	if err := verifyItems(result.AssistantContext, RoleAssistant); err != nil {
		return newError(CodeOutputFailed, err)
	}
	seenRelations := make(map[string]struct{}, len(result.Relations))
	previousRelationKey := ""
	for _, relation := range result.Relations {
		from, fromExists := items[relation.FromID]
		to, toExists := items[relation.ToID]
		key := string(relation.Kind) + "\x00" + relation.FromID + "\x00" + relation.ToID
		if !fromExists || !toExists || relation.FromID == relation.ToID ||
			relation.ID != digest("mindweaver/sessiondistill/relation/v1", []byte(key)) || !validRelationEndpoints(relation.Kind, from, to) {
			return newError(CodeOutputFailed, fmt.Errorf("result relation"))
		}
		wantDerivation := "rule_derived_candidate"
		if relation.Kind == RelationRespondsTo {
			wantDerivation = "structural"
		}
		if relation.Derivation != wantDerivation {
			return newError(CodeOutputFailed, fmt.Errorf("result relation derivation"))
		}
		if _, exists := seenRelations[key]; exists {
			return newError(CodeOutputFailed, fmt.Errorf("duplicate result relation"))
		}
		sortKey := relation.FromID + "\x00" + relation.ToID + "\x00" + string(relation.Kind)
		if previousRelationKey != "" && sortKey <= previousRelationKey {
			return newError(CodeOutputFailed, fmt.Errorf("result relation order"))
		}
		previousRelationKey = sortKey
		seenRelations[key] = struct{}{}
	}
	expectedUser, expectedAssistant, expectedRelations, err := rebuildExpectedContent(turns, result.SessionID)
	if err != nil || !reflect.DeepEqual(result.UserItems, expectedUser) || !reflect.DeepEqual(result.AssistantContext, expectedAssistant) || !reflect.DeepEqual(result.Relations, expectedRelations) {
		return newError(CodeOutputFailed, fmt.Errorf("result reconstruction"))
	}
	return nil
}

// rebuildExpectedContent reconstructs the complete visible output from the
// normalized, redacted turns. Empty classified entries deliberately preserve
// unclassified and redacted gaps, so exact relation reconstruction cannot
// accept a relation forged across an ineligible slice.
func rebuildExpectedContent(turns []normalizedTurn, sessionID string) ([]Item, []Item, []Relation, error) {
	slices, err := sliceTurns(turns)
	if err != nil {
		return nil, nil, nil, err
	}
	userItems := make([]Item, 0)
	assistantItems := make([]Item, 0)
	classified := make([]classifiedSlice, 0, len(slices))
	userIndex := make(map[string]int)
	assistantIndex := make(map[string]int)
	for _, current := range slices {
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
		items, indexes := &userItems, userIndex
		if turn.role == RoleAssistant {
			items, indexes = &assistantItems, assistantIndex
		}
		if position, exists := indexes[key]; exists {
			(*items)[position].Sources = append((*items)[position].Sources, source)
			entry.itemID = (*items)[position].ID
			entry.itemKind = (*items)[position].Kind
			entry.statement = (*items)[position].Statement
		} else {
			item := Item{
				ID:          digest("mindweaver/sessiondistill/item/v1", []byte(sessionID), []byte(key)),
				Kind:        kind,
				Attribution: turn.role,
				Statement:   current.text,
				RuleID:      ruleID,
				Sources:     []Source{source},
			}
			*items = append(*items, item)
			indexes[key] = len(*items) - 1
			entry.itemID = item.ID
			entry.itemKind = item.Kind
			entry.statement = item.Statement
		}
		classified = append(classified, entry)
	}
	return userItems, assistantItems, buildRelations(turns, classified), nil
}

func validRedactions(turn normalizedTurn) bool {
	if len(turn.redaction) > maxRedactionSpans {
		return false
	}
	previousEnd := -1
	for _, span := range turn.redaction {
		if span.start < 0 || span.end <= span.start || span.end > len(turn.text) || span.start < previousEnd ||
			!utf8.RuneStart(turn.text[span.start]) || span.end < len(turn.text) && !utf8.RuneStart(turn.text[span.end]) {
			return false
		}
		placeholder := turn.text[span.start:span.end]
		if span.class == "pre_redacted" {
			if !preRedacted.MatchString(placeholder) || preRedacted.FindString(placeholder) != placeholder {
				return false
			}
		} else if placeholder != "[redacted:"+span.class+"]" {
			return false
		}
		previousEnd = span.end
	}
	return true
}

func validItemRule(kind Kind, ruleID string) bool {
	switch kind {
	case KindIdea:
		return ruleID == "idea-explicit-v1" || ruleID == "idea-evolution-explicit-v1"
	case KindGoal:
		return ruleID == "goal-explicit-v1"
	case KindConstraint:
		return ruleID == "constraint-explicit-v1"
	case KindQuestion:
		return ruleID == "question-explicit-v1"
	case KindAssumption:
		return ruleID == "assumption-explicit-v1"
	case KindRationale:
		return ruleID == "rationale-explicit-v1"
	case KindEvidence:
		return ruleID == "evidence-explicit-v1"
	case KindDecision:
		return ruleID == "decision-explicit-v1"
	case KindLearning:
		return ruleID == "learning-explicit-v1"
	case KindConcern:
		return ruleID == "concern-explicit-v1"
	case KindOpenItem:
		return ruleID == "open-item-explicit-v1"
	case KindNextAction:
		return ruleID == "next-action-explicit-v1"
	default:
		return false
	}
}

func validKind(kind Kind) bool {
	switch kind {
	case KindIdea, KindGoal, KindConstraint, KindQuestion, KindAssumption, KindRationale,
		KindEvidence, KindDecision, KindLearning, KindConcern, KindOpenItem, KindNextAction:
		return true
	default:
		return false
	}
}

func validRelationEndpoints(kind RelationKind, from, to Item) bool {
	switch kind {
	case RelationSupports:
		return oneOfKind(from.Kind, KindRationale, KindEvidence, KindLearning) &&
			oneOfKind(to.Kind, KindIdea, KindGoal, KindDecision, KindConstraint, KindConcern, KindOpenItem, KindNextAction)
	case RelationLeadsTo:
		return oneOfKind(from.Kind, KindIdea, KindRationale, KindEvidence, KindDecision, KindGoal, KindLearning) &&
			oneOfKind(to.Kind, KindNextAction, KindDecision, KindGoal, KindIdea, KindLearning, KindOpenItem)
	case RelationDependsOn:
		return oneOfKind(from.Kind, KindAssumption, KindConstraint, KindOpenItem, KindNextAction) &&
			oneOfKind(to.Kind, KindGoal, KindDecision, KindIdea, KindNextAction, KindOpenItem)
	case RelationAddresses:
		return oneOfKind(from.Kind, KindIdea, KindDecision, KindNextAction, KindLearning) &&
			oneOfKind(to.Kind, KindQuestion, KindConcern, KindOpenItem)
	case RelationConstrains:
		return from.Kind == KindConstraint && oneOfKind(to.Kind, KindIdea, KindGoal, KindDecision, KindNextAction)
	case RelationContrastsWith:
		return oneOfKind(from.Kind, KindIdea, KindDecision, KindConstraint, KindConcern, KindRationale) &&
			oneOfKind(to.Kind, KindIdea, KindDecision, KindConstraint, KindConcern, KindRationale)
	case RelationEvolvesFrom:
		return from.Attribution == RoleUser && to.Attribution == RoleUser && from.Kind == KindIdea &&
			oneOfKind(to.Kind, KindIdea, KindDecision, KindAssumption)
	case RelationRespondsTo:
		return from.Attribution == RoleAssistant && to.Attribution == RoleUser && to.Kind == KindQuestion
	default:
		return false
	}
}

func oneOfKind(value Kind, allowed ...Kind) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
