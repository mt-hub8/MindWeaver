package formalis

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// This file validates the Belgian UBL.BE CIUS on top of the EN 16931 core. UBL.BE's
// own rules (ubl-BE-*) are UBL-structural rather than semantic — they check
// UBL-specific elements and Belgian code lists — so they are evaluated against the
// parsed tree rather than the syntax-neutral model.
//
// The rules below are transcribed from the ubl-model-BE pattern of
// GLOBALUBL.BE.sch rather than from prose, which is what they used to be. Reading
// the artefact changed five things:
//
//   - UBL.BE is a UBL profile and every ubl-BE context is a UBL path, so the rules
//     do not apply to a CII invoice. Nothing said so: validateUBLBERules ran on the
//     tree of whatever was parsed, and on a CII document it found no
//     cbc:DocumentDescription and no cbc:ID reading "UBL.BE", so **ubl-BE-02 and
//     ubl-BE-03 fired on every Factur-X and ZUGFeRD invoice put through
//     ValidateUBLBE**. That is the largest single false positive this file had.
//   - ubl-BE-02 and ubl-BE-03 are bound to the context //cac:AdditionalDocumentReference,
//     so they apply only to a document that has one. This package applied them
//     unconditionally, so an invoice with no additional document reference at all —
//     which is most invoices — was reported for both.
//   - ubl-BE-05, ubl-BE-10 and ubl-BE-11 are scoped: //cac:Delivery/cac:DeliveryTerms,
//     //cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory and the exemption reason code
//     inside that same category. This package searched the whole tree, so a
//     line-level cac:ClassifiedTaxCategory/cbc:TaxExemptionReasonCode was checked
//     against a list the rule does not apply to it. It also skipped an empty value
//     where the artefact's "some $code in $BVERC satisfies …" fails for one.
//   - ubl-BE-15 is count(cbc:Name) = 1, not "has a non-empty name": an empty
//     <cbc:Name/> satisfies it and two names fail it, and this package had both
//     backwards.
//   - ubl-BE-13 cannot fail. See ublBE13Reason below.
//
// All 15 published identifiers are flagged fatal, so the plain adder is right and
// the coverage table's fail-safe fatal turned out to be the authority's own flag.
// cius_artefacts_test.go checks both directions.
//
// The four that were left — ubl-BE-01, -04, -06 and -12 — are evaluated now, so
// every identifier the ubl-model-BE pattern publishes is either evaluated here or
// recorded as unevaluable. The two free-text lists (BELMText for ubl-BE-06,
// BVERCText for ubl-BE-12) had been dismissed as "exact-match lists of sentences",
// which is a description of the rule rather than a reason not to implement it: they
// are quoted verbatim below, and TestUBLBECodeListsQuoteTheArtefact reads all four
// $BELM/$BELMText/$BTCC/$BVERCText tokenize() calls back out of GLOBALUBL.BE.sch.
//
// The one place the transcription cannot be exact is one entry of $BELMText. The
// file's bytes hold a tab between the fourth semicolon and "Special ruling - art
// objects", and XML attribute-value normalisation (XML 1.0 §3.3.3) turns any tab in
// an attribute value into a space before the Schematron processor ever sees it — so
// the token an XSLT-based validator compares against is " Special ruling - art
// objects", with a leading space, while the sentence the authority's own
// documentation prints has none. ubl-BE-06 compares data(cbc:SpecialTerms) without
// normalize-space, so the difference is load-bearing. beDeliverySpecialTerms
// therefore accepts that one entry in both forms, which is the direction that
// cannot accuse a document the authority means to accept. It is one entry of
// sixteen and it is an upstream typo; see beSpecialTermsArtObjects.

// beDeliveryTerms is the BELM delivery-terms code list (ubl-BE-05), quoted from
// $BELM.
var beDeliveryTerms = map[string]bool{
	"BELM-001": true, "BELM-002": true, "BELM-003": true, "BELM-004": true,
	"BELM-005": true, "BELM-006": true, "BELM-007": true, "BELM-008": true,
}

