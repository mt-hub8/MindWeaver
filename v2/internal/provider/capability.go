package provider

import (
	"errors"
	"fmt"
	"strings"
)

// CapabilitySchemaVersion identifies the wire shape of Descriptor.
const CapabilitySchemaVersion = "mindweaver.provider-capabilities/v1"

// Capability is a provider feature that may be advertised by a model.
type Capability string

const (
	CapabilityChat      Capability = "chat"
	CapabilityEmbedding Capability = "embedding"
	CapabilityStream    Capability = "stream"
	CapabilityToolUse   Capability = "tool_use"
	CapabilityJSONMode  Capability = "json_mode"
	CapabilityUsage     Capability = "usage"
	CapabilityCancel    Capability = "cancel"
)

var allCapabilities = [...]Capability{
	CapabilityChat,
	CapabilityEmbedding,
	CapabilityStream,
	CapabilityToolUse,
	CapabilityJSONMode,
	CapabilityUsage,
	CapabilityCancel,
}

// SupportLevel distinguishes native support from a local compatibility layer.
// Unsupported is explicit; an empty value is invalid rather than silently
// interpreted as support.
type SupportLevel string

const (
	SupportUnsupported SupportLevel = "unsupported"
	SupportNative      SupportLevel = "native"
	SupportEmulated    SupportLevel = "emulated"
)

// CapabilitySet is intentionally a fixed, versioned structure so a missing
// field in a future producer cannot accidentally grant a capability.
type CapabilitySet struct {
	Chat      SupportLevel `json:"chat"`
	Embedding SupportLevel `json:"embedding"`
	Stream    SupportLevel `json:"stream"`
	ToolUse   SupportLevel `json:"tool_use"`
	JSONMode  SupportLevel `json:"json_mode"`
	Usage     SupportLevel `json:"usage"`
	Cancel    SupportLevel `json:"cancel"`
}

// Level returns the advertised support level. Unknown capabilities are denied.
func (s CapabilitySet) Level(capability Capability) SupportLevel {
	switch capability {
	case CapabilityChat:
		return s.Chat
	case CapabilityEmbedding:
		return s.Embedding
	case CapabilityStream:
		return s.Stream
	case CapabilityToolUse:
		return s.ToolUse
	case CapabilityJSONMode:
		return s.JSONMode
	case CapabilityUsage:
		return s.Usage
	case CapabilityCancel:
		return s.Cancel
	default:
		return SupportUnsupported
	}
}

// Supports reports whether a capability is native or explicitly emulated.
func (s CapabilitySet) Supports(capability Capability) bool {
	level := s.Level(capability)
	return level == SupportNative || level == SupportEmulated
}

// ModelConstraints contains only non-sensitive, enforceable request bounds.
// Zero does not mean unlimited: Validate rejects a missing bound whenever the
// corresponding capability needs it.
type ModelConstraints struct {
	MaxRequestBytes            uint64   `json:"max_request_bytes"`
	MaxInputTokens             uint64   `json:"max_input_tokens"`
	MaxOutputTokens            uint64   `json:"max_output_tokens,omitempty"`
	MaxBatchItems              uint32   `json:"max_batch_items,omitempty"`
	MaxToolDefinitions         uint32   `json:"max_tool_definitions,omitempty"`
	AllowedEmbeddingDimensions []uint32 `json:"allowed_embedding_dimensions,omitempty"`
}

// ModelDescriptor describes one model addressable through a provider.
type ModelDescriptor struct {
	ID           string           `json:"id"`
	Capabilities CapabilitySet    `json:"capabilities"`
	Constraints  ModelConstraints `json:"constraints"`
}

// Descriptor is the versioned provider capability document. It deliberately
// has no credential, header, prompt, or endpoint fields.
type Descriptor struct {
	SchemaVersion string            `json:"schema_version"`
	ProviderID    string            `json:"provider_id"`
	Models        []ModelDescriptor `json:"models"`
}

// RequestProfile is a content-free summary used to check a request before any
// provider call. Counts may be conservative estimates, but must never exceed
// the advertised model bounds.
type RequestProfile struct {
	Capability         Capability `json:"capability"`
	RequestBytes       uint64     `json:"request_bytes"`
	InputTokens        uint64     `json:"input_tokens"`
	MaxOutputTokens    uint64     `json:"max_output_tokens,omitempty"`
	BatchItems         uint32     `json:"batch_items,omitempty"`
	ToolDefinitions    uint32     `json:"tool_definitions,omitempty"`
	EmbeddingDimension uint32     `json:"embedding_dimension,omitempty"`
	Stream             bool       `json:"stream"`
	JSONMode           bool       `json:"json_mode"`
	IncludeUsage       bool       `json:"include_usage"`
	Cancellable        bool       `json:"cancellable"`
}

