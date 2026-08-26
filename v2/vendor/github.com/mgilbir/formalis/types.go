// Package formalis validates electronic invoices against the EN 16931 semantic
// model, the Core Invoice Usage Specifications (CIUS) layered on top of it, and
// the national invoice formats that stand outside EN 16931 altogether. It uses
// nothing but the Go standard library.
//
// One syntax-neutral rule engine (fed by parseEN16931) serves both EN 16931
// syntaxes — UN/CEFACT Cross Industry Invoice (CII, used by Factur-X/ZUGFeRD)
// and OASIS UBL (Peppol BIS, XRechnung UBL, NLCIUS) — and each CIUS adds its own
// rule layer in its own file (xrechnung.go, peppol.go, nlcius.go, cius_pt.go,
// cius_ro.go, cius_be.go, cius_rs.go). The formats that are not EN 16931
// profiles at all — FatturaPA, Facturae, ebInterface, KSeF, Finvoice, TEAPPS,
// OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT — and the Order-X order
// document are checked against their own mandatory structure and code lists by
// validators of their own. Source names every one of these authorities, and a
// rule is identified by the pair (Source, Rule) rather than by Rule alone.
//
// # Entry points
//
// Every exported validator takes a context and the document's bytes and returns
// a Report and an error. Which one to call:
//
//   - Detect routes. It reports which format a document is in, in one streaming
//     pass that builds no tree, and Detection.Validator returns the entry point
//     that checks it.
//   - ValidateCIUS applies that same arbitration and validates in one call,
//     falling back to the EN 16931 core for a document that declares nothing.
//   - Validate checks a document as Factur-X, at a named Profile: the EN 16931
//     core, the rules that tier is expected to satisfy, and the syntax binding
//     FNFE-MPE publishes for it rather than CEN's. It is for a caller who has that
//     data-richness metadata from a container the invoice itself does not carry.
//   - ValidateEN16931 checks it as CEN's own EN 16931, with CEN's binding and no
//     Factur-X rule. It takes no Profile, because CEN publishes none.
//   - The format-specific validators (ValidateXRechnung, ValidateZATCA, …) are
//     the direct route when the format is already known.
//
// The twelve Is* predicates answer "is this document format X?" for one format
// each. They are independent tests and not a partition — more than one can be
// true of the same bytes — so route with Detect, which arbitrates between them
// in an order it documents.
//
// # The four answers a validator can give
//
// The distinctions this package exists for are all in how a call can end, so they
// are worth reading once:
//
//   - An error, with the zero Report: the input could not be read at all —
//     malformed XML, an encoding this package does not implement. A statement
//     about the file. See ErrMalformedXML and ErrUnsupportedEncoding.
//   - A Report holding a RuleLimit finding: the run stopped before it had seen
//     everything, so nothing can be concluded. A statement about the run, and
//     never an empty Violations slice. See RuleLimit and IsCheckerViolation.
//   - A Report holding findings: the document departs from rules that were
//     evaluated. Each carries the Severity its authority gave the rule, so
//     Report.Fatal and Report.Warnings separate "reject this" from "note this".
//   - A Report holding no findings: everything that was evaluated passed. What was
//     evaluated is the other half of the answer, and Report.NotEvaluated is where
//     it is written down.
//
// No rule set in this package evaluates everything its authority publishes: each
// is a documented subset. Coverage names the gaps for any Source, with the
// severity of each and whether anyone could evaluate it at all, and it takes no
// document, so a caller can ask what a validator will not look at before deciding
// to call it. Report.NotEvaluated repeats those gaps for the run that just
// happened; Report.Conformant is false whenever a rule that could have rejected
// this document was one a validator could have evaluated and this package did not,
// and Report.Complete whenever any evaluable rule went unevaluated. Today CEN's
// EN 16931 core is the one rule set with no unevaluated fatal rule and the one
// whose clean documents report both Conformant and Complete — reached by
// ValidateEN16931, or by ValidateCIUS on a document declaring no profile at all.
// Validate reports Conformant and not Complete, and every national format
// validator still names a fatal gap it could close and so reports false whatever
// the document.
//
// Validate spent two releases in that last group deliberately. It reported
// Conformant for a clean Factur-X document while it was judging one by CEN's CII
// syntax binding, which Factur-X does not adopt — 76 fatal findings on 13 of
// FNFE-MPE's own 59 published examples — so scoping the binding correctly made
// the answer false until Factur-X's own rule set was evaluated in its place. It
// is now: the per-profile data model, between 48 and 1,241 assertions a tier, and
// the 33 BR-FXEXT-* rules that are fatal, nine of them Factur-X's own new ground
// and 24 restatements of a CEN identifier the profile drops. What is left under
// SourceFacturX is advisory or unevaluable, which is why Conformant is true again
// and Complete is not.
//
// The distinction the third field carries is worth one sentence here, because it
// is what makes Complete answerable rather than permanently false: CEN publishes
// seven rules that no validator can evaluate — four bound to the XPath expression
// true(), three unreachable in CEN's own Schematron rule ordering — and a rule
// nobody can check is not a rule this package skipped. See RuleFamily.Unevaluable,
// which documents how narrow that is, Report and Coverage.
//
// Three rule sets report warnings, and every other finding in this package is
// fatal. CEN flags 1,168 of its two syntax bindings' assertions warning rather
// than fatal — the UBL-CR-*, UBL-DT-*, CII-SR-* and CII-DT-* rules that hold a
// document down to the EN 16931 core subset of UBL and CII — and this package
// evaluates all of them from tables generated out of CEN's own Schematron. KoSIT
// flags eleven of XRechnung's fifty-seven rules warning or information: the
// invoice type code, the specification identifier, the two IBAN checks, the
// telephone and email formats and five more. OpenPEPPOL flags six of its
// fifty-nine warning — the Italian, Danish and Swedish participant-identifier
// format checks — and one more, PEPPOL-EN16931-R120, is fatal in OpenPEPPOL's own
// Schematron and warning in the XRechnung artefact that merges it, so the same
// rule is a non-conformance on one path and advisory on the other. None of these
// is a verdict: a document whose only findings are these is Conformant, and a
// caller gating on Report.Fatal never sees them. See Severity and
// Report.Warnings.
//
// # Concurrency
//
// There is no global mutable state. The code-list tables, the compiled regexps
// and the generated syntax-binding tables (parsed once at load by
// en16931_syntax_advisory.go)
// are package-level values initialised once at load and only read afterwards,
// and every per-call artefact — the run, the parsed tree, the semantic model —
// is allocated inside the call. Every exported function may therefore be called
// from any number of goroutines at once; TestValidatorsAreSafeForConcurrentUse
// pins that under -race.
//
// # A note on pdf0, which this package's comments refer to
//
// This package reads XML and has no PDF dependency. pdf0
// (github.com/mgilbir/pdf0) is a separate, public sibling module that wraps it
// for the Factur-X container: pdf0 opens the PDF, extracts the attached invoice
// XML, and calls in here. The dependency runs one way — pdf0 requires formalis,
// and nothing in this module imports or needs pdf0 — so the references to it in
// limits.go, orderx.go and facturx_en16931.go are design constraints, not
// dependencies.
// They record conventions the two modules deliberately share, RuleLimit above
// all, so that a caller draining one mixed slice of container and invoice
// findings has one name to look for rather than two. A reader with no interest
// in PDF containers can disregard every one of them; nothing here changes
// behaviour because of pdf0.
package formalis

