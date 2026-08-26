package formalis

import (
	"fmt"
	"math"
	"strings"
)

// The Dutch G-account extension to NLCIUS: the eight BR-GA-* rules SimplerInvoicing
// publishes in si-ubl-2.0-ext-gaccount-1.0.2.sch, and the three CEN advisory rules
// that file's own copy of EN16931-syntax.sch removes.
//
// # What a G-account is
//
// A *g-rekening* is a blocked bank account a Dutch subcontractor holds, into which a
// contractor pays the part of an invoice that covers payroll tax and social
// contributions. It exists because the contractor is jointly liable for those
// (the *Wet ketenaansprakelijkheid*), and paying that share into the blocked account
// discharges the liability. So an invoice under this extension does not carry one
// payment, it carries **two**: the ordinary amount to the beneficiary's account, and
// the withheld amount to the G-account. The eight rules below are almost entirely
// about that: two payment instructions, two payment terms, each term naming the
// instruction it belongs to, the two adding up to the amount due, and one of the two
// instructions marked GACCOUNT.
//
// # Whether it is in scope, and under which entry point
//
// PR 26 left it out on the reading that "the G-account extension is a separate
// conformant extension, not NLCIUS". That reading is half right and the half that is
// wrong is the half that decides where the rules go. Four facts from the artefacts,
// each checkable:
//
//   - **The document opts in, and the identifier says so.** The extension's
//     specification identifier is NLCIUS's with a suffix:
//     `…#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0`.
//     BR-GA-0 asserts exactly that string. It is opt-in, not a profile a caller
//     selects out of band.
//   - **NLCIUS itself is written to cover it.** si-ubl-2.0-nlcius.sch declares
//     `$is_SI-UBL-2.0-ext-gaccount` = `contains($customizationID,
//     '#conformant#urn:fdc:nen.nl:gaccount:v1.0')` and folds it into `$si` and `$s`,
//     and NLCIUS-CII-validation.sch does the same with `$is_NLCIUS-ext-gaccount`. A
//     G-account document is an NLCIUS document by the authority's own gate, which is
//     what makes this an extension of NLCIUS rather than a sibling of it.
//   - **The extension's Schematron is a superset of NLCIUS's.**
//     si-ubl-2.0-ext-gaccount-1.0.2.sch `<include>`s si-ubl-2.0-nlcius.sch and CEN's
//     model, code-list and UBL binding files. Validating a document against it is
//     validating it against SI-UBL 2.0 plus eight rules, minus three advisory CEN
//     ones. Nothing is subtracted from the national layer.
//   - **The identifiers are the same authority's.** SimplerInvoicing mints BR-NL-*
//     and BR-GA-*, so a separate Source would say two authorities where there is
//     one, and (Source, Rule) is this package's identity for a finding.
//
// So the rules are evaluated here, under SourceNLCIUS, on the ValidateNLCIUS path.
//
// # Which binding, and which document element
//
// **UBL only, and within UBL the Invoice only.** There is no CII G-account
// Schematron: phax/phive-rules ships fourteen releases of
// si-ubl-2.0-ext-gaccount-*.sch and no CII counterpart, and the NLCIUS-CII file
// recognises the identifier in its gate without publishing a single BR-GA rule. And
// every one of the seven structural rules is rooted at `/ubl:Invoice`; the file
// declares the `cn` prefix and never uses it. phive registers the extension as its
// own validation executor set, `nl.simplerinvoicing:invoice20.g-account`, over the
// UBL **Invoice** XSDs alone — the plain SI-UBL registration has an invoice and a
// creditnote coordinate, this one has only an invoice.
//
// That gate matters for the reason C32 and C36 record: a rule evaluated in a syntax
// its authority does not publish it for accuses a conforming document of breaking a
// rule that does not exist for it.
//
// # How this package decides the extension applies
//
// A reference validator is *told* which rule set to use — the caller picks the
// G-account executor set. This package is told only "validate this as NLCIUS", so it
// has to read the choice out of the document. Two arms, and both are needed:
//
//   - the specification identifier carries `#conformant#urn:fdc:nen.nl:gaccount:v1.0`,
//     which is the artefact's own `$is_SI-UBL-2.0-ext-gaccount`; or
//   - some payment instruction carries the identifier `GACCOUNT`, which is the value
//     NL-GA-04 reserves for the blocked account and which means nothing else in UBL
//     or in EN 16931.
//
// One arm alone would be a gate that hides a rule. With the identifier arm only,
// BR-GA-0 could never report: the gate and the rule would be the same test, so the
// rule would be present, reachable and inert — C41's shape exactly, and the reason
// each rule here has a fixture that makes it fire. With the GACCOUNT arm only,
// SimplerInvoicing's own si-ubl-2.0-ext-gaccount_error_no_gaccount.xml — which
// renames the marker and must report BR-GA-7 — would fall out of scope and be
// reported clean.
//
// # The three advisory CEN rules the extension removes
//
// si-ubl-2.0-ext-gaccount-1.0.2.sch does not include CEN's abstract syntax file. It
// includes EN16931-syntax-modified.sch, whose header says what the modification is:
//
//	Modifications:
//	- removed rule UBL-CR-411
//	- removed rule UBL-CR-453
//	- removed rule UBL-CR-459
//
// Those three are "a UBL invoice should not include the PaymentMeans ID", "… the
// PaymentTerms PaymentMeansID" and "… the PaymentTerms Amount" — that is, the three
// elements NL-GA-02, NL-GA-03 and NL-GA-04 are carried in. CEN flags all three
// warning, this package generates them (D9), and a G-account invoice trips all three
// by construction. SimplerInvoicing's validator for such a document reports none of
// them, so this package must not either.
//
// This is a *same-release* comparison and not the mistake C40's correction records:
// the unmodified EN16931-syntax.sch sits beside the modified one in the same
// SI-UBL 2.0.3.2 tree, the two differ in exactly those three assertions and in the
// header comment, and both are commented out in place rather than deleted.
// TestGAccountSyntaxCopyRemovesExactlyThreeAdvisoryRules re-derives the set from the
// two files rather than trusting this list.
//
// It is also a difference PR 27's condition-override survey could not see, and that
// is worth recording: that survey compares the *condition* of each identifier a copy
// carries, and a commented-out assertion is not carried at all. Its table row for
// this file reads "745 shared, 0 the authority's own", which is true on the axis it
// measures and says nothing about the axis that matters here.

