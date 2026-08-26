package formalis

import (
	"fmt"
	"regexp"
	"strings"
)

// Factur-X / ZUGFeRD as a rule set of its own.
//
// # Why this file exists
//
// EN 16931 is a semantic model plus, per syntax, a normative binding: CEN
// publishes EN16931-CII-syntax.sch, 583 CII-SR-* and CII-DT-* assertions that
// hold a CrossIndustryInvoice down to the core subset of CII the standard defines
// — "only a preceding invoice reference may carry an issue date", "an amount
// other than the VAT total shall not carry a @currencyID". A CIUS that says
// nothing about the binding inherits CEN's, and most do: KoSIT's released
// XRechnung Schematron and OpenPEPPOL's BIS Billing both include CEN's CII
// validation file outright.
//
// Factur-X does not. FNFE-MPE and FeRD publish five profile Schematrons —
// MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED — and between them they carry
// four of CEN's 583 binding assertions: CII-SR-463, CII-SR-464, CII-SR-465 and
// CII-SR-466 in every profile from BASIC WL up, plus CII-DT-097 in EXTENDED. In
// their place each profile carries a data model of its own, which the next
// section describes.
//
// Applying CEN's binding to a Factur-X document is therefore applying a rule set
// its authority did not adopt, and the two disagree exactly where it matters. The
// EXTENDED profile exists to carry terms the EN 16931 core subset of CII does not
// have; CEN's binding is written to reject precisely those. Measured over the 59
// examples FNFE-MPE publishes with the specification, validating each at the
// profile its own BT-24 declares: 13 conforming EXTENDED invoices drew 76 fatal
// findings — CII-DT-027 seventy times, CII-DT-021 three, CII-DT-018 twice,
// CII-SR-439 once — every one of them naming a rule Factur-X does not impose, and
// FNFE's own validation reports for those same documents record
// isValidBusinessRules = true. That is issue #56, and this file is the fix.
//
// # What Factur-X publishes instead, and what of it is here
//
// Two mechanical facts about these Schematrons are worth stating before anything
// else, because both defeat the way the rest of this package reads an artefact.
//
// First, a Factur-X assertion carries no id attribute. A rule is identified by an
// "[ID]-" prefix on its message text, so an enumeration written the way CEN's,
// KoSIT's and OpenPEPPOL's are written finds zero identifiers here.
//
// Second, most assertions have no identifier at all: 51 of 107 in MINIMUM, and
// 1,241 of 1,458 in EXTENDED. They are not stray. They are a generated,
// per-profile *data model* — one assertion per element of the profile's element
// table — and they come in six mechanical shapes, which between them decide
// exactly the questions CEN's CII-SR-*/CII-DT-* rules decide:
//
//	count(X)=n, count(X)<=n, count(X)>=n   "Element 'X' must occur exactly n times"
//	report true()                          "Element 'X' is marked as not used in the given context"
//	assert @a                              "Attribute '@a' is required in this context"
//	report @a                              "Attribute @a' marked as not used in the given context"
//	assert string-length($cv)=0 or         "Value of 'X' is not allowed" — a lookup in an
//	  document('..._codedb.xml')/...       external code database the bundle ships beside the .sch
//
// That layer is where the disagreement lives. CEN's CII-DT-027 forbids
// ram:FormattedIssueDateTime on every document reference but the preceding
// invoice; Factur-X's EN 16931 profile forbids it on ram:AdditionalReferencedDocument
// with type code 50, 130 and 916 by name, and its EXTENDED profile permits it there.
// Same question, per-profile answer.
//
// All three layers are evaluated here:
//
//   - the CEN-minted binding assertions Factur-X does carry, in facturXBinding
//     below, reported under SourceEN16931 because CEN minted the identifier, the
//     condition and the wording — Factur-X quotes them, which
//     TestFacturXQuotesCENsConditionVerbatim checks assertion by assertion;
//   - the nine BR-FXEXT-* rules that are Factur-X's own new ground, reported
//     under SourceFacturX, and the 24 that restate a CEN identifier the profile
//     drops, which facturx_restatements.go evaluates and argues rule by rule;
//   - the per-profile data model, all 2,159 assertions of it, generated from the
//     five Schematrons and the five code databases FNFE ships beside them by
//     internal/gen/facturx, and evaluated by facturx_datamodel.go. Not one of
//     them carries an identifier in the artefact, so each is reported under a key
//     this package mints — FX-DM-<PROFILE>-<NNNN> — which facturx_datamodel.go
//     documents, along with what that key is and is not worth.
//
// Coverage(SourceFacturX) is what is left, and none of it is fatal and evaluable:
// CII-SR-464, which FNFE rewrote into a tautology; three data-model assertions
// FNFE published that contradict their own context; one OpenPEPPOL rule FNFE
// merges in; and the eighteen -08ini/-08rev readings FNFE flags warning. So
// Report.Conformant is true again for a clean document under a Validate call, and
// Report.Complete is false on an advisory gap, which is what those two methods are
// for.
//
// # Which BR-FXEXT-* are here, and which are in facturx_restatements.go
//
// EXTENDED publishes 50 BR-FXEXT-* identifiers and MINIMUM one more, 51 in all.
// They split cleanly in two, and the split is why they are implemented in two
// files rather than one:
//
//   - 42 are restatements of CEN identifiers the EXTENDED profile drops.
//     EXTENDED omits 41 of the identifiers the EN 16931 profile publishes —
//     BR-22/23/24/26/38/44, BR-CO-04/10/11/12/13/15/16, BR-S-08/09, BR-AE-08,
//     BR-E-08, BR-G-08, BR-IC-08, BR-Z-08 and the rest — and puts back a variant
//     of most of them, usually one that carves out the sub-line structure
//     (LineStatusReasonCode = DETAIL / GROUP) or the third-party and logistics
//     charge amounts EXTENDED adds. BR-FXEXT-BR-22 is BR-22 restricted to lines
//     whose subtype is DETAIL or absent.
//
//     The 24 of those FNFE leaves unflagged are evaluated in
//     facturx_restatements.go, which states for each what CEN asserts, what FNFE
//     asserts, and in which direction the two differ — because "the restatement
//     is looser" turned out to be true of fewer than half of them. The other 18
//     are the -08ini/-08rev pair of each VAT category, which FNFE flags warning
//     and Coverage(SourceFacturX) names.
//
//     CEN's original is still evaluated for all 41, which is a separate decision
//     and is recorded in facturXCENOmissions: an authority's omission of a CEN
//     identifier is recorded, not acted on, because 18 of the 41 have no
//     replacement at all and dropping them would be 18 false negatives. The
//     consequence is that both identifiers report on a document that breaks both;
//     facturx_restatements.go argues why that is right and Source is what
//     separates them.
//
//     What is *not* reported is a CEN finding the document's own authority
//     overruled: for the 21 replacements that can be satisfied where CEN's rule
//     fires, facturXAuthorityParity drops the CEN half on the Factur-X path. That
//     is a verdict, not a coverage decision — the rule runs either way — and it
//     is what keeps this package from refusing invoices Factur-X accepts.
//
//   - 9 are Factur-X's own new ground — the BT-X-* extension terms, the sub-line
//     structure, the date format qualifier 205 CEN's binding never had a rule for.
//     Those are implemented here.
//
// # Severity
//
// Factur-X flags 21 of its assertions warning and leaves the rest unflagged, so
// within this artefact the absence of a flag is evidence rather than a default:
// an unflagged BR-FXEXT-* rule is fatal, and the one that carries flag="warning"
// among the nine implemented here — BR-FXEXT-04 — is a warning. The same reading
// is what makes the 24 restatements fatal, and it is what leaves the eighteen
// -08ini/-08rev out of them.
//
// The CEN-minted assertions are different, and they are reported at CEN's flag,
// not at Factur-X's absent one. CII-SR-463 and CII-DT-097 are fatal in
// EN16931-CII-syntax.sch and CII-SR-464/465/466 are warnings there, and that is
// how they come back from here. The reason is D7's: severity is a quotation of a
// published flag and never an inference, and for these five identifiers there is a
// published flag — CEN's. Reading Factur-X's absent attribute as "fatal" for a rule
// CEN flags warning would be this package deciding a severity, which is what C29
// found it doing to seven XRechnung rules. The consequence is stated rather than
// hidden: a Factur-X processor rejects a document for CII-SR-465 and this package
// warns. Coverage(SourceFacturX) records it.
//
// # Rule order
//
// The five Schematrons each hold one large pattern carrying the data model — 34
// rules in MINIMUM, 901 in EXTENDED — and one single-rule pattern per business
// rule. Under ISO Schematron a node goes to the first matching rule in a pattern
// only, so order matters inside the large one and nowhere else.
//
// Of the assertions implemented here, CII-SR-465 and CII-SR-466 sit in the large
// pattern (on ram:ApplicableHeaderTradeAgreement) and everything else is in a
// pattern of its own. TestFacturXRulesAreReachableInTheirPattern re-derives that
// from the artefact: for every implemented assertion it checks that no earlier
// rule in the same pattern can claim its context nodes. This is the fourth
// distinct place ISO rule ordering has decided something in this package (C42),
// and the first where checking it was cheap because the artefact's contexts are
// absolute paths.

