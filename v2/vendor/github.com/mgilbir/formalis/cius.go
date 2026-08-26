package formalis

import (
	"context"
	"strings"
)

// This file holds the one place this package reads the Specification identifier
// (BT-24, the UBL CustomizationID or the CII
// GuidelineSpecifiedDocumentContextParameter/ID) and says which rule set it
// names, and the entry point that validates a document against whatever that
// turns out to be. Callers that do not know which profile an invoice targets can
// use ValidateCIUS and let the document route itself.
//
// # Why the table below exists
//
// Every test in it is a substring of a namespaced identifier, and substrings of
// namespaced identifiers nest: "urn:peppol:pint:billing-1@my-1" contains
// "peppol". Written as an unordered set of independent tests, the routing is
// therefore ambiguous, and the ambiguity is not theoretical — it shipped. A
// switch whose last case was strings.Contains(id, "peppol") sent all 64 PINT
// sample instances in the corpus to the Peppol BIS Billing 3.0 validator, which
// opens by requiring BT-24 to be the BIS identifier a PINT invoice by definition
// does not carry (C24).
//
// So the tests are a single ordered list rather than a switch, ordered most
// specific first, and everything that has to make this decision reads that one
// list:
//
//   - DetectCIUS reports the CIUS an identifier declares;
//   - specIDSource reports the rule set it names, including the three that are
//     not CIUS (PINT, OIOUBL, UBL-TR);
//   - declaresSpecID answers the unordered question — "does this identifier
//     carry that profile's discriminator" — which is what the Is* predicates
//     ask, since each of them is documented as an independent test rather than
//     as a branch of a partition;
//   - route, in detect.go, applies it inside the wider arbitration Detect
//     documents, and validateCIUS below routes on the result.
//
// Two orderings that can disagree is how C24 happened. There is now one.

// CIUS identifies a Core Invoice Usage Specification the dispatcher recognises.
type CIUS string

const (
	CIUSNone      CIUS = ""          // plain EN 16931 core (no recognised CIUS)
	CIUSXRechnung CIUS = "XRechnung" // German public-sector CIUS
	CIUSPeppol    CIUS = "Peppol"    // OpenPEPPOL BIS Billing 3.0
	CIUSNLCIUS    CIUS = "NLCIUS"    // Dutch SimplerInvoicing / SI-UBL
	CIUSPortugal  CIUS = "CIUS-PT"   // Portuguese AT/eSPap CIUS-PT
	CIUSRomania   CIUS = "CIUS-RO"   // Romanian ANAF RO e-Factura
	CIUSBelgium   CIUS = "UBL.BE"    // Belgian UBL.BE
	CIUSSerbia    CIUS = "SRBDT"     // Serbian SRBDT

	// CIUSPINT is Peppol PINT: the global UBL 2.1 billing model with
	// jurisdiction-aligned rule sets (AE, AUNZ, EU, JP, MY, OM, SG).
	//
	// It is strictly not a CIUS of EN 16931 the way the others here are — PINT is
	// its own billing model rather than a national narrowing of the European
	// norm, and this package validates it with its own rule set, ValidatePINT.
	// It is in this type because a PINT identifier is a Specification identifier
	// like any other, and leaving it out is what made a PINT invoice answer
	// CIUSPeppol: a value that says "Peppol BIS Billing 3.0 applies to this
	// document" when it does not.
	CIUSPINT CIUS = "PINT"
)

// specIDRule is one test on the Specification identifier: the rule set the
// identifier names when it matches, the CIUS constant for it (CIUSNone for the
// three that are not CIUS), and the discriminators that match it.
type specIDRule struct {
	// src is the authority whose rules apply to a document declaring this.
	src Source
	// cius is what DetectCIUS reports for it, or CIUSNone when the profile is
	// not a CIUS this type names.
	cius CIUS
	// marks are the discriminators, lower-case, any one of which matches. They
	// are compared against a lower-cased, space-trimmed identifier, so an
	// identifier's case and surrounding whitespace never change its routing —
	// which they did while three of these tests were case-sensitive and the rest
	// were not.
	marks []string
	// prefix makes the marks prefix tests rather than substring tests. Only
	// UBL-TR uses it: "TR" is two characters over a namespace everyone shares,
	// and as a substring test it would match half the corpus.
	prefix bool
}

