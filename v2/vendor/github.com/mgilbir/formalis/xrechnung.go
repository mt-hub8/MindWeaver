package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the XRechnung CIUS (Core Invoice Usage Specification) on
// top of the EN 16931 core. XRechnung is the German public-sector profile: it
// makes several EN 16931-optional terms mandatory (the BR-DE-* rules) and, in its
// EXTENSION and CVD sub-profiles, replaces a few CEN rules (party/item identifier
// schemes, the attachment MIME list, the amount-due formula) with wider ones of
// its own. The same syntax-neutral model feeds it, so it validates CII
// (ZUGFeRD/XRechnung-CII) and UBL (XRechnung-UBL) alike.
//
// xrechnung_rules.go holds the rules whose subject is a position in the document
// tree — the payment-means groups, the settlement-discount text and both
// sub-profiles — and argues why they are per-syntax.
//
// Not vendored: the KoSIT Schematron and instance test suite are cloned by
// `make cius-oracles`. They are the oracle in both directions — the 86 business
// cases for false positives, and the 242 per-rule fixtures for the verdicts KoSIT
// declares for them, which is the only place in this repository a CIUS rule gets a
// violating verdict from its own authority.

// xrechnungTypeCodes is the restricted UNTDID 1001 set XRechnung permits (BR-DE-17).
var xrechnungTypeCodes = map[string]bool{
	"326": true, "380": true, "384": true, "389": true,
	"381": true, "875": true, "876": true, "877": true,
}

// xrechnungSuppressedForExtension are the EN 16931 rules a document claiming the
// EXTENSION sub-profile is not judged by, each because KoSIT publishes a
// BR-DEX-* rule that replaces it. The replacement is evaluated — see
// xrUBLExtensionRules and xrCIIExtensionRules — so this is a swap and not a
// discount, which is the property the map's second column exists to state and
// TestEveryExtensionSuppressionHasAReplacement checks.
//
// BR-CO-16 is not here, because BR-DEX-09 exists in KoSIT's UBL Schematron and
// not in its CII one: a CII EXTENSION invoice's amount due is judged by CEN's
// rule, unchanged. validateXRechnung applies that one by syntax.
var xrechnungSuppressedForExtension = map[string]string{
	"BR-CL-10": "BR-DEX-04", // party identifier scheme, with XR01..XR03 added
	"BR-CL-11": "BR-DEX-05", // party legal registration scheme
	"BR-CL-21": "BR-DEX-06", // item standard identifier scheme
	"BR-CL-25": "BR-DEX-07", // electronic address scheme
	"BR-CL-26": "BR-DEX-08", // deliver-to location identifier scheme
	"BR-CL-24": "BR-DEX-01", // attachment MIME code, with application/xml added
}

// xrechnungSuppressedForCVD is the same swap for the CVD sub-profile, which adds
// the scheme identifier 'CVD' to the item classification code list.
var xrechnungSuppressedForCVD = map[string]string{
	"BR-CL-13": "BR-TMP-CVD-01",
}

// The XRechnung specification identifiers, from common.sch:
//
//	XR-CIUS-ID      urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0
//	XR-EXTENSION-ID $XR-CIUS-ID#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0
//	XR-CVD-ID       $XR-CIUS-ID#compliant#urn:xeinkauf.de:kosit:xrechnung:cvd_0.9
//
// KoSIT selects a sub-profile by comparing BT-24 with the whole identifier, so
// its Schematron answers for exactly one version of XRechnung — 3.0, and CVD 0.9
// — and treats an EXTENSION document of any other version as a plain CIUS one.
// This package is not pinned to a version (BR-DE-21 checks the shape of BT-24,
// not one literal), so it matches the part of the identifier that names the
// sub-profile and leaves the version out. That is deliberately wider than KoSIT
// by exactly the versions KoSIT does not answer for, and narrower than the bare
// substring "extension" this used to look for, which would have taken any BT-24
// with that word anywhere in it — including one belonging to another authority
// entirely — for a German EXTENSION invoice.
const (
	xrExtensionMarker = "kosit:extension:xrechnung"
	xrCVDMarker       = "kosit:xrechnung:cvd"
)

// xrSubProfiles reports which XRechnung sub-profile BT-24 claims.
func xrSubProfiles(specID string) (ext, cvd bool) {
	return strings.Contains(specID, xrExtensionMarker), strings.Contains(specID, xrCVDMarker)
}

