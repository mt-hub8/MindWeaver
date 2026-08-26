package formalis

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// This file evaluates the OpenPEPPOL rule set — the PEPPOL-COMMON-* and
// PEPPOL-EN16931-* identifiers of testdata/peppol/repo/rules/sch — against the
// parsed document tree, one binding at a time.
//
// # Why the tree and not the model
//
// peppol.go evaluates the handful of Peppol rules that are statements about
// business terms on the syntax-neutral model, which is where every BR-* rule in
// this package lives. Most of Peppol's rules are not that. "A document MUST NOT
// contain empty elements", "only one cac:TaxTotal may carry cac:TaxSubtotal",
// "every @currencyID must equal BT-5", "an identifier declared under scheme 0192
// must pass the Norwegian mod-11 check" are statements about an XML tree and about
// attributes the model does not carry, and half of them are published in one
// binding only. en16931_ubl_rules.go makes the same argument at greater length.
//
// # Two bindings that are not translations of each other
//
// The UBL file publishes 58 identifiers and the CII file 44, and the union is 59 —
// so a rule present in one binding must not fire on a document written in the
// other. PEPPOL-EN16931-R006 exists only in CII; CL006, P0101, P0104..P0111,
// P0112, R008, R044, R046 and R051 exist only in UBL. Where an identifier is in
// both, its XPath can still test a different thing: R002 exempts a German
// invoice from the one-note limit in UBL and instead forbids a note subject code
// in CII, R053 counts cac:TaxTotal groups in UBL and ram:TaxTotalAmount elements
// in CII, and R121's context is the line in UBL and the price in CII. Each rule
// below cites the XPath it was transcribed from, per binding, and peppolRules is
// the table that says which binding publishes what — checked against the artefact
// by TestPeppolRuleTableMatchesTheSchematron, in both directions.
//
// This is the shape of the defect PRs 13 and 14 established the pattern for and
// this rule set had anyway: PEPPOL-EN16931-P0104..P0111 were evaluated on the
// shared model and therefore reported for CII documents, where OpenPEPPOL
// publishes no such rule. Eight identifiers were being reported against an
// authority that does not define them for that syntax.
//
// # The same rules, as XRechnung imports them
//
// KoSIT's released XRechnung Schematron merges 21 of these rules in at build time,
// driven by the whitelist in testdata/xrechnung/schematron/src/xsl/rule-list.xml
// and the stylesheet src/xsl/peppol-into-xr.xsl. The merge is not a copy, and the
// differences are load-bearing, so the same rule bodies are used for both paths
// with a dialect flag rather than being written twice:
//
//   - only the 21 whitelisted identifiers are merged, so an XRechnung validation
//     must not acquire the other 38 (peppolXRImports);
//   - the stylesheet *adds* R008, R044 and R046 to the CII binding, which
//     OpenPEPPOL's own CII file does not publish, with CII XPaths of KoSIT's own
//     writing (peppolXRCIIAdditions);
//   - it rewrites the CII wording of R053 from "= 1" to "<= 1" and of R055 to
//     tolerate an absent BT-110;
//   - it replaces the 0.02 tolerance of R040 and R120 with $slackValue, which is
//     0.5 when BT-5 is HUF;
//   - and it re-flags R120 as warning where OpenPEPPOL flags it fatal.
//
// That last one is C29's shape exactly, in a rule set nobody had compared: the
// artefact a German buyer validates against reports a line-net-amount mismatch as
// a warning, so reporting it fatal on the XRechnung path would refuse an invoice
// KoSIT accepts. The flag a finding carries is therefore read from the artefact
// the run is quoting — peppolFlags for a Peppol validation, peppolXRFlags layered
// over it for an XRechnung one — and TestPeppolSeveritiesQuoteTheArtefacts checks
// both against the files.

// peppolBindings is the set of syntax bindings that publish a rule.
type peppolBindings uint8

const (
	peppolUBL peppolBindings = 1 << iota
	peppolCII
)

// peppolRule is one identifier as OpenPEPPOL publishes it.
type peppolRule struct {
	bindings peppolBindings
	severity Severity
}

