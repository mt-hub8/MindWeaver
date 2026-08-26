package formalis

import (
	"fmt"
	"math"
)

// The 24 fatal BR-FXEXT-* rules that restate a CEN identifier the EXTENDED
// profile drops.
//
// # What this file is
//
// facturx.go implements the nine BR-FXEXT-* rules that are Factur-X's own new
// ground. The other 42 restate a CEN identifier the EXTENDED profile no longer
// carries, usually with a carve-out for something EXTENDED adds; 24 of those are
// unflagged and therefore fatal, and they are this file. The 18 that remain are
// the -08ini/-08rev pair of each of the nine VAT-category families, which FNFE
// flags warning; Coverage(SourceFacturX) keeps them and says why.
//
// This package also evaluates CEN's original for every one of the 41 identifiers
// EXTENDED drops — facturXCENOmissions records that decision — so on most
// documents the CEN rule and its restatement both fire and a caller sees two
// findings for one defect. That is deliberate; the "Duplicate reporting" section
// below argues it.
//
// # Per rule: what CEN asserts, what FNFE asserts, and which fires where
//
// The premise this work started from was that every one of these 24 is shadowed
// by a stricter CEN rule this package already evaluates, so implementing them
// could only add duplicates. That premise is wrong for more than half of them,
// and where it is wrong it is wrong in both directions. Each entry below states
// the two conditions, whether FNFE's can fire where CEN's cannot (in which case
// this file closes a real gap), and whether FNFE's is silent where CEN's fires
// (in which case this package is stricter than the authority — the C29 shape,
// recorded and not acted on).
//
//	BR-FXEXT-BR-22/23/24/26 and BR-FXEXT-CO-04 — the five line-level existence
//	rules. CEN's context is //ram:IncludedSupplyChainTradeLineItem and the test is
//	the bare presence of BT-129, BT-130, BT-131, BT-146 and BT-151. FNFE's context
//	is that line's ram:AssociatedDocumentLineDocument restricted to
//	[not(ram:LineStatusReasonCode) or ram:LineStatusReasonCode = 'DETAIL'] and the
//	test is the same expression through "..". So FNFE's node set is a subset of
//	CEN's twice over: a line with no BG-25 document group at all is not reached,
//	and a GROUP sub-line is excused. Against CEN's published rule these five can
//	never fire where CEN's does not.
//
//	They can nonetheless fire where *this package's* BR-22 does not, and that is
//	not the same claim. en16931_model.go excuses a line that another line names as
//	its parent — a rollup line carries no quantity or price — and that carve-out
//	is by parent reference, not by BT-X-8. A line marked DETAIL that some other
//	line still names as its parent is excused by this package and reported by
//	FNFE. It is a malformed sub-line structure rather than a common shape, but it
//	is the one direction in which these five add coverage.
//
//	Silent where CEN fires: yes, and this is the live half. A GROUP or a
//	subtype-carrying line missing BT-129, BT-130, BT-131, BT-146 or BT-151 is
//	reported by this package under CEN's identifier and by no Factur-X processor.
//	Also: CEN's test and FNFE's are both existence tests, and this package's
//	BR-22/23/26 test the *value* — an element present and empty satisfies FNFE and
//	does not satisfy this package.
//
//	BR-FXEXT-BR-38 and BR-FXEXT-BR-44 — the document-level and line-level charge
//	reason. FNFE's context and test are CEN's, character for character:
//	//ram:ApplicableHeaderTradeSettlement/ram:SpecifiedTradeAllowanceCharge/
//	ram:ChargeIndicator[udt:Indicator='true'] with (../ram:Reason) or
//	(../ram:ReasonCode), and the line-level twin. There is no carve-out. Only the
//	message differs, FNFE's naming BT-177 and BT-193, the non-VAT tax codes
//	EXTENDED adds, as a third way to satisfy the same two elements. So these two
//	fire exactly where CEN's do and nowhere else, and implementing them adds an
//	identifier and no coverage. They are here because a Factur-X processor prints
//	BR-FXEXT-BR-38 and not BR-38, and because "no coverage added" is a conclusion
//	worth having checked rather than assumed.
//
//	BR-FXEXT-CO-10 — Σ line net amounts. CEN sums every line and requires exact
//	equality with BT-106. FNFE sums only the lines whose BT-X-8 is DETAIL or
//	absent, and allows 0,01 × the number of lines. Fires where CEN's does not:
//	yes. A sub-line document whose GROUP lines carry their own BT-131 has a
//	CEN sum that double-counts the rollup; if BT-106 matches that double-counted
//	figure CEN passes and FNFE reports. Silent where CEN fires: yes, both from the
//	tolerance and from the DETAIL restriction. This package's own BR-CO-10 sums
//	only lines with no parent reference, which is a third rule again — closer to
//	FNFE's than to CEN's, and still not the same one.
//
//	BR-FXEXT-CO-11 — Σ document-level allowances. FNFE is CEN's with a
//	0,01 × count tolerance, so against CEN's rule it can only be silent where
//	CEN's fires, and it is a rule this package's own BR-CO-11 used to be skipped
//	for: en16931_model.go exempted BR-CO-11 and BR-CO-12 at ProfileExtended
//	outright until C45 removed that gate. Both are evaluated at every profile now,
//	and the CEN half of this pair yields to FNFE's tolerance.
//
//	BR-FXEXT-CO-12 — Σ document-level charges. FNFE adds the logistics service
//	charges (BT-X-272) EXTENDED introduces to the sum, and a tolerance. Fires
//	where CEN's does not: yes — a document whose BT-108 counts only the BG-21
//	entries while a ram:SpecifiedLogisticsServiceCharge is present passes CEN's
//	rule and fails FNFE's. Silent where CEN's fires: yes, and this is the
//	interesting direction — a conforming Factur-X EXTENDED invoice that folds its
//	logistics charges into BT-108 is reported by CEN's BR-CO-12 and accepted by
//	FNFE. Two of FNFE's own 59 examples are in exactly that position, and once C45
//	removed the profile gate the divergence stopped being latent: it is
//	facturXAuthorityParity that keeps this package's verdict on them equal to
//	valitool's.
//
//	BR-FXEXT-CO-13 — BT-109 against the line, allowance and charge sums. CEN
//	computes it from the three *totals* BT-106/107/108; FNFE computes it from the
//	line, allowance, charge and logistics *sums* directly, over DETAIL lines only,
//	with a tolerance. Fires where CEN's does not: yes, wherever BT-106 is itself
//	wrong in a way that cancels — CEN's BR-CO-13 cannot see past BT-106 and
//	FNFE's does not read it at all.
//
//	BR-FXEXT-CO-15 — BT-112 = BT-109 + BT-110. Fires where CEN's does not, and
//	the reason is a disjunct CEN has and FNFE does not. CEN accepts either
//	"exactly one BT-110 in the invoice currency and the sum holds" or, with no
//	condition attached, "BT-112 = BT-109". FNFE gates that second escape on there
//	*not* being exactly one BT-110 in the invoice currency. So an invoice carrying
//	one BT-110 of 5,00 with BT-109 = BT-112 = 100,00 satisfies CEN and fails
//	FNFE. This package's BR-CO-15 has no escape clause at all and is stricter than
//	both, so the added coverage is against CEN's artefact rather than against this
//	package.
//
//	Silent where CEN fires: yes, and PR 33 recorded "no" here, which was wrong.
//	FNFE's first disjunct is not CEN's equality but
//	`$abs le $maxTolerance and $nbTaxTotalAmountInvoiceCurrency eq 1`, where
//	$maxTolerance is 0,01 × (DETAIL lines + document allowances + document and
//	logistics charges) — read back out of FACTUR-X_EXTENDED.sch rather than from
//	the earlier summary. CEN's BR-CO-15 is exact and this package's is exact to
//	half a cent, so a twelve-line invoice one cent out fails here and passes every
//	Factur-X processor. It carries the weaker verdict for that reason.
//
//	BR-FXEXT-CO-16 — BT-115. FNFE adds Σ BT-179, the charges collected on behalf
//	of a third party (ram:SpecifiedFinancialAdjustment) EXTENDED introduces.
//	Fires where CEN's does not: yes — an invoice with a third-party charge whose
//	BT-115 ignores it satisfies CEN and fails FNFE. Silent where CEN's fires:
//	yes, and this one is live. A conforming EXTENDED invoice that adds BT-179
//	into BT-115 was reported by this package under BR-CO-16 and accepted by every
//	Factur-X processor — C46, recorded by PR 33 and not acted on. It is acted on
//	now: facturXAuthorityParity drops the BR-CO-16 finding on the Factur-X path
//	when BR-FXEXT-CO-16 holds.
//
//	BR-FXEXT-{AE,E,G,IC,AF,AG,O,S,Z}-08b — nine VAT-category taxable-amount
//	summations, and BR-FXEXT-S-09b, the category-S tax amount. Each is FNFE's two
//	readings of the same summation joined by "or": one matching operands on
//	category and rate alone (the EN 16931:2017 text), one matching also on the
//	exemption reason code and text (the 2026 text). The rule fires only if both
//	readings fail, which makes each -08b the *weaker* of the pair; the two
//	readings taken separately are the eighteen -08ini/-08rev identifiers FNFE
//	flags warning.
//
//	Against CEN, FNFE differs on four axes at once: the operands are restricted
//	to DETAIL lines; ram:SpecifiedLogisticsServiceCharge is subtracted from the
//	base; every family matches on rate, where CEN matches on rate only for S, AF
//	and AG; and the tolerance is 0,01 × the operand count where CEN's is ±1 for
//	AE, E, G, IC, Z and exact for AF, AG, O, S. Every one of those can fire where
//	CEN cannot: an invoice with three lines and a taxable amount 0,50 out passes
//	CEN's ±1 and fails FNFE's 0,03; a category-Z base that ignores a logistics
//	charge passes CEN and fails FNFE; two AE breakdowns at different rates are one
//	sum to CEN and two to FNFE.
//
//	And against *this package* they used to add more than that, because
//	en16931_vat.go's BR-{fam}-08 was skipped outright on any document carrying a
//	sub-line structure (the hasSubLines gate C45 named), so an EXTENDED invoice
//	with sub-lines — the shape these rules exist for — had no VAT taxable-amount
//	check at all. That gate is gone; the sums now exclude the sub-lines whose
//	amounts their parents roll up, which is what the gate was reaching for. Two
//	preconditions remain and are properties of the document rather than of its
//	profile: every line and document allowance/charge must carry a category, and
//	BT-107/BT-108 must be accounted for by the BG-20/21 entries.
//
//	Silent where CEN fires: yes, for the same four axes read the other way, and
//	the line-set one is not hypothetical. A GROUP line with no children carries
//	BT-131, has no parent to be rolled into, and so is an operand of this package's
//	sum and not of FNFE's.
//
//	BR-FXEXT-G-08 — the 51st identifier, and the only one outside EXTENDED. FNFE
//	publishes it in FACTUR-X_MINIMUM.sch beside CEN's own BR-G-08, so the MINIMUM
//	profile carries both readings of that summation. It is the -08b shape without
//	the rate and without the exemption reason: one reading, DETAIL lines,
//	logistics charges included, 0,01 × count tolerance. Its context also drops
//	CEN's [upper-case(../ram:TypeCode) = 'VAT'] predicate, so a category-G
//	breakdown on a non-VAT tax is reached by FNFE's rule and not by CEN's — one
//	more way it fires where CEN cannot.
//
// # Duplicate reporting, and the one place it is not kept
//
// Both rules of each pair fire on a document that breaks both, so a caller
// validating an EXTENDED invoice with a bad category-S base gets BR-S-08 under
// SourceEN16931 and BR-FXEXT-S08b under SourceFacturX. Neither is suppressed.
//
// That is not what a Factur-X processor does: FACTUR-X_EXTENDED.sch does not
// carry BR-S-08 at all, so FNFE's own validator prints one line. But this package
// deliberately reports the union of the two rule sets — facturXCENOmissions is
// that decision, taken because 18 of the 41 identifiers EXTENDED drops have no
// replacement and dropping them would be 18 false negatives — and having taken
// it, suppressing the CEN half only where a replacement exists would make the
// same document report under CEN's identifier or FNFE's depending on which of the
// 41 it broke. Source is the discriminator, it is on every Violation, and a
// caller who wants exactly FNFE's verdict filters on it. This is the reasoning
// PR 20 recorded for UBL-SR-07 duplicating BR-55, with the difference stated
// rather than glossed: there, CEN's own reference validator prints both.
//
// The exception is the direction that changes a verdict rather than a list. Where
// FNFE's restatement is *satisfied* and CEN's original fires, keeping the CEN
// finding makes this package refuse a document Factur-X accepts, and that is not
// a cosmetic difference in identifiers — it is the C29 defect. So on the
// Factur-X path, and there only, a CEN finding is dropped when the weaker rule
// that replaced it holds. facturXAuthorityParity implements it and argues the
// boundaries; the "silent where CEN fires" verdict recorded per rule below is
// what decides which identifiers it covers.
//
// # What is not reported
//
// FNFE writes these tests in XPath 2.0 with xs:decimal() constructors over
// elements the profile data model requires. Two cases have no verdict to give and
// both resolve to silence here:
//
//   - an amount or rate whose text is not a number. xs:decimal("n/a") raises
//     FORG0001, which aborts the pattern rather than failing the assertion, so
//     the authority's processor reports nothing about this document either.
//   - an operand element that is absent where the arithmetic needs it — a VAT
//     breakdown with no BT-116, say. xs:decimal(()) is the empty sequence, the
//     enclosing "for" returns empty, and an assert over an empty sequence is
//     false, so FNFE's processor *would* report. This package does not, because
//     the finding it would produce says "your taxable amount does not equal the
//     sum" about a document that has no taxable amount, which BR-45 already
//     reports in the terms the reader needs. Recorded as a deliberate
//     under-report rather than left to be discovered.
//
// Numeric comparisons carry an epsilon of 1e-9 on top of FNFE's own tolerance,
// because 0.01 × n and a sum of parsed decimals are both inexact in float64 and a
// document exactly on the boundary must not be reported. It is smaller than any
// difference these rules are written to detect by seven orders of magnitude.