// ciiBinding names whose CII syntax binding a validation applies.
//
// It is a separate axis from Profile because the two are independent facts: which
// tier of Factur-X's data model a document claims, and whose binding it is judged
// by. Eight callers in this package pass ciiBindingCEN — the CIUS whose published
// artefacts include CEN's EN16931-CII-validation — and only Validate passes
// ciiBindingFacturX.
type ciiBinding int

const (
	// ciiBindingCEN is EN 16931-3-3 as CEN publishes it: the 42 fatal CII-SR-*,
	// the 70 fatal CII-DT-* and the 471 advisory assertions of both families.
	ciiBindingCEN ciiBinding = iota
	// ciiBindingFacturX is the binding the Factur-X profile Schematrons publish.
	ciiBindingFacturX
)

// facturXProfileFromSpecID reads the data-richness tier out of a Specification
// identifier (BT-24), for the routing path where nobody handed this package a
// Profile.
//
// Four of the five tiers name themselves in BT-24 and the fifth does not: a
// Factur-X EN 16931 invoice declares CEN's own "urn:cen.eu:en16931:2017", which
// is the claim that it is exactly EN 16931 and is why specIDRules does not route
// it here at all. So the EN 16931 tier is what a Factur-X identifier this
// function does not recognise falls back to, which is the richest tier that
// excuses nothing — a future ":1p0:something" is then judged by the rules every
// tier from BASIC upwards shares rather than by CEN's binding, which is the
// defect this whole file exists to fix.
//
// basicwl is tested before basic because "factur-x.eu:1p0:basic" is a prefix of
// "factur-x.eu:1p0:basicwl", and the wrong order silently reads every BASIC WL
// document as BASIC — which would apply the invoice-line rules to a head-only
// document.
//
// # Both brands
//
// Each of the four self-naming tiers has two identifiers, not one. Factur-X 1.0
// and ZUGFeRD 2.x are the same specification published by two bodies — FNFE-MPE
// in France and FeRD in Germany — and FNFE's own code database says so: the
// enumeration its Schematron looks a document's BT-24 up in holds exactly
// {"urn:factur-x.eu:1p0:minimum", "urn:zugferd.de:2p0:minimum"} at MINIMUM, and
// the matching pair at BASIC WL, BASIC and EXTENDED. Only the EN 16931 tier
// publishes one value, CEN's own, for the reason above.
//
// Matching the French half alone read a ZUGFeRD-branded MINIMUM invoice as a
// plain EN 16931 CII document and judged it by CEN's binding, which is C44 with a
// different string in it: a MINIMUM invoice carries no invoice line and no VAT
// breakdown by design, so this package reported BR-16 and BR-CO-18 against a
// document its authority publishes as correct. Both brands are matched here and
// in specIDRules, and TestFacturXRoutingAcceptsEveryIdentifierTheAuthorityPublishes
// reads the pairs back out of the committed code lists, so a third brand — or a
// version bump past 1p0 / 2p0 — arrives as a failure rather than as silence.
func facturXProfileFromSpecID(specID string) (Profile, bool) {
	id := normSpecID(specID)
	for _, brand := range facturXBrands {
		switch {
		case strings.Contains(id, brand+"minimum"):
			return ProfileMinimum, true
		case strings.Contains(id, brand+"basicwl"):
			return ProfileBasicWL, true
		case strings.Contains(id, brand+"basic"):
			return ProfileBasic, true
		case strings.Contains(id, brand+"extended"):
			return ProfileExtended, true
		}
	}
	return ProfileEN16931, false
}