// peppolRules is every identifier the vendored OpenPEPPOL Schematron publishes,
// with the bindings that publish it and the flag it carries.
//
// It is the table three guards read. TestPeppolRuleTableMatchesTheSchematron
// compares it with an XML decoder's reading of both .sch files in both directions,
// so an identifier OpenPEPPOL adds or withdraws fails here rather than being
// absorbed; TestEveryPublishedPeppolRuleHasBothVerdicts requires every entry to
// have a document that trips it and one that does not; and the severity column is
// what peppolAdder quotes.
//
// PEPPOL-COMMON-R048 is deliberately absent. It is in both binding files and
// inside an XML comment in both, so OpenPEPPOL publishes no such rule — see
// Coverage(SourcePeppol), which used to name it as an advisory gap.
var peppolRules = map[string]peppolRule{
	"PEPPOL-COMMON-R040":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R041":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R042":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R043":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R044":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-COMMON-R045":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-COMMON-R046":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-COMMON-R047":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-COMMON-R049":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R050":   {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-COMMON-R052":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-COMMON-R053":   {peppolUBL | peppolCII, SeverityWarning},
	"PEPPOL-EN16931-CL001": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-CL002": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-CL003": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-CL006": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-CL007": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-CL008": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-F001":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-P0100": {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-P0101": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0104": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0105": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0106": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0107": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0108": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0109": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0110": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0111": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-P0112": {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-R001":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R002":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R003":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R004":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R005":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R006":  {peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R007":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R008":  {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-R010":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R020":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R040":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R041":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R042":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R043":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R044":  {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-R046":  {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-R051":  {peppolUBL, SeverityFatal},
	"PEPPOL-EN16931-R053":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R054":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R055":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R061":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R080":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R100":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R101":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R110":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R111":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R120":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R121":  {peppolUBL | peppolCII, SeverityFatal},
	"PEPPOL-EN16931-R130":  {peppolUBL | peppolCII, SeverityFatal},
}

// peppolXRImports are the identifiers KoSIT's rule-list.xml merges into the
// released XRechnung Schematron: the live <r:rule> entries of
// testdata/xrechnung/schematron/src/xsl/rule-list.xml, which comments out the
// candidates it does not take.
//
// The list is the gate on the XRechnung path, so an XRechnung validation cannot
// silently acquire a Peppol rule KoSIT does not import — R002, R003, R004, R006,
// R007, R051, R080, R100, P0100, P0101, F001, every CL* and every PEPPOL-COMMON-*
// are among the ones it does not. R051 is commented out with a reason (an open
// OpenPEPPOL pull request), which is the difference between "withdrawn" and "not
// yet"; either way KoSIT does not ship it.
//
// TestXRechnungImportsExactlyKoSITsWhitelist reads the file and holds this map to
// it in both directions.
var peppolXRImports = map[string]bool{
	"PEPPOL-EN16931-R001": true,
	"PEPPOL-EN16931-R005": true,
	"PEPPOL-EN16931-R008": true,
	"PEPPOL-EN16931-R010": true,
	"PEPPOL-EN16931-R020": true,
	"PEPPOL-EN16931-R040": true,
	"PEPPOL-EN16931-R041": true,
	"PEPPOL-EN16931-R042": true,
	"PEPPOL-EN16931-R043": true,
	"PEPPOL-EN16931-R044": true,
	"PEPPOL-EN16931-R046": true,
	"PEPPOL-EN16931-R053": true,
	"PEPPOL-EN16931-R054": true,
	"PEPPOL-EN16931-R055": true,
	"PEPPOL-EN16931-R061": true,
	"PEPPOL-EN16931-R101": true,
	"PEPPOL-EN16931-R110": true,
	"PEPPOL-EN16931-R111": true,
	"PEPPOL-EN16931-R120": true,
	"PEPPOL-EN16931-R121": true,
	"PEPPOL-EN16931-R130": true,
}

// peppolXRCIIAdditions are the three rules KoSIT's merge writes into the CII
// binding itself, in src/xsl/peppol-into-xr.xsl, because OpenPEPPOL publishes them
// for UBL only and KoSIT wanted them for both. Their CII XPaths are KoSIT's, not
// OpenPEPPOL's, and are transcribed in the CII rule bodies below.
var peppolXRCIIAdditions = map[string]bool{
	"PEPPOL-EN16931-R008": true,
	"PEPPOL-EN16931-R044": true,
	"PEPPOL-EN16931-R046": true,
}

// peppolXRFlags is KoSIT's flag where it differs from OpenPEPPOL's for an
// imported rule, folded onto this package's two severities.
//
// There is exactly one, and it is the reason this map exists rather than an
// assumption that an imported rule keeps its flag: peppol-into-xr.xsl gives
// PEPPOL-EN16931-R120 `<xsl:attribute name="flag">warning</xsl:attribute>` while
// OpenPEPPOL flags it fatal. On the XRechnung path KoSIT's flag governs — it is
// the artefact a German buyer validates against — so a line whose net amount does
// not match quantity × price is a warning there and a non-conformance on the
// Peppol path. TestPeppolSeveritiesQuoteTheArtefacts checks both readings against
// both files.
var peppolXRFlags = map[string]Severity{
	"PEPPOL-EN16931-R120": SeverityWarning,
}

// peppolEval carries the two facts every rule body needs: which artefact's
// wording of the rule set is being quoted, and where the findings go.
type peppolEval struct {
	// xr selects KoSIT's merged wording over OpenPEPPOL's own.
	xr bool
	// cii is the binding the document was expressed in.
	cii bool
	// slack is the tolerance R040 and R120 allow: OpenPEPPOL's literal 0.02, or
	// KoSIT's $slackValue, which is 0.5 for a HUF invoice.
	slack float64
	out   *[]Violation
}

// peppolPublished is the rule as the vendored OpenPEPPOL Schematron publishes it,
// looked up across both tables: peppolRules for the PEPPOL-* identifiers and
// peppolCountryRules for the country-specific ones in the same two files.
//
// The two are separate tables because they are separate rule sets with separate
// guards, but every emission and every severity reads them through here, so a
// country rule cannot reach a binding its file does not publish it in.
func peppolPublished(rule string) peppolRule {
	if r, ok := peppolRules[rule]; ok {
		return r
	}
	return peppolCountryRules[rule]
}

// has reports whether the artefact this evaluation quotes publishes rule for this
// binding. Every emission goes through it, so a rule cannot reach a document whose
// syntax its authority did not bind it to, and an XRechnung validation cannot
// acquire a Peppol rule KoSIT does not import.
func (e *peppolEval) has(rule string) bool {
	want := peppolUBL
	if e.cii {
		want = peppolCII
	}
	published := peppolPublished(rule).bindings&want != 0
	if !e.xr {
		return published
	}
	if !peppolXRImports[rule] {
		return false
	}
	return published || (e.cii && peppolXRCIIAdditions[rule])
}

// add records a finding, with the severity the artefact publishes for it.
func (e *peppolEval) add(rule, msg string) {
	if !e.has(rule) {
		return
	}
	sev := peppolPublished(rule).severity
	if e.xr {
		if s, ok := peppolXRFlags[rule]; ok {
			sev = s
		}
	}
	*e.out = append(*e.out, Violation{Source: SourcePeppol, Rule: rule, Severity: sev, Message: msg})
}

func (e *peppolEval) addf(rule, format string, args ...any) {
	if !e.has(rule) {
		return
	}
	e.add(rule, fmt.Sprintf(format, args...))
}

// validatePeppolRuleSet evaluates the OpenPEPPOL rule set against a parsed
// document, in the dialect xr selects: OpenPEPPOL's own Schematron when it is
// false, and the subset KoSIT merges into XRechnung when it is true.
func validatePeppolRuleSet(r *run, p *parsed, xr bool) []Violation {
	var out []Violation
	e := &peppolEval{xr: xr, cii: p.inv.syntax == "CII", slack: 0.02, out: &out}
	// $slackValue, from peppol-into-xr.xsl: "if($documentCurrencyCode = 'HUF')
	// then 0.5 else 0.02". OpenPEPPOL's own files carry the literal 0.02, so the
	// widening applies to the XRechnung path only.
	if xr && p.inv.currency == "HUF" {
		e.slack = 0.5
	}
	peppolModelRules(e, p.inv)
	if r.stopped() {
		return out
	}
	if e.cii {
		peppolCIIRules(e, r, p.root)
	} else {
		peppolUBLRules(e, r, p.root)
	}
	// The country-specific half of the same two files. It is skipped outright on
	// the XRechnung path rather than left to has(): KoSIT imports no country rule,
	// so every emission would be declined anyway, and the walk would be work done
	// to reach a gate that is always shut.
	if !xr {
		if r.stopped() {
			return out
		}
		if e.cii {
			peppolCountryCIIRules(e, r, p.root)
		} else {
			peppolCountryUBLRules(e, r, p.root)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The rules both bindings express identically over the semantic model
// ---------------------------------------------------------------------------

// peppolProfileRE is the business-process identifier pattern of the $profile
// variable both binding files declare:
//
//	matches(normalize-space(/*/cbc:ProfileID), 'urn:fdc:peppol.eu:2017:poacc:billing:([0-9]{2}):1.0')
//
// It is deliberately unanchored, because XPath's matches() is: the assertion is a
// substring search and this package used an anchored pattern, which is stricter
// than the rule and would report R007 against a BT-23 a reference validator
// accepts.
var peppolProfileRE = regexp.MustCompile(`urn:fdc:peppol\.eu:2017:poacc:billing:[0-9]{2}:1\.0`)

// peppolProfileNumber is the $profile variable: the two-digit process number of
// BT-23, or "Unknown".
//
//	if (/*/cbc:ProfileID and matches(normalize-space(/*/cbc:ProfileID), '...'))
//	then tokenize(normalize-space(/*/cbc:ProfileID), ':')[7] else 'Unknown'
//
// The seventh colon-separated token of a well-formed identifier is the process
// number; for a value that matches the pattern only as a substring it is
// something else, which is exactly what the Schematron computes and why P0100 and
// P0101 then apply to nothing.
func peppolProfileNumber(profileID string) string {
	id := strings.Join(strings.Fields(profileID), " ")
	if id == "" || !peppolProfileRE.MatchString(id) {
		return "Unknown"
	}
	parts := strings.Split(id, ":")
	if len(parts) < 7 {
		return "Unknown"
	}
	return parts[6]
}

// peppolModelRules are the Peppol rules whose subject is a business term the
// syntax-neutral model already carries, and whose two bindings say the same thing
// about it.
func peppolModelRules(e *peppolEval, inv *en16931Invoice) {
	// R007, context ubl:Invoice | cn:CreditNote / rsm:ExchangedDocumentContext:
	//   $profile != 'Unknown'
	//
	// R001 is the other assertion of the same Schematron rule and is an existence
	// test, so it is on the tree with the other four — see peppolExistenceRules.
	// The two are independent assertions, so a document with no BT-23 at all trips
	// both; this package reported only R001, because it read the second as the
	// else-branch of the first.
	if peppolProfileNumber(inv.profileID) == "Unknown" {
		e.add("PEPPOL-EN16931-R007", "The Business process (BT-23) MUST be 'urn:fdc:peppol.eu:2017:poacc:billing:NN:1.0'")
	}
	// R004: starts-with(normalize-space(cbc:CustomizationID), '...') and, in CII,
	// the same over ram:GuidelineSpecifiedDocumentContextParameter/ram:ID.
	if !strings.HasPrefix(strings.TrimSpace(inv.specID), peppolSpecID) {
		e.add("PEPPOL-EN16931-R004", "The Specification identifier (BT-24) MUST be the Peppol BIS Billing 3.0 identifier")
	}
	// R005, context cbc:TaxCurrencyCode / ram:ApplicableHeaderTradeSettlement:
	//   not(normalize-space(text()) = normalize-space(../cbc:DocumentCurrencyCode/text()))
	if inv.taxCurrency != "" && inv.currency != "" && inv.taxCurrency == inv.currency {
		e.add("PEPPOL-EN16931-R005", "The VAT accounting currency code (BT-6) MUST differ from the invoice currency code (BT-5)")
	}
	// R110 / R111 contain the line period (BG-26) within the invoicing period
	// (BG-14). Both ends are ordered only when both parse as calendar dates; an
	// unreadable date cannot place the line inside or outside the period, so it is
	// not reported either way.
	for _, li := range inv.lines {
		if !li.period.present || !inv.period.present {
			continue
		}
		lineStart, okLine := normDate(li.period.start)
		invStart, okInv := normDate(inv.period.start)
		if okLine && okInv && lineStart < invStart {
			e.add("PEPPOL-EN16931-R110", "The Invoice line period start date (BT-134) MUST be within the Invoicing period (BT-73)")
		}
		lineEnd, okLine := normDate(li.period.end)
		invEnd, okInv := normDate(inv.period.end)
		if okLine && okInv && lineEnd > invEnd {
			e.add("PEPPOL-EN16931-R111", "The Invoice line period end date (BT-135) MUST be within the Invoicing period (BT-74)")
		}
	}
}

// ---------------------------------------------------------------------------
// The UBL binding
// ---------------------------------------------------------------------------

// peppolUBLNodes is every node population the UBL rules read, gathered in one
// walk of the tree.
//
// Eleven of these rules have a document-wide context — `//*`, `cbc:CompanyID`,
// `cbc:Amount` — and this package sweeps 1,690 documents through several
// validators per test run, so one walk that dispatches on the local name costs
// what the largest population costs rather than what the sum of them does.
// gatherUBLSyntaxNodes in en16931_ubl_rules.go makes the same argument.
type peppolUBLNodes struct {
	root         *ciiNode
	isCreditNote bool

	// empty is `//*[not(*) and not(normalize-space())]`, R008's whole context.
	empty []*ciiNode
	// schemeIDs is the union of the three contexts every PEPPOL-COMMON-* rule
	// shares — cbc:EndpointID, cac:PartyIdentification/cbc:ID, cbc:CompanyID —
	// and endpoints is the subset R046 and CL008 are bound to (cbc:EndpointID
	// alone).
	schemeIDs []*ciiNode
	endpoints []*ciiNode

	taxCategories  []*ciiNode // cac:TaxCategory (P0104..P0111)
	binaryObjects  []*ciiNode // cbc:EmbeddedDocumentBinaryObject[@mimeCode] (CL001)
	periodDescs    []*ciiNode // cac:InvoicePeriod/cbc:DescriptionCode (CL006)
	invoiceTypes   []*ciiNode // cbc:InvoiceTypeCode (P0100, P0112)
	creditTypes    []*ciiNode // cbc:CreditNoteTypeCode (P0101)
	dates          []*ciiNode // the six date elements F001 is bound to
	amounts        []peppolUBLAmount
	allowCharges   []*ciiNode // the document- and line-level cac:AllowanceCharge
	priceCharges   []*ciiNode // cac:Price/cac:AllowanceCharge (R044, R046)
	lines          []*ciiNode // cac:InvoiceLine | cac:CreditNoteLine
	baseQuantities []*ciiNode // cac:Price/cbc:BaseQuantity[@unitCode] (R130)
	paymentMeans   []*ciiNode // cac:PaymentMeans (R061)
}

// peppolUBLAmount is one amount-bearing element, with the one fact that tells
// CL007's context from R051's apart.
type peppolUBLAmount struct {
	node *ciiNode
	// inR051 is whether R051's context list holds this node. The two lists are
	// identical but for cbc:TaxAmount: CL007 takes every one of them, R051 takes
	// only `cac:TaxTotal[cac:TaxSubtotal]/cbc:TaxAmount | cac:TaxSubtotal/
	// cbc:TaxAmount`, so that the Invoice total VAT amount in accounting currency
	// (BT-111) — the one amount whose @currencyID is deliberately not BT-5 — is
	// out of scope. That exclusion is the whole point of the rule.
	inR051 bool
}

// peppolUBLAmountNames is CL007's context list.
var peppolUBLAmountNames = map[string]bool{
	"Amount": true, "BaseAmount": true, "PriceAmount": true, "TaxAmount": true, "TaxableAmount": true,
	"LineExtensionAmount": true, "TaxExclusiveAmount": true, "TaxInclusiveAmount": true,
	"AllowanceTotalAmount": true, "ChargeTotalAmount": true, "PrepaidAmount": true,
	"PayableRoundingAmount": true, "PayableAmount": true,
}

// peppolUBLDateNames is F001's context list.
var peppolUBLDateNames = map[string]bool{
	"IssueDate": true, "DueDate": true, "TaxPointDate": true,
	"StartDate": true, "EndDate": true, "ActualDeliveryDate": true,
}

func gatherPeppolUBLNodes(root *ciiNode) *peppolUBLNodes {
	g := &peppolUBLNodes{root: root, isCreditNote: root.name == "CreditNote"}
	var walk func(n, parent *ciiNode)
	walk = func(n, parent *ciiNode) {
		if len(n.children) == 0 && strings.TrimSpace(n.text) == "" {
			g.empty = append(g.empty, n)
		}
		switch n.name {
		case "EndpointID":
			g.schemeIDs = append(g.schemeIDs, n)
			g.endpoints = append(g.endpoints, n)
		case "CompanyID":
			g.schemeIDs = append(g.schemeIDs, n)
		case "ID":
			if parent != nil && parent.name == "PartyIdentification" {
				g.schemeIDs = append(g.schemeIDs, n)
			}
		case "TaxCategory":
			g.taxCategories = append(g.taxCategories, n)
		case "EmbeddedDocumentBinaryObject":
			if n.hasAttr("mimeCode") {
				g.binaryObjects = append(g.binaryObjects, n)
			}
		case "DescriptionCode":
			if parent != nil && parent.name == "InvoicePeriod" {
				g.periodDescs = append(g.periodDescs, n)
			}
		case "InvoiceTypeCode":
			g.invoiceTypes = append(g.invoiceTypes, n)
		case "CreditNoteTypeCode":
			g.creditTypes = append(g.creditTypes, n)
		case "AllowanceCharge":
			switch {
			case parent == nil:
			case parent.name == "Price":
				g.priceCharges = append(g.priceCharges, n)
			case parent == root, parent.name == "InvoiceLine", parent.name == "CreditNoteLine":
				g.allowCharges = append(g.allowCharges, n)
			}
		case "InvoiceLine", "CreditNoteLine":
			g.lines = append(g.lines, n)
		case "BaseQuantity":
			if parent != nil && parent.name == "Price" && n.hasAttr("unitCode") {
				g.baseQuantities = append(g.baseQuantities, n)
			}
		case "PaymentMeans":
			g.paymentMeans = append(g.paymentMeans, n)
		}
		if peppolUBLAmountNames[n.name] {
			inR051 := true
			if n.name == "TaxAmount" {
				inR051 = parent != nil && (parent.name == "TaxSubtotal" ||
					(parent.name == "TaxTotal" && parent.child("TaxSubtotal") != nil))
			}
			g.amounts = append(g.amounts, peppolUBLAmount{node: n, inR051: inR051})
		}
		if peppolUBLDateNames[n.name] {
			g.dates = append(g.dates, n)
		}
		for _, c := range n.children {
			walk(c, n)
		}
	}
	walk(root, nil)
	return g
}

// peppolUBLRules evaluates the UBL binding of PEPPOL-EN16931-UBL.sch. It is a
// no-op for any other root, so a CII invoice reaching the same entry point is
// never asked to answer a UBL rule.
func peppolUBLRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return
	}
	g := gatherPeppolUBLNodes(root)

	peppolUBLDocumentRules(e, g)
	if r.stopped() {
		return
	}
	peppolCommonIdentifierRules(e, g.schemeIDs, g.endpoints)
	if r.stopped() {
		return
	}
	peppolUBLCodeListRules(e, g)
	if r.stopped() {
		return
	}
	peppolUBLAllowanceChargeRules(e, g)
	if r.stopped() {
		return
	}
	peppolUBLLineRules(e, g)
}

// peppolExistenceRules are the four Peppol assertions that test whether an element
// is *there*, not whether it has a value.
//
// They read the tree and not the model, and the reason is a false positive
// OpenPEPPOL's own test suite catches: every one of its fixtures for these four
// rules proves the rule passes by writing the element empty — `<cbc:ProfileID/>`,
// `<cbc:EndpointID/>` — because the Schematron test is the bare path `cbc:ProfileID`
// and an empty element satisfies it. Read off the semantic model, where an absent
// term and an empty one are both the empty string, all four reported a document
// OpenPEPPOL accepts.
//
// R010 and R020 also depend on their context existing: the assertion hangs off the
// party group, so an invoice with no cac:AccountingCustomerParty at all is not
// reported for a missing buyer electronic address. EN 16931's BR-07 is the rule
// with something to say about that.
func peppolExistenceRules(e *peppolEval, root *ciiNode, cii bool) {
	var buyers, sellers []*ciiNode
	if cii {
		// R001: rsm:ExchangedDocumentContext/ram:BusinessProcessSpecifiedDocumentContextParameter/ram:ID
		if len(nodesAt(root, "ExchangedDocumentContext",
			"BusinessProcessSpecifiedDocumentContextParameter", "ID")) == 0 {
			e.add("PEPPOL-EN16931-R001", "The Business process (BT-23) MUST be provided")
		}
		// R003, context ram:ApplicableHeaderTradeAgreement.
		for _, agr := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement") {
			if agr.child("BuyerReference") == nil &&
				len(nodesAt(agr, "BuyerOrderReferencedDocument", "IssuerAssignedID")) == 0 {
				e.add("PEPPOL-EN16931-R003", "A Buyer reference (BT-10) or purchase order reference (BT-13) MUST be provided")
			}
			buyers = append(buyers, agr.all("BuyerTradeParty")...)
			sellers = append(sellers, agr.all("SellerTradeParty")...)
		}
	} else {
		// R001 / R003, context ubl:Invoice | cn:CreditNote.
		if root.child("ProfileID") == nil {
			e.add("PEPPOL-EN16931-R001", "The Business process (BT-23) MUST be provided")
		}
		if root.child("BuyerReference") == nil && len(nodesAt(root, "OrderReference", "ID")) == 0 {
			e.add("PEPPOL-EN16931-R003", "A Buyer reference (BT-10) or purchase order reference (BT-13) MUST be provided")
		}
		buyers = nodesAt(root, "AccountingCustomerParty", "Party")
		sellers = nodesAt(root, "AccountingSupplierParty", "Party")
	}
	// R010 / R020: cbc:EndpointID under each party in UBL,
	// ram:URIUniversalCommunication/ram:URIID in CII.
	present := func(party *ciiNode) bool {
		if cii {
			return len(nodesAt(party, "URIUniversalCommunication", "URIID")) > 0
		}
		return party.child("EndpointID") != nil
	}
	for _, p := range buyers {
		if !present(p) {
			e.add("PEPPOL-EN16931-R010", "The Buyer electronic address (BT-49) MUST be provided")
		}
	}
	for _, p := range sellers {
		if !present(p) {
			e.add("PEPPOL-EN16931-R020", "The Seller electronic address (BT-34) MUST be provided")
		}
	}
}

// peppolUBLDocumentRules are the rules bound to the document element, plus the
// two whose context is `//*` and `cbc:InvoiceTypeCode`.
func peppolUBLDocumentRules(e *peppolEval, g *peppolUBLNodes) {
	root := g.root
	peppolExistenceRules(e, root, false)

	// R008, context //*[not(*) and not(normalize-space())]: false().
	//
	// The assertion cannot pass, so the context is the rule: every element with no
	// child element and no non-whitespace content is a finding. One per element, as
	// a Schematron processor reports it.
	if e.has("PEPPOL-EN16931-R008") {
		for _, n := range g.empty {
			e.addf("PEPPOL-EN16931-R008", "The element %q is empty; a document MUST NOT contain empty elements", n.name)
		}
	}

	// R002, context ubl:Invoice | cn:CreditNote:
	//   count(cbc:Note) <= 1 or ($supplierCountryIsDE and $customerCountryIsDE)
	if len(root.all("Note")) > 1 && !(peppolUBLCountryIsDE(root, "AccountingSupplierParty") &&
		peppolUBLCountryIsDE(root, "AccountingCustomerParty")) {
		e.add("PEPPOL-EN16931-R002", "No more than one Invoice note (BT-22) is allowed on document level, "+
			"unless both the Buyer and the Seller are German organisations")
	}

	// R080, context cn:CreditNote — the UBL binding publishes this rule for a
	// credit note only, so an invoice with two project references is not reported:
	//   count(cac:AdditionalDocumentReference[cbc:DocumentTypeCode='50']) <= 1
	if g.isCreditNote && peppolUBLCountDocRefs(root, "50") > 1 {
		e.add("PEPPOL-EN16931-R080", "Only one project reference (BT-11) is allowed on document level")
	}

	// R053: count(cac:TaxTotal[cac:TaxSubtotal]) = 1
	// R054: count(cac:TaxTotal[not(cac:TaxSubtotal)]) = (if (cbc:TaxCurrencyCode) then 1 else 0)
	withSub, withoutSub := 0, 0
	for _, tt := range root.all("TaxTotal") {
		if tt.child("TaxSubtotal") != nil {
			withSub++
		} else {
			withoutSub++
		}
	}
	if withSub != 1 {
		e.addf("PEPPOL-EN16931-R053", "Exactly one VAT total (cac:TaxTotal with cac:TaxSubtotal) MUST be provided; found %d", withSub)
	}
	wantWithout := 0
	if root.child("TaxCurrencyCode") != nil {
		wantWithout = 1
	}
	if withoutSub != wantWithout {
		e.addf("PEPPOL-EN16931-R054", "%d VAT totals without tax subtotals are present; %d is required when the "+
			"VAT accounting currency code (BT-6) is %s", withoutSub, wantWithout,
			map[bool]string{true: "provided", false: "absent"}[wantWithout == 1])
	}

	// R055, context ubl:Invoice | cn:CreditNote:
	//   not(cbc:TaxCurrencyCode) or
	//   (cac:TaxTotal/cbc:TaxAmount[@currencyID=$tax] <= 0 and cac:TaxTotal/cbc:TaxAmount[@currencyID=$doc] <= 0) or
	//   (... >= 0 and ... >= 0)
	//
	// Both halves are node-set comparisons, so each is "some node satisfies", and a
	// currency with no VAT total at all satisfies neither — which is why an invoice
	// declaring BT-6 and carrying no amount in it is reported.
	if tc := root.child("TaxCurrencyCode"); tc != nil {
		taxCur, docCur := tc.rawText(), root.child("DocumentCurrencyCode").rawText()
		taxAmts := nodesAt(root, "TaxTotal", "TaxAmount")
		if !(peppolAnySign(taxAmts, taxCur, false) && peppolAnySign(taxAmts, docCur, false)) &&
			!(peppolAnySign(taxAmts, taxCur, true) && peppolAnySign(taxAmts, docCur, true)) {
			e.add("PEPPOL-EN16931-R055", "The Invoice total VAT amount (BT-110) and the same amount in the VAT "+
				"accounting currency (BT-111) MUST have the same operational sign")
		}
	}

	// P0100, context cbc:InvoiceTypeCode; P0112 in the same rule.
	profile := peppolProfileNumber(root.str("ProfileID"))
	for _, tc := range g.invoiceTypes {
		code := strings.Join(strings.Fields(tc.text), " ")
		if profile == "01" && !peppolInvoiceTypeCodes[code] {
			e.addf("PEPPOL-EN16931-P0100", "The Invoice type code (BT-3=%q) is not one of the codes profile 01 permits", code)
		}
		// P0112: not(normalize-space(.) = '326' or normalize-space(.) = '384') or
		//        ($supplierCountryIsDE and $customerCountryIsDE)
		if (code == "326" || code == "384") && !(peppolUBLCountryIsDE(root, "AccountingSupplierParty") &&
			peppolUBLCountryIsDE(root, "AccountingCustomerParty")) {
			e.addf("PEPPOL-EN16931-P0112", "The Invoice type code %q is only allowed when both the Buyer and the "+
				"Seller are German organisations", code)
		}
	}
	// P0101, context cbc:CreditNoteTypeCode.
	for _, tc := range g.creditTypes {
		code := strings.Join(strings.Fields(tc.text), " ")
		if profile == "01" && !peppolCreditNoteTypeCodes[code] {
			e.addf("PEPPOL-EN16931-P0101", "The Credit note type code (BT-3=%q) is not one of the codes profile 01 permits", code)
		}
	}

	// F001, context cbc:IssueDate | cbc:DueDate | cbc:TaxPointDate | cbc:StartDate
	// | cbc:EndDate | cbc:ActualDeliveryDate:
	//   string-length(text()) = 10 and (string(.) castable as xs:date)
	//
	// The length is over the raw text and not the normalized value, so a date
	// padded with whitespace fails even though it would still cast.
	for _, d := range g.dates {
		if !peppolIsCalendarDate(d.rawText()) {
			e.addf("PEPPOL-EN16931-F001", "The date %q in %s MUST be formatted YYYY-MM-DD", d.rawText(), d.name)
		}
	}
}

// peppolUBLCountryIsDE is $supplierCountryIsDE / $customerCountryIsDE:
//
//	upper-case(normalize-space(/*/cac:<party>/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode)) = 'DE'
func peppolUBLCountryIsDE(root *ciiNode, party string) bool {
	return strings.EqualFold(root.str(party, "Party", "PostalAddress", "Country", "IdentificationCode"), "DE")
}

// peppolUBLCountDocRefs counts `cac:AdditionalDocumentReference[cbc:DocumentTypeCode=$code]`.
func peppolUBLCountDocRefs(root *ciiNode, code string) int {
	n := 0
	for _, ref := range root.all("AdditionalDocumentReference") {
		for _, tc := range ref.all("DocumentTypeCode") {
			if tc.rawText() == code {
				n++
				break
			}
		}
	}
	return n
}

// peppolAnySign is one half of R055's node-set comparison: does any amount
// carrying this currency identifier compare the requested way against zero.
// A node whose value is not a number compares false either way, as NaN does.
func peppolAnySign(amounts []*ciiNode, currency string, nonNegative bool) bool {
	for _, a := range amounts {
		if a.attr("currencyID") != currency {
			continue
		}
		v, ok := parseAmount(a.text)
		if !ok {
			continue
		}
		if (nonNegative && v >= 0) || (!nonNegative && v <= 0) {
			return true
		}
	}
	return false
}

// peppolUBLCodeListRules are the PEPPOL-EN16931-CL* restrictions and the
// VAT-exemption-reason rules P0104..P0111, which the CII binding does not publish.
func peppolUBLCodeListRules(e *peppolEval, g *peppolUBLNodes) {
	// CL001, context cbc:EmbeddedDocumentBinaryObject[@mimeCode].
	for _, b := range g.binaryObjects {
		if !en16931MIME[b.attr("mimeCode")] {
			e.addf("PEPPOL-EN16931-CL001", "The attachment MIME code %q is not in the subset of the IANA list Peppol permits", b.attr("mimeCode"))
		}
	}
	// CL002 / CL003, context cac:AllowanceCharge[cbc:ChargeIndicator = 'false'|
	// 'true']/cbc:AllowanceChargeReasonCode. The predicate is a comparison against
	// the element's string value with no normalization, so only the exact literal
	// selects the group — and the context is every cac:AllowanceCharge, price level
	// included, unlike R040..R043.
	for _, ac := range append(append([]*ciiNode{}, g.allowCharges...), g.priceCharges...) {
		charge := ac.child("ChargeIndicator").rawText()
		for _, rc := range ac.all("AllowanceChargeReasonCode") {
			code := strings.Join(strings.Fields(rc.text), " ")
			switch charge {
			case "false":
				if !en16931AllowanceReasons[code] {
					e.addf("PEPPOL-EN16931-CL002", "The allowance reason code (BT-98=%q) is not in the subset of UNCL 5189 Peppol permits", code)
				}
			case "true":
				if !en16931ChargeReasons[code] {
					e.addf("PEPPOL-EN16931-CL003", "The charge reason code (BT-105=%q) is not in UNCL 7161", code)
				}
			}
		}
	}
	// CL006, context cac:InvoicePeriod/cbc:DescriptionCode.
	for _, d := range g.periodDescs {
		code := strings.Join(strings.Fields(d.text), " ")
		if !peppolPeriodDescCodes[code] {
			e.addf("PEPPOL-EN16931-CL006", "The invoice period description code (BT-8=%q) is not in UNCL 2005", code)
		}
	}
	// CL007: every amount's @currencyID must be an ISO 4217 code. An amount with
	// no @currencyID at all satisfies no code and is reported, which is what the
	// `some $code ... satisfies @currencyID = $code` form says.
	for _, a := range g.amounts {
		if !peppolCurrencies[a.node.attr("currencyID")] {
			e.addf("PEPPOL-EN16931-CL007", "The currency identifier %q on %s is not an ISO 4217 code", a.node.attr("currencyID"), a.node.name)
		}
	}
	// CL008, context cbc:EndpointID[@schemeID].
	for _, ep := range g.endpoints {
		if !ep.hasAttr("schemeID") {
			continue
		}
		if !peppolEAS[ep.attr("schemeID")] {
			e.addf("PEPPOL-EN16931-CL008", "The electronic address scheme identifier %q is not in Peppol's "+
				"Electronic Address Identifier Scheme list", ep.attr("schemeID"))
		}
	}
	// R051: every amount in scope must carry the invoice currency (BT-5).
	docCurrency := g.root.child("DocumentCurrencyCode").rawText()
	for _, a := range g.amounts {
		if !a.inR051 {
			continue
		}
		if a.node.attr("currencyID") != docCurrency {
			e.addf("PEPPOL-EN16931-R051", "The currency identifier %q on %s differs from the invoice currency "+
				"code (BT-5=%q)", a.node.attr("currencyID"), a.node.name, docCurrency)
		}
	}
	// P0104..P0111, context cac:TaxCategory[upper-case(cbc:TaxExemptionReasonCode)
	// = 'VATEX-EU-x']: normalize-space(cbc:ID) = '<category>'.
	//
	// The context is every cac:TaxCategory, so a VAT breakdown group and an
	// allowance's or charge's tax category are both in scope; reading these off the
	// model's vatBreakdowns saw only the first of those.
	for _, tc := range g.taxCategories {
		reason := strings.ToUpper(tc.child("TaxExemptionReasonCode").rawText())
		m, ok := peppolVATEX[reason]
		if !ok {
			continue
		}
		if got := tc.str("ID"); got != m.category {
			e.addf(m.rule, "The VAT exemption reason code %q requires VAT category %q, not %q", reason, m.category, got)
		}
	}
}

// peppolUBLAllowanceChargeRules are R040..R043 over the document- and line-level
// allowances and charges, and R044/R046 over the price-level ones.
func peppolUBLAllowanceChargeRules(e *peppolEval, g *peppolUBLNodes) {
	for _, ac := range g.allowCharges {
		pct := ac.child("MultiplierFactorNumeric")
		base := ac.child("BaseAmount")
		switch {
		// R041, context cac:AllowanceCharge[cbc:MultiplierFactorNumeric and
		// not(cbc:BaseAmount)]: false().
		case pct != nil && base == nil:
			e.add("PEPPOL-EN16931-R041", "The allowance/charge base amount (BT-93/BT-100) MUST be provided when "+
				"the percentage (BT-94/BT-101) is")
		// R042, the mirror image.
		case pct == nil && base != nil:
			e.add("PEPPOL-EN16931-R042", "The allowance/charge percentage (BT-94/BT-101) MUST be provided when "+
				"the base amount (BT-93/BT-100) is")
		// R040: u:slack(if (cbc:Amount) then cbc:Amount else 0,
		//               (xs:decimal(cbc:BaseAmount) * xs:decimal(cbc:MultiplierFactorNumeric)) div 100, 0.02)
		case pct != nil && base != nil:
			b, okB := parseAmount(base.text)
			p, okP := parseAmount(pct.text)
			amount := 0.0
			if a := ac.child("Amount"); a != nil {
				v, ok := parseAmount(a.text)
				if !ok {
					continue
				}
				amount = v
			}
			if okB && okP && !peppolSlack(amount, b*p/100, e.slack) {
				e.addf("PEPPOL-EN16931-R040", "The allowance/charge amount %.2f MUST equal the base amount %.2f × %s%%", amount, b, pct.rawText())
			}
		}
		// R043: normalize-space(cbc:ChargeIndicator/text()) = 'true' or 'false'.
		if ind := strings.Join(strings.Fields(ac.child("ChargeIndicator").rawText()), " "); ind != "true" && ind != "false" {
			e.addf("PEPPOL-EN16931-R043", "The allowance/charge indicator %q MUST be 'true' or 'false'", ind)
		}
	}
	for _, ac := range g.priceCharges {
		// R044, context cac:Price/cac:AllowanceCharge:
		//   normalize-space(cbc:ChargeIndicator) = 'false'
		if strings.Join(strings.Fields(ac.child("ChargeIndicator").rawText()), " ") != "false" {
			e.add("PEPPOL-EN16931-R044", "A charge on price level is not allowed: cac:Price/cac:AllowanceCharge/"+
				"cbc:ChargeIndicator MUST be 'false'")
		}
		// R046: not(cbc:BaseAmount) or
		//       xs:decimal(../cbc:PriceAmount) = xs:decimal(cbc:BaseAmount) - xs:decimal(cbc:Amount)
		base := ac.child("BaseAmount")
		if base == nil {
			continue
		}
		gross, okG := parseAmount(base.text)
		disc, okD := parseAmount(ac.str("Amount"))
		// ../cbc:PriceAmount is the sibling of the allowance, on the cac:Price.
		netPrice, okNet := parseAmount(peppolUBLPriceAmount(ac, g))
		if okG && okD && okNet && !peppolDecimalEqual(netPrice, gross-disc) {
			e.addf("PEPPOL-EN16931-R046", "The Item net price (BT-146=%.4f) MUST equal the gross price (BT-148=%.4f) "+
				"less the price discount (BT-147=%.4f)", netPrice, gross, disc)
		}
	}
}

// peppolUBLPriceAmount is `../cbc:PriceAmount` from a price-level allowance: the
// cbc:PriceAmount of the cac:Price the allowance hangs off. The parent is not
// recorded on the node, so it is found by looking for the cac:Price that holds
// this allowance.
func peppolUBLPriceAmount(allowance *ciiNode, g *peppolUBLNodes) string {
	for _, li := range g.lines {
		for _, price := range li.all("Price") {
			for _, ac := range price.all("AllowanceCharge") {
				if ac == allowance {
					return price.str("PriceAmount")
				}
			}
		}
	}
	return ""
}

// peppolUBLLineRules are the per-line rules R100, R101, R120, R121 and R130, and
// R061 over the payment means.
func peppolUBLLineRules(e *peppolEval, g *peppolUBLNodes) {
	for _, li := range g.lines {
		refs := li.all("DocumentReference")
		// R100: count(cac:DocumentReference) <= 1
		if len(refs) > 1 {
			e.addf("PEPPOL-EN16931-R100", "Only one invoiced object identifier (BT-128) is allowed per line; found %d", len(refs))
		}
		// R101: not(cac:DocumentReference) or cac:DocumentReference/cbc:DocumentTypeCode = '130'
		if len(refs) > 0 {
			ok := false
			for _, ref := range refs {
				for _, tc := range ref.all("DocumentTypeCode") {
					if tc.rawText() == "130" {
						ok = true
					}
				}
			}
			if !ok {
				e.add("PEPPOL-EN16931-R101", "A line's cac:DocumentReference may only carry the invoiced object "+
					"identifier (BT-128), whose document type code is 130")
			}
		}
		// R121, context cac:InvoiceLine | cac:CreditNoteLine:
		//   not(cac:Price/cbc:BaseQuantity) or xs:decimal(cac:Price/cbc:BaseQuantity) > 0
		if bq := li.child("Price", "BaseQuantity"); bq != nil {
			if v, ok := parseAmount(bq.text); ok && v <= 0 {
				e.addf("PEPPOL-EN16931-R121", "The Item price base quantity (BT-149=%s) MUST be greater than zero", bq.rawText())
			}
		}
		peppolUBLLineTotal(e, g, li)
	}
	// R130, context cac:Price/cbc:BaseQuantity[@unitCode]:
	//   not($hasQuantity) or @unitCode = $quantity/@unitCode
	// where $hasQuantity is `../../cbc:InvoicedQuantity or ../../cbc:CreditedQuantity`
	// and $quantity is whichever of the two the document element selects.
	qtyName := "InvoicedQuantity"
	if g.isCreditNote {
		qtyName = "CreditedQuantity"
	}
	for _, bq := range g.baseQuantities {
		li := peppolUBLLineOf(bq, g)
		if li == nil {
			continue
		}
		if li.child("InvoicedQuantity") == nil && li.child("CreditedQuantity") == nil {
			continue
		}
		// `@unitCode = $quantity/@unitCode` is false when either side is an empty
		// sequence, so a credit note line carrying cbc:InvoicedQuantity — the
		// element the *invoice* uses — is reported even though it has a unit code:
		// $quantity selects cbc:CreditedQuantity, which is absent.
		q := li.child(qtyName)
		if q == nil || !q.hasAttr("unitCode") || bq.attr("unitCode") != q.attr("unitCode") {
			e.addf("PEPPOL-EN16931-R130", "The Item price base quantity unit (BT-150=%q) MUST equal the invoiced "+
				"quantity unit (BT-130=%q)", bq.attr("unitCode"), q.attr("unitCode"))
		}
	}
	// R061, context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = '49'
	// or '59']: cac:PaymentMandate/cbc:ID.
	//
	// The context is the payment means code and not the presence of a mandate,
	// which is the difference between this rule and the one this package had: read
	// off the model it fired only when a cac:PaymentMandate existed *without* an
	// identifier, so the case the rule exists for — a direct debit with no mandate
	// group at all — was reported by nothing.
	for _, pm := range g.paymentMeans {
		code := strings.Join(strings.Fields(pm.child("PaymentMeansCode").rawText()), " ")
		if code != "49" && code != "59" {
			continue
		}
		if pm.child("PaymentMandate", "ID") == nil {
			e.add("PEPPOL-EN16931-R061", "A Mandate reference identifier (BT-89) MUST be provided for a direct debit")
		}
	}
}

// peppolUBLLineOf finds the line a cac:Price/cbc:BaseQuantity belongs to.
func peppolUBLLineOf(baseQty *ciiNode, g *peppolUBLNodes) *ciiNode {
	for _, li := range g.lines {
		for _, price := range li.all("Price") {
			for _, bq := range price.all("BaseQuantity") {
				if bq == baseQty {
					return li
				}
			}
		}
	}
	return nil
}

// peppolUBLLineTotal is R120, context cac:InvoiceLine | cac:CreditNoteLine:
//
//	u:slack($lineExtensionAmount, ($quantity * ($priceAmount div $baseQuantity)) + $chargesTotal - $allowancesTotal, 0.02)
//
// with the five <let> variables of the same rule resolved. Each defaults rather
// than failing when its element is absent — a missing quantity is 1, a missing
// price 0 — so the rule still has an opinion about an incomplete line, and a base
// quantity of zero is read as 1 rather than dividing by it.
func peppolUBLLineTotal(e *peppolEval, g *peppolUBLNodes, li *ciiNode) {
	if !e.has("PEPPOL-EN16931-R120") {
		return
	}
	lineTotal, ok := peppolDecimalOr(li.child("LineExtensionAmount"), 0)
	if !ok {
		return
	}
	qtyName := "InvoicedQuantity"
	if g.isCreditNote {
		qtyName = "CreditedQuantity"
	}
	quantity, ok := peppolDecimalOr(li.child(qtyName), 1)
	if !ok {
		return
	}
	price, ok := peppolDecimalOr(li.child("Price", "PriceAmount"), 0)
	if !ok {
		return
	}
	baseQty, ok := peppolDecimalOr(li.child("Price", "BaseQuantity"), 1)
	if !ok {
		return
	}
	if baseQty == 0 {
		baseQty = 1
	}
	allowances, ok1 := peppolUBLChargeSum(li, "false")
	charges, ok2 := peppolUBLChargeSum(li, "true")
	if !ok1 || !ok2 {
		return
	}
	want := quantity*(price/baseQty) + charges - allowances
	if !peppolSlack(lineTotal, want, e.slack) {
		e.addf("PEPPOL-EN16931-R120", "The Invoice line net amount (BT-131=%.2f) MUST equal the invoiced quantity × "+
			"(item net price ÷ base quantity) plus line charges less line allowances (%.2f)", lineTotal, want)
	}
}

// peppolUBLChargeSum is R120's $allowancesTotal / $chargesTotal:
//
//	if (cac:AllowanceCharge[normalize-space(cbc:ChargeIndicator) = $ind]) then
//	  round(sum(cac:AllowanceCharge[...]/cbc:Amount/xs:decimal(.)) * 10 * 10) div 100
//	else 0
//
// The rounding is XPath's round(), which is half-up towards positive infinity and
// not Go's half-away-from-zero, so it is written out rather than taken from math.
func peppolUBLChargeSum(li *ciiNode, indicator string) (float64, bool) {
	sum, any := 0.0, false
	for _, ac := range li.all("AllowanceCharge") {
		if strings.Join(strings.Fields(ac.child("ChargeIndicator").rawText()), " ") != indicator {
			continue
		}
		any = true
		for _, a := range ac.all("Amount") {
			v, ok := parseAmount(a.text)
			if !ok {
				return 0, false
			}
			sum += v
		}
	}
	if !any {
		return 0, true
	}
	return math.Floor(sum*100+0.5) / 100, true
}

// ---------------------------------------------------------------------------
// The CII binding
// ---------------------------------------------------------------------------

// peppolCIINodes is the CII counterpart of peppolUBLNodes.
type peppolCIINodes struct {
	root      *ciiNode
	empty     []*ciiNode // KoSIT's R008 addition
	schemeIDs []*ciiNode // ram:URIID | ram:ID | ram:GlobalID
	uriIDs    []*ciiNode // ram:URIID alone (R046)

	settlement []*ciiNode // ram:ApplicableHeaderTradeSettlement
	agreements []*ciiNode // ram:ApplicableHeaderTradeAgreement
	documents  []*ciiNode // rsm:ExchangedDocument
	typeCodes  []*ciiNode // ram:ExchangedDocument/ram:TypeCode (P0100)

	specifiedCharges []*ciiNode // ram:SpecifiedTradeAllowanceCharge
	appliedCharges   []*ciiNode // ram:AppliedTradeAllowanceCharge
	attachments      []*ciiNode // ram:AttachmentBinaryObject[@mimeCode]
	taxTotalAmounts  []*ciiNode // ram:TaxTotalAmount[@currencyID] (CL007)
	dateStrings      []*ciiNode // udt:DateTimeString (F001)
	endpointURIIDs   []*ciiNode // the two parties' ram:URIID (CL008)
	lines            []*ciiNode // ram:IncludedSupplyChainTradeLineItem
	prices           []*ciiNode // ram:NetPriceProductTradePrice | ram:GrossPriceProductTradePrice
	grossPrices      []*ciiNode // ram:GrossPriceProductTradePrice (KoSIT's R044/R046)
	paymentMeans     []*ciiNode // ram:SpecifiedTradeSettlementPaymentMeans
}

func gatherPeppolCIINodes(root *ciiNode) *peppolCIINodes {
	g := &peppolCIINodes{root: root}
	var walk func(n, parent *ciiNode)
	walk = func(n, parent *ciiNode) {
		// KoSIT's R008 for CII excludes one element by name:
		//   //*[not(name() = 'ram:ApplicableHeaderTradeDelivery') and not(*) and not(normalize-space())]
		// The exclusion is written against the qualified name, and this package
		// walks by local name throughout, so it is matched on the local name — which
		// is the same test for any document using the ram: prefix the CII namespace
		// is conventionally bound to.
		if len(n.children) == 0 && strings.TrimSpace(n.text) == "" && n.name != "ApplicableHeaderTradeDelivery" {
			g.empty = append(g.empty, n)
		}
		switch n.name {
		case "URIID":
			g.schemeIDs = append(g.schemeIDs, n)
			g.uriIDs = append(g.uriIDs, n)
			if parent != nil && parent.name == "URIUniversalCommunication" {
				g.endpointURIIDs = append(g.endpointURIIDs, n)
			}
		case "ID", "GlobalID":
			g.schemeIDs = append(g.schemeIDs, n)
		case "ApplicableHeaderTradeSettlement":
			g.settlement = append(g.settlement, n)
		case "ApplicableHeaderTradeAgreement":
			g.agreements = append(g.agreements, n)
		case "ExchangedDocument":
			g.documents = append(g.documents, n)
			g.typeCodes = append(g.typeCodes, n.all("TypeCode")...)
		case "SpecifiedTradeAllowanceCharge":
			g.specifiedCharges = append(g.specifiedCharges, n)
		case "AppliedTradeAllowanceCharge":
			g.appliedCharges = append(g.appliedCharges, n)
		case "AttachmentBinaryObject":
			if n.hasAttr("mimeCode") {
				g.attachments = append(g.attachments, n)
			}
		case "TaxTotalAmount":
			if n.hasAttr("currencyID") {
				g.taxTotalAmounts = append(g.taxTotalAmounts, n)
			}
		case "DateTimeString":
			g.dateStrings = append(g.dateStrings, n)
		case "IncludedSupplyChainTradeLineItem":
			g.lines = append(g.lines, n)
		case "NetPriceProductTradePrice":
			g.prices = append(g.prices, n)
		case "GrossPriceProductTradePrice":
			g.prices = append(g.prices, n)
			g.grossPrices = append(g.grossPrices, n)
		case "SpecifiedTradeSettlementPaymentMeans":
			g.paymentMeans = append(g.paymentMeans, n)
		}
		for _, c := range n.children {
			walk(c, n)
		}
	}
	walk(root, nil)
	return g
}

// peppolCIIRules evaluates the CII binding of PEPPOL-EN16931-CII.sch, plus the
// three rules KoSIT's merge adds to it. It is a no-op for any other root.
func peppolCIIRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || root.name != "CrossIndustryInvoice" {
		return
	}
	g := gatherPeppolCIINodes(root)

	peppolCIIDocumentRules(e, g)
	if r.stopped() {
		return
	}
	peppolCommonIdentifierRules(e, g.schemeIDs, g.uriIDs)
	if r.stopped() {
		return
	}
	peppolCIICodeListRules(e, g)
	if r.stopped() {
		return
	}
	peppolCIIAllowanceChargeRules(e, g)
	if r.stopped() {
		return
	}
	peppolCIILineRules(e, g)
}