// facturXRestatement is one BR-FXEXT-* identifier that restates a CEN identifier
// the profile's Schematron drops: the identifier, the CEN identifier it restates,
// and the one profile that publishes it.
//
// profile is a single value and not a set because FNFE publishes 23 of these in
// EXTENDED only and the 24th, BR-FXEXT-G-08, in MINIMUM only.
// TestFacturXRestatementsMatchTheArtefact derives both halves from the files.
type facturXRestatement struct {
	id      string
	cen     string
	profile Profile
	// weaker records that FNFE's reading can be *silent* where CEN's original
	// fires: that there exists a document CEN's rule reports and a Factur-X
	// processor accepts. It is the direction that matters for a verdict, because a
	// document the authority accepts must not draw a fatal finding here (C29), and
	// facturXAuthorityParity is what acts on it.
	//
	// It is per rule and argued per rule in the file comment above. 21 of the 23
	// EXTENDED restatements carry it. The two that do not are BR-FXEXT-BR-38 and
	// BR-FXEXT-BR-44, whose context and test are CEN's character for character —
	// (../ram:Reason) or (../ram:ReasonCode) on the same context node — so there is
	// no document one reports and the other does not, and suppressing CEN's half
	// there would be churning identifiers for a duplicate that costs nothing.
	//
	// MINIMUM's BR-FXEXT-G-08 does not carry it either, and for a different
	// reason: FACTUR-X_MINIMUM.sch publishes it *beside* CEN's BR-G-08 rather than
	// in place of it, so a Factur-X processor at MINIMUM reports both readings and
	// there is no divergence to close.
	weaker bool
}

