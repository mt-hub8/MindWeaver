package provider

import (
	"net/netip"
	"testing"
)

func TestParseEndpointCanonicalizesCredentialFreeBaseURL(t *testing.T) {
	endpoint, err := ParseEndpoint("https://API.Example.COM/v1/")
	if err != nil {
		t.Fatalf("ParseEndpoint() error = %v", err)
	}
	if endpoint.Scheme != "https" || endpoint.Host != "api.example.com" || endpoint.Port != 443 || endpoint.PortExplicit || endpoint.BasePath != "/v1" {
		t.Fatalf("ParseEndpoint() = %#v", endpoint)
	}

	explicit, err := ParseEndpoint("https://api.example.com:8443/v1")
	if err != nil {
		t.Fatalf("ParseEndpoint(explicit port) error = %v", err)
	}
	if explicit.Port != 8443 || !explicit.PortExplicit {
		t.Fatalf("explicit port lost: %#v", explicit)
	}
}

func TestParseEndpointRejectsUnsafeForms(t *testing.T) {
	tests := map[string]string{
		"userinfo":       "https://api-key@example.com/v1",
		"password":       "https://user:secret@example.com/v1",
		"query":          "https://example.com/v1?api_key=secret",
		"empty query":    "https://example.com/v1?",
		"fragment":       "https://example.com/v1#secret",
		"ftp":            "ftp://example.com/v1",
		"relative":       "/v1",
		"encoded path":   "https://example.com/%2e%2e/admin",
		"path traversal": "https://example.com/v1/../admin",
		"invalid host":   "https://bad_host.example/v1",
		"zero port":      "https://example.com:0/v1",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if endpoint, err := ParseEndpoint(raw); err == nil {
				t.Fatalf("ParseEndpoint(%q) = %#v, want error", raw, endpoint)
			}
		})
	}
}

func TestDefaultEgressPolicyDeniesEverything(t *testing.T) {
	endpoint := mustEndpoint(t, "https://api.example.com/v1")
	decision := DefaultEgressPolicy().Decide(EgressRequest{
		ProviderID:        "remote",
		Capability:        CapabilityChat,
		Endpoint:          endpoint,
		ResolvedAddresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")},
	})
	if decision.Allowed || decision.Reason != EgressDefaultDeny {
		t.Fatalf("Decide() = %#v, want default deny", decision)
	}
	if decision.SchemaVersion != EgressPolicySchemaVersion {
		t.Fatalf("decision schema = %q", decision.SchemaVersion)
	}
}

func TestEgressPolicyAllowsExactPublicHTTPSAndPinsAddresses(t *testing.T) {
	policy := mustPolicy(t, EgressRule{
		ID:                    "public-chat",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"api.example.com"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{443},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	})
	address := netip.MustParseAddr("93.184.216.34")
	decision := policy.Decide(EgressRequest{
		ProviderID:        "remote",
		Capability:        CapabilityChat,
		Endpoint:          mustEndpoint(t, "https://api.example.com/v1"),
		ResolvedAddresses: []netip.Addr{address},
	})
	if !decision.Allowed || decision.Reason != EgressAllowed || decision.RuleID != "public-chat" {
		t.Fatalf("Decide() = %#v", decision)
	}
	if len(decision.ApprovedAddresses) != 1 || decision.ApprovedAddresses[0] != address {
		t.Fatalf("ApprovedAddresses = %v", decision.ApprovedAddresses)
	}
}

func TestEgressPolicyRequiresResolutionAndDeniesDNSRebinding(t *testing.T) {
	policy := mustPolicy(t, EgressRule{
		ID:                    "public-chat",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"api.example.com"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{443},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	})
	request := EgressRequest{
		ProviderID: "remote",
		Capability: CapabilityChat,
		Endpoint:   mustEndpoint(t, "https://api.example.com/v1"),
	}
	decision := policy.Decide(request)
	if decision.Allowed || decision.Reason != EgressResolutionRequired {
		t.Fatalf("without resolution Decide() = %#v", decision)
	}

	request.ResolvedAddresses = []netip.Addr{
		netip.MustParseAddr("93.184.216.34"),
		netip.MustParseAddr("127.0.0.1"),
	}
	decision = policy.Decide(request)
	if decision.Allowed || decision.Reason != EgressAddressDenied {
		t.Fatalf("mixed public/loopback Decide() = %#v", decision)
	}
}

func TestLoopbackAndPrivateRequireExplicitAddressClass(t *testing.T) {
	loopbackEndpoint := mustEndpoint(t, "http://127.0.0.1:11434")
	publicOnly := mustPolicy(t, EgressRule{
		ID:                    "local-wrong-class",
		ProviderID:            "ollama",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"localhost"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{11434},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	})
	decision := publicOnly.Decide(EgressRequest{
		ProviderID:        "ollama",
		Capability:        CapabilityChat,
		Endpoint:          mustEndpoint(t, "https://localhost:11434"),
		ResolvedAddresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
	})
	if decision.Allowed {
		t.Fatal("loopback allowed by public-only rule")
	}

	loopbackPolicy := mustPolicy(t, EgressRule{
		ID:                    "local-ollama",
		ProviderID:            "ollama",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"127.0.0.1"},
		Schemes:               []string{"http"},
		Ports:                 []uint16{11434},
		AllowedAddressClasses: []AddressClass{AddressLoopback},
	})
	decision = loopbackPolicy.Decide(EgressRequest{ProviderID: "ollama", Capability: CapabilityChat, Endpoint: loopbackEndpoint})
	if !decision.Allowed {
		t.Fatalf("explicit loopback rule denied: %#v", decision)
	}

	privatePolicy := mustPolicy(t, EgressRule{
		ID:                    "lan-worker",
		ProviderID:            "worker",
		Capabilities:          []Capability{CapabilityEmbedding},
		Hosts:                 []string{"10.1.2.3"},
		Schemes:               []string{"http"},
		Ports:                 []uint16{8001},
		AllowedAddressClasses: []AddressClass{AddressPrivate},
	})
	decision = privatePolicy.Decide(EgressRequest{
		ProviderID: "worker",
		Capability: CapabilityEmbedding,
		Endpoint:   mustEndpoint(t, "http://10.1.2.3:8001"),
	})
	if !decision.Allowed {
		t.Fatalf("explicit private rule denied: %#v", decision)
	}
}