// peppolCIIDocumentRules are the CII rules whose context is one of the header
// groups, plus KoSIT's empty-element addition.
func peppolCIIDocumentRules(e *peppolEval, g *peppolCIINodes) {
	peppolExistenceRules(e, g.root, true)

	// R008 — KoSIT's addition to the CII binding; OpenPEPPOL publishes it for UBL
	// only, so on the Peppol path has() declines it.
	if e.has("PEPPOL-EN16931-R008") {
		for _, n := range g.empty {
			e.addf("PEPPOL-EN16931-R008", "The element %q is empty; a document MUST NOT contain empty elements", n.name)
		}
	}

	// R002, context rsm:ExchangedDocument:
	//   count(ram:IncludedNote) <= 1 and not(ram:IncludedNote/ram:SubjectCode)
	//
	// The CII wording is not the UBL one: there is no German exemption, and it adds
	// a ban on the note subject code (BT-21) that the UBL rule does not have.
	for _, doc := range g.documents {
		notes := doc.all("IncludedNote")
		subject := false
		for _, n := range notes {
			if n.child("SubjectCode") != nil {
				subject = true
			}
		}
		if len(notes) > 1 || subject {
			e.add("PEPPOL-EN16931-R002", "No more than one Invoice note (BT-22) is allowed on document level, "+
				"and it MUST NOT carry a subject code (BT-21)")
		}
	}

	// R006 / R080, context ram:ApplicableHeaderTradeAgreement:
	//   count(ram:AdditionalReferencedDocument[ram:TypeCode='130']) <= 1
	//   count(ram:AdditionalReferencedDocument[ram:TypeCode='50'])  <= 1
	for _, agr := range g.agreements {
		if peppolCIICountRefDocs(agr, "130") > 1 {
			e.add("PEPPOL-EN16931-R006", "Only one invoiced object identifier (BT-18) is allowed on document level")
		}
		if peppolCIICountRefDocs(agr, "50") > 1 {
			e.add("PEPPOL-EN16931-R080", "Only one project reference (BT-11) is allowed on document level")
		}
	}

	docCurrency := peppolCIIDocumentCurrency(g.root)
	taxCurrency := peppolCIITaxCurrency(g.root)
	for _, st := range g.settlement {
		amts := nodesAt(st, "SpecifiedTradeSettlementHeaderMonetarySummation", "TaxTotalAmount")
		inDoc, notInDoc := 0, 0
		for _, a := range amts {
			// `@currencyID != $documentCurrencyCode` is false for an amount with no
			// @currencyID at all, so such an amount is in neither count.
			switch {
			case a.attr("currencyID") == docCurrency:
				inDoc++
			case a.hasAttr("currencyID"):
				notInDoc++
			}
		}
		// R053: count(...TaxTotalAmount[@currencyID = $documentCurrencyCode]) = 1.
		// KoSIT's merge rewrites the test to "<= 1" and the message with it, because
		// BT-110 is optional in the German profile.
		if (e.xr && inDoc > 1) || (!e.xr && inDoc != 1) {
			e.addf("PEPPOL-EN16931-R053", "%d VAT totals carry the invoice currency code (BT-5=%q); %s", inDoc, docCurrency,
				map[bool]string{true: "no more than one is allowed", false: "exactly one is required"}[e.xr])
		}
		// R054: count(...TaxTotalAmount[@currencyID != $documentCurrencyCode]) =
		//       (if (ram:TaxCurrencyCode) then 1 else 0)
		wantOther := 0
		if st.child("TaxCurrencyCode") != nil {
			wantOther = 1
		}
		if notInDoc != wantOther {
			e.addf("PEPPOL-EN16931-R054", "%d VAT totals carry a currency other than the invoice currency; %d is "+
				"required when the VAT accounting currency code (BT-6) is %s", notInDoc, wantOther,
				map[bool]string{true: "provided", false: "absent"}[wantOther == 1])
		}
		// R055, and KoSIT's rewrite of it. OpenPEPPOL guards on BT-6 alone;
		// KoSIT adds "and a VAT total in the invoice currency exists", so a CII
		// invoice that omits the optional BT-110 is not reported.
		if st.child("TaxCurrencyCode") == nil {
			continue
		}
		if e.xr && inDoc == 0 {
			continue
		}
		amtsWithSummation := nodesAt(st, "SpecifiedTradeSettlementHeaderMonetarySummation", "TaxTotalAmount")
		if !(peppolAnySign(amtsWithSummation, taxCurrency, false) && peppolAnySign(amtsWithSummation, docCurrency, false)) &&
			!(peppolAnySign(amtsWithSummation, taxCurrency, true) && peppolAnySign(amtsWithSummation, docCurrency, true)) {
			e.add("PEPPOL-EN16931-R055", "The Invoice total VAT amount (BT-110) and the same amount in the VAT "+
				"accounting currency (BT-111) MUST have the same operational sign")
		}
	}

	// P0100, context ram:ExchangedDocument/ram:TypeCode. CII expresses an invoice
	// and a credit note with one root and one BT-3, so the permitted set is the
	// union of the two UBL lists and there is no P0101.
	profile := peppolProfileNumber(g.root.str("ExchangedDocumentContext",
		"BusinessProcessSpecifiedDocumentContextParameter", "ID"))
	for _, tc := range g.typeCodes {
		code := strings.Join(strings.Fields(tc.text), " ")
		if profile == "01" && !peppolCIITypeCodes[code] {
			e.addf("PEPPOL-EN16931-P0100", "The document type code (BT-3=%q) is not one of the codes profile 01 permits", code)
		}
	}

	// F001, context udt:DateTimeString:
	//   normalize-space(@format) = '102' and string-length(text()) = 8 and
	//   matches(normalize-space(text()), '20[0-9]{6}')
	for _, d := range g.dateStrings {
		if strings.Join(strings.Fields(d.attr("format")), " ") != "102" ||
			len(d.rawText()) != 8 || !peppolCIIDateRE.MatchString(strings.TrimSpace(d.text)) {
			e.addf("PEPPOL-EN16931-F001", "The date %q MUST carry format 102 and be formatted YYYYMMDD", d.rawText())
		}
	}
}

