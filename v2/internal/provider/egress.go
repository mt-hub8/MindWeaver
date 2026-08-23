package provider

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
)

// EgressPolicySchemaVersion identifies the policy and decision contract.
const EgressPolicySchemaVersion = "mindweaver.provider-egress-policy/v1"

// Endpoint is a parsed, credential-free provider base endpoint. Query strings,
// fragments, and userinfo are rejected by ParseEndpoint, so this value is safe
// to serialize. It intentionally does not implement fmt.Stringer.
type Endpoint struct {
	Scheme       string `json:"scheme"`
	Host         string `json:"host"`
	Port         uint16 `json:"port"`
	PortExplicit bool   `json:"port_explicit"`
	BasePath     string `json:"base_path,omitempty"`
}

// Authority is the non-sensitive endpoint portion bound into an egress decision.
type Authority struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   uint16 `json:"port"`
}

// EndpointError exposes a stable reason without echoing the rejected URL.
type EndpointError struct {
	Code string
}

func (e *EndpointError) Error() string { return "invalid provider endpoint: " + e.Code }

// ParseEndpoint parses and canonicalizes a provider base URL. Credentials and
// token-like query parameters never enter Endpoint.
func ParseEndpoint(raw string) (Endpoint, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 2048 {
		return Endpoint{}, &EndpointError{Code: "invalid_length_or_whitespace"}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Endpoint{}, &EndpointError{Code: "malformed"}
	}
	if !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return Endpoint{}, &EndpointError{Code: "absolute_url_required"}
	}
	if parsed.User != nil {
		return Endpoint{}, &EndpointError{Code: "userinfo_forbidden"}
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return Endpoint{}, &EndpointError{Code: "query_forbidden"}
	}
	if parsed.Fragment != "" || strings.Contains(raw, "#") {
		return Endpoint{}, &EndpointError{Code: "fragment_forbidden"}
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return Endpoint{}, &EndpointError{Code: "encoded_path_forbidden"}
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return Endpoint{}, &EndpointError{Code: "scheme_forbidden"}
	}
	host, err := canonicalHost(parsed.Hostname())
	if err != nil {
		return Endpoint{}, &EndpointError{Code: "invalid_host"}
	}

	portExplicit := parsed.Port() != ""
	if strings.HasSuffix(parsed.Host, ":") {
		return Endpoint{}, &EndpointError{Code: "invalid_port"}
	}
	var portValue uint64
	if portExplicit {
		portValue, err = strconv.ParseUint(parsed.Port(), 10, 16)
		if err != nil || portValue == 0 {
			return Endpoint{}, &EndpointError{Code: "invalid_port"}
		}
	} else if scheme == "https" {
		portValue = 443
	} else {
		portValue = 80
	}

	basePath, err := canonicalBasePath(parsed.Path)
	if err != nil {
		return Endpoint{}, err
	}
	endpoint := Endpoint{
		Scheme:       scheme,
		Host:         host,
		Port:         uint16(portValue),
		PortExplicit: portExplicit,
		BasePath:     basePath,
	}
	if err := endpoint.Validate(); err != nil {
		return Endpoint{}, err
	}
	return endpoint, nil
}

// Validate rejects manually-constructed endpoints that bypass ParseEndpoint.
func (e Endpoint) Validate() error {
	if e.Scheme != "https" && e.Scheme != "http" {
		return &EndpointError{Code: "scheme_forbidden"}
	}
	host, err := canonicalHost(e.Host)
	if err != nil || host != e.Host {
		return &EndpointError{Code: "noncanonical_host"}
	}
	if e.Port == 0 {
		return &EndpointError{Code: "invalid_port"}
	}
	if !e.PortExplicit && ((e.Scheme == "https" && e.Port != 443) || (e.Scheme == "http" && e.Port != 80)) {
		return &EndpointError{Code: "nondefault_port_must_be_explicit"}
	}
	basePath, err := canonicalBasePath(e.BasePath)
	if err != nil || basePath != e.BasePath {
		return &EndpointError{Code: "noncanonical_path"}
	}
	return nil
}

// HostResolver is injected by a transport. A transport must resolve first,
// authorize all returned addresses, and dial one of Decision.ApprovedAddresses;
// resolving again after authorization would reintroduce DNS-rebinding risk.
type HostResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// EgressDecider is the transport-facing default-deny authorization protocol.
type EgressDecider interface {
	Decide(EgressRequest) EgressDecision
}

// AddressClass is assigned to each post-resolution IP address.
type AddressClass string