func TestLinkLocalCannotBeGranted(t *testing.T) {
	_, err := NewEgressPolicy([]EgressRule{{
		ID:                    "metadata",
		ProviderID:            "bad",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"169.254.169.254"},
		Schemes:               []string{"http"},
		Ports:                 []uint16{80},
		AllowedAddressClasses: []AddressClass{AddressLinkLocal},
	}})
	if err == nil {
		t.Fatal("link-local grant accepted")
	}
}

func TestPublicPlaintextCannotBeGranted(t *testing.T) {
	_, err := NewEgressPolicy([]EgressRule{{
		ID:                    "plaintext-public",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"api.example.com"},
		Schemes:               []string{"http"},
		Ports:                 []uint16{80},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	}})
	if err == nil {
		t.Fatal("public plaintext grant accepted")
	}
}

func TestEgressPortMustBeExplicitlyAllowed(t *testing.T) {
	policy := mustPolicy(t, EgressRule{
		ID:                    "standard-port-only",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"api.example.com"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{443},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	})
	decision := policy.Decide(EgressRequest{
		ProviderID:        "remote",
		Capability:        CapabilityChat,
		Endpoint:          mustEndpoint(t, "https://api.example.com:8443/v1"),
		ResolvedAddresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")},
	})
	if decision.Allowed || decision.Reason != EgressNoMatchingRule {
		t.Fatalf("unexpected explicit port allowed: %#v", decision)
	}
}

func TestIPAddressResolutionMismatchIsDenied(t *testing.T) {
	policy := mustPolicy(t, EgressRule{
		ID:                    "fixed-ip",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"93.184.216.34"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{443},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	})
	decision := policy.Decide(EgressRequest{
		ProviderID:        "remote",
		Capability:        CapabilityChat,
		Endpoint:          mustEndpoint(t, "https://93.184.216.34/v1"),
		ResolvedAddresses: []netip.Addr{netip.MustParseAddr("1.1.1.1")},
	})
	if decision.Allowed || decision.Reason != EgressAddressMismatch {
		t.Fatalf("mismatched resolved IP Decide() = %#v", decision)
	}
}

func TestClassifyAddressFailsClosedForSpecialUseRanges(t *testing.T) {
	tests := map[string]AddressClass{
		"8.8.8.8":              AddressPublic,
		"10.0.0.1":             AddressPrivate,
		"127.0.0.1":            AddressLoopback,
		"169.254.1.1":          AddressLinkLocal,
		"224.0.0.1":            AddressMulticast,
		"0.0.0.0":              AddressUnspecified,
		"100.64.0.1":           AddressReserved,
		"192.0.2.1":            AddressReserved,
		"2001:db8::1":          AddressReserved,
		"2606:4700:4700::1111": AddressPublic,
		"fc00::1":              AddressPrivate,
		"fe80::1":              AddressLinkLocal,
	}
	for raw, wanted := range tests {
		if got := ClassifyAddress(netip.MustParseAddr(raw)); got != wanted {
			t.Errorf("ClassifyAddress(%s) = %q, want %q", raw, got, wanted)
		}
	}
}

func TestEgressPolicyDefensivelyCopiesRules(t *testing.T) {
	rule := EgressRule{
		ID:                    "copy-test",
		ProviderID:            "remote",
		Capabilities:          []Capability{CapabilityChat},
		Hosts:                 []string{"api.example.com"},
		Schemes:               []string{"https"},
		Ports:                 []uint16{443},
		AllowedAddressClasses: []AddressClass{AddressPublic},
	}
	policy := mustPolicy(t, rule)
	rule.Hosts[0] = "attacker.example"
	rule.Ports[0] = 8443

	decision := policy.Decide(EgressRequest{
		ProviderID:        "remote",
		Capability:        CapabilityChat,
		Endpoint:          mustEndpoint(t, "https://api.example.com/v1"),
		ResolvedAddresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")},
	})
	if !decision.Allowed {
		t.Fatalf("mutating input rule changed policy: %#v", decision)
	}
}

func mustEndpoint(t *testing.T, raw string) Endpoint {
	t.Helper()
	endpoint, err := ParseEndpoint(raw)
	if err != nil {
		t.Fatalf("ParseEndpoint(%q) error = %v", raw, err)
	}
	return endpoint
}

func mustPolicy(t *testing.T, rule EgressRule) EgressPolicy {
	t.Helper()
	policy, err := NewEgressPolicy([]EgressRule{rule})
	if err != nil {
		t.Fatalf("NewEgressPolicy() error = %v", err)
	}
	return policy
}