// peppolCIIDateRE is F001's pattern in the CII binding, unanchored as XPath's
// matches() is.
var peppolCIIDateRE = regexp.MustCompile(`20[0-9]{6}`)

// peppolCIICountRefDocs counts `ram:AdditionalReferencedDocument[ram:TypeCode=$code]`.
func peppolCIICountRefDocs(agr *ciiNode, code string) int {
	n := 0
	for _, ref := range agr.all("AdditionalReferencedDocument") {
		for _, tc := range ref.all("TypeCode") {
			if tc.rawText() == code {
				n++
				break
			}
		}
	}
	return n
}

// peppolCIIDocumentCurrency is the $documentCurrencyCode of the CII binding:
// /rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/ram:ApplicableHeaderTradeSettlement/ram:InvoiceCurrencyCode.
func peppolCIIDocumentCurrency(root *ciiNode) string {
	return root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement", "InvoiceCurrencyCode").rawText()
}

// peppolCIITaxCurrency is $taxCurrencyCode, the same path with ram:TaxCurrencyCode.
func peppolCIITaxCurrency(root *ciiNode) string {
	return root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement", "TaxCurrencyCode").rawText()
}

// peppolCIICodeListRules are the CL* restrictions in the CII binding, which is a
// narrower set than UBL's: there is no CL006, and CL007 is bound to
// ram:TaxTotalAmount alone rather than to every amount in the document.
func peppolCIICodeListRules(e *peppolEval, g *peppolCIINodes) {
	// CL001, context ram:AttachmentBinaryObject[@mimeCode].
	for _, b := range g.attachments {
		if !en16931MIME[b.attr("mimeCode")] {
			e.addf("PEPPOL-EN16931-CL001", "The attachment MIME code %q is not in the subset of the IANA list Peppol permits", b.attr("mimeCode"))
		}
	}
	// CL002 / CL003, context ram:SpecifiedTradeAllowanceCharge[normalize-space(
	// ram:ChargeIndicator/udt:Indicator) = 'false'|'true']/ram:ReasonCode.
	for _, ac := range g.specifiedCharges {
		ind := strings.Join(strings.Fields(ac.str("ChargeIndicator", "Indicator")), " ")
		for _, rc := range ac.all("ReasonCode") {
			code := strings.Join(strings.Fields(rc.text), " ")
			switch ind {
			case "false":
				if !en16931AllowanceReasons[code] {
					e.addf("PEPPOL-EN16931-CL002", "The allowance reason code (BT-98=%q) is not in the subset of UNCL 5189 Peppol permits", code)
				}
			case "true":
				if !en16931ChargeReasons[code] {
					e.addf("PEPPOL-EN16931-CL003", "The charge reason code (BT-105=%q) is not in UNCL 7161", code)
				}
			}
		}
	}
	// CL007, context ram:TaxTotalAmount[@currencyID].
	for _, a := range g.taxTotalAmounts {
		if !peppolCurrencies[a.attr("currencyID")] {
			e.addf("PEPPOL-EN16931-CL007", "The currency identifier %q on ram:TaxTotalAmount is not an ISO 4217 code", a.attr("currencyID"))
		}
	}
	// CL008, context ram:BuyerTradeParty/ram:URIUniversalCommunication/ram:URIID |
	// ram:SellerTradeParty/... — with no [@schemeID] predicate, so an address with
	// no scheme identifier at all is reported.
	for _, ep := range g.endpointURIIDs {
		if !peppolEAS[ep.attr("schemeID")] {
			e.addf("PEPPOL-EN16931-CL008", "The electronic address scheme identifier %q is not in Peppol's "+
				"Electronic Address Identifier Scheme list", ep.attr("schemeID"))
		}
	}
}