import (
	"fmt"
	"strings"
)

// Profile is a Factur-X/ZUGFeRD conformance profile: how much of the EN 16931
// semantic model a document undertakes to carry, in increasing data richness.
// The five values below are the whole set, and Profile means only this. It is
// not a rule set chosen by nationality: a Core Invoice Usage Specification is a
// separate concept carried by CIUS, reached through ValidateCIUS or a
// CIUS-specific validator such as ValidateXRechnung.
//
// Keeping the two apart is what makes the type checkable. While XRechnung was
// also a Profile constant it was accepted by Validate and applied no BR-DE-*
// rule, so the call that looked most like "validate this as XRechnung" was the
// one that validated it least; and because any string was accepted, a
// mistyped profile was silently read as EN 16931. Validate now reports a
// Profile it does not implement — see RuleProfile.
//
// # A Profile selects a rule set, and not only an excuse list
//
// Naming a Profile says "judge this document as Factur-X". Two things follow.
//
// The first is the excuse list, which is all a Profile used to be: MINIMUM and
// BASIC WL are head-only, so the invoice-line rules (BR-12, BR-16) are not
// applied to them, and MINIMUM additionally omits the buyer postal address
// (BR-10, BR-11), the VAT breakdown (BR-CO-18) and the amount-due summation
// (BR-CO-16). TestProfilesThatDifferStillDiffer pins each of those differences.
//
// EXTENDED used to carry a fourth entry — an exemption from the allowance and
// charge total summations (BR-CO-11, BR-CO-12) "whose operands it may carry
// unitemized" — and it is gone. Measured over the corpus, unitemized totals are a
// producer habit and not a property of the tier: two EXTENDED documents have them
// and three UBL documents do, while 23 EXTENDED documents were exempted that
// never needed it. Both rules now apply at every profile, and the two documents
// that needed the exemption get it from the authority that governs them rather
// than from a profile test — see facturXAuthorityParity.
//
// The second is which syntax binding a CII document is held to, and it is why
// this type is no longer only an excuse list. Factur-X publishes a CII binding of
// its own and does not adopt CEN's: the five profile Schematrons carry four of
// CEN's 583 CII-SR-*/CII-DT-* assertions, and in their place a per-profile data
// model of their own. Applying CEN's binding to a Factur-X document applies a rule
// set whose purpose is to hold a document down to the EN 16931 core subset of CII
// to a document whose profile exists to carry more than that subset — measured over
// FNFE's own 59 published examples, 13 conforming EXTENDED invoices were reported
// with 76 fatal findings naming rules Factur-X does not impose. So a Profile now
// selects Factur-X's binding, and EXTENDED additionally brings in 33 BR-FXEXT-*
// rules that are Factur-X's own — nine that are new ground and 24 that restate a
// CEN identifier EXTENDED drops — while MINIMUM brings in one more, BR-FXEXT-G-08.
// Those are the second way the named rule sets differ between tiers; the third is
// that 21 of those 24 restatements *supersede* the CEN identifier they restate, so
// EXTENDED can be silent on a CEN rule whose Factur-X replacement is satisfied.
// TestBasicEN16931AndExtendedDifferOnlyInTheRulesEXTENDEDPublishes pins that the
// list is exhaustive. See Validate, SourceFacturX and facturx_restatements.go.
//
// That data model is the larger half of what a Profile now decides, and it is
// where the five tiers differ most. It is one assertion per element of that
// tier's element table — 48 in MINIMUM, 196 in BASIC WL, 262 in BASIC, 412 in
// EN 16931, 1,241 in EXTENDED — and the answers are genuinely per tier: MINIMUM
// does not use the buyer postal address at all, the EN 16931 tier forbids a
// formatted issue date on a document reference by type code, and EXTENDED permits
// it. So the same document is not conformant at every tier, and it is not meant to
// be: a Profile names the tier a document *claims*, and the tier's element table
// is what that claim is worth. facturx_datamodel.go evaluates it and
// TestFacturXTiersDifferInTheirDataModel measures the difference.
//
// A caller who wants CEN's own EN 16931 verdict, with CEN's binding and no
// Factur-X rule at all, calls ValidateEN16931, which takes no Profile because CEN
// publishes none.
type Profile string