const (
	AddressPublic      AddressClass = "public"
	AddressPrivate     AddressClass = "private"
	AddressLoopback    AddressClass = "loopback"
	AddressLinkLocal   AddressClass = "link_local"
	AddressMulticast   AddressClass = "multicast"
	AddressUnspecified AddressClass = "unspecified"
	AddressReserved    AddressClass = "reserved"
)

// EgressRule grants only an exact provider/capability/host/scheme/port tuple.
// Wildcards are intentionally absent from v1. Link-local, multicast,
// unspecified, and reserved ranges cannot be granted.
type EgressRule struct {
	ID                    string         `json:"id"`
	ProviderID            string         `json:"provider_id"`
	Capabilities          []Capability   `json:"capabilities"`
	Hosts                 []string       `json:"hosts"`
	Schemes               []string       `json:"schemes"`
	Ports                 []uint16       `json:"ports"`
	AllowedAddressClasses []AddressClass `json:"allowed_address_classes"`
}

// EgressRequest contains no headers, credentials, prompt, response, or URL
// query. ResolvedAddresses are required for DNS names and bind the decision to
// the exact IPs the transport is permitted to dial.
type EgressRequest struct {
	ProviderID        string       `json:"provider_id"`
	Capability        Capability   `json:"capability"`
	Endpoint          Endpoint     `json:"endpoint"`
	ResolvedAddresses []netip.Addr `json:"resolved_addresses,omitempty"`
}

// EgressDecisionReason is stable and safe for audit/API use.
type EgressDecisionReason string

const (
	EgressAllowed            EgressDecisionReason = "allowed"
	EgressDefaultDeny        EgressDecisionReason = "default_deny"
	EgressInvalidRequest     EgressDecisionReason = "invalid_request"
	EgressInvalidEndpoint    EgressDecisionReason = "invalid_endpoint"
	EgressNoMatchingRule     EgressDecisionReason = "no_matching_rule"
	EgressResolutionRequired EgressDecisionReason = "resolution_required"
	EgressAddressMismatch    EgressDecisionReason = "address_mismatch"
	EgressAddressDenied      EgressDecisionReason = "address_denied"
)

// EgressDecision authorizes only its Authority and ApprovedAddresses. Allowed
// is false unless a validated rule explicitly matches every dimension.
type EgressDecision struct {
	SchemaVersion     string               `json:"schema_version"`
	Allowed           bool                 `json:"allowed"`
	Reason            EgressDecisionReason `json:"reason"`
	RuleID            string               `json:"rule_id,omitempty"`
	Authority         Authority            `json:"authority"`
	ApprovedAddresses []netip.Addr         `json:"approved_addresses,omitempty"`
}

// EgressPolicy stores a validated copy of the rules. Its zero value and the
// value returned by DefaultEgressPolicy both deny every request.
type EgressPolicy struct {
	rules []EgressRule
}

// DefaultEgressPolicy returns a policy with no grants.
func DefaultEgressPolicy() EgressPolicy { return EgressPolicy{} }

// NewEgressPolicy validates and defensively copies explicit grants.
func NewEgressPolicy(rules []EgressRule) (EgressPolicy, error) {
	validated := make([]EgressRule, len(rules))
	seenIDs := make(map[string]struct{}, len(rules))
	for index := range rules {
		rule, err := normalizeEgressRule(rules[index])
		if err != nil {
			return EgressPolicy{}, err
		}
		if _, duplicate := seenIDs[rule.ID]; duplicate {
			return EgressPolicy{}, errors.New("invalid egress policy: duplicate_rule_id")
		}
		seenIDs[rule.ID] = struct{}{}
		validated[index] = rule
	}
	return EgressPolicy{rules: validated}, nil
}