// ConstraintError is safe to return from an API: it names only a schema field
// and a stable code, never request content or a provider response.
type ConstraintError struct {
	Field string
	Code  string
}

func (e *ConstraintError) Error() string {
	return "provider constraint " + e.Field + ": " + e.Code
}

// Validate checks the complete descriptor and all cross-capability invariants.
func (d Descriptor) Validate() error {
	if d.SchemaVersion != CapabilitySchemaVersion {
		return &ConstraintError{Field: "schema_version", Code: "unsupported_version"}
	}
	if err := validateIdentifier(d.ProviderID, 128); err != nil {
		return &ConstraintError{Field: "provider_id", Code: "invalid_identifier"}
	}
	if len(d.Models) == 0 {
		return &ConstraintError{Field: "models", Code: "required"}
	}

	seen := make(map[string]struct{}, len(d.Models))
	for index := range d.Models {
		model := d.Models[index]
		field := fmt.Sprintf("models[%d]", index)
		if err := validateIdentifier(model.ID, 256); err != nil {
			return &ConstraintError{Field: field + ".id", Code: "invalid_identifier"}
		}
		if _, duplicate := seen[model.ID]; duplicate {
			return &ConstraintError{Field: field + ".id", Code: "duplicate"}
		}
		seen[model.ID] = struct{}{}

		if err := validateCapabilitySet(model.Capabilities); err != nil {
			return &ConstraintError{Field: field + ".capabilities", Code: err.Error()}
		}
		if err := validateModelConstraints(model); err != nil {
			return &ConstraintError{Field: field + ".constraints", Code: err.Error()}
		}
	}
	return nil
}

// ValidateRequest finds modelID and fail-closes against its declared limits.
func (d Descriptor) ValidateRequest(modelID string, request RequestProfile) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if err := validateIdentifier(modelID, 256); err != nil {
		return &ConstraintError{Field: "model_id", Code: "invalid_identifier"}
	}

	var model *ModelDescriptor
	for index := range d.Models {
		if d.Models[index].ID == modelID {
			model = &d.Models[index]
			break
		}
	}
	if model == nil {
		return &ConstraintError{Field: "model_id", Code: "not_advertised"}
	}
	if request.Capability != CapabilityChat && request.Capability != CapabilityEmbedding {
		return &ConstraintError{Field: "capability", Code: "invalid_primary_capability"}
	}
	if !model.Capabilities.Supports(request.Capability) {
		return &ConstraintError{Field: "capability", Code: "unsupported"}
	}
	if request.RequestBytes == 0 {
		return &ConstraintError{Field: "request_bytes", Code: "required"}
	}
	if request.RequestBytes > model.Constraints.MaxRequestBytes {
		return &ConstraintError{Field: "request_bytes", Code: "limit_exceeded"}
	}
	if request.InputTokens > model.Constraints.MaxInputTokens {
		return &ConstraintError{Field: "input_tokens", Code: "limit_exceeded"}
	}

	if request.Capability == CapabilityChat {
		if request.MaxOutputTokens == 0 {
			return &ConstraintError{Field: "max_output_tokens", Code: "required"}
		}
		if request.MaxOutputTokens > model.Constraints.MaxOutputTokens {
			return &ConstraintError{Field: "max_output_tokens", Code: "limit_exceeded"}
		}
		if request.BatchItems > 1 {
			return &ConstraintError{Field: "batch_items", Code: "not_applicable"}
		}
		if request.EmbeddingDimension != 0 {
			return &ConstraintError{Field: "embedding_dimension", Code: "not_applicable"}
		}
	} else {
		if request.MaxOutputTokens != 0 || request.ToolDefinitions != 0 || request.Stream || request.JSONMode {
			return &ConstraintError{Field: "capability", Code: "chat_options_on_embedding_request"}
		}
		if request.BatchItems == 0 {
			return &ConstraintError{Field: "batch_items", Code: "required"}
		}
		if request.BatchItems > model.Constraints.MaxBatchItems {
			return &ConstraintError{Field: "batch_items", Code: "limit_exceeded"}
		}
		if request.EmbeddingDimension != 0 && !containsDimension(model.Constraints.AllowedEmbeddingDimensions, request.EmbeddingDimension) {
			return &ConstraintError{Field: "embedding_dimension", Code: "unsupported"}
		}
	}

	if request.Stream && !model.Capabilities.Supports(CapabilityStream) {
		return &ConstraintError{Field: "stream", Code: "unsupported"}
	}
	if request.ToolDefinitions > 0 {
		if !model.Capabilities.Supports(CapabilityToolUse) {
			return &ConstraintError{Field: "tool_definitions", Code: "unsupported"}
		}
		if request.ToolDefinitions > model.Constraints.MaxToolDefinitions {
			return &ConstraintError{Field: "tool_definitions", Code: "limit_exceeded"}
		}
	}
	if request.JSONMode && !model.Capabilities.Supports(CapabilityJSONMode) {
		return &ConstraintError{Field: "json_mode", Code: "unsupported"}
	}
	if request.IncludeUsage && !model.Capabilities.Supports(CapabilityUsage) {
		return &ConstraintError{Field: "include_usage", Code: "unsupported"}
	}
	if request.Cancellable && !model.Capabilities.Supports(CapabilityCancel) {
		return &ConstraintError{Field: "cancellable", Code: "unsupported"}
	}
	return nil
}