const (
	ProfileMinimum  Profile = "MINIMUM"
	ProfileBasicWL  Profile = "BASIC WL"
	ProfileBasic    Profile = "BASIC"
	ProfileEN16931  Profile = "EN 16931"
	ProfileExtended Profile = "EXTENDED"
)

// profiles is every Profile this package implements, in the order Profile
// documents them. It is the single list that knownProfile, the message
// unknownProfile writes, and TestProfileForRoundTrips all read.
var profiles = []Profile{ProfileMinimum, ProfileBasicWL, ProfileBasic, ProfileEN16931, ProfileExtended}

// knownProfile reports whether p is one of the five profiles this package
// implements. Validate consults it before doing any work; see RuleProfile.
func knownProfile(p Profile) bool {
	for _, k := range profiles {
		if p == k {
			return true
		}
	}
	return false
}

// conformanceKey folds an XMP ConformanceLevel string for matching: producers
// write both "EN 16931" and "EN16931", and "BASIC WL" and "BASICWL".
func conformanceKey(level string) string {
	return strings.ToUpper(strings.ReplaceAll(level, " ", ""))
}

// ProfileFor maps an XMP ConformanceLevel string to the Factur-X profile it
// names. The value is matched case- and space-insensitively.
//
// It reports false for "XRECHNUNG", which is a level a ZUGFeRD 2.x producer
// really does write but which names the German CIUS rather than a data-richness
// profile; CIUSFor maps that one. A caller reading a PDF's XMP therefore asks
// both, and gets from the pair the two facts the metadata actually carries —
// how rich the data claims to be, and which national rule set it claims to
// follow — instead of one value that conflates them. Neither returning
// ("", false) for the level (which loses it) nor returning a Profile that no
// validator honours (which was the bug) would do that.
func ProfileFor(level string) (Profile, bool) {
	switch conformanceKey(level) {
	case "MINIMUM":
		return ProfileMinimum, true
	case "BASICWL":
		return ProfileBasicWL, true
	case "BASIC":
		return ProfileBasic, true
	case "EN16931":
		return ProfileEN16931, true
	case "EXTENDED":
		return ProfileExtended, true
	}
	return "", false
}