// Decide applies exact rule matching followed by post-DNS address validation.
func (p EgressPolicy) Decide(request EgressRequest) EgressDecision {
	decision := EgressDecision{SchemaVersion: EgressPolicySchemaVersion, Reason: EgressInvalidRequest}
	if err := validateIdentifier(request.ProviderID, 128); err != nil || !knownCapability(request.Capability) {
		return decision
	}
	if err := request.Endpoint.Validate(); err != nil {
		decision.Reason = EgressInvalidEndpoint
		return decision
	}
	decision.Authority = Authority{Scheme: request.Endpoint.Scheme, Host: request.Endpoint.Host, Port: request.Endpoint.Port}
	if len(p.rules) == 0 {
		decision.Reason = EgressDefaultDeny
		return decision
	}

	candidates := make([]EgressRule, 0, 1)
	for _, rule := range p.rules {
		if ruleMatchesAuthority(rule, request) {
			candidates = append(candidates, rule)
		}
	}
	if len(candidates) == 0 {
		decision.Reason = EgressNoMatchingRule
		return decision
	}

	addresses, reason := bindResolvedAddresses(request.Endpoint, request.ResolvedAddresses)
	if reason != "" {
		decision.Reason = reason
		return decision
	}
	for _, rule := range candidates {
		if ruleAllowsAllAddresses(rule, addresses) {
			decision.Allowed = true
			decision.Reason = EgressAllowed
			decision.RuleID = rule.ID
			decision.ApprovedAddresses = append([]netip.Addr(nil), addresses...)
			return decision
		}
	}
	decision.Reason = EgressAddressDenied
	return decision
}

// ClassifyAddress classifies an IP after unmapping IPv4-in-IPv6. Only globally
// routable unicast space is Public; documentation, shared, benchmark, and other
// special-use ranges fail closed as Reserved.
func ClassifyAddress(address netip.Addr) AddressClass {
	if !address.IsValid() || address.Zone() != "" {
		return AddressReserved
	}
	address = address.Unmap()
	if address.IsUnspecified() {
		return AddressUnspecified
	}
	if address.IsLoopback() {
		return AddressLoopback
	}
	if address.IsLinkLocalUnicast() {
		return AddressLinkLocal
	}
	if address.IsMulticast() {
		return AddressMulticast
	}
	if address.IsPrivate() {
		return AddressPrivate
	}
	if isSpecialUse(address) {
		return AddressReserved
	}
	if address.IsGlobalUnicast() {
		return AddressPublic
	}
	return AddressReserved
}

func normalizeEgressRule(input EgressRule) (EgressRule, error) {
	if err := validateIdentifier(input.ID, 128); err != nil {
		return EgressRule{}, errors.New("invalid egress policy: invalid_rule_id")
	}
	if err := validateIdentifier(input.ProviderID, 128); err != nil {
		return EgressRule{}, errors.New("invalid egress policy: invalid_provider_id")
	}
	if len(input.Capabilities) == 0 || len(input.Hosts) == 0 || len(input.Schemes) == 0 || len(input.Ports) == 0 || len(input.AllowedAddressClasses) == 0 {
		return EgressRule{}, errors.New("invalid egress policy: incomplete_rule")
	}

	rule := EgressRule{ID: input.ID, ProviderID: input.ProviderID}
	seenCapabilities := make(map[Capability]struct{}, len(input.Capabilities))
	for _, capability := range input.Capabilities {
		if !knownCapability(capability) {
			return EgressRule{}, errors.New("invalid egress policy: invalid_capability")
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return EgressRule{}, errors.New("invalid egress policy: duplicate_capability")
		}
		seenCapabilities[capability] = struct{}{}
		rule.Capabilities = append(rule.Capabilities, capability)
	}

	seenHosts := make(map[string]struct{}, len(input.Hosts))
	for _, rawHost := range input.Hosts {
		host, err := canonicalHost(rawHost)
		if err != nil || rawHost != host {
			return EgressRule{}, errors.New("invalid egress policy: host_must_be_canonical")
		}
		if _, duplicate := seenHosts[host]; duplicate {
			return EgressRule{}, errors.New("invalid egress policy: duplicate_host")
		}
		seenHosts[host] = struct{}{}
		rule.Hosts = append(rule.Hosts, host)
	}

	seenSchemes := make(map[string]struct{}, len(input.Schemes))
	for _, scheme := range input.Schemes {
		if scheme != "https" && scheme != "http" {
			return EgressRule{}, errors.New("invalid egress policy: invalid_scheme")
		}
		if _, duplicate := seenSchemes[scheme]; duplicate {
			return EgressRule{}, errors.New("invalid egress policy: duplicate_scheme")
		}
		seenSchemes[scheme] = struct{}{}
		rule.Schemes = append(rule.Schemes, scheme)
	}

	seenPorts := make(map[uint16]struct{}, len(input.Ports))
	for _, port := range input.Ports {
		if port == 0 {
			return EgressRule{}, errors.New("invalid egress policy: invalid_port")
		}
		if _, duplicate := seenPorts[port]; duplicate {
			return EgressRule{}, errors.New("invalid egress policy: duplicate_port")
		}
		seenPorts[port] = struct{}{}
		rule.Ports = append(rule.Ports, port)
	}

	seenClasses := make(map[AddressClass]struct{}, len(input.AllowedAddressClasses))
	for _, class := range input.AllowedAddressClasses {
		if class != AddressPublic && class != AddressPrivate && class != AddressLoopback {
			return EgressRule{}, errors.New("invalid egress policy: unsafe_address_class")
		}
		if _, duplicate := seenClasses[class]; duplicate {
			return EgressRule{}, errors.New("invalid egress policy: duplicate_address_class")
		}
		seenClasses[class] = struct{}{}
		rule.AllowedAddressClasses = append(rule.AllowedAddressClasses, class)
	}
	if _, plaintext := seenSchemes["http"]; plaintext {
		if _, public := seenClasses[AddressPublic]; public {
			return EgressRule{}, errors.New("invalid egress policy: public_plaintext_forbidden")
		}
	}
	return rule, nil
}