func validateCapabilitySet(set CapabilitySet) error {
	for _, capability := range allCapabilities {
		level := set.Level(capability)
		if level != SupportUnsupported && level != SupportNative && level != SupportEmulated {
			return errors.New("invalid_support_level")
		}
	}
	if !set.Supports(CapabilityChat) && !set.Supports(CapabilityEmbedding) {
		return errors.New("no_primary_capability")
	}
	if !set.Supports(CapabilityChat) && (set.Supports(CapabilityStream) || set.Supports(CapabilityToolUse) || set.Supports(CapabilityJSONMode)) {
		return errors.New("chat_modifier_without_chat")
	}
	return nil
}

func validateModelConstraints(model ModelDescriptor) error {
	limits := model.Constraints
	if limits.MaxRequestBytes == 0 {
		return errors.New("max_request_bytes_required")
	}
	if limits.MaxInputTokens == 0 {
		return errors.New("max_input_tokens_required")
	}
	if model.Capabilities.Supports(CapabilityChat) && limits.MaxOutputTokens == 0 {
		return errors.New("max_output_tokens_required")
	}
	if !model.Capabilities.Supports(CapabilityChat) && limits.MaxOutputTokens != 0 {
		return errors.New("max_output_tokens_not_applicable")
	}
	if model.Capabilities.Supports(CapabilityToolUse) && limits.MaxToolDefinitions == 0 {
		return errors.New("max_tool_definitions_required")
	}
	if !model.Capabilities.Supports(CapabilityToolUse) && limits.MaxToolDefinitions != 0 {
		return errors.New("max_tool_definitions_not_applicable")
	}

	if model.Capabilities.Supports(CapabilityEmbedding) {
		if limits.MaxBatchItems == 0 {
			return errors.New("max_batch_items_required")
		}
		if len(limits.AllowedEmbeddingDimensions) == 0 {
			return errors.New("embedding_dimensions_required")
		}
		seen := make(map[uint32]struct{}, len(limits.AllowedEmbeddingDimensions))
		for _, dimension := range limits.AllowedEmbeddingDimensions {
			if dimension == 0 || dimension > 1<<20 {
				return errors.New("invalid_embedding_dimension")
			}
			if _, duplicate := seen[dimension]; duplicate {
				return errors.New("duplicate_embedding_dimension")
			}
			seen[dimension] = struct{}{}
		}
	} else if limits.MaxBatchItems != 0 || len(limits.AllowedEmbeddingDimensions) != 0 {
		return errors.New("embedding_constraints_not_applicable")
	}
	return nil
}

func containsDimension(dimensions []uint32, requested uint32) bool {
	for _, dimension := range dimensions {
		if dimension == requested {
			return true
		}
	}
	return false
}

func validateIdentifier(value string, maxBytes int) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxBytes {
		return errors.New("invalid identifier")
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return errors.New("invalid identifier")
		}
	}
	return nil
}