// CIUSFor maps an XMP ConformanceLevel string to the CIUS it names, for the
// levels that name one rather than a Factur-X profile. It is the companion to
// ProfileFor over the same input, matched the same way; exactly one of the two
// reports true for any level either recognises.
//
// Today that is only "XRECHNUNG" (ZUGFeRD 2.x). The CIUS it returns is the one
// ValidateCIUS routes on and ValidateXRechnung implements, so a caller that
// reaches here reaches a validator that actually applies the BR-DE-* rules.
//
// This says what the container's metadata claims. DetectCIUS says what the
// invoice itself declares in BT-24, and that is the more reliable of the two:
// prefer it, and treat a disagreement as the container and its attachment
// describing different documents.
func CIUSFor(level string) (CIUS, bool) {
	switch conformanceKey(level) {
	case "XRECHNUNG":
		return CIUSXRechnung, true
	}
	return CIUSNone, false
}

// Source identifies the authority that defines a rule, so that a rule identifier
// is unique within it. Two authorities may mint the same string; (Source, Rule)
// is what identifies a rule, and Rule alone is not an identity.
//
// The distinction is not decorative. This package reports identifiers minted by
// CEN, by seven national bodies, and by itself, in one flat string field, and
// they have already collided: the Order-X validator once emitted "BR-O-01" for
// "an Order shall have an order number" while the EN 16931 rule engine emitted
// "BR-O-01" for the "Not subject to VAT" category family. A caller aggregating
// findings across a mailbox keyed by Rule merged two unrelated defects, and a
// suppression list ("we accept BR-O-03 from this supplier") suppressed the wrong
// thing in the other document type. Scoping every finding by its author makes
// that class of mistake impossible to express.
//
// Three judgement calls are recorded here because they are not obvious:
//
//   - The UBL and CII syntax-binding rules (UBL-DT-*, UBL-SR-*, CII-SR-*,
//     CII-DT-*) carry
//     SourceEN16931, not a Source of their own. Source names the *authority*, and
//     CEN publishes the bindings (EN 16931-3-2, EN 16931-3-3) as normative parts
//     of the same standard as the semantic model (EN 16931-1); they arrive in the
//     same conformance artefacts this package is tested against, and the FP=0
//     oracle counts them among the rules it must catch. Splitting them would also
//     make one finding's Source depend on the invoice's syntax, since the same
//     defect — two disagreeing payment means codes — is UBL-SR-47 on a UBL
//     invoice and CII-SR-467 on a CII one, which is a distinction the caller did
//     not ask for. A caller that does want
//     "syntax binding" separately has the prefix, which is already disjoint from
//     the core BR-* space.
//
//   - RuleLimit, RuleProfile and RuleRoot carry SourceChecker. They are statements
//     by this checker — "I stopped early", "you named a profile I do not
//     implement", "this is not an invoice" — rather than by any rule authority, so
//     attributing them to CEN would be a lie, and leaving them unattributed would
//     make Source unusable as a filter. See IsCheckerViolation for why that
//     predicate still tests Rule alone. A document this package could not read at
//     all produces no finding under any Source: it is an error, because there is
//     no document to attribute anything about.
//
//   - Most national formats below (FatturaPA, Facturae, ebInterface, KSeF,
//     Finvoice, TEAPPS, OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT,
//     Order-X) do not publish a rule identifier this package could quote, so the
//     identifiers under those Sources — "FPA-number", "ZA-uuid", "ORDER-01" —
//     were invented here. The Source is still the format they judge the document
//     against, which is what a caller routing or suppressing by format needs; it
//     is not a claim that the format's own documentation uses these names. The
//     Sources whose identifiers *are* quoted from a published rule set are
//     EN 16931, Factur-X, XRechnung, Peppol, NLCIUS, CIUS-PT, CIUS-RO, UBL.BE and
//     SRBDT.
type Source string