// beSpecialTermsArtObjects is the one $BELMText entry whose transcription is not a
// quotation, kept apart from the list so the exception is visible. See the file
// comment: the artefact's byte is a tab, XML attribute-value normalisation makes it
// a space, and the sentence the authority publishes in prose has neither.
const beSpecialTermsArtObjects = "Special ruling - art objects"

// beDeliverySpecialTerms is the BELMText bilingual delivery-terms description list
// (ubl-BE-06), quoted from $BELMText: eight terms, each in English and Dutch.
//
// ubl-BE-06 tests `some $code in $BELMText satisfies data(cbc:SpecialTerms) = $code`
// — no normalize-space on either side, so the comparison is exact, and an absent
// cbc:SpecialTerms is the empty sequence, which equals no member and fails.
var beDeliverySpecialTerms = map[string]bool{
	"Special ruling - travelagencies":                                           true,
	"Bijzondere regeling - reisbureaus":                                         true,
	"Special ruling - used goods":                                               true,
	"Bijzondere regeling - gebruikte goederen":                                  true,
	beSpecialTermsArtObjects:                                                    true,
	" " + beSpecialTermsArtObjects:                                              true,
	"Bijzondere regeling - kunstvoorwerpen":                                     true,
	"Special ruling - antiques":                                                 true,
	"Bijzondere regeling - antiquiteiten":                                       true,
	"Small company under exempt from taxes ruling":                              true,
	"Kleine onderneming onderworpen aan de vrijstellingsregeling van belasting": true,
	"Invoice issued by the receiver":                                            true,
	"Factuur uitgereikt door de afnemer":                                        true,
	"Copy issued at request from the customer":                                  true,
	"Dubbel uitgereikt op vraag van de klant":                                   true,
	"VAT to be refunded to the state to the extent that it was deducted":        true,
	"BTW terug te storten aan de staat in de mate waarin ze oorspronkelijk in aftrek werd gebracht": true,
}

// beTaxCategoryNames is the BTCC Belgian tax-category code list (ubl-BE-10).
var beTaxCategoryNames = map[string]bool{
	"00": true, "01": true, "02": true, "03": true, "45": true, "NA": true, "FD": true,
	"SC": true, "00/44": true, "03/SE": true, "MA": true, "46/GO": true, "47/TO": true,
	"47/AS": true, "47/DI": true, "47/SE": true, "44": true, "46/TR": true, "47/EX": true,
	"47/EI": true, "47/EE": true, "NS": true, "OSS-S": true, "OSS-G": true, "OSS-I": true,
}

// beExemptionReasonCodes is the BVERC Belgian VAT-exemption reason code list
// (ubl-BE-11).
var beExemptionReasonCodes = map[string]bool{
	"BETE-45": true, "BETE-EX": true, "BETE-FD": true, "BETE-SC": true, "BETE-00/44": true,
	"BETE-03/SE": true, "BETE-MA": true, "BETE-46/GO": true, "BETE-47/TO": true, "BETE-47/AS": true,
	"BETE-47/DI": true, "BETE-47/SE": true, "BETE-44": true, "BETE-46/TR": true, "BETE-47/EX": true,
	"BETE-47/EI": true, "BETE-47/EE": true, "BETE-NS": true,
}

// beExemptionReasons is the BVERCText Belgian VAT-exemption reason description list
// (ubl-BE-12), quoted from $BVERCText: one sentence per BVERC code, in the same
// order.
//
// Unlike ubl-BE-06, ubl-BE-12 applies normalize-space to the document's value
// before comparing, and it is guarded on the element's presence rather than on its
// value — `if (cbc:TaxExemptionReason) then … else 1` — so an invoice that carries
// no exemption reason satisfies it.
var beExemptionReasons = map[string]bool{
	"Reverse charge - Contractor":    true,
	"Exempt":                         true,
	"Financial discount":             true,
	"Small company":                  true,
	"0% Clause 44":                   true,
	"Standard exchange":              true,
	"Margin":                         true,
	"Intra-community supply - Goods": true,
	"Intra-community supply - Manufacturing cost": true,
	"Intra-community supply - Assembly":           true,
	"Intra-community supply - Distance":           true,
	"Intra-community supply - Services":           true,
	"Intra-community supply - Services B2B":       true,
	"Intra-community supply - Triangle a-B-c":     true,
	"Export non E.U.":                             true,
	"Indirect export":                             true,
	"Export via E.U.":                             true,
	"Not subject to VAT":                          true,
}