// xrechnungFlags is KoSIT's flag for every XRechnung identifier this package
// evaluates, folded onto this package's two severities: fatal is fatal, and both
// warning and information are advisory.
//
// SeverityFatal is the zero value, so an identifier absent from this map is
// reported fatal. That is the fail-safe direction and it is also checked:
// TestXRechnungSeveritiesQuoteKoSIT requires this map to name every identifier
// the package emits and to agree with the Schematron on each, so a rule cannot
// acquire a severity by being left out.
var xrechnungFlags = map[string]Severity{
	// flag="warning"
	"BR-DE-17":  SeverityWarning,
	"BR-DE-19":  SeverityWarning,
	"BR-DE-20":  SeverityWarning,
	"BR-DE-21":  SeverityWarning,
	"BR-DE-26":  SeverityWarning,
	"BR-DE-27":  SeverityWarning,
	"BR-DE-28":  SeverityWarning,
	"BR-DEX-02": SeverityWarning,
	"BR-DEX-15": SeverityWarning,
	"BR-TMP-2":  SeverityWarning,
	// flag="information"
	"BR-DE-TMP-32": SeverityWarning,
}

// xrAdder is adder for the XRechnung rule set, with the severity read off
// xrechnungFlags instead of assumed.
func xrAdder(out *[]Violation) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{
			Source:   SourceXRechnung,
			Rule:     rule,
			Severity: xrechnungFlags[rule],
			Message:  msg,
		})
	}
}

// ValidateXRechnung validates an invoice XML against the XRechnung CIUS: the
// EN 16931 core (with the XRechnung sub-profile overrides applied) plus the
// BR-DE-* rules. It accepts either syntax.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice or credit note.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// The Report names the rule families neither rule set evaluates — the union of
// Coverage(SourceEN16931) and Coverage(SourceXRechnung). The XRechnung rule set
// is evaluated in full: KoSIT's own 57 identifiers, both sub-profiles included,
// and the 21 Peppol BIS Billing rules its released Schematron merges in. The
// severity of each finding is the flag the artefact publishes — eleven of KoSIT's
// fifty-seven are warning or information rather than fatal, and so is
// PEPPOL-EN16931-R120 as KoSIT re-flags it — so a document can fail one of them
// and still be Conformant.
//
// The imported findings carry SourcePeppol, because OpenPEPPOL is the authority
// that wrote those rules and Source names the author. They are still part of the
// XRechnung rule set, so they are covered by Coverage(SourceXRechnung) rather
// than by Coverage(SourcePeppol): a caller reading this Report's NotEvaluated is
// told what the *XRechnung* Schematron leaves unchecked, and the rules Peppol
// publishes that XRechnung does not import are not that. See the Coverage table.
func ValidateXRechnung(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceXRechnung}, validateXRechnung)
}

func validateXRechnung(r *run, p *parsed) []Violation {
	inv := p.inv
	ext, cvd := xrSubProfiles(inv.specID)

	var out []Violation
	// XRechnung documents carry the full EN 16931 data set — BR-DE-* makes more
	// terms mandatory, never fewer — so the core runs at the EN 16931 profile,
	// which is what the removed ProfileXRechnung constant did anyway: it matched
	// none of the three profile predicates in validateEN16931, and produced an
	// identical finding set on every EN 16931 document in testdata.
	for _, v := range validateEN16931(r, p, ProfileEN16931, ciiBindingCEN) {
		switch {
		case ext && xrechnungSuppressedForExtension[v.Rule] != "":
			continue
		case cvd && xrechnungSuppressedForCVD[v.Rule] != "":
			continue
		// BR-DEX-09 replaces the amount-due formula, and KoSIT publishes it for the
		// UBL binding only. A CII EXTENSION invoice keeps CEN's BR-CO-16.
		case ext && inv.syntax == "UBL" && v.Rule == "BR-CO-16":
			continue
		}
		out = append(out, v)
	}
	out = append(out, validateXRechnungRules(inv)...)
	out = append(out, validateXRechnungTreeRules(r, p, ext, cvd)...)
	// The 21 Peppol rules KoSIT's build merges into the released Schematron, in
	// KoSIT's wording of them and gated on its whitelist. peppol_rules.go is where
	// the merge's five departures from OpenPEPPOL's own files are transcribed.
	out = append(out, validatePeppolRuleSet(r, p, true)...)
	return out
}