// facturXRestatementRules is the 24, in the order the rule bodies below report
// them. It is what the coverage guard and the firing guard enumerate, so "which
// restatements are implemented" is one list rather than a grep for add() calls —
// the arrangement facturXExtensionRules already uses.
var facturXRestatementRules = []facturXRestatement{
	{id: "BR-FXEXT-BR-22", cen: "BR-22", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-BR-23", cen: "BR-23", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-BR-24", cen: "BR-24", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-BR-26", cen: "BR-26", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-04", cen: "BR-CO-04", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-BR-38", cen: "BR-38", profile: ProfileExtended},
	{id: "BR-FXEXT-BR-44", cen: "BR-44", profile: ProfileExtended},
	{id: "BR-FXEXT-CO-10", cen: "BR-CO-10", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-11", cen: "BR-CO-11", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-12", cen: "BR-CO-12", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-13", cen: "BR-CO-13", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-15", cen: "BR-CO-15", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-CO-16", cen: "BR-CO-16", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-AE-08b", cen: "BR-AE-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-E-08b", cen: "BR-E-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-G-08b", cen: "BR-G-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-IC-08b", cen: "BR-IC-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-AF-08b", cen: "BR-AF-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-AG-08b", cen: "BR-AG-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-O-08b", cen: "BR-O-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-S08b", cen: "BR-S-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-Z-08b", cen: "BR-Z-08", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-S-09b", cen: "BR-S-09", profile: ProfileExtended, weaker: true},
	{id: "BR-FXEXT-G-08", cen: "BR-G-08", profile: ProfileMinimum},
}

// facturXVATSummationRules is the nine -08b families in the order FNFE writes
// them, with the VAT category code each selects and whether its operands are
// matched on the rate. Category O carries no rate, and FNFE's BR-FXEXT-O-08b
// binds no $rate variable at all — the one shape difference among the nine.
var facturXVATSummationRules = []struct {
	id       string
	category string
	byRate   bool
}{
	{id: "BR-FXEXT-AE-08b", category: "AE", byRate: true},
	{id: "BR-FXEXT-E-08b", category: "E", byRate: true},
	{id: "BR-FXEXT-G-08b", category: "G", byRate: true},
	{id: "BR-FXEXT-IC-08b", category: "K", byRate: true},
	{id: "BR-FXEXT-AF-08b", category: "L", byRate: true},
	{id: "BR-FXEXT-AG-08b", category: "M", byRate: true},
	{id: "BR-FXEXT-O-08b", category: "O", byRate: false},
	{id: "BR-FXEXT-S08b", category: "S", byRate: true},
	{id: "BR-FXEXT-Z-08b", category: "Z", byRate: true},
}

// fxEpsilon absorbs float64 representation error in FNFE's "le" comparisons. See
// the file comment.
const fxEpsilon = 1e-9

// validateFacturXRestatements evaluates the 24 fatal BR-FXEXT-* restatements the
// named profile publishes.
//
// It is a no-op at BASIC WL, BASIC and the EN 16931 tier, whose Schematrons carry
// no BR-FXEXT-* identifier at all, and on any root but a CrossIndustryInvoice.
//
// seen is the reachability bookkeeping the context guard reads; it is nil on
// every production path.
func validateFacturXRestatements(r *run, p *parsed, profile Profile, seen ruleContexts) []Violation {
	if p == nil || p.root == nil || p.root.name != "CrossIndustryInvoice" {
		return nil
	}
	if profile != ProfileExtended && profile != ProfileMinimum {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceFacturX)
	doc := newFacturXDocument(p.root)

	if profile == ProfileMinimum {
		// MINIMUM publishes exactly one of these, and publishes it beside CEN's
		// own BR-G-08 rather than in place of it. It is a summation over the same
		// operands as the nine -08b, so it draws from the same budget.
		if facturXSummationBudget(r, doc) {
			facturXMinimumG08(doc, add, seen)
		}
		return out
	}
	if r.stopped() {
		return out
	}
	facturXLineExistenceRestatements(doc, add, seen)
	facturXChargeReasonRestatements(doc, add, seen)
	if r.stopped() {
		return out
	}
	facturXTotalRestatements(doc, add, seen)
	if r.stopped() {
		return out
	}
	facturXVATSummationRestatements(r, doc, add, seen)
	return out
}

// facturXDocument is the operand set every rule in this file reads, resolved once
// from the tree: the invoice lines, the header settlement's allowances, charges
// and logistics charges, and the monetary summations.
//
// It is resolved once because the -08b family alone would otherwise walk the
// whole line list nine times over two readings each. The paths are FNFE's own,
// absolute from /rsm:CrossIndustryInvoice, which is how the artefact writes every
// operand of these rules — only the context is relative.
type facturXDocument struct {
	root *ciiNode
	// settlements is every /rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/
	// ram:ApplicableHeaderTradeSettlement, in document order.
	settlements []*ciiNode
	// lines is every /rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/
	// ram:IncludedSupplyChainTradeLineItem.
	lines []*ciiNode
	// allowCharges is every ram:SpecifiedTradeAllowanceCharge of those
	// settlements, and logistics every ram:SpecifiedLogisticsServiceCharge.
	allowCharges []*ciiNode
	logistics    []*ciiNode
	// readings memoises facturXOneReading on its filter. Two VAT breakdowns of
	// the same category and rate select the same operands and therefore produce
	// the same sums, and an invoice may carry many; without this, resolving them
	// is quadratic in the document exactly as CEN's BR-{fam}-08 was before
	// validateVATTaxableSums grew its own cache. facturXTaxFilter is comparable,
	// which is why it can be the key.
	readings map[facturXTaxFilter]facturXReading
}

func newFacturXDocument(root *ciiNode) *facturXDocument {
	d := &facturXDocument{root: root, readings: map[facturXTaxFilter]facturXReading{}}
	for _, tx := range root.all("SupplyChainTradeTransaction") {
		d.lines = append(d.lines, tx.all("IncludedSupplyChainTradeLineItem")...)
		d.settlements = append(d.settlements, tx.all("ApplicableHeaderTradeSettlement")...)
	}
	for _, s := range d.settlements {
		d.allowCharges = append(d.allowCharges, s.all("SpecifiedTradeAllowanceCharge")...)
		d.logistics = append(d.logistics, s.all("SpecifiedLogisticsServiceCharge")...)
	}
	return d
}

// facturXIsDetailLine is FNFE's line filter, written the way the artefact writes
// it: [not(ram:AssociatedDocumentLineDocument/ram:LineStatusReasonCode) or
// ram:AssociatedDocumentLineDocument/ram:LineStatusReasonCode = 'DETAIL'].
//
// It is not facturXSubtype. That function resolves BT-X-8 through the three-way
// union BR-FXEXT-06 uses; the summation rules write one path, and the rule is the
// one the artefact states.
func facturXIsDetailLine(line *ciiNode) bool {
	codes := nodesAt(line, "AssociatedDocumentLineDocument", "LineStatusReasonCode")
	if len(codes) == 0 {
		return true
	}
	for _, c := range codes {
		if c.stringValue() == "DETAIL" {
			return true
		}
	}
	return false
}

// facturXLineExistenceRestatements evaluates BR-FXEXT-BR-22, -23, -24, -26 and
// BR-FXEXT-CO-04, the five assertions of one rule whose context is
// //ram:IncludedSupplyChainTradeLineItem/ram:AssociatedDocumentLineDocument[
// not(ram:LineStatusReasonCode) or ram:LineStatusReasonCode = 'DETAIL'].
//
// The context node is the BG-25 document group and every test reaches the line
// through "..", so a line carrying no ram:AssociatedDocumentLineDocument is not
// reached at all — which is the first of the two ways FNFE's node set is smaller
// than CEN's.
func facturXLineExistenceRestatements(d *facturXDocument, add func(rule, msg string), seen ruleContexts) {
	for _, line := range d.lines {
		for _, adl := range line.all("AssociatedDocumentLineDocument") {
			// The predicate is on the context node itself, so it reads that
			// group's own ram:LineStatusReasonCode rather than the line's.
			if codes := adl.all("LineStatusReasonCode"); len(codes) > 0 && !anyStringValue(codes, "DETAIL") {
				continue
			}
			seen.reached("BR-FXEXT-BR-22", "BR-FXEXT-BR-23", "BR-FXEXT-BR-24", "BR-FXEXT-BR-26", "BR-FXEXT-CO-04")

			qty := nodesAt(line, "SpecifiedLineTradeDelivery", "BilledQuantity")
			if len(qty) == 0 {
				add("BR-FXEXT-BR-22", "Each invoice line (BG-25) whose line item subtype (BT-X-8) is DETAIL or absent shall have an Invoiced quantity (BT-129)")
			}
			if !anyHasAttr(qty, "unitCode") {
				add("BR-FXEXT-BR-23", "Each invoice line (BG-25) whose line item subtype (BT-X-8) is DETAIL or absent shall have an Invoiced quantity unit of measure code (BT-130)")
			}
			if len(nodesAt(line, "SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount")) == 0 {
				add("BR-FXEXT-BR-24", "Each invoice line (BG-25) whose line item subtype (BT-X-8) is DETAIL or absent shall have an Invoice line net amount (BT-131)")
			}
			if len(nodesAt(line, "SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "ChargeAmount")) == 0 {
				add("BR-FXEXT-BR-26", "Each invoice line (BG-25) whose line item subtype (BT-X-8) is DETAIL or absent shall have an Item net price (BT-146)")
			}
			// BR-FXEXT-CO-04 selects the line's VAT tax by type code before
			// asking for the category, which is CEN's own
			// [upper-case(ram:TypeCode) = 'VAT'] predicate carried over.
			vatCategory := false
			for _, tax := range nodesAt(line, "SpecifiedLineTradeSettlement", "ApplicableTradeTax") {
				if facturXTypeCodeIsVAT(tax) && len(tax.all("CategoryCode")) > 0 {
					vatCategory = true
				}
			}
			if !vatCategory {
				add("BR-FXEXT-CO-04", "Each invoice line (BG-25) whose line item subtype (BT-X-8) is DETAIL or absent shall be categorized with an Invoiced item VAT category code (BT-151)")
			}
		}
	}
}

// facturXChargeReasonRestatements evaluates BR-FXEXT-BR-38 and BR-FXEXT-BR-44.
//
// Both are CEN's rule unaltered — same context, same test — so the only thing
// this function decides is the identifier the finding carries. FNFE's message
// names BT-177 and BT-193, the non-VAT tax type codes EXTENDED adds, as further
// ways to satisfy the same two elements, and the messages here say so.
func facturXChargeReasonRestatements(d *facturXDocument, add func(rule, msg string), seen ruleContexts) {
	for _, ac := range d.allowCharges {
		if !facturXIndicatorIs(ac, "true") {
			continue
		}
		seen.reached("BR-FXEXT-BR-38")
		if len(ac.all("Reason")) == 0 && len(ac.all("ReasonCode")) == 0 {
			add("BR-FXEXT-BR-38", "Each Document level charge (BG-21) shall have a Document level charge reason (BT-104), a Document level charge reason code (BT-105) or a Document level non-VAT tax code (BT-177)")
		}
	}
	// //ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeAllowanceCharge, which
	// is every line's, at whatever depth.
	for _, s := range d.root.findAll("SpecifiedLineTradeSettlement") {
		for _, ac := range s.all("SpecifiedTradeAllowanceCharge") {
			if !facturXIndicatorIs(ac, "true") {
				continue
			}
			seen.reached("BR-FXEXT-BR-44")
			if len(ac.all("Reason")) == 0 && len(ac.all("ReasonCode")) == 0 {
				add("BR-FXEXT-BR-44", "Each Invoice line charge (BG-28) shall have an Invoice line charge reason (BT-144), an Invoice line charge reason code (BT-145) or an Invoice line non-VAT tax type code (BT-193)")
			}
		}
	}
}

// facturXIndicatorIs is `ram:ChargeIndicator/udt:Indicator = want`, a string
// comparison against the element's string value.
//
// FNFE writes 'true' as a string literal in BR-FXEXT-BR-38/BR-44 and as the
// function true() in the CO-11/CO-12 and -08b operand filters. The two agree on
// every document the CII schema admits — udt:IndicatorType is xs:boolean, whose
// lexical space here is "true" or "false" — and the difference is recorded rather
// than smoothed over because "1" and "0" are also xs:boolean lexical forms and
// FNFE's string comparison rejects them where its boolean one would not.
func facturXIndicatorIs(ac *ciiNode, want string) bool {
	return anyStringValue(nodesAt(ac, "ChargeIndicator", "Indicator"), want)
}

// facturXTypeCodeIsVAT is `upper-case(ram:TypeCode) = 'VAT'` over the tax
// element's own type code children.
func facturXTypeCodeIsVAT(tax *ciiNode) bool {
	for _, tc := range tax.all("TypeCode") {
		if upperASCII(tc.stringValue()) == "VAT" {
			return true
		}
	}
	return false
}

// anyStringValue is an XPath general comparison against a string: true when any
// node in the set has that string value.
func anyStringValue(ns []*ciiNode, want string) bool {
	for _, n := range ns {
		if n.stringValue() == want {
			return true
		}
	}
	return false
}

// upperASCII is XPath's upper-case() over the ASCII letters, which is the whole
// of the domain here: these codes are UNCL 5153 values.
func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// facturXAmount is one operand of a summation: its value, and whether the text
// was a number at all. An operand element that is present and unreadable gives
// the whole assertion no verdict — see the file comment.
type facturXAmount struct {
	value float64
	ok    bool
}

// facturXSum is a summation over matching operands: the rounded total, how many
// nodes matched, and whether every amount read parsed. ok is false when one did
// not, and no rule reports from a sum that is not ok.
type facturXSum struct {
	total float64
	count int
	ok    bool
}

// sumAmountsAt is `round(sum(nodes/xs:decimal(path)) * 100) div 100` over the
// nodes that matched a filter, with the count of matching nodes beside it.
//
// A matching node with no such child contributes nothing to the sum, which is
// what an XPath path step producing an empty sequence does inside sum(). A child
// whose text is not a number makes ok false.
func sumAmountsAt(nodes []*ciiNode, path ...string) facturXSum {
	out := facturXSum{count: len(nodes), ok: true}
	for _, n := range nodes {
		for _, a := range nodesAt(n, path...) {
			v, ok := parseAmount(a.text)
			if !ok {
				out.ok = false
				continue
			}
			out.total += v
		}
	}
	out.total = round2(out.total)
	return out
}

// facturXTaxFilter is one reading's operand filter: the VAT category, optionally
// the rate, and optionally the exemption reason code and text.
//
// withExemption distinguishes FNFE's two readings of every -08b summation — the
// EN 16931:2017 one that matches on category and rate, and the 2026 one that also
// matches the exemption reason. byRate is false for category O only.
type facturXTaxFilter struct {
	category       string
	rate           float64
	byRate         bool
	withExemption  bool
	exCode, exText string
}

// matches applies the filter to a node carrying its VAT terms under taxName —
// ram:ApplicableTradeTax on a line settlement, ram:CategoryTradeTax on an
// allowance or charge, ram:AppliedTradeTax on a logistics charge.
//
// Each conjunct is an independent XPath general comparison over *all* the tax
// children, which is what the artefact's predicates are: a node with two tax
// children satisfying one conjunct each satisfies the predicate. That is XPath's
// semantics rather than a convenience, and it is transcribed rather than
// tightened to "one child satisfies all".
func (f facturXTaxFilter) matches(n *ciiNode, taxName string) bool {
	taxes := n.all(taxName)
	if len(taxes) == 0 {
		return false
	}
	if !f.matchesTaxes(taxes) {
		return false
	}
	return true
}

func (f facturXTaxFilter) matchesTaxes(taxes []*ciiNode) bool {
	category := false
	for _, t := range taxes {
		if anyStringValue(t.all("CategoryCode"), f.category) {
			category = true
		}
	}
	if !category {
		return false
	}
	if f.byRate {
		rate := false
		for _, t := range taxes {
			for _, rp := range t.all("RateApplicablePercent") {
				if v, ok := parseAmount(rp.text); ok && math.Abs(v-f.rate) < fxEpsilon {
					rate = true
				}
			}
		}
		if !rate {
			return false
		}
	}
	if f.withExemption {
		if facturXNormSpaceOf(taxes, "ExemptionReasonCode") != f.exCode {
			return false
		}
		if facturXNormSpaceOf(taxes, "ExemptionReason") != f.exText {
			return false
		}
	}
	return true
}

// facturXNormSpaceOf is `normalize-space(taxes/name)`: the normalized string
// value of the first such child, or "" when there is none, which is what
// normalize-space() of an empty sequence returns.
func facturXNormSpaceOf(taxes []*ciiNode, name string) string {
	for _, t := range taxes {
		if cs := t.all(name); len(cs) > 0 {
			return normalizeSpace(cs[0].text)
		}
	}
	return ""
}

// facturXFilterNodes selects the nodes a filter matches, by the tax element name
// their group uses.
func facturXFilterNodes(nodes []*ciiNode, taxName string, f facturXTaxFilter) []*ciiNode {
	var out []*ciiNode
	for _, n := range nodes {
		if f.matches(n, taxName) {
			out = append(out, n)
		}
	}
	return out
}

// facturXIndicatorNodes narrows an allowance/charge set to one polarity, FNFE's
// `ram:ChargeIndicator/udt:Indicator = true()` or `= false()`.
func facturXIndicatorNodes(nodes []*ciiNode, want string) []*ciiNode {
	var out []*ciiNode
	for _, n := range nodes {
		if facturXIndicatorIs(n, want) {
			out = append(out, n)
		}
	}
	return out
}

// facturXVATSummationRestatements evaluates the nine -08b taxable-amount
// summations and BR-FXEXT-S-09b.
//
// Each fires only when both of FNFE's readings fail, which is what the "or" in
// the artefact's return clause says and is what makes each -08b weaker than
// either of the -08ini/-08rev pair it is published beside.
func facturXVATSummationRestatements(r *run, d *facturXDocument, add func(rule, msg string), seen ruleContexts) {
	for _, s := range d.settlements {
		for _, tax := range s.all("ApplicableTradeTax") {
			for _, spec := range facturXVATSummationRules {
				if !anyStringValue(tax.all("CategoryCode"), spec.category) || !facturXTypeCodeIsVAT(tax) {
					continue
				}
				seen.reached(spec.id)
				if !facturXTaxableSum(r, d, tax, spec.id, spec.category, spec.byRate, add) {
					return
				}
			}
			// BR-FXEXT-S-09b's context carries no type-code predicate, which is
			// the one difference from BR-FXEXT-S08b's and is FNFE's.
			if anyStringValue(tax.all("CategoryCode"), "S") {
				seen.reached("BR-FXEXT-S-09b")
				if !facturXCategorySTaxAmount(r, d, tax, add) {
					return
				}
			}
		}
	}
}

// facturXSummationBudget draws the cost of resolving one breakdown's two
// readings from the run's VAT summation budget, and reports whether the work may
// proceed.
//
// It is the same budget and the same accounting unit validateVATTaxableSums uses
// for CEN's BR-{fam}-08, because it is the same work on the same document: an
// invoice with B breakdowns and L operands costs B x L here as it does there, and
// limits.go records that a 7.3 MB document made that quadratic before the memo
// was added. The memo is here too — facturXDocument.readings caches on the
// filter, so two breakdowns of one category and rate resolve their operands once
// — and this is the backstop for input that defeats it.
//
// A trip stops the whole pass rather than skipping one breakdown, for
// validateVATTaxableSums's reason: a partial sum accuses a conforming invoice.
func facturXSummationBudget(r *run, d *facturXDocument) bool {
	if r.stopped() {
		return false
	}
	return r.spendVAT(2 * (len(d.lines) + len(d.allowCharges) + len(d.logistics)))
}

// facturXReading is one of the two operand matchings a -08b assertion joins with
// "or": the sums and the operand count that set the tolerance.
type facturXReading struct {
	lines, allowances, charges, logistics facturXSum
	// count is FNFE's $nbLineItems + $nbAllowancesOrCharges + $nblogisticCharge:
	// the line count uses the *line*-level predicate rather than the settlement
	// one the sum uses, and the allowance/charge count is over both polarities.
	count int
	ok    bool
}

// facturXReadings resolves both of a summation's readings for one breakdown.
func facturXReadings(d *facturXDocument, tax *ciiNode, category string, byRate bool) (rev, ini facturXReading, ok bool) {
	rate, rateOK := 0.0, !byRate
	if byRate {
		for _, rp := range tax.all("RateApplicablePercent") {
			if v, parsed := parseAmount(rp.text); parsed {
				rate, rateOK = v, true
				break
			}
		}
	}
	if !rateOK {
		// $rate is bound from xs:decimal(ram:RateApplicablePercent). Absent, the
		// binding is empty and FNFE's assertion is false; unreadable, it is a
		// dynamic error. Neither is a statement about the summation, and the file
		// comment says why this package stays silent for both.
		return rev, ini, false
	}
	base := facturXTaxFilter{category: category, rate: rate, byRate: byRate}
	withEx := base
	withEx.withExemption = true
	withEx.exCode = facturXNormSpaceOf([]*ciiNode{tax}, "ExemptionReasonCode")
	withEx.exText = facturXNormSpaceOf([]*ciiNode{tax}, "ExemptionReason")
	return d.reading(withEx), d.reading(base), true
}

// reading is facturXOneReading through the memo.
func (d *facturXDocument) reading(f facturXTaxFilter) facturXReading {
	if rd, ok := d.readings[f]; ok {
		return rd
	}
	rd := facturXOneReading(d, f)
	d.readings[f] = rd
	return rd
}

func facturXOneReading(d *facturXDocument, f facturXTaxFilter) facturXReading {
	// The line sum's predicate is on ram:SpecifiedLineTradeSettlement and the
	// line count's is on the line, which is FNFE's own asymmetry.
	var settlements []*ciiNode
	lineCount := 0
	for _, line := range d.lines {
		if !facturXIsDetailLine(line) {
			continue
		}
		ss := line.all("SpecifiedLineTradeSettlement")
		settlements = append(settlements, facturXFilterNodes(ss, "ApplicableTradeTax", f)...)
		var taxes []*ciiNode
		for _, s := range ss {
			taxes = append(taxes, s.all("ApplicableTradeTax")...)
		}
		if len(taxes) > 0 && f.matchesTaxes(taxes) {
			lineCount++
		}
	}
	allowances := facturXFilterNodes(facturXIndicatorNodes(d.allowCharges, "false"), "CategoryTradeTax", f)
	charges := facturXFilterNodes(facturXIndicatorNodes(d.allowCharges, "true"), "CategoryTradeTax", f)
	logistics := facturXFilterNodes(d.logistics, "AppliedTradeTax", f)

	rd := facturXReading{
		lines:      sumAmountsAt(settlements, "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount"),
		allowances: sumAmountsAt(allowances, "ActualAmount"),
		charges:    sumAmountsAt(charges, "ActualAmount"),
		logistics:  sumAmountsAt(logistics, "AppliedAmount"),
	}
	// $nbAllowancesOrCharges counts every BG-20/21 entry of the category
	// regardless of its charge indicator, which is a third node set again.
	rd.count = lineCount + len(facturXFilterNodes(d.allowCharges, "CategoryTradeTax", f)) + len(logistics)
	rd.ok = rd.lines.ok && rd.allowances.ok && rd.charges.ok && rd.logistics.ok
	return rd
}

// holds is FNFE's `abs($basisAmount - $lines + $allowances - $charges -
// $logistics) le 0.01 * $count`.
func (rd facturXReading) holds(basis float64) bool {
	diff := basis - rd.lines.total + rd.allowances.total - rd.charges.total - rd.logistics.total
	return math.Abs(diff) <= 0.01*float64(rd.count)+fxEpsilon
}

// facturXTaxableSum is one -08b assertion on one VAT breakdown.
func facturXTaxableSum(r *run, d *facturXDocument, tax *ciiNode, rule, category string, byRate bool, add func(rule, msg string)) bool {
	basis, ok := facturXFirstAmount(tax, "BasisAmount")
	if !ok {
		return true
	}
	if !facturXSummationBudget(r, d) {
		return false
	}
	rev, ini, resolved := facturXReadings(d, tax, category, byRate)
	if !resolved || !rev.ok || !ini.ok {
		return true
	}
	if rev.holds(basis) || ini.holds(basis) {
		return true
	}
	add(rule, fmt.Sprintf("the VAT category taxable amount (BT-116=%.2f) for category %q shall equal the sum of the DETAIL lines' net amounts + document charges + logistics service charges (BT-X-272) - document allowances (%.2f)",
		basis, category, ini.lines.total-ini.allowances.total+ini.charges.total+ini.logistics.total))
	return true
}

// facturXCategorySTaxAmount is BR-FXEXT-S-09b: the category-S tax amount
// (BT-117) against the taxable amount and the rate, at FNFE's operand-count
// tolerance rather than CEN's ±1.
func facturXCategorySTaxAmount(r *run, d *facturXDocument, tax *ciiNode, add func(rule, msg string)) bool {
	basis, okB := facturXFirstAmount(tax, "BasisAmount")
	calc, okC := facturXFirstAmount(tax, "CalculatedAmount")
	if !okB || !okC {
		return true
	}
	if !facturXSummationBudget(r, d) {
		return false
	}
	rev, ini, resolved := facturXReadings(d, tax, "S", true)
	if !resolved {
		return true
	}
	rate := 0.0
	for _, rp := range tax.all("RateApplicablePercent") {
		if v, parsed := parseAmount(rp.text); parsed {
			rate = v
			break
		}
	}
	want := round2(basis * rate / 100)
	diff := math.Abs(calc - want)
	if diff <= 0.01*float64(rev.count)+fxEpsilon || diff <= 0.01*float64(ini.count)+fxEpsilon {
		return true
	}
	add("BR-FXEXT-S-09b", fmt.Sprintf("the VAT category tax amount (BT-117=%.2f) for the standard rate shall equal the taxable amount (BT-116=%.2f) x the rate (BT-119=%.2f%%), which is %.2f",
		calc, basis, rate, want))
	return true
}

// facturXFirstAmount is `xs:decimal(n/name)`: the value of the first such child,
// and whether there was one that parsed.
func facturXFirstAmount(n *ciiNode, name string) (float64, bool) {
	for _, c := range n.all(name) {
		return parseAmount(c.text)
	}
	return 0, false
}

// facturXMinimumG08 is BR-FXEXT-G-08, the only restatement outside EXTENDED.
//
// FNFE publishes it in FACTUR-X_MINIMUM.sch beside CEN's own BR-G-08, so MINIMUM
// carries both readings of the same summation. Its context is the
// ram:CategoryCode element rather than the tax group, and it carries none of
// CEN's [upper-case(../ram:TypeCode) = 'VAT'] predicate — so a category-G
// breakdown on a non-VAT tax is reached here and not by CEN's rule.
//
// The condition is the -08b shape with neither the rate nor the exemption reason:
// one reading, DETAIL lines, logistics charges added to the base.
func facturXMinimumG08(d *facturXDocument, add func(rule, msg string), seen ruleContexts) {
	for _, s := range d.settlements {
		for _, tax := range s.all("ApplicableTradeTax") {
			for _, cc := range tax.all("CategoryCode") {
				if cc.stringValue() != "G" {
					continue
				}
				seen.reached("BR-FXEXT-G-08")
				basis, ok := facturXFirstAmount(tax, "BasisAmount")
				if !ok {
					continue
				}
				rd := d.reading(facturXTaxFilter{category: "G"})
				if !rd.ok || rd.holds(basis) {
					continue
				}
				add("BR-FXEXT-G-08", fmt.Sprintf("the VAT category taxable amount (BT-116=%.2f) for category \"G\" shall equal the sum of the DETAIL lines' net amounts + document charges + logistics service charges (BT-X-272) - document allowances (%.2f)",
					basis, rd.lines.total-rd.allowances.total+rd.charges.total+rd.logistics.total))
			}
		}
	}
}

// facturXTotalRestatements evaluates BR-FXEXT-CO-10 through -16, the seven
// assertions of one rule whose context is
// //ram:SpecifiedTradeSettlementHeaderMonetarySummation.
func facturXTotalRestatements(d *facturXDocument, add func(rule, msg string), seen ruleContexts) {
	for _, s := range d.settlements {
		for _, sum := range s.all("SpecifiedTradeSettlementHeaderMonetarySummation") {
			seen.reached("BR-FXEXT-CO-10", "BR-FXEXT-CO-11", "BR-FXEXT-CO-12",
				"BR-FXEXT-CO-13", "BR-FXEXT-CO-15", "BR-FXEXT-CO-16")
			facturXCO10(d, s, sum, add)
			facturXCO11(s, sum, add)
			facturXCO12(s, sum, add)
			facturXCO13(d, s, sum, add)
			facturXCO15(d, s, sum, add)
			facturXCO16(s, sum, add)
		}
	}
}

// facturXDetailLineTotals is `sum(../../ram:IncludedSupplyChainTradeLineItem[
// DETAIL or no subtype]/ram:SpecifiedLineTradeSettlement/
// ram:SpecifiedTradeSettlementLineMonetarySummation/ram:LineTotalAmount)` with
// the count of the lines that contributed.
func (d *facturXDocument) detailLineTotals() facturXSum {
	var settlements []*ciiNode
	n := 0
	for _, line := range d.lines {
		if !facturXIsDetailLine(line) {
			continue
		}
		n++
		settlements = append(settlements, line.all("SpecifiedLineTradeSettlement")...)
	}
	out := sumAmountsAt(settlements, "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount")
	out.count = n
	return out
}

// facturXCO10 is BR-FXEXT-CO-10: BT-106 against the DETAIL lines' BT-131, at a
// tolerance of 0,01 x the number of lines — every line, not only the DETAIL ones,
// which is FNFE's own asymmetry and is transcribed rather than tidied.
func facturXCO10(d *facturXDocument, _, sum *ciiNode, add func(rule, msg string)) {
	total, ok := facturXFirstAmount(sum, "LineTotalAmount")
	if !ok {
		return
	}
	lines := d.detailLineTotals()
	if !lines.ok {
		return
	}
	if math.Abs(total-lines.total) <= 0.01*float64(len(d.lines))+fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-10", fmt.Sprintf("Sum of Invoice line net amounts (BT-106=%.2f) shall equal the sum of the DETAIL lines' net amounts (%.2f)", total, lines.total))
}

// facturXCO11 is BR-FXEXT-CO-11: BT-107 against Σ BT-92, at 0,01 x the number of
// document-level allowances, with FNFE's escape for an invoice that has neither.
func facturXCO11(s, sum *ciiNode, add func(rule, msg string)) {
	allowances := facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "false")
	if len(allowances) == 0 && len(sum.all("AllowanceTotalAmount")) == 0 {
		return
	}
	total, ok := facturXFirstAmount(sum, "AllowanceTotalAmount")
	if !ok {
		return
	}
	got := sumAmountsAt(allowances, "ActualAmount")
	if !got.ok {
		return
	}
	if math.Abs(total-got.total) <= 0.01*float64(len(allowances))+fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-11", fmt.Sprintf("Sum of allowances on document level (BT-107=%.2f) shall equal the sum of the Document level allowance amounts (BT-92) (%.2f)", total, got.total))
}

// facturXCO12 is BR-FXEXT-CO-12: BT-108 against Σ BT-99 *plus* the logistics
// service charges (BT-X-272) EXTENDED adds, which is where it parts company with
// CEN's rule in both directions.
func facturXCO12(s, sum *ciiNode, add func(rule, msg string)) {
	charges := facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "true")
	if len(charges) == 0 && len(sum.all("ChargeTotalAmount")) == 0 {
		return
	}
	total, ok := facturXFirstAmount(sum, "ChargeTotalAmount")
	if !ok {
		return
	}
	got := sumAmountsAt(charges, "ActualAmount")
	logistics := sumAmountsAt(s.all("SpecifiedLogisticsServiceCharge"), "AppliedAmount")
	if !got.ok || !logistics.ok {
		return
	}
	n := len(charges) + logistics.count
	if math.Abs(total-(got.total+logistics.total)) <= 0.01*float64(n)+fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-12", fmt.Sprintf("Sum of charges on document level (BT-108=%.2f) shall equal the sum of the Document level charge amounts (BT-99) and the Logistics service fee amounts (BT-X-272) (%.2f)",
		total, got.total+logistics.total))
}