// ublBE13Reason is the evidence for the one unevaluable family outside CEN's, kept
// beside the rule it is about and quoted verbatim into Coverage(SourceUBLBE).
//
// GLOBALUBL.BE.sch declares, in the ubl-model-BE pattern,
//
//	<let name="TaxAmount" value="if (cbc:TaxAmount) then xs:decimal(cbc:TaxAmount) else -1"/>
//
// and binds ubl-BE-13, in the context //cac:InvoiceLine/cac:TaxTotal |
// //cac:CreditNoteLine/cac:TaxTotal, to the test abs($TaxAmount) >= 0. The absolute
// value of a decimal is never negative and the fallback is the decimal -1, so the
// assertion is true for every document: a line tax total with no cbc:TaxAmount gets
// -1, whose absolute value is 1, and one with any amount at all gets that amount's
// magnitude. No conforming validator reports ubl-BE-13, which is D10's definition
// of unevaluable and the same shape as CEN binding BR-CO-05..08 to true().
//
// This package did report it — whenever a line's tax amount was absent or not a
// number — so the correction is a false-positive fix and not a coverage reduction.
// The only input on which a real processor behaves differently is a non-numeric
// amount, where xs:decimal() raises a dynamic error rather than failing the
// assertion, and a dynamic error is not a finding either.
const ublBE13Reason = "GLOBALUBL.BE.sch declares <let name=\"TaxAmount\" value=\"if (cbc:TaxAmount) then " +
	"xs:decimal(cbc:TaxAmount) else -1\"/> in the ubl-model-BE pattern and binds ubl-BE-13 to the test " +
	"abs($TaxAmount) >= 0. The absolute value of a decimal is never negative and the fallback is -1, so the " +
	"assertion holds for every document, including a line tax total with no amount at all: no conforming " +
	"validator reports this rule. This package reported it whenever a line's cbc:TaxAmount was absent or not " +
	"numeric, so recording it here is a false-positive fix. TestUBLBE13IsBoundToATautology reads the binding " +
	"back out of the file"

// ValidateUBLBE validates an invoice XML against the Belgian UBL.BE CIUS: the
// EN 16931 core plus the UBL.BE-specific rules.
//
// The EN 16931 core accepts either syntax. The ubl-BE rules are evaluated for a UBL
// document only, because UBL.BE is a UBL profile and every rule in it is bound to a
// UBL path: a CII invoice is validated against the core and reported as carrying no
// UBL.BE finding, which is what a reference UBL.BE validator says about it too.
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
// Coverage(SourceEN16931) and Coverage(SourceUBLBE).
func ValidateUBLBE(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceUBLBE}, validateUBLBE)
}

func validateUBLBE(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	// The ubl-BE rules read the tree the parse already built. This used to parse
	// the bytes a second time to obtain it, which produced a byte-identical tree
	// at the cost of a second full pass over the shared element budget.
	if p.inv.syntax != "UBL" {
		return out
	}
	return append(out, validateUBLBERules(p.root, nil)...)
}

