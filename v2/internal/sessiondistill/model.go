package sessiondistill

import "context"

const (
	SchemaVersion = 1
	PolicyVersion = "sessiondistill/rules/v1"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Kind string

const (
	KindIdea       Kind = "idea"
	KindGoal       Kind = "goal"
	KindConstraint Kind = "constraint"
	KindQuestion   Kind = "question"
	KindAssumption Kind = "assumption"
	KindRationale  Kind = "rationale"
	KindEvidence   Kind = "evidence"
	KindDecision   Kind = "decision"
	KindLearning   Kind = "learning"
	KindConcern    Kind = "concern"
	KindOpenItem   Kind = "open_item"
	KindNextAction Kind = "next_action"
)

type RelationKind string

const (
	RelationSupports      RelationKind = "supports"
	RelationConstrains    RelationKind = "constrains"
	RelationDependsOn     RelationKind = "depends_on"
	RelationAddresses     RelationKind = "addresses"
	RelationLeadsTo       RelationKind = "leads_to"
	RelationContrastsWith RelationKind = "contrasts_with"
	RelationEvolvesFrom   RelationKind = "evolves_from"
	RelationRespondsTo    RelationKind = "responds_to"
)

type Request struct {
	SchemaVersion int           `json:"schema_version"`
	SessionID     string        `json:"session_id"`
	Turns         []VisibleTurn `json:"turns"`
}

type InputTurn struct {
	Role Role
	Text string
}

type VisibleTurn struct {
	EventID string `json:"event_id"`
	Ordinal uint32 `json:"ordinal"`
	Role    Role   `json:"role"`
	Text    string `json:"text"`
}

type Source struct {
	EventID string `json:"event_id"`
	Ordinal uint32 `json:"ordinal"`
	Role    Role   `json:"role"`
	Basis   string `json:"basis"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Hash    string `json:"hash"`
}

type Item struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Attribution Role     `json:"attribution"`
	Statement   string   `json:"statement"`
	RuleID      string   `json:"rule_id"`
	Sources     []Source `json:"sources"`
}

type Relation struct {
	ID         string       `json:"id"`
	Kind       RelationKind `json:"kind"`
	FromID     string       `json:"from_id"`
	ToID       string       `json:"to_id"`
	RuleID     string       `json:"rule_id"`
	Derivation string       `json:"derivation"`
}

type Distillation struct {
	SchemaVersion           int              `json:"schema_version"`
	PolicyVersion           string           `json:"policy_version"`
	SessionID               string           `json:"session_id"`
	InputDigest             string           `json:"input_digest"`
	UntrustedVisibleContent bool             `json:"untrusted_visible_content"`
	RedactedCount           int              `json:"redacted_count"`
	UserItems               []Item           `json:"user_items"`
	AssistantContext        []Item           `json:"assistant_context"`
	Relations               []Relation       `json:"relations"`
	ModelAssistance         *ModelAssistance `json:"model_assistance,omitempty"`
}

type ModelAssistance struct {
	SchemaVersion        int      `json:"schema_version"`
	Provider             string   `json:"provider"`
	Purpose              string   `json:"purpose"`
	Status               string   `json:"status"`
	ProviderConfigDigest string   `json:"provider_config_digest"`
	InputDigest          string   `json:"input_digest"`
	LimitationCode       string   `json:"limitation_code,omitempty"`
	RankedItemIDs        []string `json:"ranked_item_ids"`
}

type ModelAssistanceInput struct {
	ProviderConfigDigest string
	LimitationCode       string
	RankedItemIDs        []string
}

type Bundle struct {
	result   Distillation
	json     []byte
	markdown []byte
	proof    [32]byte
}

type Core interface {
	Distill(context.Context, Request) (Bundle, error)
}

func NewV1() Core {
	return &coreV1{}
}