const (
	// SourceNone is the absent authority — no rule set. It is the zero Source,
	// and it is what Detect reports for a document it read and recognised as no
	// format this package validates; Detection.Recognised tests for it. No
	// Violation ever carries it: a finding with no Source is an emission site
	// that did not decide whose rule it reports, which
	// TestNoRuleIdentifierIsClaimedByTwoSources fails on.
	SourceNone Source = ""
	// SourceEN16931 is CEN's EN 16931 — the semantic model's core business rules
	// (BR-*, BR-CO-*, BR-CL-*, BR-DEC-*, BR-IC-*, and the VAT category families)
	// together with the UBL and CII syntax bindings (UBL-DT-*, UBL-SR-*, CII-SR-*,
	// CII-DT-*).
	SourceEN16931 Source = "EN 16931"
	// SourceFacturX is FNFE-MPE and FeRD's Factur-X / ZUGFeRD, the Franco-German
	// CII profile family. Its own identifiers are BR-FXEXT-*, the rule set the
	// EXTENDED profile adds on top of EN 16931.
	//
	// It is a Source rather than only a Profile because Factur-X binds EN 16931
	// with a rule set of its own and does not adopt CEN's CII syntax binding. Of
	// the 583 CII-SR-* and CII-DT-* assertions CEN publishes, the five Factur-X
	// Schematrons carry four (CII-SR-463..466, and CII-DT-097 in EXTENDED only);
	// in their place they carry a profile data-model layer of their own — 51
	// assertions at MINIMUM rising to 1,241 at EXTENDED — deciding, per profile,
	// which element may appear where, how often, and with which attributes. Those
	// are the same questions CEN's binding decides, and they get different answers,
	// which is the whole point of a profile family whose richest tier is defined by
	// carrying more than the EN 16931 core.
	//
	// So the identifiers a Factur-X document is judged by are mostly CEN's — the
	// BR-* core, which Factur-X republishes verbatim — and those keep
	// SourceEN16931, as CIUS-RO's copy of BR-27 does. This Source carries the
	// identifiers FNFE minted. See Validate, which is the entry point that selects
	// this rule set, and Coverage(SourceFacturX), which says what of it is not
	// evaluated.
	SourceFacturX Source = "Factur-X"
	// SourceXRechnung is the German KoSIT XRechnung CIUS (BR-DE-*).
	SourceXRechnung Source = "XRechnung"
	// SourcePeppol is OpenPEPPOL BIS Billing 3.0: PEPPOL-EN16931-* and
	// PEPPOL-COMMON-*, and the country-specific rules the same two Schematron files
	// publish under a comment reading "National rules" — DE-R-*, DK-R-*,
	// GR-R-*/GR-S-*, IS-R-*, IT-R-*, NL-R-*, NO-R-* and SE-R-*.
	//
	// Those last are OpenPEPPOL's own national rule sets and not the CIUS of the same
	// countries: NL-R-* is distinct from the BR-NL-* of SourceNLCIUS, and DE-R-* from
	// the BR-DE-* of SourceXRechnung, which is why they carry this Source and not
	// those.
	//
	// It is the one Source a validator for another authority emits: the released
	// XRechnung Schematron merges twenty-one PEPPOL-EN16931-* rules in, so
	// ValidateXRechnung reports them — under OpenPEPPOL's Source, because Source
	// names the authority that wrote the rule. It imports none of the country rules;
	// see the comment on the coverage table in report.go.
	SourcePeppol Source = "Peppol"
	// SourceNLCIUS is the Dutch SimplerInvoicing NLCIUS (BR-NL-*).
	//
	// SI-UBL includes CEN's Schematron from a copy of its own, in both bindings and
	// in the G-account extension. Every CEN condition in all three copies is one CEN
	// published at some release — none is Dutch — so nothing here is overridden, and
	// that is derived rather than assumed: see ciusCENCopyVerdicts.
	SourceNLCIUS Source = "NLCIUS"
	// SourceCIUSPT is the Portuguese AT/eSPap CIUS-PT: BR-CIUS-PT-*, and AT's own
	// BR-AA-* — eight rules for the "Lower rate" (AA) VAT category, written by
	// cloning CEN's BR-S-* template for a category code EN 16931 leaves out of
	// BT-118's restricted list. CEN publishes no BR-AA-* family, so the identifier
	// looks like CEN's and is not, and this Source is where it belongs.
	//
	// It is also the one authority whose *conditions* this package substitutes for
	// CEN's. AT/eSPap ships a copy of CEN's UBL binding in which nine CEN
	// identifiers carry a condition CEN never published, and under ValidateCIUSPT
	// those nine are evaluated as AT/eSPap wrote them. Such a finding keeps
	// SourceEN16931 and CEN's identifier and carries Reading == SourceCIUSPT; see
	// Violation.Reading and cius_overrides.go.
	//
	// The same copy is CEN's validation-1.1.0 of June 2018 and omits 192 CEN
	// identifiers this package evaluates — 114 CEN had already published and AT left
	// out, 78 CEN has added since. None is suppressed, so ValidateCIUSPT reports
	// fatal EN 16931 findings on documents a reference CIUS-PT validator accepts,
	// including all 20 instances AT publishes as conformant. The split, the release
	// pin and the reasoning are in ciusCENCopyOmissions.
	SourceCIUSPT Source = "CIUS-PT"
	// SourceCIUSRO is the Romanian ANAF RO e-Factura CIUS. Its identifiers are
	// BR-RO-* — the business rules, the BR-RO-L* length limits, the BR-RO-DT* date
	// formats and the BR-RO-A* occurrence limits — and BR-DEC-RO-*, ANAF's
	// decimal-place limits, which are the one family here whose prefix is not
	// BR-RO. A reader scoping on "BR-RO-" alone misses a fifth of the rule set.
	//
	// BR-27 is not among them although ANAF re-publishes it inside its own national
	// file: it is a CEN identifier and this package reports CEN's under
	// SourceEN16931, with CEN's condition.
	//
	// ANAF also ships a whole copy of CEN's UBL binding beside its own file, and
	// every one of the 930 CEN identifiers in it carries a condition CEN published
	// at some release — none is Romanian. So there is nothing to override here, and
	// that is a derived fact rather than a decision: see ciusCENCopyVerdicts.
	SourceCIUSRO Source = "CIUS-RO"
	// SourceUBLBE is the Belgian UBL.BE CIUS (ubl-BE-*).
	SourceUBLBE Source = "UBL.BE"
	// SourceSRBDT is the Serbian SRBDT CIUS (RSR-*).
	SourceSRBDT Source = "SRBDT"
	// SourceFatturaPA is the Italian SdI FatturaPA format (FPA-*).
	SourceFatturaPA Source = "FatturaPA"
	// SourceFacturae is the Spanish Facturae format (FE-*).
	SourceFacturae Source = "Facturae"
	// SourceEbInterface is the Austrian ebInterface format (EB-*).
	SourceEbInterface Source = "ebInterface"
	// SourceKSeF is the Polish KSeF FA(2) format (KS-*).
	SourceKSeF Source = "KSeF"
	// SourceFinvoice is the Finnish Finvoice format (FI-*).
	SourceFinvoice Source = "Finvoice"
	// SourceTEAPPS is the Finnish Tieto TEAPPSXML format (TP-*).
	SourceTEAPPS Source = "TEAPPS"
	// SourceOIOUBL is the Danish OIOUBL profile (OIO-*).
	SourceOIOUBL Source = "OIOUBL"
	// SourceSvefaktura is the Swedish Svefaktura format (SV-*).
	SourceSvefaktura Source = "Svefaktura"
	// SourceZATCA is the Saudi ZATCA e-invoicing profile (ZA-*).
	SourceZATCA Source = "ZATCA"
	// SourceOSA is the Hungarian NAV Online Számla format (HU-*).
	SourceOSA Source = "NAV OSA"
	// SourceUBLTR is the Turkish UBL-TR e-Fatura profile (TR-*).
	SourceUBLTR Source = "UBL-TR"
	// SourcePINT is the Peppol International (PINT) billing model (PINT-*).
	SourcePINT Source = "PINT"
	// SourceOrderX is the Franco-German Order-X order document (ORDER-*).
	SourceOrderX Source = "Order-X"
	// SourceChecker is this package speaking about its own run, or about the file
	// it was handed, rather than about any rule: RuleLimit, RuleProfile and
	// RuleRoot.
	SourceChecker Source = "checker"
)