// specIDRules is the ordered list, most specific test first. The order is part
// of the package's contract — Detect documents it — and each entry that is only
// reachable because of where it sits says why.
var specIDRules = []specIDRule{
	// PINT before Peppol. "urn:peppol:pint:billing-1@my-1" contains "peppol",
	// and PINT and Peppol BIS Billing 3.0 are different rule sets with different
	// mandatory terms. This is C24.
	//
	// The second mark is the pre-release Japanese identifier, and it is a
	// deliberate, narrow exception rather than an accident of matching: JP PINT
	// 0.1.2 declared "urn:fdc:peppol:jp:billing:3.0", which never says "pint"
	// and which the released JP profiles replaced with
	// "urn:peppol:pint:billing-1@jp-1" and
	// "urn:peppol:pint:nontaxinvoice-1@jp-1". The eight instances carrying it in
	// the corpus ship in phive-rules' PINT test files, under pint-jp, and they
	// satisfy every PINT rule this package implements. Without the exception
	// they route to Peppol BIS on the strength of the "peppol" in
	// "fdc:peppol:jp" — the same defect as C24, reached by a different string —
	// and are then accused of PEPPOL-EN16931-R004. It is not the Peppol BIS
	// Billing 3.0 identifier, which is
	// "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"
	// and is matched by the Peppol entry below.
	{src: SourcePINT, cius: CIUSPINT, marks: []string{"peppol:pint", "fdc:peppol:jp:billing"}},

	// OIOUBL before everything below it. Denmark's identifier carries a brand
	// name that appears in no other profile, so it is safe this early, and it
	// has to be ahead of the UBL-TR prefix test for "TR-OIOUBL-2.02" to come out
	// as OIOUBL.
	{src: SourceOIOUBL, marks: []string{"oioubl"}},

	// XRechnung before Peppol, because an XRechnung identifier may also
	// reference the Peppol base.
	{src: SourceXRechnung, cius: CIUSXRechnung, marks: []string{"xrechnung"}},

	// Factur-X after XRechnung, because a ZUGFeRD 2.x document may carry the
	// XRechnung identifier and is an XRechnung when it does — KoSIT's rule set is
	// the one its recipient validates against.
	//
	// cius is CIUSNone: Factur-X is a profile family and not a Core Invoice Usage
	// Specification, which is the same reason OIOUBL and UBL-TR carry CIUSNone.
	// What it does name is a Profile, and validateFacturXRouted reads that back
	// out of this same identifier.
	//
	// The marks are the two authorities' domains and nothing wider. They match the
	// four tiers that name themselves — "urn:factur-x.eu:1p0:minimum", ":basicwl",
	// and the "#compliant#"/"#conformant#" forms of ":basic" and ":extended" —
	// and deliberately not the EN 16931 tier, whose Specification identifier is
	// CEN's own "urn:cen.eu:en16931:2017" with nothing added. A Factur-X EN 16931
	// invoice is therefore indistinguishable from a plain EN 16931 CII invoice by
	// this test, which is what FNFE intends by writing that identifier: at that
	// tier the document claims to be exactly EN 16931. A caller who knows better
	// — because the PDF container's XMP said EN 16931 — calls Validate with the
	// Profile.
	//
	// Two marks and not one because Factur-X 1.0 and ZUGFeRD 2.x are one
	// specification under two brands, and FNFE's own code database enumerates both
	// identifiers for every tier that names itself. Matching "factur-x.eu" alone
	// sent a ZUGFeRD-branded MINIMUM invoice to CEN's CII binding, which reports
	// BR-16 and BR-CO-18 against a tier that has no invoice lines: C44, in the
	// German half of the identifier space. See facturXProfileFromSpecID.
	{src: SourceFacturX, marks: []string{"factur-x.eu", "zugferd.de"}},
	{src: SourceNLCIUS, cius: CIUSNLCIUS, marks: []string{"nlcius", "nen.nl"}},
	{src: SourceCIUSPT, cius: CIUSPortugal, marks: []string{"cius-pt", "feap.gov.pt"}},
	{src: SourceCIUSRO, cius: CIUSRomania, marks: []string{"cius-ro", "mfinante.ro"}},
	{src: SourceUBLBE, cius: CIUSBelgium, marks: []string{"ubl.be"}},
	{src: SourceSRBDT, cius: CIUSSerbia, marks: []string{"srbdt", "mfin.gov.rs"}},
	{src: SourcePeppol, cius: CIUSPeppol, marks: []string{"peppol"}},

	// Last: the weakest identifier test in the package, two characters at the
	// front of an identifier over a namespace everyone shares.
	{src: SourceUBLTR, marks: []string{"tr"}, prefix: true},
}