// validateUBLBERules applies the ubl-model-BE pattern to a UBL document, in the
// order GLOBALUBL.BE.sch writes its rules.
//
// seen is nil on every production path; the reachability test passes a map so that
// "no ubl-BE finding over the corpus" can be told apart from "no ubl-BE rule was
// ever asked". See ruleContexts.
func validateUBLBERules(root *ciiNode, seen ruleContexts) []Violation {
	var out []Violation
	add := adder(&out, SourceUBLBE)

	// ubl-BE-01, context /*: count(cac:AdditionalDocumentReference) >= 2.
	//
	// The context is the document element, so this applies to every UBL document
	// unconditionally — it is the rule that makes UBL.BE's two markers mandatory,
	// and ubl-BE-02/03 are what say what has to be in them. `cac:AdditionalDocument
	// Reference` is relative to that context, so it counts the root's own children
	// and not the whole tree, which is the difference between this rule and
	// ubl-BE-02/03's `//` counts.
	seen.reached("ubl-BE-01")
	if len(root.all("AdditionalDocumentReference")) < 2 {
		add("ubl-BE-01", "at least two AdditionalDocumentReference groups shall be present")
	}

	// ubl-BE-02/03, context //cac:AdditionalDocumentReference. Both are
	// document-wide counts evaluated in that context, so they say nothing at all
	// about a document that carries no additional document reference — which is
	// why the guard is on the context and not on the count.
	for range root.findAll("AdditionalDocumentReference") {
		seen.reached("ubl-BE-02", "ubl-BE-03")
	}
	if len(root.findAll("AdditionalDocumentReference")) > 0 {
		docType := 0
		for _, d := range root.findAll("DocumentDescription") {
			if t := d.text; t == "CommercialInvoice" || t == "CreditNote" {
				docType++
			}
		}
		if docType != 1 {
			add("ubl-BE-02", "exactly one document type (DocumentDescription 'CommercialInvoice' or 'CreditNote') shall be specified")
		}
		marker := 0
		for _, x := range root.findAll("ID") {
			if x.text == "UBL.BE" {
				marker++
			}
		}
		if marker != 1 {
			add("ubl-BE-03", "exactly one cbc:ID with the value 'UBL.BE' shall be present")
		}
	}

	// ubl-BE-04, context //cac:AdditionalDocumentReference/cbc:ID:
	//   count(../cbc:DocumentDescription) = 1
	// The context is the identifier and the test is about its parent, so a group
	// with no cbc:ID at all is not reached and a group with two is asked twice.
	for _, adr := range root.findAll("AdditionalDocumentReference") {
		for range adr.all("ID") {
			seen.reached("ubl-BE-04")
			if len(adr.all("DocumentDescription")) != 1 {
				add("ubl-BE-04", "an AdditionalDocumentReference carrying a cbc:ID shall carry exactly one cbc:DocumentDescription")
			}
		}
	}

	// ubl-BE-05/06, context //cac:Delivery/cac:DeliveryTerms:
	//   05  some $code in $BELM     satisfies normalize-space(data(cbc:ID)) = $code
	//   06  some $code in $BELMText satisfies data(cbc:SpecialTerms) = $code
	// An absent cbc:ID normalizes to "", which is in no list, so 05 fails too; an
	// absent cbc:SpecialTerms is the empty sequence, which equals no member, so 06
	// fails for it as well. 06 is the one comparison in this file that is not
	// normalize-space'd on the document's side, which is why the whitespace of the
	// list entry it is compared against matters — see beSpecialTermsArtObjects.
	for _, d := range root.findAll("Delivery") {
		for _, dt := range d.all("DeliveryTerms") {
			seen.reached("ubl-BE-05", "ubl-BE-06")
			if id := strings.TrimSpace(dt.str("ID")); !beDeliveryTerms[id] {
				add("ubl-BE-05", "the Delivery terms ID ("+id+") shall be in the BELM list")
			}
			matched := false
			for _, st := range dt.all("SpecialTerms") {
				if beDeliverySpecialTerms[st.text] {
					matched = true
				}
			}
			if !matched {
				add("ubl-BE-06", "the Delivery terms special terms ("+dt.str("SpecialTerms")+") shall be one of the BELMText descriptions")
			}
		}
	}

	// ubl-BE-10/11, context //cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory. Both
	// are scoped to the VAT breakdown's category — a line's
	// cac:ClassifiedTaxCategory is a different element and is not in this context.
	for _, tt := range root.findAll("TaxTotal") {
		for _, ts := range tt.all("TaxSubtotal") {
			for _, tc := range ts.all("TaxCategory") {
				seen.reached("ubl-BE-10", "ubl-BE-11", "ubl-BE-12")
				if nm := strings.TrimSpace(tc.str("Name")); !beTaxCategoryNames[nm] {
					add("ubl-BE-10", "the VAT category name ("+nm+") shall be in the BTCC list")
				}
				// ubl-BE-11 and ubl-BE-12 are guarded on their element's presence,
				// not on its value:
				//   if (cbc:TaxExemptionReasonCode) then some $code in $BVERC     … else 1
				//   if (cbc:TaxExemptionReason)     then some $code in $BVERCText … else 1
				if c := tc.child("TaxExemptionReasonCode"); c != nil {
					if v := strings.TrimSpace(c.text); !beExemptionReasonCodes[v] {
						add("ubl-BE-11", "the VAT exemption reason code ("+v+") shall be in the BVERC list")
					}
				}
				if c := tc.child("TaxExemptionReason"); c != nil {
					if v := strings.TrimSpace(c.text); !beExemptionReasons[v] {
						add("ubl-BE-12", "the VAT exemption reason ("+v+") shall be one of the BVERCText descriptions")
					}
				}
			}
		}
	}

	// ubl-BE-07/08/09, context //cac:PaymentTerms. All three are guarded on
	// cbc:SettlementDiscountPercent being present — $SettlementDiscountPercent
	// falls back to -1, which ubl-BE-07 permits, and the other two test it
	// directly.
	for _, pt := range root.findAll("PaymentTerms") {
		seen.reached("ubl-BE-07", "ubl-BE-08", "ubl-BE-09")
		sdp := strings.TrimSpace(pt.str("SettlementDiscountPercent"))
		if sdp == "" {
			continue
		}
		if v, err := strconv.ParseFloat(sdp, 64); err != nil || !((v > 0 && v < 100) || v == -1) {
			add("ubl-BE-07", "the settlement discount percent shall be numeric and between 0 and 100")
		}
		// xs:decimal(cbc:Amount) as an assertion test: an absent amount is the
		// empty sequence and a zero amount is false, so both fail.
		amt := strings.TrimSpace(pt.str("Amount"))
		if v, err := strconv.ParseFloat(amt, 64); amt == "" || err != nil || v == 0 {
			add("ubl-BE-08", "a settlement discount shall have a non-zero Amount")
		}
		due := strings.TrimSpace(pt.str("PaymentDueDate"))
		if _, err := time.Parse("2006-01-02", due); len(due) != 10 || err != nil {
			add("ubl-BE-09", "a settlement discount shall have a PaymentDueDate formatted YYYY-MM-DD")
		}
	}

	// ubl-BE-14, context //cac:InvoiceLine | //cac:CreditNoteLine:
	//   count(cac:TaxTotal) = 1
	// ubl-BE-13, in the context //cac:InvoiceLine/cac:TaxTotal, is not evaluated:
	// it is bound to an expression that cannot be false. See ublBE13Reason.
	lines := append(root.findAll("InvoiceLine"), root.findAll("CreditNoteLine")...)
	for _, ln := range lines {
		seen.reached("ubl-BE-14")
		if len(ln.all("TaxTotal")) != 1 {
			add("ubl-BE-14", "each invoice line shall have exactly one TaxTotal")
		}
	}

	// ubl-BE-15, context //cac:ClassifiedTaxCategory: count(cbc:Name) = 1. A count,
	// not a non-empty test — an empty <cbc:Name/> satisfies it, and two of them do
	// not.
	for _, ctc := range root.findAll("ClassifiedTaxCategory") {
		seen.reached("ubl-BE-15")
		if len(ctc.all("Name")) != 1 {
			add("ubl-BE-15", "each ClassifiedTaxCategory shall have exactly one Name")
		}
	}

	return out
}