// validateXRechnungRules applies the mandatory-term rules XRechnung adds on top
// of EN 16931 that the syntax-neutral model already answers. The rules whose
// subject is a position in the document tree — the payment-means groups, the
// settlement-discount text, both sub-profiles — are in xrechnung_rules.go.
//
// The severity of each finding is KoSIT's flag, read from xrechnungFlags rather
// than assumed fatal. Seven of these rules are not fatal — BR-DE-17, 19, 20, 21,
// 26, 27 and 28 — and were reported as though they were, which made a document
// KoSIT accepts with a warning about its telephone number non-conformant here.
func validateXRechnungRules(inv *en16931Invoice) []Violation {
	var out []Violation
	add := xrAdder(&out)
	req := func(rule, msg, val string) {
		if val == "" {
			add(rule, msg)
		}
	}

	if !inv.paymentInstrPresent {
		add("BR-DE-1", "An Invoice shall contain Payment instructions (BG-16)")
	}
	if !inv.sellerContactPresent {
		add("BR-DE-2", "The Seller contact group (BG-6) shall be provided")
	}
	req("BR-DE-3", "The Seller city (BT-37) shall be provided", inv.sellerCity)
	req("BR-DE-4", "The Seller post code (BT-38) shall be provided", inv.sellerPostCode)
	req("BR-DE-5", "The Seller contact point (BT-41) shall be provided", inv.sellerContactName)
	req("BR-DE-6", "The Seller contact telephone number (BT-42) shall be provided", inv.sellerPhone)
	req("BR-DE-7", "The Seller contact email address (BT-43) shall be provided", inv.sellerEmail)
	req("BR-DE-8", "The Buyer city (BT-52) shall be provided", inv.buyerCity)
	req("BR-DE-9", "The Buyer post code (BT-53) shall be provided", inv.buyerPostCode)
	if inv.deliverToPresent {
		req("BR-DE-10", "The Deliver to city (BT-77) shall be provided when a Deliver to address is present", inv.deliverToCity)
		req("BR-DE-11", "The Deliver to post code (BT-78) shall be provided when a Deliver to address is present", inv.deliverToPostCode)
	}
	for _, b := range inv.vatBreakdowns {
		if b.rate == "" {
			add("BR-DE-14", "The VAT category rate (BT-119) shall be provided in each VAT breakdown")
		}
	}
	req("BR-DE-15", "The Buyer reference (BT-10) shall be provided", inv.buyerReference)

	if tc := inv.typeCode; tc != "" && !xrechnungTypeCodes[tc] {
		add("BR-DE-17", fmt.Sprintf("Invoice type code (BT-3=%q) is not one of the codes XRechnung permits", tc))
	}
	if s := inv.specID; !strings.Contains(s, "kosit") || !strings.Contains(s, "xrechnung") {
		add("BR-DE-21", "The Specification identifier (BT-24) shall be an XRechnung identifier")
	}
	// BR-DE-22: attachment file names shall be unique.
	names := map[string]bool{}
	for _, d := range inv.docRefs {
		if d.filename == "" {
			continue
		}
		if names[d.filename] {
			add("BR-DE-22", fmt.Sprintf("Attachment file name %q is not unique", d.filename))
		}
		names[d.filename] = true
	}
	// BR-DE-27: a telephone number shall contain at least three digits.
	if p := inv.sellerPhone; p != "" && countDigits(p) < 3 {
		add("BR-DE-27", "The Seller contact telephone number (BT-42) shall contain at least three digits")
	}
	// BR-DE-28: an email address shall contain exactly one @.
	if e := inv.sellerEmail; e != "" && strings.Count(e, "@") != 1 {
		add("BR-DE-28", "The Seller contact email address (BT-43) shall contain exactly one @ sign")
	}

	// BR-DE-16: a taxed VAT category requires the Seller VAT / tax registration /
	// tax representative identifier.
	usedTaxed := false
	for _, li := range inv.lines {
		if xrechnungVATCodes[li.vatCategory] {
			usedTaxed = true
		}
	}
	for _, ac := range inv.allowCharges {
		if xrechnungVATCodes[ac.category] {
			usedTaxed = true
		}
	}
	if usedTaxed && !(inv.sellerVATID || inv.sellerTaxReg || inv.taxRepVATID) {
		add("BR-DE-16", "A taxed VAT category requires the Seller VAT identifier, tax registration or tax representative VAT identifier")
	}

	// BR-DE-26: a corrected invoice (type 384) requires a preceding invoice reference.
	if inv.typeCode == "384" && !inv.hasBillingRef {
		add("BR-DE-26", "A corrected invoice (type code 384) shall contain a Preceding Invoice reference (BG-3)")
	}
	return out
}

// xrechnungVATCodes are the taxed VAT category codes that trigger BR-DE-16.
var xrechnungVATCodes = map[string]bool{
	"S": true, "Z": true, "E": true, "AE": true, "K": true, "G": true, "L": true, "M": true,
}

// validIBAN reports whether s is a syntactically valid IBAN (ISO 13616): two
// letters, two check digits, up to 30 alphanumerics, passing the mod-97 check.
func validIBAN(s string) bool {
	s = strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s))
	if len(s) < 5 || len(s) > 34 {
		return false
	}
	if s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' || s[2] < '0' || s[2] > '9' || s[3] < '0' || s[3] > '9' {
		return false
	}
	// Move the first four characters to the end and reduce mod 97.
	rearranged := s[4:] + s[:4]
	rem := 0
	for _, r := range rearranged {
		var d int
		switch {
		case r >= '0' && r <= '9':
			d = int(r - '0')
		case r >= 'A' && r <= 'Z':
			d = int(r-'A') + 10
		default:
			return false
		}
		if d >= 10 {
			rem = (rem*100 + d) % 97
		} else {
			rem = (rem*10 + d) % 97
		}
	}
	return rem == 1
}

// countDigits returns the number of ASCII digits in s.
func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