// matches reports whether a lower-cased, trimmed identifier carries one of this
// rule's discriminators.
func (r specIDRule) matches(id string) bool {
	for _, m := range r.marks {
		if r.prefix {
			if strings.HasPrefix(id, m) {
				return true
			}
			continue
		}
		if strings.Contains(id, m) {
			return true
		}
	}
	return false
}

// normSpecID puts an identifier in the form the marks are written in, so that
// case and surrounding whitespace cannot change a document's routing. The two
// callers that reach this — the streaming scan and the semantic model — trim
// already; a caller passing DetectCIUS a raw string does not.
func normSpecID(specID string) string {
	return strings.ToLower(strings.TrimSpace(specID))
}

// matchSpecID returns the first rule the identifier matches, or nil.
func matchSpecID(specID string) *specIDRule {
	id := normSpecID(specID)
	if id == "" {
		return nil
	}
	for i := range specIDRules {
		if specIDRules[i].matches(id) {
			return &specIDRules[i]
		}
	}
	return nil
}

// specIDSource reports the rule set the Specification identifier names, or
// SourceNone when it names none this package validates. It is the shared half of
// the routing: Detect reaches it through route, and ValidateCIUS reaches it
// through the same function with the facts it already has.
func specIDSource(specID string) Source {
	if r := matchSpecID(specID); r != nil {
		return r.src
	}
	return SourceNone
}

// declaresSpecID reports whether the identifier carries the discriminator of
// src, ignoring the order — the question an Is* predicate asks. IsPINT is true
// of "urn:peppol:pint:x" whatever else the identifier also says, because the
// predicates are documented as independent tests; only the routing arbitrates.
func declaresSpecID(src Source, specID string) bool {
	id := normSpecID(specID)
	if id == "" {
		return false
	}
	for _, r := range specIDRules {
		if r.src == src {
			return r.matches(id)
		}
	}
	return false
}

// ciusSource maps a CIUS onto the authority that publishes it, so the CIUS reach
// Detection.Source without a second taxonomy. It reports SourceNone for
// CIUSNone.
func ciusSource(c CIUS) Source {
	if c == CIUSNone {
		return SourceNone
	}
	for _, r := range specIDRules {
		if r.cius == c {
			return r.src
		}
	}
	return SourceNone
}

// DetectCIUS reports the CIUS that a Specification identifier (BT-24) declares,
// or CIUSNone when it names no CIUS this package recognises.
//
// The tests are applied in the order specIDRules states, which matters wherever
// one profile's identifier contains another's discriminator: PINT is checked
// before Peppol ("urn:peppol:pint:billing-1@my-1" contains "peppol" and is not a
// Peppol BIS Billing 3.0 document), and XRechnung before Peppol (an XRechnung
// identifier may also reference the Peppol base). Matching ignores case and
// surrounding whitespace.
//
// CIUSNone is not the same as "no rule set": three of the profiles the
// identifier can name — PINT is a CIUS here, but OIOUBL and UBL-TR are national
// formats rather than CIUS — so an OIOUBL identifier reports CIUSNone while
// Detect reports SourceOIOUBL for the same document. Route on Detection.Source;
// read the CIUS to know which CIUS was declared.
//
// It takes the identifier, not the document. To ask the question of XML, call
// Detect: Detection.SpecID is this identifier, extracted from either syntax in
// one streaming pass, and Detection.CIUS is this function applied to it.
func DetectCIUS(specID string) CIUS {
	if r := matchSpecID(specID); r != nil {
		return r.cius
	}
	return CIUSNone
}