// facturXCO13 is BR-FXEXT-CO-13: BT-109 against the DETAIL lines' net amounts,
// the document allowances and the document charges including logistics. CEN
// computes the same quantity from BT-106/107/108 instead, so this one can fire on
// a document whose BT-106 is itself wrong in a cancelling way.
func facturXCO13(d *facturXDocument, s, sum *ciiNode, add func(rule, msg string)) {
	basis, ok := facturXFirstAmount(sum, "TaxBasisTotalAmount")
	if !ok {
		return
	}
	lines := d.detailLineTotals()
	allowances := facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "false")
	charges := facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "true")
	allowSum := sumAmountsAt(allowances, "ActualAmount")
	chargeSum := sumAmountsAt(charges, "ActualAmount")
	logistics := sumAmountsAt(s.all("SpecifiedLogisticsServiceCharge"), "AppliedAmount")
	if !lines.ok || !allowSum.ok || !chargeSum.ok || !logistics.ok {
		return
	}
	n := lines.count + len(allowances) + len(charges) + logistics.count
	got := chargeSum.total + logistics.total
	if math.Abs(basis-lines.total+allowSum.total-got) <= 0.01*float64(n)+fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-13", fmt.Sprintf("Invoice total amount without VAT (BT-109=%.2f) shall equal the DETAIL lines' net amounts (%.2f) - the document allowances (%.2f) + the document and logistics charges (%.2f)",
		basis, lines.total, allowSum.total, got))
}