func ruleMatchesAuthority(rule EgressRule, request EgressRequest) bool {
	return rule.ProviderID == request.ProviderID &&
		containsCapability(rule.Capabilities, request.Capability) &&
		containsString(rule.Hosts, request.Endpoint.Host) &&
		containsString(rule.Schemes, request.Endpoint.Scheme) &&
		containsPort(rule.Ports, request.Endpoint.Port)
}

func bindResolvedAddresses(endpoint Endpoint, supplied []netip.Addr) ([]netip.Addr, EgressDecisionReason) {
	endpointIP, parseErr := netip.ParseAddr(endpoint.Host)
	endpointIsIP := parseErr == nil
	if endpointIsIP {
		endpointIP = endpointIP.Unmap()
	}
	if !endpointIsIP && len(supplied) == 0 {
		return nil, EgressResolutionRequired
	}
	if len(supplied) > 16 {
		return nil, EgressAddressDenied
	}

	addresses := make([]netip.Addr, 0, len(supplied)+1)
	if endpointIsIP {
		addresses = append(addresses, endpointIP)
	}
	seen := make(map[netip.Addr]struct{}, len(supplied)+1)
	if endpointIsIP {
		seen[endpointIP] = struct{}{}
	}
	for _, address := range supplied {
		if !address.IsValid() || address.Zone() != "" {
			return nil, EgressAddressDenied
		}
		address = address.Unmap()
		if endpointIsIP && address != endpointIP {
			return nil, EgressAddressMismatch
		}
		if _, duplicate := seen[address]; duplicate {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses, ""
}

func ruleAllowsAllAddresses(rule EgressRule, addresses []netip.Addr) bool {
	if len(addresses) == 0 {
		return false
	}
	for _, address := range addresses {
		if !containsAddressClass(rule.AllowedAddressClasses, ClassifyAddress(address)) {
			return false
		}
	}
	return true
}

func canonicalHost(host string) (string, error) {
	if host == "" || host != strings.TrimSpace(host) || len(host) > 253 || strings.Contains(host, "%") {
		return "", errors.New("invalid host")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Unmap().String(), nil
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > 253 {
		return "", errors.New("invalid host")
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid host")
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return "", errors.New("invalid host")
			}
		}
	}
	return host, nil
}

func canonicalBasePath(value string) (string, error) {
	if value == "" || value == "/" {
		return "", nil
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "//") {
		return "", &EndpointError{Code: "invalid_path"}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return "", &EndpointError{Code: "path_traversal_forbidden"}
		}
		for _, character := range segment {
			if character < 0x20 || character == 0x7f {
				return "", &EndpointError{Code: "invalid_path"}
			}
		}
	}
	cleaned := strings.TrimSuffix(value, "/")
	if pathpkg.Clean(cleaned) != cleaned {
		return "", &EndpointError{Code: "noncanonical_path"}
	}
	return cleaned, nil
}

func knownCapability(value Capability) bool {
	for _, capability := range allCapabilities {
		if value == capability {
			return true
		}
	}
	return false
}

func containsCapability(values []Capability, wanted Capability) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsPort(values []uint16, wanted uint16) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsAddressClass(values []AddressClass, wanted AddressClass) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isSpecialUse(address netip.Addr) bool {
	prefixes := specialIPv6Prefixes
	if address.Is4() {
		prefixes = specialIPv4Prefixes
	}
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	if address.Is6() {
		globalV6 := netip.MustParsePrefix("2000::/3")
		return !globalV6.Contains(address)
	}
	return false
}

var specialIPv4Prefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

var specialIPv6Prefixes = []netip.Prefix{
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}
