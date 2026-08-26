package formalis

import "context"

// This file is the entry point for the Portuguese CIUS-PT
// (urn:feap.gov.pt:CIUS-PT), the AT/eSPap public-sector profile of EN 16931. The
// rule bodies are in cius_pt_rules.go; the artefact guards are in
// cius_pt_artefact_test.go.
//
// CIUS-PT makes several EN 16931-optional terms mandatory — the parties' tax-scheme
// identifiers and VAT schemes, the Seller and Deliver-to postal addresses, the
// document totals and total VAT amount, a delivery — and adds a large tier of
// *conditional completeness* rules: "an optional UBL group that is present shall
// carry its identifying child". Thirty-three of the sixty-five are of that second
// kind, and every one of them is a Schematron <report> whose test reads as the
// defect rather than as the requirement.
//
// Every identifier AT/eSPap publishes is evaluated: the 65 BR-CIUS-PT-*, the eight
// BR-AA-* below, and the 290 DT-CIUS-PT-* datatype and arithmetic rules over the
// 291 assertions that carry them. The last of those is four fifths of the rule set
// by count and is generated from the Schematron rather than transcribed — see
// cius_pt_datatype.go — so Coverage(SourceCIUSPT) is now empty and SourceCIUSPT is
// in completeSources.
//
// AT/eSPap's copy of CEN's Schematron is CEN's validation-1.1.0 of 2018-06-26 —
// derived from the copy's own content in ciusCENCopyOmissions, not from a version
// string — and it has not been refreshed since. Two consequences a caller can see,
// and neither is a defect in the rules above:
//
//   - 78 CEN identifiers this package evaluates postdate that release. A CIUS is by
//     construction a restriction of EN 16931, so a rule CEN has added since 2018
//     applies to a Portuguese invoice whether or not AT's copy has caught up.
//   - 114 CEN identifiers CEN had already published are absent from AT's rule set —
//     the whole BR-CL-* code-list tier (AT's master Schematron includes no code-list
//     file of any name), the BR-AE/G/IC/O/Z-* VAT category families, BR-CO-09..17
//     and BR-DEC-*. None is suppressed here. AT replaced the arithmetic ones with
//     DT-CIUS-PT-160..167 and 171..175, but with a ±1.00 € acceptance range where
//     CEN's are exact identities, and TestATsArithmeticReplacementsAreWeakerThanCENs
//     measures the corpus documents that difference is live on; for the code-list
//     tier and the five deleted VAT families AT publishes nothing at all. A
//     suppression whose replacement is weaker converts a divergence from AT's
//     validator into a class of invoice nothing checks.
//
// So ValidateCIUSPT reports fatal EN 16931 findings on all 20 instances AT/eSPap
// publishes as conformant. Every one of them is under an identifier AT's own rule set
// does not contain — not one is this package over-reporting a rule AT publishes,
// which is checked in both directions by
// TestATInstancesFailOnlyIdentifiersATDoesNotPublish.
//
// AT/eSPap also ships a **copy of CEN's own Schematron**, not a reference to it, and
// nine CEN conditions in that copy are AT's own rather than CEN's at any release.
// Under ValidateCIUSPT those nine are evaluated as AT wrote them: eight because AT
// treats the VAT category codes 'NOR' and 'ISE' as synonyms of CEN's 'S' and 'E'
// throughout BR-S-* and BR-E-*, and BR-23 because AT inverts CEN's assertion into a
// report. Such a finding keeps CEN's identifier and SourceEN16931 and carries
// Reading == SourceCIUSPT, so a caller can tell whose condition decided it. The
// mechanism, and the reason the other 735 differences in the same copy are *not*
// overridden, are in cius_overrides.go.
//
// Two facts about the artefact decide how these rules are written, and both were
// found the expensive way in earlier rule sets:
//
//   - AT/eSPap publishes CIUS-PT for **UBL only**. urn_feap.gov.pt_CIUS-PT_*.sch
//     ships an abstract half and a UBL binding and no CII binding at all, and every
//     context in the abstract model resolves through a UBL <param>. Before PR 22
//     this package evaluated the rules on the shared syntax-neutral model, so every
//     Factur-X/ZUGFeRD invoice was liable to be accused of a Portuguese rule that
//     does not exist for its syntax — C32's eight-rule defect, again.
//   - Most of these rules are exists() tests, which an element written
//     *empty* satisfies. The rules therefore read the parsed tree and not the
//     syntax-neutral model, which carries a trimmed string per term and cannot tell
//     an absent element from an empty one. Four Peppol rules reported invoices
//     OpenPEPPOL's own fixtures hold up as conforming for want of that distinction
//     (C32), and seven CIUS-PT rules had it wrong before PR 22.
//
// Every published identifier is flagged fatal — 65 BR-CIUS-PT-*, 8 BR-AA-* and 290
// DT-CIUS-PT-* — so the plain adder is right, and the coverage table's fail-safe
// fatal turned out to be the authority's own flag. cius_artefacts_test.go checks
// both directions.
//
// One further family lives in these files under a CEN-looking name and is not
// CEN's: BR-AA-01..07 and BR-AA-10, eight fatal assertions AT wrote for the
// "Lower rate" (AA) VAT category by cloning CEN's BR-S-* template. CEN publishes no
// BR-AA-* family, because 'AA' is a UNCL5305 code EN 16931 leaves out of BT-118's
// restricted list. They are evaluated here and reported under SourceCIUSPT, which is
// where they belong: they are Portuguese rules wearing a CEN-shaped identifier. See
// ptLowerRateRules.