// facturXCO15 is BR-FXEXT-CO-15: BT-112 = BT-109 + BT-110, where BT-110 is the
// VAT total in the invoice currency.
//
// FNFE's second disjunct — BT-109 = BT-112 — is gated on there *not* being
// exactly one BT-110 in the invoice currency, and CEN's is not gated at all. That
// gate is the one place among the 24 where FNFE is strictly stricter than CEN,
// and it is why this rule can fire on a document CEN's own binding accepts.
func facturXCO15(d *facturXDocument, s, sum *ciiNode, add func(rule, msg string)) {
	basis, okB := facturXFirstAmount(sum, "TaxBasisTotalAmount")
	grand, okG := facturXFirstAmount(sum, "GrandTotalAmount")
	if !okB || !okG {
		return
	}
	currency := ""
	if cs := s.all("InvoiceCurrencyCode"); len(cs) > 0 {
		currency = cs[0].stringValue()
	}
	var inCurrency []*ciiNode
	for _, t := range sum.all("TaxTotalAmount") {
		if t.attr("currencyID") == currency {
			inCurrency = append(inCurrency, t)
		}
	}
	if len(inCurrency) != 1 {
		// FNFE: ($BT109 eq $BT112 and $nbTaxTotalAmountInvoiceCurrency ne 1).
		if math.Abs(basis-grand) > fxEpsilon {
			add("BR-FXEXT-CO-15", fmt.Sprintf("with no single Invoice total VAT amount (BT-110) in the invoice currency, the Invoice total amount with VAT (BT-112=%.2f) shall equal the Invoice total amount without VAT (BT-109=%.2f)", grand, basis))
		}
		return
	}
	tax, okT := parseAmount(inCurrency[0].text)
	if !okT {
		return
	}
	n := facturXCO15Operands(d, s)
	if math.Abs(grand-tax-basis) <= 0.01*float64(n)+fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-15", fmt.Sprintf("Invoice total amount with VAT (BT-112=%.2f) shall equal the Invoice total amount without VAT (BT-109=%.2f) plus the Invoice total VAT amount (BT-110=%.2f)", grand, basis, tax))
}