// Severity is how much weight the authority that wrote a rule puts on breaking
// it: enough to reject the document, or not.
//
// It is a property of the rule and not of the run or of the document, and it is
// not this package's opinion. CEN writes it into the Schematron as
// flag="fatal" or flag="warning" on every assertion, and the national
// authorities quoted here do the same (KoSIT adds flag="information", which is
// advisory under another name). This type carries that flag folded onto two
// values, because two is what a caller can act on: a fatal finding is a reason
// to refuse an invoice and a warning is not.
//
// The same assertion can carry different flags in the two EN 16931 syntax
// bindings — BR-51 is fatal in the CII binding and a warning in the UBL one —
// so severity belongs on the finding, where the binding that produced it is
// known, rather than on a table keyed by rule identifier.
//
// # Why SeverityFatal is the zero value
//
// An unstamped Severity reads as fatal, deliberately. The two orderings are not
// symmetric. With SeverityWarning at zero, an emission site that forgot the
// field would report a genuine non-conformance as advisory: Report.Conformant
// would pass over it, Report.Fatal would omit it, and an invoice would ship on
// the strength of a field nobody set. With SeverityFatal at zero the same
// omission over-reports — a caller sees a blocking finding for an advisory rule,
// notices, and the stamp gets fixed. This package exists to keep silence from
// being read as a clean invoice, so the default has to lean the other way.
//
// The default is not a substitute for deciding, and it is not left unchecked:
// TestOnlyTheAdvisoryBindingsAreEmittedAsWarnings sweeps the whole corpus and
// fails on any finding whose severity does not match the half of the package it
// came from, and TestEveryEmittedEN16931RuleCarriesCENsFlag holds the one Source
// with vendored ground truth to the flag CEN publishes — in both directions, so
// an advisory rule stamped fatal by the zero value fails as loudly as a fatal one
// stamped advisory.
//
// # This package's own rule identifiers
//
// Most national formats publish no rule identifier this package could quote, so
// the identifiers under those Sources — FPA-*, FE-*, EB-*, KS-*, FI-*, ZA-*,
// SV-*, TP-*, OIO-*, TR-*, HU-*, PINT-*, ORDER-* — were minted here (see
// Source). No authority has flagged them, so their severity is a decision rather
// than a quotation, and the decision is that all of them are fatal.
//
// The reason is what those rules check rather than who wrote them down. Each is a
// mandatory element of the format's own schema or a value outside its own code
// list, so a document that breaks one is a document the authority's gateway
// rejects at the border: the SdI refuses a FatturaPA with no invoice number, and
// Fatoora refuses a ZATCA invoice with no UUID. That is precisely what fatal
// means here. If one of these formats is later found to publish a genuinely
// advisory expectation and this package implements it, it belongs at
// SeverityWarning and TestOnlyTheAdvisoryBindingsAreEmittedAsWarnings is where
// the change gets recorded.
//
// # SourceChecker findings
//
// RuleLimit, RuleProfile and RuleRoot are this package's statements about its
// own run or about the file it was handed, so no authority has flagged them and
// severity is this package's classification rather than a quotation. They are
// fatal, for the reason above rather than by analogy: a caller who writes
// len(r.Fatal()) == 0 as their release gate must not have a cancelled run, a
// rejected Profile or a document that is not an invoice slip through it. A third
// value for "not a rule" was considered and rejected — it would make Fatal and
// Warnings no longer a partition of Violations, and every caller who switched on
// severity would have to invent a policy for the third case, most of them
// choosing the unsafe one. The distinction those callers actually need is
// already exported, precisely and tested: IsCheckerViolation.
type Severity int