// nlciusGAccountCustomization is the specification identifier BR-GA-0 requires, in
// full. The rule is an equality test against this exact string.
const nlciusGAccountCustomization = "urn:cen.eu:en16931:2017#compliant#urn:fdc:nen.nl:nlcius:v1.0#conformant#urn:fdc:nen.nl:gaccount:v1.0"

// nlciusGAccountConformant is the substring si-ubl-2.0-nlcius.sch tests for in
// $is_SI-UBL-2.0-ext-gaccount. It is deliberately not the whole identifier: the
// artefact's own gate is a containment test even though BR-GA-0's is an equality
// one, so a document that declares the extension with anything else appended is
// inside the extension *and* reports BR-GA-0, which is what the authority intends.
const nlciusGAccountConformant = "#conformant#urn:fdc:nen.nl:gaccount:v1.0"

// nlciusGAccountMeansID is NL-GA-04's reserved value: the payment instruction that
// pays into the blocked account is the one whose identifier is this.
const nlciusGAccountMeansID = "GACCOUNT"

// nlciusGAccountRemovedCEN are the advisory CEN identifiers the extension's copy of
// EN16931-syntax.sch comments out. See the file comment; the set is re-derived from
// the two vendored files by a test.
var nlciusGAccountRemovedCEN = map[string]bool{
	"UBL-CR-411": true, // A UBL invoice should not include the PaymentMeans ID
	"UBL-CR-453": true, // A UBL invoice should not include the PaymentTerms PaymentMeansID
	"UBL-CR-459": true, // A UBL invoice should not include the PaymentTerms Amount
}

// nlciusGAccountApplies reports whether the G-account extension governs this
// document: it is a UBL Invoice, and it either declares the extension or carries the
// GACCOUNT payment instruction. See the file comment for why both arms are needed.
func nlciusGAccountApplies(p *parsed) bool {
	if p == nil || p.root == nil || p.root.name != "Invoice" {
		return false
	}
	if strings.Contains(p.inv.specID, nlciusGAccountConformant) {
		return true
	}
	for _, pm := range p.root.all("PaymentMeans") {
		for _, id := range pm.all("ID") {
			if id.text == nlciusGAccountMeansID {
				return true
			}
		}
	}
	return false
}