// facturXBrands are the two authorities' identifier stems, tier suffix excluded.
// They are the same specification: see facturXProfileFromSpecID.
var facturXBrands = []string{"factur-x.eu:1p0:", "zugferd.de:2p0:"}

// validateFacturXRouted is the Factur-X entry in modelValidators: the rule set
// ValidateCIUS and Detection.Validator reach for a document whose own BT-24
// declares a Factur-X profile.
//
// It exists because the routing path had the same defect the direct one did.
// ValidateCIUS is the entry point this package tells a caller to prefer, it
// arbitrates on the document's own Specification identifier, and an identifier
// reading "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended"
// matched no rule and fell through to the EN 16931 core with CEN's CII binding —
// which is issue #56 reached by a different door.
func validateFacturXRouted(r *run, p *parsed) []Violation {
	profile, _ := facturXProfileFromSpecID(p.inv.specID)
	return validateEN16931(r, p, profile, ciiBindingFacturX)
}

// facturXBindingRule is one CEN-minted binding assertion a Factur-X profile
// Schematron carries: the identifier, the profiles that carry it, and whether
// this package evaluates it.
//
// The profiles field is a set rather than a floor because the five profiles are
// not nested. MINIMUM publishes eleven identifiers BASIC WL does not, and
// EXTENDED drops 41 the EN 16931 profile publishes, so "from BASIC WL upwards" is
// a claim that has to be written out and checked rather than computed from an
// ordering that does not hold.
type facturXBindingRule struct {
	id       string
	profiles []Profile
	// anonymousIn are the profiles that carry this assertion with an empty
	// message, and therefore with no [ID] prefix to identify it by. MINIMUM
	// carries three of them. They are the same assertions, XPath for XPath, as
	// the ones BASIC WL names, and this package reports them under the identifier
	// the other four profiles give them: a rule the authority's own processor
	// reports without a name is still a rule it reports. It is also the reason
	// the survey in the issue counted MINIMUM's binding as empty.
	anonymousIn []Profile
	// evaluated is false for CII-SR-464, which Factur-X rewrote into a tautology.
	// See facturXInertBinding.
	evaluated bool
}

// facturXBinding is every CEN-minted CII binding identifier the five Factur-X
// Schematrons carry, with the profiles that carry it. It is the whole of
// Factur-X's overlap with EN 16931-3-3: four identifiers out of 583.
//
// TestFacturXBindingMatchesTheArtefact derives this table from the vendored
// Schematrons in both directions, so an identifier Factur-X publishes and this
// table omits fails as loudly as one this table invents.
var facturXBinding = []facturXBindingRule{
	{id: "CII-SR-463", profiles: allFacturXProfiles, anonymousIn: []Profile{ProfileMinimum}, evaluated: true},
	{id: "CII-SR-464", profiles: []Profile{ProfileBasicWL, ProfileBasic, ProfileEN16931, ProfileExtended}, evaluated: false},
	{id: "CII-SR-465", profiles: allFacturXProfiles, anonymousIn: []Profile{ProfileMinimum}, evaluated: true},
	{id: "CII-SR-466", profiles: allFacturXProfiles, anonymousIn: []Profile{ProfileMinimum}, evaluated: true},
	{id: "CII-DT-097", profiles: []Profile{ProfileExtended}, evaluated: true},
}

// allFacturXProfiles is the five profiles, for the binding entries that every
// Schematron carries. It is written out rather than reusing profiles because
// that slice is the order Profile documents its constants in and this is a set.
var allFacturXProfiles = []Profile{ProfileMinimum, ProfileBasicWL, ProfileBasic, ProfileEN16931, ProfileExtended}

// facturXOmission is one CEN identifier the EXTENDED profile's Schematron does
// not carry although the EN 16931 profile's does, with the BR-FXEXT-* rule
// FNFE put in its place, or "" where FNFE put nothing.
type facturXOmission struct {
	cen        string
	replacedBy string
}