// facturXCO15Operands is BR-FXEXT-CO-15's tolerance multiplier: the DETAIL
// lines, the document allowances, the document charges and the logistics
// charges.
func facturXCO15Operands(d *facturXDocument, s *ciiNode) int {
	n := 0
	for _, line := range d.lines {
		if facturXIsDetailLine(line) {
			n++
		}
	}
	n += len(facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "false"))
	n += len(facturXIndicatorNodes(s.all("SpecifiedTradeAllowanceCharge"), "true"))
	n += len(s.all("SpecifiedLogisticsServiceCharge"))
	return n
}

// facturXSuperseded is, per profile, the CEN identifier each weaker restatement
// stands in front of: the map facturXAuthorityParity consults.
//
// It is derived from facturXRestatementRules rather than written out, so a rule
// whose weaker verdict changes cannot leave a stale copy behind, and
// TestFacturXSupersessionMatchesTheOmissionTable checks every entry against
// facturXCENOmissions — a CEN identifier may only be superseded here if the
// profile's Schematron actually drops it.
var facturXSuperseded = func() map[Profile]map[string]string {
	out := map[Profile]map[string]string{}
	for _, rs := range facturXRestatementRules {
		if !rs.weaker {
			continue
		}
		if out[rs.profile] == nil {
			out[rs.profile] = map[string]string{}
		}
		out[rs.profile][rs.cen] = rs.id
	}
	return out
}()