// peppolCIIAllowanceChargeRules are R040..R043 over ram:SpecifiedTradeAllowanceCharge,
// R043's second rule over ram:AppliedTradeAllowanceCharge, and the R044/R046
// KoSIT writes for the CII binding.
func peppolCIIAllowanceChargeRules(e *peppolEval, g *peppolCIINodes) {
	for _, ac := range g.specifiedCharges {
		pct := ac.child("CalculationPercent")
		base := ac.child("BasisAmount")
		switch {
		case pct != nil && base == nil:
			e.add("PEPPOL-EN16931-R041", "The allowance/charge base amount (BT-93/BT-100) MUST be provided when "+
				"the percentage (BT-94/BT-101) is")
		case pct == nil && base != nil:
			e.add("PEPPOL-EN16931-R042", "The allowance/charge percentage (BT-94/BT-101) MUST be provided when "+
				"the base amount (BT-93/BT-100) is")
		case pct != nil && base != nil:
			b, okB := parseAmount(base.text)
			p, okP := parseAmount(pct.text)
			amount := 0.0
			if a := ac.child("ActualAmount"); a != nil {
				v, ok := parseAmount(a.text)
				if !ok {
					continue
				}
				amount = v
			}
			if okB && okP && !peppolSlack(amount, b*p/100, e.slack) {
				e.addf("PEPPOL-EN16931-R040", "The allowance/charge amount %.2f MUST equal the base amount %.2f × %s%%", amount, b, pct.rawText())
			}
		}
	}
	// R043 has two rules in the CII binding, one per allowance/charge group, and
	// they carry the same identifier — which is why KoSIT's merge renames them
	// R043-1 and R043-2 in the released artefact. This package reports the
	// identifier OpenPEPPOL publishes; the suffix is an artefact of KoSIT
	// de-duplicating XSLT template names rather than a rule of its own.
	for _, ac := range append(append([]*ciiNode{}, g.specifiedCharges...), g.appliedCharges...) {
		if ind := strings.Join(strings.Fields(ac.str("ChargeIndicator", "Indicator")), " "); ind != "true" && ind != "false" {
			e.addf("PEPPOL-EN16931-R043", "The allowance/charge indicator %q MUST be 'true' or 'false'", ind)
		}
	}
	// R044 and R046, as KoSIT writes them for CII in peppol-into-xr.xsl, context
	// rsm:SupplyChainTradeTransaction/ram:IncludedSupplyChainTradeLineItem/
	// ram:SpecifiedLineTradeAgreement/ram:GrossPriceProductTradePrice.
	if !e.has("PEPPOL-EN16931-R044") && !e.has("PEPPOL-EN16931-R046") {
		return
	}
	for _, gross := range g.grossPrices {
		applied := gross.child("AppliedTradeAllowanceCharge")
		// R044: not(ram:AppliedTradeAllowanceCharge/ram:ActualAmount) or
		//       ram:AppliedTradeAllowanceCharge/ram:ChargeIndicator/udt:Indicator = 'false'
		if applied != nil && applied.child("ActualAmount") != nil &&
			applied.str("ChargeIndicator", "Indicator") != "false" {
			e.add("PEPPOL-EN16931-R044", "A charge on price level is not allowed: the applied allowance's charge "+
				"indicator MUST be 'false'")
		}
		// R046: not(ram:ChargeAmount) or xs:decimal(../ram:NetPriceProductTradePrice/
		//       ram:ChargeAmount) = xs:decimal(ram:ChargeAmount) -
		//       u:decimalOrZero(ram:AppliedTradeAllowanceCharge/ram:ActualAmount[1])
		grossAmt, okG := parseAmount(gross.str("ChargeAmount"))
		if !okG {
			continue
		}
		discount := 0.0
		if applied != nil {
			if a := applied.child("ActualAmount"); a != nil {
				v, ok := parseAmount(a.text)
				if !ok {
					continue
				}
				discount = v
			}
		}
		netAmt, okN := parseAmount(peppolCIINetPrice(gross, g))
		if okN && !peppolDecimalEqual(netAmt, grossAmt-discount) {
			e.addf("PEPPOL-EN16931-R046", "The Item net price (BT-146=%.4f) MUST equal the gross price (BT-148=%.4f) "+
				"less the price discount (BT-147=%.4f)", netAmt, grossAmt, discount)
		}
	}
}