// ValidateCIUS validates an invoice against whichever rule set the document
// itself declares, falling back to the EN 16931 core when it declares none. It
// routes both syntaxes (CII and UBL).
//
// It applies the same arbitration Detect does — the one function, route, reached
// with the facts each caller already has — so the rule set this runs and the one
// Detect names for the same bytes cannot differ. Before that was so, they did:
// every PINT invoice was routed here to the Peppol BIS Billing 3.0 validator
// while Detect reported PINT (C24), and 154 documents in the conformance corpus
// were validated against a rule set Detect disagreed with.
//
// What that means in practice is that this is no longer a CIUS-only dispatcher,
// though the name is kept: a document whose Specification identifier names
// PINT, OIOUBL or UBL-TR is validated against that rule set, and a UBL document
// that declares no identifier but carries a format's distinguishing mark — a
// ZATCA profile identifier, an ebInterface Biller, a Svefaktura SellerParty — is
// validated against that one. Every one of those was previously checked against
// the EN 16931 core, which is a rule set they do not claim to follow and which
// therefore reported findings that were not defects.
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
// The Report's coverage follows the document, not this entry point: it names
// the gaps of the rule set the dispatch actually ran, so an XRechnung invoice
// comes back with the XRechnung gaps and a Portuguese one with the CIUS-PT gaps.
// A document that declares nothing recognisable is validated against the EN
// 16931 core and reports the core's gaps alone.
func ValidateCIUS(ctx context.Context, xmlData []byte) (Report, error) {
	r := newRun(ctx)
	p, err := parseEN16931(r, xmlData)
	if err != nil {
		// The document never got as far as declaring anything, so the core — the
		// rule set every branch of the dispatch either runs or is measured
		// against — is the honest coverage claim on the paths that still produce a
		// Report. parseFailure decides which those are.
		return parseFailure(r, err, SourceEN16931)
	}
	out, sources := validateCIUS(r, p)
	return newReport(r.finish(out), sources...), nil
}

// modelValidators are the rule sets that read the syntax-neutral EN 16931 model,
// with the authorities each of them runs: every one of these layers a CIUS on
// the core and reports both.
var modelValidators = map[Source]struct {
	check   func(*run, *parsed) []Violation
	sources []Source
}{
	SourceFacturX:   {validateFacturXRouted, []Source{SourceEN16931, SourceFacturX}},
	SourceXRechnung: {validateXRechnung, []Source{SourceEN16931, SourceXRechnung}},
	SourcePeppol:    {validatePeppol, []Source{SourceEN16931, SourcePeppol}},
	SourceNLCIUS:    {validateNLCIUS, []Source{SourceEN16931, SourceNLCIUS}},
	SourceCIUSPT:    {validateCIUSPT, []Source{SourceEN16931, SourceCIUSPT}},
	SourceCIUSRO:    {validateCIUSRO, []Source{SourceEN16931, SourceCIUSRO}},
	SourceUBLBE:     {validateUBLBE, []Source{SourceEN16931, SourceUBLBE}},
	SourceSRBDT:     {validateSRBDT, []Source{SourceEN16931, SourceSRBDT}},
}

// treeValidators are the formats checked by walking the element tree, keyed by
// the Source that names them.
//
// Every format is listed, not only the ones a document parseEN16931 accepts can
// route to, so that the map is the same set Detection.Validator dispatches over
// and a format cannot be added to one without the other noticing.
var treeValidators = map[Source]treeValidator{
	SourcePINT:        pintValidator,
	SourceOIOUBL:      oioublValidator,
	SourceUBLTR:       turkishValidator,
	SourceZATCA:       zatcaValidator,
	SourceEbInterface: ebInterfaceValidator,
	SourceSvefaktura:  svefakturaValidator,
	SourceFatturaPA:   fatturaPAValidator,
	SourceFacturae:    facturaeValidator,
	SourceKSeF:        ksefValidator,
	SourceFinvoice:    finvoiceValidator,
	SourceTEAPPS:      teappsValidator,
	SourceOSA:         osaValidator,
	SourceOrderX:      orderXValidator,
}

// validateCIUS routes the document and reports which authorities' rules it ran,
// so the caller's Report can name that rule set's gaps rather than a fixed set
// this entry point guessed at.
func validateCIUS(r *run, p *parsed) ([]Violation, []Source) {
	// The dispatch is to the unexported forms so the whole dispatched call shares
	// this run: one cancellation signal and one set of budgets across the routing
	// and the validating, rather than a fresh (uncancellable) allowance for the
	// second half of the work.
	//
	// It also hands on the document already parsed, and routes on facts read off
	// that same parse. Detect answers the identical question from a streaming
	// scan because it has no tree; here there is one, so reading the bytes a
	// second time to rebuild a byte-identical tree would charge the shared
	// element budget twice for one document, and the general entry point would
	// refuse documents the specific one accepts. What the two share is the
	// decision, not the reading.
	src := route(p.facts())
	if m, ok := modelValidators[src]; ok {
		return m.check(r, p), m.sources
	}
	if tv, ok := treeValidators[src]; ok {
		return tv.checkParsed(p), []Source{src}
	}
	// SourceEN16931 — a document declaring no profile — and, unreachably from
	// here, SourceNone: parseEN16931 accepts three roots and route answers one
	// of the two maps above for every profile they can declare.
	return validateEN16931(r, p, ProfileEN16931, ciiBindingCEN), []Source{SourceEN16931}
}