// facturXCENOmissions is the per-rule coverage analysis for the EXTENDED
// profile, and the record of a decision not to act.
//
// EXTENDED publishes 206 - 41 + 52 identifiers relative to the EN 16931 profile:
// it drops 41 of CEN's and adds 50 BR-FXEXT-* of its own plus CII-DT-097 and
// BR-CO-25. Of the 41 dropped, 23 have a BR-FXEXT-* replacement — the same rule
// with a carve-out for the sub-line structure (BT-X-8) EXTENDED adds, or for the
// third-party charge amounts (BT-179) it adds — and 18 have none at all.
//
// This package evaluates CEN's original for all 41 and neither of the two
// alternatives to that is better. Dropping them would be 18 outright false
// negatives, which is the trap PR 29 avoided for CIUS-PT, where 114 dropped CEN
// rules had replacements for 11. Substituting the replacement for the 23 would
// change what BR-22 means for a caller and would need the 23 replacements
// implemented first. So the omission is recorded here, checked against the
// artefact by TestFacturXOmissionsMatchTheArtefact, and named in
// Coverage(SourceFacturX) — the same shape as ciusCENCopyOmissions, and for the
// same reason.
//
// The practical consequence used to be stated here as an accepted cost: this
// package reported CEN's BR-22 on an EXTENDED invoice whose GROUP line carries no
// invoiced quantity, and a Factur-X processor does not. That is no longer what
// happens. Evaluating CEN's rule and reporting it are two decisions, and the
// second one belongs to whichever authority governs the document:
// facturXAuthorityParity drops the CEN finding at EXTENDED whenever the
// restatement that replaced it is satisfied, for the 21 identifiers where that
// can happen. The omission table below is unchanged and is still what records
// which CEN identifier the profile dropped and what FNFE put in its place.
var facturXCENOmissions = []facturXOmission{
	{cen: "BR-22", replacedBy: "BR-FXEXT-BR-22"},
	{cen: "BR-23", replacedBy: "BR-FXEXT-BR-23"},
	{cen: "BR-24", replacedBy: "BR-FXEXT-BR-24"},
	{cen: "BR-26", replacedBy: "BR-FXEXT-BR-26"},
	{cen: "BR-27"},
	{cen: "BR-28"},
	{cen: "BR-38", replacedBy: "BR-FXEXT-BR-38"},
	{cen: "BR-44", replacedBy: "BR-FXEXT-BR-44"},
	{cen: "BR-AE-08", replacedBy: "BR-FXEXT-AE-08b"},
	{cen: "BR-AF-08", replacedBy: "BR-FXEXT-AF-08b"},
	{cen: "BR-AF-10"},
	{cen: "BR-AG-08", replacedBy: "BR-FXEXT-AG-08b"},
	{cen: "BR-AG-10"},
	{cen: "BR-CO-04", replacedBy: "BR-FXEXT-CO-04"},
	{cen: "BR-CO-06"},
	{cen: "BR-CO-08"},
	{cen: "BR-CO-10", replacedBy: "BR-FXEXT-CO-10"},
	{cen: "BR-CO-11", replacedBy: "BR-FXEXT-CO-11"},
	{cen: "BR-CO-12", replacedBy: "BR-FXEXT-CO-12"},
	{cen: "BR-CO-13", replacedBy: "BR-FXEXT-CO-13"},
	{cen: "BR-CO-15", replacedBy: "BR-FXEXT-CO-15"},
	{cen: "BR-CO-16", replacedBy: "BR-FXEXT-CO-16"},
	{cen: "BR-CO-17"},
	{cen: "BR-CO-22"},
	{cen: "BR-CO-24"},
	{cen: "BR-E-08", replacedBy: "BR-FXEXT-E-08b"},
	// BR-G-08's replacement is the one BR-FXEXT-* identifier that is not an
	// EXTENDED rule. FNFE carries "BR-FXEXT-G-08" — no trailing b, ini or rev — in
	// FACTUR-X_MINIMUM.sch, beside BR-G-08 itself, which no other profile does;
	// the MINIMUM Schematron publishes both readings of the same summation. It is
	// counted here rather than given a row of its own because it restates the same
	// CEN identifier, and it is the 51st BR-FXEXT-* identifier the inventory finds.
	{cen: "BR-G-08", replacedBy: "BR-FXEXT-G-08b"},
	{cen: "BR-IC-08", replacedBy: "BR-FXEXT-IC-08b"},
	{cen: "BR-O-02"},
	{cen: "BR-O-03"},
	{cen: "BR-O-04"},
	{cen: "BR-O-08", replacedBy: "BR-FXEXT-O-08b"},
	{cen: "BR-O-11"},
	{cen: "BR-O-12"},
	{cen: "BR-O-13"},
	{cen: "BR-O-14"},
	{cen: "BR-S-08", replacedBy: "BR-FXEXT-S08b"},
	{cen: "BR-S-09", replacedBy: "BR-FXEXT-S-09b"},
	{cen: "BR-S-10"},
	{cen: "BR-Z-08", replacedBy: "BR-FXEXT-Z-08b"},
	{cen: "BR-Z-10"},
}

// facturXInertBinding is the identifier Factur-X carries and no processor can
// report, with the evidence. It is the same kind of fact as CEN's BR-CO-05..08
// bound to true(), and it is recorded in Coverage(SourceFacturX) as Unevaluable
// for the same reason: a rule nobody can evaluate is not a rule this package
// skipped.
//
// CEN's CII-SR-464 tests not(ram:PayerSpecifiedDebtorFinancialInstitution).
// Factur-X's, in all four profiles that carry it, tests
//
//	(A or B) or (not(A) and not(B))
//
// over A = ram:PayeeSpecifiedCreditorFinancialInstitution and
// B = ram:PayerSpecifiedDebtorFinancialInstitution. That is a tautology: the first
// disjunct holds unless both are absent, and then the second does. It cannot fail
// on any document. TestFacturXInertBindingIsStillInert re-derives the XPath from
// the artefact and fails if FNFE ever fixes it.
const facturXInertBinding = "CII-SR-464"

// facturXCarries reports whether the profile's Schematron carries this binding
// identifier.
func (b facturXBindingRule) carries(p Profile) bool {
	for _, q := range b.profiles {
		if q == p {
			return true
		}
	}
	return false
}