// facturXAuthorityParity drops a CEN finding that the profile's own authority
// would not have made.
//
// # What it does
//
// For a document being validated *as* Factur-X at a profile whose Schematron
// drops a CEN identifier and puts a weaker restatement in its place, a finding
// under CEN's identifier is dropped when the restatement that replaced it is
// satisfied. Both are kept when both fire.
//
// # Why
//
// Because otherwise this package refuses invoices Factur-X accepts, and that is
// a defect of the same kind as reporting the wrong rule. C29 is the precedent: it
// found seven XRechnung rules emitted fatal that KoSIT flags advisory, so the
// package rejected German invoices Germany accepts, and no false-positive oracle
// could see it because the documents in question genuinely depart from a rule —
// just not from one their authority enforces.
//
// It is live here, not hypothetical. testdata/facturx/examples/X11_01_Kostenrechnung.xml
// and X19_01_Warenrechnung.xml carry a BT-108 that folds in a
// ram:SpecifiedLogisticsServiceCharge; FNFE's BR-FXEXT-CO-12 adds that charge to
// the sum and passes them — valitool's report beside each says
// isValidBusinessRules=true — and CEN's BR-CO-12, which has never heard of
// BT-X-272, does not. The same shape is what C46 records for BR-CO-16 and BT-179.
//
// # Why per rule and not wholesale
//
// PR 33 kept both identifiers firing deliberately: this package reports the
// *union* of CEN's rule set and FNFE's, because 18 of the 41 identifiers EXTENDED
// drops have no replacement at all and dropping those would be 18 false
// negatives; and having taken that, suppressing CEN's half everywhere a
// replacement exists would make one defect report under CEN's identifier or
// FNFE's depending on which of the 41 a document broke.
//
// That argument holds for a document that fails either way, and it is untouched
// here: when both rules fire, both findings stand and a caller filtering on
// Source still gets exactly FNFE's list. What it does not survive is a
// pass/fail divergence, and this is the narrowest condition that removes one —
// the CEN half goes only when the authority's own reading of that same rule is
// satisfied, on a document that authority governs, at a profile where that
// authority replaced the rule.
//
// # What it deliberately does not do
//
//   - It does not run for BR-38 or BR-44, whose restatements are CEN's rule
//     character for character, so there is no document to diverge on.
//   - It does not run at MINIMUM, whose one restatement FNFE publishes beside
//     CEN's rule rather than in place of it.
//   - It does not run for a UBL document, at any profile. Factur-X publishes no
//     UBL binding, so no restatement was evaluated and there is no authority
//     reading to defer to; without the root test every CEN identifier in the
//     table would be dropped from a UBL invoice validated at EXTENDED, which
//     would be 21 false negatives for a syntax the authority says nothing about.
//   - It does not run when the run stopped. A restatement that was never reached
//     is not a restatement that was satisfied, and a budget trip must not be able
//     to turn a fatal finding into silence.
//   - It changes nothing under ValidateEN16931, which is CEN's own verdict on the
//     document and is documented as being exactly that: a caller who wants to
//     know what a CEN reference validator says about a Factur-X EXTENDED invoice
//     still gets it, findings included.
func facturXAuthorityParity(r *run, p *parsed, profile Profile, out []Violation) []Violation {
	if p == nil || p.root == nil || p.root.name != "CrossIndustryInvoice" {
		return out
	}
	superseded := facturXSuperseded[profile]
	if len(superseded) == 0 || r.stopped() {
		return out
	}
	var fired map[string]bool
	for _, v := range out {
		if v.Source != SourceFacturX {
			continue
		}
		if fired == nil {
			fired = map[string]bool{}
		}
		fired[v.Rule] = true
	}
	kept := out[:0]
	for _, v := range out {
		if v.Source == SourceEN16931 {
			if id, ok := superseded[v.Rule]; ok && !fired[id] {
				continue
			}
		}
		kept = append(kept, v)
	}
	return kept
}