const (
	// SeverityFatal is a rule its authority rejects a document for breaking.
	SeverityFatal Severity = iota
	// SeverityWarning is a rule its authority reports without rejecting: CEN's
	// flag="warning", KoSIT's flag="information", NLCIUS's "not recommended".
	// A document with warnings and no fatal finding is conformant.
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityFatal:
		return "fatal"
	case SeverityWarning:
		return "warning"
	}
	return fmt.Sprintf("Severity(%d)", int(s))
}

// Violation reports one way in which a document departs from a rule set. Source
// names the authority that defines the rule and Rule is that authority's
// identifier for it (e.g. SourceEN16931/"BR-CO-15", SourceNLCIUS/"BR-NL-1");
// neither is an identity on its own. Severity is whether that authority rejects
// a document for it.
type Violation struct {
	Source   Source
	Rule     string
	Severity Severity
	Message  string

	// Reading names the authority whose *condition* for Rule was evaluated, when
	// that is not Source's own. It is SourceNone — the zero value, and the case on
	// every finding this package reports outside a CIUS with condition overrides —
	// when the rule was judged by the condition its own Source publishes.
	//
	// It exists because a national CIUS may ship a modified copy of CEN's
	// Schematron rather than referencing it, so a document validated under that
	// CIUS is judged by its authority's reading of a CEN rule. The finding is still
	// CEN's BR-02 or BR-S-02: the identifier was minted by CEN, means what CEN says
	// it means, and re-stamping it with the CIUS's Source would make one identifier
	// name two rules — the collision TestNoRuleIdentifierIsClaimedByTwoSources
	// exists to prevent. What changes is *whose condition decided it*, and that is
	// what this field carries. See cius_overrides.go.
	//
	// A caller gating on "no fatal finding" is unaffected. A caller reconciling
	// this package's verdict against a reference validator needs it: a
	// SourceEN16931/BR-S-02 with Reading == SourceCIUSPT will not reproduce under a
	// plain EN 16931 validation, and that is correct rather than a defect.
	Reading Source
}