// facturXBindingApplies reports whether the binding assertion for id is both
// carried by this profile and evaluable.
func facturXBindingApplies(id string, p Profile) bool {
	for _, b := range facturXBinding {
		if b.id == id {
			return b.evaluated && b.carries(p)
		}
	}
	return false
}

// validateFacturXRules is the CII half of a validation run under a Factur-X
// profile: the binding assertions that profile's Schematron carries, and the
// BR-FXEXT-* rules EXTENDED adds.
//
// It is a no-op for any other root, so a UBL invoice passing through Validate is
// never asked to answer a Factur-X rule — Factur-X publishes no UBL binding, and
// inventing one from the CII text is what C36 found four national rule sets doing.
func validateFacturXRules(r *run, p *parsed, profile Profile) []Violation {
	if p == nil || p.root == nil || p.root.name != "CrossIndustryInvoice" {
		return nil
	}
	out := facturXBindingRules(p.root, profile)
	if r.stopped() {
		return out
	}
	out = append(out, validateFacturXExtensionRules(p, profile, nil)...)
	if r.stopped() {
		return out
	}
	// And the 24 fatal BR-FXEXT-* restatements of a CEN identifier the profile
	// drops, which facturx_restatements.go argues rule by rule.
	out = append(out, validateFacturXRestatements(r, p, profile, nil)...)
	if r.stopped() {
		return out
	}
	// And the profile data model, which is the bulk of what Factur-X publishes in
	// place of CEN's binding: 48 assertions in MINIMUM up to 1,241 in EXTENDED.
	// It comes last because the named rules are the ones a reader of FNFE's own
	// validation report will recognise, and a caller reading Violations in order
	// meets those first.
	return append(out, facturXDataModelRules(r, p.root, profile)...)
}

// facturXBindingRules evaluates the CEN-minted binding assertions this profile
// carries, in the wording and at the severity CEN publishes for them.
func facturXBindingRules(root *ciiNode, profile Profile) []Violation {
	var out []Violation
	add := adder(&out, SourceEN16931)
	warn := advisoryAdder(&out, SourceEN16931)

	// CII-SR-463, on `//ram:SpecifiedTradeAllowanceCharge`: `(ram:ChargeIndicator)`.
	// CEN flags it fatal.
	if facturXBindingApplies("CII-SR-463", profile) {
		for _, ac := range root.findAll("SpecifiedTradeAllowanceCharge") {
			if !hasChild(ac, "ChargeIndicator") {
				add("CII-SR-463", "An allowance or charge group (BG-20/21, BG-27/28) shall carry a charge indicator")
			}
		}
	}

	// CII-SR-465 and CII-SR-466, on the header trade agreement:
	// `not(ram:SellerTradeParty/ram:DefinedTradeContact/ram:PersonName and
	// ram:SellerTradeParty/ram:DefinedTradeContact/ram:DepartmentName)`, and the
	// same over ram:BuyerTradeParty. CEN flags both warning.
	for _, tx := range root.all("SupplyChainTradeTransaction") {
		for _, ag := range tx.all("ApplicableHeaderTradeAgreement") {
			both := func(party string) bool {
				return countAt(ag, party, "DefinedTradeContact", "PersonName") > 0 &&
					countAt(ag, party, "DefinedTradeContact", "DepartmentName") > 0
			}
			if facturXBindingApplies("CII-SR-465", profile) && both("SellerTradeParty") {
				warn("CII-SR-465", "Only one BT-41 element is allowed on an invoice")
			}
			if facturXBindingApplies("CII-SR-466", profile) && both("BuyerTradeParty") {
				warn("CII-SR-466", "Only one BT-56 element is allowed on an invoice")
			}
		}
	}

	// CII-DT-097, on `//udt:DateTimeString[@format = '102']`. EXTENDED only, and
	// CEN flags it fatal. The udt/qdt distinction is recovered from the position
	// exactly as en16931_cii_rules.go recovers it, because this tree is keyed by
	// local name.
	if facturXBindingApplies("CII-DT-097", profile) {
		for _, d := range facturXDateStrings(root, "102") {
			if !ciiDate102.MatchString(d.text) {
				add("CII-DT-097", fmt.Sprintf("A date declaring format 102 shall be written YYYYMMDD, not %q", strings.TrimSpace(d.text)))
			}
		}
	}
	return out
}

// facturXDateStrings is `//udt:DateTimeString[@format = f]`: every date string
// declaring that format qualifier, excluding the one place the CII schema puts a
// qdt:DateTimeString rather than a udt one.
func facturXDateStrings(root *ciiNode, format string) []*ciiNode {
	var out []*ciiNode
	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		if n.name == "DateTimeString" && n.attr("format") == format &&
			(parent == nil || parent.name != "FormattedIssueDateTime") {
			out = append(out, n)
		}
		for _, c := range n.children {
			rec(c, n)
		}
	}
	rec(root, nil)
	return out
}

// facturXDate205 is the form UN/EDIFACT format qualifier 205 names,
// CCYYMMDDHHMM. FNFE writes it as an XSD regular expression and this is the same
// expression; Go's syntax accepts it unchanged. The message calls it
// YYYYMMDDHHMMSS and the expression admits no seconds, which is FNFE's
// discrepancy and not this package's: the XPath is the rule.
var facturXDate205 = regexp.MustCompile(`^\s*(\d{4})(1[0-2]|0[1-9])(3[01]|[12][0-9]|0[1-9])([01][0-9]|2[0-3])[0-5][0-9]\s*$`)

