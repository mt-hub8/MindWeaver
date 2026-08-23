package provider

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDescriptorValidateAndAdvertiseEveryV1Capability(t *testing.T) {
	descriptor := testDescriptor()
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	chat := descriptor.Models[0]
	wantLevels := map[Capability]SupportLevel{
		CapabilityChat:      SupportNative,
		CapabilityEmbedding: SupportUnsupported,
		CapabilityStream:    SupportNative,
		CapabilityToolUse:   SupportEmulated,
		CapabilityJSONMode:  SupportNative,
		CapabilityUsage:     SupportNative,
		CapabilityCancel:    SupportNative,
	}
	for capability, wanted := range wantLevels {
		if got := chat.Capabilities.Level(capability); got != wanted {
			t.Errorf("Level(%q) = %q, want %q", capability, got, wanted)
		}
	}

	encoded, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, field := range []string{"chat", "embedding", "stream", "tool_use", "json_mode", "usage", "cancel"} {
		if !strings.Contains(string(encoded), `"`+field+`"`) {
			t.Errorf("serialized descriptor does not advertise %q", field)
		}
	}
}

func TestDescriptorFailsClosedOnMissingCapabilityLevel(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Models[0].Capabilities.Cancel = ""
	err := descriptor.Validate()
	var constraint *ConstraintError
	if !errors.As(err, &constraint) || constraint.Code != "invalid_support_level" {
		t.Fatalf("Validate() error = %v, want invalid_support_level", err)
	}
}

func TestDescriptorRejectsInvalidCrossCapabilityAndConstraints(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Descriptor)
	}{
		{
			name: "chat modifier without chat",
			mutate: func(d *Descriptor) {
				d.Models[0].Capabilities.Chat = SupportUnsupported
			},
		},
		{
			name: "unbounded output",
			mutate: func(d *Descriptor) {
				d.Models[0].Constraints.MaxOutputTokens = 0
			},
		},
		{
			name: "embedding without dimensions",
			mutate: func(d *Descriptor) {
				d.Models[1].Constraints.AllowedEmbeddingDimensions = nil
			},
		},
		{
			name: "duplicate model",
			mutate: func(d *Descriptor) {
				d.Models[1].ID = d.Models[0].ID
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := testDescriptor()
			test.mutate(&descriptor)
			if err := descriptor.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestDescriptorValidateChatRequest(t *testing.T) {
	descriptor := testDescriptor()
	request := RequestProfile{
		Capability:      CapabilityChat,
		RequestBytes:    1024,
		InputTokens:     200,
		MaxOutputTokens: 512,
		ToolDefinitions: 3,
		Stream:          true,
		JSONMode:        true,
		IncludeUsage:    true,
		Cancellable:     true,
	}
	if err := descriptor.ValidateRequest("chat-model", request); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RequestProfile)
		field  string
	}{
		{"request bytes", func(r *RequestProfile) { r.RequestBytes = 2 << 20 }, "request_bytes"},
		{"input tokens", func(r *RequestProfile) { r.InputTokens = 5000 }, "input_tokens"},
		{"output tokens", func(r *RequestProfile) { r.MaxOutputTokens = 2000 }, "max_output_tokens"},
		{"tools", func(r *RequestProfile) { r.ToolDefinitions = 9 }, "tool_definitions"},
		{"embedding option", func(r *RequestProfile) { r.EmbeddingDimension = 768 }, "embedding_dimension"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := request
			test.mutate(&invalid)
			err := descriptor.ValidateRequest("chat-model", invalid)
			var constraint *ConstraintError
			if !errors.As(err, &constraint) || constraint.Field != test.field {
				t.Fatalf("ValidateRequest() error = %v, want field %s", err, test.field)
			}
		})
	}
}

func TestDescriptorValidateEmbeddingRequest(t *testing.T) {
	descriptor := testDescriptor()
	request := RequestProfile{
		Capability:         CapabilityEmbedding,
		RequestBytes:       2048,
		InputTokens:        512,
		BatchItems:         16,
		EmbeddingDimension: 768,
		IncludeUsage:       true,
		Cancellable:        true,
	}
	if err := descriptor.ValidateRequest("embedding-model", request); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}

	request.EmbeddingDimension = 1024
	if err := descriptor.ValidateRequest("embedding-model", request); err == nil {
		t.Fatal("unsupported embedding dimension accepted")
	}
	request.EmbeddingDimension = 768
	request.Stream = true
	if err := descriptor.ValidateRequest("embedding-model", request); err == nil {
		t.Fatal("chat option accepted for embedding request")
	}
}

func TestDescriptorRejectsUnknownModelAndCapability(t *testing.T) {
	descriptor := testDescriptor()
	request := RequestProfile{Capability: CapabilityChat, RequestBytes: 1, MaxOutputTokens: 1}
	if err := descriptor.ValidateRequest("not-advertised", request); err == nil {
		t.Fatal("unknown model accepted")
	}
	request.Capability = Capability("future_capability")
	if err := descriptor.ValidateRequest("chat-model", request); err == nil {
		t.Fatal("unknown capability accepted")
	}
}

func testDescriptor() Descriptor {
	return Descriptor{
		SchemaVersion: CapabilitySchemaVersion,
		ProviderID:    "offline-fake",
		Models: []ModelDescriptor{
			{
				ID: "chat-model",
				Capabilities: CapabilitySet{
					Chat:      SupportNative,
					Embedding: SupportUnsupported,
					Stream:    SupportNative,
					ToolUse:   SupportEmulated,
					JSONMode:  SupportNative,
					Usage:     SupportNative,
					Cancel:    SupportNative,
				},
				Constraints: ModelConstraints{
					MaxRequestBytes:    1 << 20,
					MaxInputTokens:     4096,
					MaxOutputTokens:    1024,
					MaxToolDefinitions: 8,
				},
			},
			{
				ID: "embedding-model",
				Capabilities: CapabilitySet{
					Chat:      SupportUnsupported,
					Embedding: SupportNative,
					Stream:    SupportUnsupported,
					ToolUse:   SupportUnsupported,
					JSONMode:  SupportUnsupported,
					Usage:     SupportNative,
					Cancel:    SupportNative,
				},
				Constraints: ModelConstraints{
					MaxRequestBytes:            1 << 20,
					MaxInputTokens:             8192,
					MaxBatchItems:              32,
					AllowedEmbeddingDimensions: []uint32{384, 768},
				},
			},
		},
	}
}