// ValidateCIUSPT validates an invoice XML against the Portuguese CIUS-PT: the
// EN 16931 core plus every rule AT/eSPap publishes — the 65 BR-CIUS-PT-* business
// rules, AT's own eight BR-AA-*, and the 290 DT-CIUS-PT-* datatype and arithmetic
// rules.
//
// The EN 16931 core accepts either syntax. The CIUS-PT rules are evaluated for a
// UBL document only, because that is the only binding AT/eSPap publishes: a CII
// invoice is validated against the core and reported as carrying no CIUS-PT
// finding, which is what a reference CIUS-PT validator says about it too.
//
// For a UBL document the core is evaluated with AT/eSPap's own condition wherever
// AT/eSPap wrote one — nine CEN identifiers, listed in ciusCENCopyVerdicts. Those
// findings keep SourceEN16931 and CEN's identifier, and carry Reading ==
// SourceCIUSPT. No other entry point substitutes them.
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
// Coverage(SourceEN16931) and Coverage(SourceCIUSPT). Coverage(SourceCIUSPT) is
// empty, so a clean Portuguese invoice reports Conformant() == true and
// Complete() == true; what remains in the union is the seven rules CEN publishes
// that no validator can evaluate, which carry Unevaluable and do not hold the
// verdict down.
//
// "Clean" there means clean against the union, which is stricter than AT/eSPap's own
// validator: AT vendored CEN's rules in 2018 and left 114 of them out, and this
// package evaluates CEN's current set regardless. A caller who needs "what a
// reference CIUS-PT validator would say" rather than "is this both EN 16931- and
// CIUS-PT-conformant" should read Violation.Rule against ciusCENCopyOmissions. See
// the file comment above.
func ValidateCIUSPT(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceCIUSPT}, validateCIUSPT)
}

func validateCIUSPT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	// AT/eSPap ships a copy of CEN's Schematron and wrote nine of its conditions
	// itself, so on this path those nine are judged by AT's reading and not by CEN's.
	// The findings keep CEN's identifier and carry Reading = SourceCIUSPT. See
	// cius_overrides.go; this is the only place the substitution happens, and it
	// happens on no other entry point.
	out = applyConditionOverrides(r, ptOverrides, p, out)
	return append(out, validateCIUSPTRules(r, p.inv, p.root)...)
}