// facturXProductCharacteristicCodes is the code list BR-FXEXT-04 admits:
// UNTDED 6313 plus the Factur-X extension values, transcribed from the space
// separated list inside FNFE's own contains() test. 315 values.
var facturXProductCharacteristicCodes = func() map[string]bool {
	const codes = "A AAA AAB AAC AAD AAF AAG AAH AAI AAJ AAK AAM AAN AAO AAP AAQ AAR AAS AAT AAU AAV AAW AAX AAY AAZ " +
		"ABA ABB ABC ABD ABE ABF ABG ABH ABI ABJ ABK ABL ABM ABN ABO ABP ABS ABT ABX ABY ABZ ACA ACE ACG ACN ACP " +
		"ACS ACV ACW ACX ADR ADS ADT ADU ADV ADW ADX ADY ADZ AEA AEB AEC AED AEE AEF AEG AEH AEI AEJ AEK AEL AEM " +
		"AEN AEO AEP AEQ AER AES AET AEU AEV AEW AEX AEY AEZ AF AFA AFB AFC AFD AFE AFF AFG AFH AFI AFJ AFK AFL " +
		"AFM AFN AFO AFP AFQ AFR AFS AFT AFU AFV AFW AFX B BL BMY BMZ BNA BNB BNC BND BNE BNF BNG BNH BNI BNJ " +
		"BNK BNL BNM BNN BNO BNP BNQ BNR BNS BNT BNU BNV BNW BNX BNY BNZ BR BRA BRB BRC BRD BRE BRF BRG BRH BRI " +
		"BRJ BRK BRL BRM BRN BRO BRP BRQ BRR BRS BRT BRU BRV BS BSW BSX BSY BSZ BTA BTB BTC BTD BTE BTF BTG BTH " +
		"BTI BTJ BTK BTL BTM BW CHN CHO CM CT CV CZ D DI DL DN DP DR DS DW E EA F FI FL FN FV GG GW HF HM HT IB " +
		"ID L LM LN LND M MO MW N OD PRS PTN RA RF RJ RMW RP RUN RY SQ T TC TH TN TT VGM VH VW WA WD WM WU XH XQ " +
		"XZ YS ZAL ZAS ZB ZBI ZC ZCA ZCB ZCE ZCL ZCO ZCR ZCU ZFE ZFS ZGE ZH ZK ZMG ZMN ZMO ZN ZNA ZNB ZNI ZO ZP " +
		"ZPB ZS ZSB ZSE ZSI ZSL ZSN ZTA ZTE ZTI ZV ZW ZWA ZZN ZZR ZZZ " +
		"BEST_BEFORE_DATE COLOR_TEXT COMMISSION DEPOSIT_SYSTEM DEPOSIT_TYPE ENERGY_CLASS EXPIRATION_DATE FEE " +
		"KIND_OF_ARTICLE MATERIAL METER_LOCATION METER_NUMBER ORGANIC_CONTROL_BODY PACKAGING_MATERIAL " +
		"PACKAGING_TYPE PROMOTIONAL_VARIANT SEAL_NUMBER SIZE_CODE SIZE_TEXT TRADING_UNIT WASTE_CODE " +
		"WASTE_FRACTION WEEE_NUMBER"
	out := map[string]bool{}
	for _, c := range strings.Fields(codes) {
		out[c] = true
	}
	return out
}()

// facturXExtensionRules is every BR-FXEXT-* identifier this package evaluates,
// in the order the rule bodies below report them. It is read by the coverage
// guard, so "which BR-FXEXT-* are implemented" is answered by one list rather
// than by grepping for add() calls.
var facturXExtensionRules = []string{
	"BR-FXEXT-01", "BR-FXEXT-02", "BR-FXEXT-03", "BR-FXEXT-04",
	"BR-FXEXT-06", "BR-FXEXT-08", "BR-FXEXT-11", "BR-FXEXT-12",
	"BR-FXEXT-CII-DT-097a",
}

// facturXExtensionSeverity is the flag FNFE puts on each of them, and it is what
// decides the severity at the emission site rather than a choice of adder in each
// rule body. One table, read by the code and by the severity guards, so the two
// cannot drift — which is the arrangement xrechnungFlags and peppolRules already
// use for the same reason.
//
// Only BR-FXEXT-04 carries flag="warning" in FACTUR-X_EXTENDED.sch. The rest
// carry no flag, and within an artefact that uses the attribute explicitly on 21
// assertions that is evidence rather than a default; facturx.go's severity
// section argues it, and TestFacturXExtensionSeveritiesMatchTheArtefact reads the
// flags back out of the file.
var facturXExtensionSeverity = map[string]Severity{
	"BR-FXEXT-04": SeverityWarning,
}

// facturXLine is one ram:IncludedSupplyChainTradeLineItem with the three terms
// EXTENDED's sub-line rules read, resolved once.
type facturXLine struct {
	node *ciiNode
	// id and parent are BT-126 and BT-X-304, normalize-space'd.
	id, parent string
	// subtype is BT-X-8, the "[1]" of FNFE's union
	// (ram:LineStatusReasonCode | ram:AssociatedDocumentLineDocument/ram:LineStatusReasonCode |
	// ram:SpecifiedLineTradeSettlement/ram:LineStatusReasonCode) — the first of the
	// three in document order, which is what a "[1]" on a union selects.
	subtype string
	// total is BT-131 as written, and totalOK whether it parses as a number. An
	// unparseable or absent amount is what makes FNFE's number() NaN, and a
	// comparison against NaN is false, so the two are one fact here.
	total   float64
	totalOK bool
	// hasTotal is whether the element is present with non-empty content, which is
	// FNFE's `normalize-space(...) != ''` and is a different test from totalOK.
	hasTotal bool
}

