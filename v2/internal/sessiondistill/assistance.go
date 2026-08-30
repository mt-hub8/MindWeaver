package sessiondistill

import (
	"fmt"
)

const maxModelRankedItems = 12

// AttachModelAssistance adds a non-authoritative ordering of already verified
// user item IDs. It cannot modify base items, sources, attribution, or
// relations, and it always re-renders and reseals the opaque bundle.
func AttachModelAssistance(bundle Bundle, input ModelAssistanceInput) (Bundle, error) {
	if !validBundle(bundle) || bundle.result.ModelAssistance != nil {
		return Bundle{}, newError(CodeInvalidArgument, fmt.Errorf("invalid base bundle"))
	}
	status := "completed"
	if input.LimitationCode != "" {
		status = "fallback"
	}
	assistance := &ModelAssistance{
		SchemaVersion:        1,
		Provider:             "ollama",
		Purpose:              "rank_existing_user_candidates",
		Status:               status,
		ProviderConfigDigest: input.ProviderConfigDigest,
		InputDigest:          bundle.result.InputDigest,
		LimitationCode:       input.LimitationCode,
		RankedItemIDs:        append([]string{}, input.RankedItemIDs...),
	}
	result := cloneDistillation(bundle.result)
	result.ModelAssistance = assistance
	if !validModelAssistance(result) {
		return Bundle{}, newError(CodeInvalidArgument, fmt.Errorf("invalid model assistance"))
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

func validModelAssistance(result Distillation) bool {
	assistance := result.ModelAssistance
	if assistance == nil {
		return true
	}
	if assistance.SchemaVersion != 1 || assistance.Provider != "ollama" ||
		assistance.Purpose != "rank_existing_user_candidates" ||
		!validOpaqueID(assistance.ProviderConfigDigest) || assistance.InputDigest != result.InputDigest ||
		assistance.RankedItemIDs == nil || len(assistance.RankedItemIDs) > maxModelRankedItems {
		return false
	}
	if assistance.Status == "completed" {
		if assistance.LimitationCode != "" || len(assistance.RankedItemIDs) == 0 {
			return false
		}
	} else if assistance.Status == "fallback" {
		if len(assistance.RankedItemIDs) != 0 || !validModelLimitation(assistance.LimitationCode) {
			return false
		}
	} else {
		return false
	}
	userIDs := make(map[string]struct{}, len(result.UserItems))
	for _, item := range result.UserItems {
		userIDs[item.ID] = struct{}{}
	}
	selected := make(map[string]struct{}, len(assistance.RankedItemIDs))
	for _, itemID := range assistance.RankedItemIDs {
		if _, exists := userIDs[itemID]; !exists {
			return false
		}
		if _, duplicate := selected[itemID]; duplicate {
			return false
		}
		selected[itemID] = struct{}{}
	}
	return true
}

func validModelLimitation(code string) bool {
	switch code {
	case "NO_USER_CANDIDATES", "MODEL_INPUT_LIMIT", "MODEL_TIMEOUT", "MODEL_UNAVAILABLE", "MODEL_RESPONSE_INVALID", "OUTCOME_UNCERTAIN":
		return true
	default:
		return false
	}
}