// dropRemovedGAccountRules removes the findings an authority's own copy of CEN's
// Schematron does not carry.
//
// It filters rather than declining to evaluate, because the advisory tier is one
// generated pattern over the whole document (en16931_syntax_advisory_eval.go) and
// three identifiers are not a reason to give it a second shape. The findings are
// SourceEN16931 and only that Source is filtered: BR-GA-* and BR-NL-* are
// SimplerInvoicing's own and are unaffected.
func dropRemovedGAccountRules(vs []Violation) []Violation {
	out := vs[:0]
	for _, v := range vs {
		if v.Source == SourceEN16931 && nlciusGAccountRemovedCEN[v.Rule] {
			continue
		}
		out = append(out, v)
	}
	return out
}

// validateNLCIUSGAccount evaluates the eight BR-GA-* assertions of the
// g-account-extension pattern, in the order and at the contexts the artefact binds
// them to.
//
// Rule order is not a concern inside this pattern and that is a checked fact rather
// than an assumption: its five rules have the contexts //cbc:CustomizationID,
// /ubl:Invoice, /ubl:Invoice/cac:PaymentTerms, /ubl:Invoice/cac:PaymentMeans and
// /ubl:Invoice/cac:PaymentTerms/cbc:PaymentMeansID, no one of which selects a node
// another selects, so all eight assertions are reachable.
// TestGAccountRuleContextsAreNotShadowed re-derives that from the file, because in
// this repository ISO Schematron rule order has decided a rule's meaning five times
// (PR 14, PR 23, PR 25, PR 26 and the four unreachable NLCIUS assertions).
//
// Every context is a *direct child* of the document element: the artefact writes
// absolute paths from /ubl:Invoice, so a cac:PaymentTerms nested somewhere else is
// not a context node. That is `all`, not `matchPath`.
//
// seen is nil on every production path; the reachability test passes a map.
func validateNLCIUSGAccount(p *parsed, seen ruleContexts) []Violation {
	if !nlciusGAccountApplies(p) {
		return nil
	}
	root := p.root
	var out []Violation
	add := adder(&out, SourceNLCIUS)

	// BR-GA-0, context //cbc:CustomizationID:
	//   normalize-space(.) = '<the extension's identifier>'
	// The context is every specification identifier in the document, which in a UBL
	// invoice is the one at the top. It is the rule that says an invoice carrying a
	// G-account split has to declare it.
	for _, c := range root.findAll("CustomizationID") {
		seen.reached("BR-GA-0")
		if normSpace(c.text) != nlciusGAccountCustomization {
			add("BR-GA-0", fmt.Sprintf("an invoice using the G-account extension must declare the specification "+
				"identifier %q, not %q", nlciusGAccountCustomization, normSpace(c.text)))
		}
	}

	// BR-GA-1, -2, -3 and -7 share one rule on the context /ubl:Invoice, so all four
	// are evaluated once, at the document element.
	terms := root.all("PaymentTerms")
	means := root.all("PaymentMeans")
	seen.reached("BR-GA-1", "BR-GA-2", "BR-GA-3", "BR-GA-7")

	// BR-GA-1: count(cac:PaymentTerms) = 2. The two are the beneficiary's share and
	// the share withheld into the blocked account.
	if len(terms) != 2 {
		add("BR-GA-1", fmt.Sprintf("a G-account invoice must state exactly two Payment Terms (NL-GA-01), not %d", len(terms)))
	}

	// BR-GA-2: count(cac:PaymentMeans) = 2.
	if len(means) != 2 {
		add("BR-GA-2", fmt.Sprintf("a G-account invoice must state exactly two Payment Instructions (BG-16), not %d", len(means)))
	}

	// BR-GA-3:
	//   cac:LegalMonetaryTotal/xs:decimal(cbc:PayableAmount)
	//     = sum(cac:PaymentTerms/xs:decimal(cbc:Amount))
	//
	// Compared to the cent rather than as exact decimals, which is what every other
	// monetary rule in this package does (BR-CO-16 and the summation rules use the
	// same 0.005 window) and what binary floating point requires: the authority's own
	// conforming sample is 578.98 + 193, whose IEEE-754 sum is not bit-equal to its
	// 771.98 payable amount. EN 16931 amounts carry two decimals — CEN's UBL-DT tier
	// is what enforces that — so the window cannot swallow a real difference.
	//
	// A side this package cannot read as a number is not reported on, for the reason
	// the period rules give: an accusation built on a value nobody could parse is a
	// guess. Such a document is already reported by CEN's own BR-DEC-* tier.
	if payable, ok := parseAmount(root.str("LegalMonetaryTotal", "PayableAmount")); ok {
		sum, allKnown := 0.0, true
		for _, t := range terms {
			for _, a := range t.all("Amount") {
				v, ok := parseAmount(a.text)
				if !ok {
					allKnown = false
					break
				}
				sum += v
			}
		}
		if allKnown && math.Abs(round2(sum)-round2(payable)) > 0.005 {
			add("BR-GA-3", fmt.Sprintf("the Payment Amounts (NL-GA-03) sum to %.2f and the Amount due for payment "+
				"(BT-115) is %.2f; a G-account invoice splits the amount due between its two payment terms",
				round2(sum), round2(payable)))
		}
	}

	// BR-GA-7: count(cac:PaymentMeans/cbc:ID[text()='GACCOUNT']) = 1. Exactly one of
	// the two payment instructions is the blocked account. The predicate is on the
	// text node and is not normalised, so it is compared verbatim.
	marked := 0
	for _, pm := range means {
		for _, id := range pm.all("ID") {
			if id.text == nlciusGAccountMeansID {
				marked++
			}
		}
	}
	if marked != 1 {
		add("BR-GA-7", fmt.Sprintf("exactly one Payment Instruction must carry the Payment Means identifier "+
			"(NL-GA-04) %q, and %d do", nlciusGAccountMeansID, marked))
	}

	// BR-GA-4, context /ubl:Invoice/cac:PaymentTerms: count(cbc:PaymentMeansID) = 1.
	// Each payment term names the instruction it is paid by.
	for _, t := range terms {
		seen.reached("BR-GA-4")
		if n := len(t.all("PaymentMeansID")); n != 1 {
			add("BR-GA-4", fmt.Sprintf("each Payment Terms group must carry exactly one Payment Means reference "+
				"(NL-GA-02), and one carries %d", n))
		}
	}

	// BR-GA-5, context /ubl:Invoice/cac:PaymentMeans: count(cbc:ID) = 1.
	for _, pm := range means {
		seen.reached("BR-GA-5")
		if n := len(pm.all("ID")); n != 1 {
			add("BR-GA-5", fmt.Sprintf("each Payment Instruction must carry exactly one Payment Means identifier "+
				"(NL-GA-04), and one carries %d", n))
		}
	}

	// BR-GA-6, context /ubl:Invoice/cac:PaymentTerms/cbc:PaymentMeansID:
	//   . = $payment-means-ids
	// where the artefact declares
	//   $payment-means-ids = /ubl:Invoice/cac:PaymentMeans/cbc:ID/text()
	//
	// The variable is a sequence of *text nodes*, which is the detail that decides
	// what an empty identifier does: `<cbc:ID/>` has no text node and so contributes
	// nothing, and a reference to it therefore matches nothing and reports. This is
	// C32's lesson pointing the other way — there, four rules were existence
	// assertions an empty element satisfies; here an empty element is not a value.
	//
	// This rule is the one assertion in the file with no flag attribute. It is
	// reported fatal: ph-schematron, which is what phive runs, folds an absent or
	// unrecognised flag onto its DEFAULT_ERROR_LEVEL, and that is ERROR rather than
	// WARN. TestGAccountSeveritiesAreThePublishedFlags pins both the absence and the
	// reading.
	var ids []string
	for _, pm := range means {
		for _, id := range pm.all("ID") {
			if id.text != "" {
				ids = append(ids, id.text)
			}
		}
	}
	for _, t := range terms {
		for _, ref := range t.all("PaymentMeansID") {
			seen.reached("BR-GA-6")
			if !containsString(ids, ref.stringValue()) {
				add("BR-GA-6", fmt.Sprintf("the Payment Means reference (NL-GA-02) %q names no Payment Means "+
					"identifier (NL-GA-04) in this invoice", normSpace(ref.stringValue())))
			}
		}
	}

	return out
}