// facturXLines resolves every invoice line in the document, in document order.
func facturXLines(root *ciiNode) []facturXLine {
	var out []facturXLine
	for _, n := range root.findAll("IncludedSupplyChainTradeLineItem") {
		l := facturXLine{node: n}
		if ids := nodesAt(n, "AssociatedDocumentLineDocument", "LineID"); len(ids) > 0 {
			l.id = normalizeSpace(ids[0].text)
		}
		if ps := nodesAt(n, "AssociatedDocumentLineDocument", "ParentLineID"); len(ps) > 0 {
			l.parent = normalizeSpace(ps[0].text)
		}
		l.subtype = facturXSubtype(n)
		if ts := nodesAt(n, "SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount"); len(ts) > 0 {
			l.hasTotal = normalizeSpace(ts[0].text) != ""
			l.total, l.totalOK = parseAmount(ts[0].text)
		}
		out = append(out, l)
	}
	return out
}

// facturXSubtype is BT-X-8 for one line: the first ram:LineStatusReasonCode in
// document order among the line's own children, its AssociatedDocumentLineDocument
// and its SpecifiedLineTradeSettlement, which is the node set FNFE's union
// expression selects and the "[1]" reduces.
func facturXSubtype(line *ciiNode) string {
	for _, c := range line.children {
		switch c.name {
		case "LineStatusReasonCode":
			return normalizeSpace(c.text)
		case "AssociatedDocumentLineDocument", "SpecifiedLineTradeSettlement":
			for _, g := range c.children {
				if g.name == "LineStatusReasonCode" {
					return normalizeSpace(g.text)
				}
			}
		}
	}
	return ""
}

// validateFacturXExtensionRules evaluates the nine BR-FXEXT-* rules that are
// Factur-X's own new ground. They are published by the EXTENDED profile only —
// with the one exception recorded in facturXCENOmissions — so this is a no-op at
// every other profile, which is the profile scoping this whole file exists for.
//
// seen is the reachability bookkeeping cius_contexts_test.go reads; it is nil on
// every production path.
func validateFacturXExtensionRules(p *parsed, profile Profile, seen ruleContexts) []Violation {
	if profile != ProfileExtended || p == nil || p.root == nil || p.root.name != "CrossIndustryInvoice" {
		return nil
	}
	root := p.root
	var out []Violation
	fatal := adder(&out, SourceFacturX)
	advisory := advisoryAdder(&out, SourceFacturX)
	// One emission helper for the whole file, reading the severity out of the
	// table rather than out of which function a rule body happened to call.
	add := func(rule, msg string) {
		if facturXExtensionSeverity[rule] == SeverityWarning {
			advisory(rule, msg)
			return
		}
		fatal(rule, msg)
	}

	// BR-FXEXT-01, on /rsm:CrossIndustryInvoice/rsm:ExchangedDocument/
	// ram:IncludedNote[ram:SubjectCode]: `(ram:ContentCode) or (ram:Content)`.
	for _, ed := range root.all("ExchangedDocument") {
		for _, note := range ed.all("IncludedNote") {
			if !hasChild(note, "SubjectCode") {
				continue
			}
			seen.reached("BR-FXEXT-01")
			if !hasChild(note, "ContentCode") && !hasChild(note, "Content") {
				add("BR-FXEXT-01", "An invoice note carrying a subject code (BT-21) shall carry the coded message free text (BT-X-5) or the message free text (BT-22)")
			}
		}
	}

	// BR-FXEXT-02, the same at line level, on
	// //ram:AssociatedDocumentLineDocument/ram:IncludedNote[ram:SubjectCode].
	for _, adl := range root.findAll("AssociatedDocumentLineDocument") {
		for _, note := range adl.all("IncludedNote") {
			if !hasChild(note, "SubjectCode") {
				continue
			}
			seen.reached("BR-FXEXT-02")
			if !hasChild(note, "ContentCode") && !hasChild(note, "Content") {
				add("BR-FXEXT-02", "An invoice line note carrying a subject code (BT-X-10) shall carry the coded line free text (BT-X-9) or the line free text (BT-127)")
			}
		}
	}

	// BR-FXEXT-03, on //ram:SpecifiedTaxRegistration/ram:ID[not(ancestor::
	// ram:SellerTradeParty)]: `@schemeID='VA'`. Every party but the seller may
	// carry a VAT registration identifier and nothing else.
	for _, id := range facturXNonSellerTaxRegistrationIDs(root) {
		seen.reached("BR-FXEXT-03")
		if id.attr("schemeID") != "VA" {
			add("BR-FXEXT-03", "Only a VAT registration identifier (@schemeID='VA') may be given for a party other than the Seller")
		}
	}

	// BR-FXEXT-04, on //ram:ApplicableProductCharacteristic/ram:TypeCode: the
	// value shall carry no space and shall be in UNTDED 6313 as Factur-X extends
	// it. FNFE flags this one warning.
	for _, ch := range root.findAll("ApplicableProductCharacteristic") {
		for _, tc := range ch.all("TypeCode") {
			seen.reached("BR-FXEXT-04")
			v := normalizeSpace(tc.text)
			if strings.Contains(v, " ") || !facturXProductCharacteristicCodes[v] {
				add("BR-FXEXT-04", fmt.Sprintf("An item attribute type code (BT-X-11) should be a UNTDED 6313 or Factur-X extension value, not %q", v))
			}
		}
	}

	lines := facturXLines(root)

	// BR-FXEXT-06, on //ram:IncludedSupplyChainTradeLineItem: a line that names a
	// parent, or that is named as a parent by another line, shall carry a subtype
	// (BT-X-8). FNFE writes it as the negation of the two failing cases.
	parented := map[string]bool{}
	for _, l := range lines {
		if l.parent != "" {
			parented[l.parent] = true
		}
	}
	for _, l := range lines {
		seen.reached("BR-FXEXT-06")
		if l.subtype != "" {
			continue
		}
		if l.parent != "" || (l.id != "" && parented[l.id]) {
			add("BR-FXEXT-06", "An invoice line that is part of a sub-line structure shall carry a line item subtype (BT-X-8)")
		}
	}

	// BR-FXEXT-08, on //ram:IncludedSupplyChainTradeLineItem. The test is
	// document-global — "every GROUP line carrying a net amount equals the sum of
	// its DETAIL and GROUP children's net amounts" — and the context is every
	// line, so a document that breaks it reports once per line. That is what a
	// Schematron processor does with this rule and it is transcribed rather than
	// deduplicated, as CII-SR-439/441 are in en16931_cii_rules.go.
	//
	// FNFE compares with `=` over number(), so a child with no net amount makes
	// the sum NaN and the assertion fail; that case is kept. The comparison itself
	// is to two decimals, which is this package's reading of equality between two
	// amounts and the same epsilon the BR-CO-* summations use.
	if bad := facturXGroupTotalMismatch(lines); bad != "" {
		for range lines {
			seen.reached("BR-FXEXT-08")
			add("BR-FXEXT-08", bad)
		}
	} else {
		for range lines {
			seen.reached("BR-FXEXT-08")
		}
	}

	// BR-FXEXT-11, on //ram:IncludedSupplyChainTradeLineItem[normalize-space(
	// ram:AssociatedDocumentLineDocument/ram:ParentLineID) != '']: the parent
	// identifier shall be some line's own BT-126.
	ids := map[string]bool{}
	for _, l := range lines {
		if l.id != "" {
			ids[l.id] = true
		}
	}
	for _, l := range lines {
		if l.parent == "" {
			continue
		}
		seen.reached("BR-FXEXT-11")
		if !ids[l.parent] {
			add("BR-FXEXT-11", fmt.Sprintf("The parent line identifier (BT-X-304) %q names no invoice line identifier (BT-126) in this invoice", l.parent))
		}
	}

	// BR-FXEXT-12, on a GROUP line that carries a net amount: every GROUP child of
	// it shall carry one too. FNFE writes it as a count comparison, which is the
	// same claim.
	for _, l := range lines {
		if l.subtypeIsGroup() && l.hasTotal {
			seen.reached("BR-FXEXT-12")
			for _, c := range lines {
				if c.parent == l.id && c.subtypeIsGroup() && !c.hasTotal {
					add("BR-FXEXT-12", fmt.Sprintf("The invoice line %q is a subtotal (BT-X-8 = GROUP) of a subtotal carrying a net amount (BT-131), so it shall carry one as well", c.id))
				}
			}
		}
	}

	// BR-FXEXT-CII-DT-097a, on //udt:DateTimeString[@format = '205'].
	for _, d := range facturXDateStrings(root, "205") {
		seen.reached("BR-FXEXT-CII-DT-097a")
		if !facturXDate205.MatchString(d.text) {
			add("BR-FXEXT-CII-DT-097a", fmt.Sprintf("A date declaring format 205 shall be written YYYYMMDDHHMM, not %q", strings.TrimSpace(d.text)))
		}
	}
	return out
}