// facturXCO16 is BR-FXEXT-CO-16: BT-115 = BT-112 - BT-113 + BT-114 + Σ BT-179,
// the charges collected on behalf of a third party EXTENDED adds. Exact
// equality, as CEN's is, with the extra term FNFE's is.
func facturXCO16(s, sum *ciiNode, add func(rule, msg string)) {
	due, ok := facturXFirstAmount(sum, "DuePayableAmount")
	if !ok {
		return
	}
	grand := sumAmountsAt([]*ciiNode{sum}, "GrandTotalAmount")
	paid := sumAmountsAt([]*ciiNode{sum}, "TotalPrepaidAmount")
	rounding := sumAmountsAt([]*ciiNode{sum}, "RoundingAmount")
	thirdParty := sumAmountsAt(s.all("SpecifiedFinancialAdjustment"), "ActualAmount")
	if !grand.ok || !paid.ok || !rounding.ok || !thirdParty.ok {
		return
	}
	want := grand.total - paid.total + rounding.total + thirdParty.total
	if math.Abs(due-want) <= fxEpsilon {
		return
	}
	add("BR-FXEXT-CO-16", fmt.Sprintf("Amount due for payment (BT-115=%.2f) shall equal the Invoice total amount with VAT (BT-112=%.2f) - the Paid amount (BT-113=%.2f) + the Rounding amount (BT-114=%.2f) + the charges collected on behalf of a third party (BT-179=%.2f)",
		due, grand.total, paid.total, rounding.total, thirdParty.total))
}