// Error renders the finding, severity included. The severity is in the string
// rather than only in the field because this type satisfies error: a caller that
// logs a Violation would otherwise present an advisory finding in exactly the
// same words as a blocking one, which is the confusion Severity exists to end.
// The authority whose condition was evaluated is in the string for the same
// reason: a caller who logs the finding would otherwise be unable to tell a BR-S-02
// CEN's own condition reported from one AT/eSPap's copy of it reported, and those
// are different claims about the document.
func (v Violation) Error() string {
	if v.Reading != SourceNone && v.Reading != v.Source {
		return fmt.Sprintf("%s %s (%s, as %s reads it): %s", v.Source, v.Rule, v.Severity, v.Reading, v.Message)
	}
	return fmt.Sprintf("%s %s (%s): %s", v.Source, v.Rule, v.Severity, v.Message)
}

// adder returns the emission helper a rule set uses to append its findings,
// stamping src on each one.
//
// Every rule set builds its own, so the Source is fixed at the point of
// emission rather than painted on afterwards. That matters because several
// validators emit under more than one authority in a single call — the
// XRechnung validator reports EN 16931 core findings alongside BR-DE-*, as do
// the CIUS-PT, CIUS-RO, NLCIUS, Peppol, SRBDT and UBL.BE validators — so
// stamping one Source over a returned slice would misattribute the core half.
//
// The severity is written out rather than left to Severity's zero value, and
// the helpers do not take a Severity argument for the same reason: which
// severity a rule carries is a fact about the rule, so it belongs where the rule
// set decides it and not in an argument a call could get wrong by omission.
//
// There are four of them, one per way that fact is known. adder is for a rule set
// whose rules are all fatal, which is most of them. advisoryAdder is for the
// generated EN 16931 syntax bindings, where the whole table is advisory. xrAdder,
// in xrechnung_rules.go, is for a rule set that is neither: KoSIT flags eleven of
// its fifty-seven identifiers warning or information and the rest fatal, so its
// severity is a per-identifier lookup with a test that reads the flags back out of
// the Schematron. peppolEval.add, in peppol_rules.go, is the fourth and the only one
// that needs to know *which artefact* is being quoted, because one Peppol rule
// carries two published flags: fatal in OpenPEPPOL's own Schematron and warning in
// the XRechnung build that merges it.
func adder(out *[]Violation, src Source) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{Source: src, Rule: rule, Severity: SeverityFatal, Message: msg})
	}
}

// advisoryAdder is adder for a rule its authority reports without rejecting the
// document: CEN's flag="warning". Its findings are what Report.Warnings returns,
// they are absent from Report.Fatal, and they do not move Report.Conformant.
//
// The generated EN 16931 syntax-binding tables use it — see
// en16931_syntax_advisory.go, which is also where the argument for reporting them
// at all is made. A national format's minted identifier does not belong here: no
// authority has flagged those, so this package had to decide, and it decided they
// are fatal for the reason Severity gives.
func advisoryAdder(out *[]Violation, src Source) func(rule, msg string) {
	return func(rule, msg string) {
		*out = append(*out, Violation{Source: src, Rule: rule, Severity: SeverityWarning, Message: msg})
	}
}

// ruleContexts counts, per rule identifier, how many context nodes a hand-written
// rule set was asked about. It is nil on every production path and non-nil only in
// the reachability tests, where writing to it costs one map increment per context
// node.
//
// It exists because a clean sweep is not evidence. A rule that reports nothing over
// 1,690 documents is either a rule that was asked and answered "conforms" or a rule
// that was never asked at all — bound to a misspelt element name, or to a path the
// mapper never builds — and the two are indistinguishable from the outside. The
// generated CIUS-PT and CIUS-RO tiers get this for free, because their contexts are
// data and a test can walk them (TestCIUSPTDatatypeContextsAreReachable,
// TestCIUSRORuleContextsAreReachable). A hand-written rule body has no such
// structure, so the count has to be taken where the context is: at the point the
// rule body reaches a node it is about, before it decides anything.
//
// reached is called with the identifiers evaluated at that node, which is why it is
// variadic: three of UBL.BE's rules share the context //cac:TaxTotal/cac:TaxSubtotal/
// cac:TaxCategory and two of SRBDT's share cac:AccountingCustomerParty/cac:Party/
// cac:PartyLegalEntity/cbc:CompanyID, exactly as their Schematron writes them.
type ruleContexts map[string]int

// reached records one context node for each identifier evaluated at it. A nil
// receiver is the production case and does nothing.
func (c ruleContexts) reached(ids ...string) {
	if c == nil {
		return
	}
	for _, id := range ids {
		c[id]++
	}
}