// subtypeIsGroup is FNFE's `ram:AssociatedDocumentLineDocument/
// ram:LineStatusReasonCode = 'GROUP'`. BR-FXEXT-12 writes the path out rather
// than using the union BR-FXEXT-06 uses, and this follows the path it writes:
// the two are the same node in every document the corpus holds, and the rule that
// matters is the one the artefact states.
func (l facturXLine) subtypeIsGroup() bool {
	return childValueIs(orEmpty(nodesAt(l.node, "AssociatedDocumentLineDocument")), "LineStatusReasonCode", "GROUP")
}

// orEmpty returns the first node or a childless stand-in, so a rule body can ask
// about a group that is not there without a nil check of its own.
func orEmpty(ns []*ciiNode) *ciiNode {
	if len(ns) > 0 {
		return ns[0]
	}
	return &ciiNode{}
}

// facturXGroupTotalMismatch returns the message for the first GROUP line whose
// net amount does not equal the sum of its DETAIL and GROUP children's, or "".
func facturXGroupTotalMismatch(lines []facturXLine) string {
	for _, l := range lines {
		if !l.subtypeIsGroup() || !l.hasTotal {
			continue
		}
		sum, ok := 0.0, l.totalOK
		for _, c := range lines {
			if c.parent != l.id {
				continue
			}
			if !c.subtypeIsDetail() && !c.subtypeIsGroup() {
				continue
			}
			if !c.totalOK {
				ok = false
				continue
			}
			sum += c.total
		}
		if !ok || round2(sum) != round2(l.total) {
			return fmt.Sprintf("The subtotal line %q carries a net amount (BT-131) that is not the sum of its DETAIL and GROUP lines' net amounts", l.id)
		}
	}
	return ""
}

// subtypeIsDetail is the DETAIL half of BR-FXEXT-08's child selection, written
// the way the artefact writes it.
func (l facturXLine) subtypeIsDetail() bool {
	return childValueIs(orEmpty(nodesAt(l.node, "AssociatedDocumentLineDocument")), "LineStatusReasonCode", "DETAIL")
}

// facturXNonSellerTaxRegistrationIDs is `//ram:SpecifiedTaxRegistration/ram:ID[
// not(ancestor::ram:SellerTradeParty)]` — every tax registration identifier
// except the seller's, which BR-FXEXT-03 restricts to VAT.
func facturXNonSellerTaxRegistrationIDs(root *ciiNode) []*ciiNode {
	var out []*ciiNode
	var rec func(n *ciiNode, underSeller bool)
	rec = func(n *ciiNode, underSeller bool) {
		if n.name == "SellerTradeParty" {
			underSeller = true
		}
		if !underSeller && n.name == "SpecifiedTaxRegistration" {
			out = append(out, n.all("ID")...)
		}
		for _, c := range n.children {
			rec(c, underSeller)
		}
	}
	rec(root, false)
	return out
}