// peppolCIINetPrice is `../ram:NetPriceProductTradePrice/ram:ChargeAmount` from a
// gross price: the net price beside it in the same ram:SpecifiedLineTradeAgreement.
func peppolCIINetPrice(gross *ciiNode, g *peppolCIINodes) string {
	for _, li := range g.lines {
		for _, agr := range li.all("SpecifiedLineTradeAgreement") {
			for _, p := range agr.all("GrossPriceProductTradePrice") {
				if p == gross {
					return agr.str("NetPriceProductTradePrice", "ChargeAmount")
				}
			}
		}
	}
	return ""
}

// peppolCIILineRules are the per-line rules of the CII binding and R061.
func peppolCIILineRules(e *peppolEval, g *peppolCIINodes) {
	for _, li := range g.lines {
		refs := nodesAt(li, "SpecifiedLineTradeSettlement", "AdditionalReferencedDocument")
		// R100: count(ram:SpecifiedLineTradeSettlement/ram:AdditionalReferencedDocument
		//             [ram:TypeCode='130']) <= 1
		n130 := 0
		hasOther := false
		for _, ref := range refs {
			is130 := false
			for _, tc := range ref.all("TypeCode") {
				if tc.rawText() == "130" {
					is130 = true
				}
			}
			if is130 {
				n130++
			} else {
				hasOther = true
			}
		}
		if n130 > 1 {
			e.addf("PEPPOL-EN16931-R100", "Only one invoiced object identifier (BT-128) is allowed per line; found %d", n130)
		}
		// R101: not(ram:SpecifiedLineTradeSettlement/ram:AdditionalReferencedDocument)
		//       or (.../ram:TypeCode = '130')
		if len(refs) > 0 && n130 == 0 && hasOther {
			e.add("PEPPOL-EN16931-R101", "A line's ram:AdditionalReferencedDocument may only carry the invoiced "+
				"object identifier (BT-128), whose type code is 130")
		}
		peppolCIILineTotal(e, li)
	}
	// R121, context ram:NetPriceProductTradePrice | ram:GrossPriceProductTradePrice:
	//   not(ram:BasisQuantity) or xs:decimal(ram:BasisQuantity) > 0
	//
	// The context is the price and not the line, so a gross price's base quantity is
	// in scope too — which reading BT-149 off the model, where only the net price's
	// is mapped, could not see.
	for _, p := range g.prices {
		bq := p.child("BasisQuantity")
		if bq == nil {
			continue
		}
		if v, ok := parseAmount(bq.text); ok && v <= 0 {
			e.addf("PEPPOL-EN16931-R121", "The Item price base quantity (BT-149=%s) MUST be greater than zero", bq.rawText())
		}
		// R130, context ram:NetPriceProductTradePrice/ram:BasisQuantity[@unitCode] |
		// ram:GrossPriceProductTradePrice/ram:BasisQuantity[@unitCode]:
		//   @unitCode = ../../../ram:SpecifiedLineTradeDelivery/ram:BilledQuantity/@unitCode
		if !bq.hasAttr("unitCode") {
			continue
		}
		li := peppolCIILineOfPrice(p, g)
		if li == nil {
			continue
		}
		billed := li.child("SpecifiedLineTradeDelivery", "BilledQuantity")
		if bq.attr("unitCode") != billed.attr("unitCode") {
			e.addf("PEPPOL-EN16931-R130", "The Item price base quantity unit (BT-150=%q) MUST equal the invoiced "+
				"quantity unit (BT-130=%q)", bq.attr("unitCode"), billed.attr("unitCode"))
		}
	}
	// R061, context ram:SpecifiedTradeSettlementPaymentMeans[normalize-space(
	// ram:TypeCode) = '49' or '59']: ../ram:SpecifiedTradePaymentTerms/ram:DirectDebitMandateID.
	for _, st := range g.settlement {
		for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
			code := strings.Join(strings.Fields(pm.child("TypeCode").rawText()), " ")
			if code != "49" && code != "59" {
				continue
			}
			if st.child("SpecifiedTradePaymentTerms", "DirectDebitMandateID") == nil {
				e.add("PEPPOL-EN16931-R061", "A Mandate reference identifier (BT-89) MUST be provided for a direct debit")
			}
		}
	}
}

// peppolCIILineOfPrice finds the line item a price belongs to.
func peppolCIILineOfPrice(price *ciiNode, g *peppolCIINodes) *ciiNode {
	for _, li := range g.lines {
		for _, agr := range li.all("SpecifiedLineTradeAgreement") {
			for _, name := range []string{"NetPriceProductTradePrice", "GrossPriceProductTradePrice"} {
				for _, p := range agr.all(name) {
					if p == price {
						return li
					}
				}
			}
		}
	}
	return nil
}

// peppolCIILineTotal is R120 in the CII binding, with the <let> variables of the
// same rule resolved. KoSIT's merge widens $baseQuantity to fall back to the gross
// price's base quantity when the net price carries none.
func peppolCIILineTotal(e *peppolEval, li *ciiNode) {
	if !e.has("PEPPOL-EN16931-R120") {
		return
	}
	agr := li.child("SpecifiedLineTradeAgreement")
	settle := li.child("SpecifiedLineTradeSettlement")
	lineTotal, ok := peppolDecimalOr(settle.child("SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount"), 0)
	if !ok {
		return
	}
	quantity, ok := peppolDecimalOr(li.child("SpecifiedLineTradeDelivery", "BilledQuantity"), 1)
	if !ok {
		return
	}
	price, ok := peppolDecimalOr(agr.child("NetPriceProductTradePrice", "ChargeAmount"), 0)
	if !ok {
		return
	}
	baseQty, ok := peppolDecimalOr(agr.child("NetPriceProductTradePrice", "BasisQuantity"), 0)
	if !ok {
		return
	}
	if baseQty == 0 && e.xr {
		if v, ok := peppolDecimalOr(agr.child("GrossPriceProductTradePrice", "BasisQuantity"), 0); ok {
			baseQty = v
		}
	}
	if baseQty == 0 {
		baseQty = 1
	}
	allowances, ok1 := peppolCIIChargeSum(settle, "false")
	charges, ok2 := peppolCIIChargeSum(settle, "true")
	if !ok1 || !ok2 {
		return
	}
	want := quantity*(price/baseQty) + charges - allowances
	if !peppolSlack(lineTotal, want, e.slack) {
		e.addf("PEPPOL-EN16931-R120", "The Invoice line net amount (BT-131=%.2f) MUST equal the invoiced quantity × "+
			"(item net price ÷ base quantity) plus line charges less line allowances (%.2f)", lineTotal, want)
	}
}

// peppolCIIChargeSum is the CII $allowancesTotal / $chargesTotal.
func peppolCIIChargeSum(settle *ciiNode, indicator string) (float64, bool) {
	sum, any := 0.0, false
	for _, ac := range settle.orNil().all("SpecifiedTradeAllowanceCharge") {
		if strings.Join(strings.Fields(ac.str("ChargeIndicator", "Indicator")), " ") != indicator {
			continue
		}
		any = true
		for _, a := range ac.all("ActualAmount") {
			v, ok := parseAmount(a.text)
			if !ok {
				return 0, false
			}
			sum += v
		}
	}
	if !any {
		return 0, true
	}
	return math.Floor(sum*100+0.5) / 100, true
}

// ---------------------------------------------------------------------------
// Shared arithmetic
// ---------------------------------------------------------------------------

// peppolSlack is u:slack:
//
//	xs:decimal($exp + $slack) >= $val and xs:decimal($exp - $slack) <= $val
func peppolSlack(exp, val, slack float64) bool {
	return exp+slack >= val && exp-slack <= val
}

// peppolDecimalEqual compares two amounts the way xs:decimal equality does. The
// values arrive as float64, so an exact comparison would fail on a difference no
// decimal arithmetic could produce; the tolerance is far below the smallest
// difference any amount in an invoice can express.
func peppolDecimalEqual(a, b float64) bool { return math.Abs(a-b) < 5e-9 }

// peppolDecimalOr is the `if (X) then xs:decimal(X) else <default>` shape R120's
// five <let> variables share. The second return is false when the element is
// present and not a number, where XPath raises a dynamic error rather than
// reporting the assertion — so this package reports nothing either.
func peppolDecimalOr(n *ciiNode, def float64) (float64, bool) {
	if n == nil {
		return def, true
	}
	return parseAmount(n.text)
}

// peppolIsCalendarDate is F001's UBL test: exactly ten characters, and castable
// as xs:date.
func peppolIsCalendarDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
