package formalis

import (
	"fmt"
	"math"
	"strings"
)

// This file holds the syntax-neutral EN 16931 semantic model and the business-
// rule engine that validates it. An EN 16931 invoice may be expressed in either
// of two XML syntaxes — UN/CEFACT Cross Industry Invoice (CII, used by Factur-X/
// ZUGFeRD) or OASIS UBL (used by Peppol BIS and XRechnung UBL). Both carry the
// same business terms (BT-*) and groups (BG-*), so each is mapped into this model
// (mapCII / mapUBL) and the one rule engine below validates it. Keeping a single
// rule set avoids two divergent implementations of the ~200 EN 16931 rules.

// en16931Invoice is the subset of the EN 16931 semantic model the core rules
// examine. Scalar business terms are stored as their raw string values ("" when
// absent); groups are slices.
type en16931Invoice struct {
	syntax       string // "CII" or "UBL" — the binding the invoice was expressed in
	isCreditNote bool   // the UBL binding used a CreditNote (not Invoice) root
	specID       string // BT-24 Specification identifier
	profileID    string // BT-23 Business process type
	orderRef     string // BT-13 Purchase order reference
	mandateRef   string // BT-89 Mandate reference identifier
	number       string // BT-1  Invoice number
	issueDate    string // BT-2  Issue date
	typeCode     string // BT-3  Invoice type code
	currency     string // BT-5  Invoice currency code

	sellerName           string // BT-27 Seller name
	buyerName            string // BT-44 Buyer name
	sellerCountry        string // BT-40 Seller country code
	sellerAddressPresent bool   // whether the Seller postal address group (BG-5) is present
	buyerCountry         string // BT-55 Buyer country code
	buyerAddressPresent  bool   // whether the Buyer postal address group (BG-8) is present

	sellerVATID   bool // BT-31 Seller VAT identifier present
	sellerTaxReg  bool // BT-32 Seller tax registration identifier present
	taxRepVATID   bool // BT-63 Seller tax representative VAT identifier present
	buyerVATID    bool // BT-48 Buyer VAT identifier present
	buyerLegalReg bool // BT-47 Buyer legal registration identifier present

	sellerVATIDValue string // BT-31 Seller VAT identifier value
	taxRepVATIDValue string // BT-63 tax representative VAT identifier value
	buyerVATIDValue  string // BT-48 Buyer VAT identifier value
	sellerID         string // BT-29 Seller identifier
	sellerLegalReg   string // BT-30 Seller legal registration identifier

	sellerEndpointScheme  string // BT-34 Seller electronic address scheme
	sellerEndpointPresent bool   // BT-34 Seller electronic address present
	buyerEndpointScheme   string // BT-49 Buyer electronic address scheme
	buyerEndpointPresent  bool   // BT-49 Buyer electronic address present

	paymentMeans []string // BT-81 Payment means type codes

	vatPointDate  string   // BT-7 Value added tax point date
	deliveryDate  string   // BT-72 Actual delivery date
	partySchemes  []string // party identification schemes (BR-CL-10)
	legalSchemes  []string // party legal registration schemes (BR-CL-11)
	currencyIDs   []string // amount @currencyID values (BR-CL-03)
	objectSchemes []string // invoiced object identifier schemes (BR-CL-07)

	// noteSubjectCodes is BT-21, the Invoice note subject code, once per note that
	// carries one (BR-CL-08). The two syntaxes spell it very differently — CII has
	// an element for it, UBL encodes it as a "#CODE#" prefix on the note text — so
	// the mappers, not the rule, are where that difference is resolved.
	noteSubjectCodes []string
	// deliverToLocSchemes is the scheme identifier of each Deliver-to location
	// identifier (BT-71) that carries one (BR-CL-26).
	deliverToLocSchemes []string

	// Address/contact detail (optional in EN 16931; several are mandatory in the
	// XRechnung CIUS).
	buyerReference       string // BT-10 Buyer reference
	sellerCity           string // BT-37 Seller city
	sellerStreet         string // BT-35 Seller address line 1
	sellerPostCode       string // BT-38 Seller post code
	sellerLegalScheme    string // BT-30 Seller legal registration scheme identifier
	buyerLegalScheme     string // BT-47 Buyer legal registration scheme identifier
	buyerStreet          string // BT-50 Buyer address line 1
	taxRepStreet         string // Seller tax representative address line 1
	taxRepCity           string // Seller tax representative city
	taxRepPostCode       string // Seller tax representative post code
	hasOrderLineRef      bool   // BT-132 any Invoice line has an order line reference
	sellerContactPresent bool   // BG-6 Seller contact group present
	sellerContactName    string // BT-41 Seller contact point
	sellerPhone          string // BT-42 Seller contact telephone
	sellerEmail          string // BT-43 Seller contact email
	buyerCity            string // BT-52 Buyer city
	buyerPostCode        string // BT-53 Buyer post code
	deliverToCity        string // BT-77 Deliver-to city
	deliverToPostCode    string // BT-78 Deliver-to post code
	deliverToStreet      string // BT-75 Deliver-to address line 1
	sellerSubentity      string // BT-39 Seller country subdivision
	buyerSubentity       string // BT-54 Buyer country subdivision
	deliverToSubentity   string // BT-79 Deliver-to country subdivision
	taxRepSubentity      string // Seller tax representative country subdivision

	taxRepPresent        bool   // BG-11 Seller tax representative party present
	taxRepName           string // BT-62 Seller tax representative name
	taxRepAddressPresent bool   // BG-12 Seller tax representative postal address present
	taxRepCountry        string // BT-69 Tax representative country code
	payeePresent         bool   // BG-10 Payee present
	payeeName            string // BT-59 Payee name
	deliverToPresent     bool   // BG-15 Deliver-to address present
	deliverToCountry     string // BT-80 Deliver-to country code

	paymentInstrPresent  bool     // BG-16 Payment instructions present
	creditAccountPresent bool     // BG-17 Credit transfer (payee financial account) present
	creditAccountID      string   // BT-84 Payment account identifier
	cardPANs             []string // BT-87 Payment card primary account numbers
	taxCurrency          string   // BT-6 VAT accounting currency code
	vatInTaxCurrency     bool     // BT-111 VAT total in accounting currency present
	vatInTaxCurrencyAmt  string   // BT-111 VAT total in accounting currency, as written

	period invoicePeriod // BG-14 Invoicing period

	docRefs        []docReference // BG-24 Additional supporting documents
	billingRefNoID bool           // a Preceding invoice reference (BG-3) missing its ID (BT-25)
	hasBillingRef  bool           // any Preceding invoice reference (BG-3) present

	directDebitPresent bool   // BG-19 Direct debit present
	creditorID         string // BT-90 Bank assigned creditor identifier
	debitedAccount     string // BT-91 Debited account identifier

	amountDecimalsBad bool // an amount (any @currencyID element) exceeds two decimals

	hasTotals bool           // whether a document monetary summation (BG-22) is present
	totals    monetaryTotals // BG-22 Document totals

	vatBreakdowns []vatBreakdown       // BG-23 VAT breakdown
	allowCharges  []docAllowanceCharge // BG-20 allowances / BG-21 charges
	lines         []invoiceLine        // BG-25 Invoice lines
}

// monetaryTotals holds the document total amounts (BG-22); absent terms are "".
type monetaryTotals struct {
	lineTotal       string // BT-106 Sum of line net amounts
	allowanceTotal  string // BT-107 Sum of allowances on document level
	chargeTotal     string // BT-108 Sum of charges on document level
	taxBasisTotal   string // BT-109 Invoice total without VAT
	taxTotal        string // BT-110 Invoice total VAT amount (optional)
	grandTotal      string // BT-112 Invoice total with VAT
	paidAmount      string // BT-113 Paid amount
	payableRounding string // BT-114 Rounding amount
	duePayable      string // BT-115 Amount due for payment
}

type vatBreakdown struct {
	basis      string // BT-116 VAT category taxable amount
	calc       string // BT-117 VAT category tax amount
	category   string // BT-118 VAT category code
	rate       string // BT-119 VAT category rate
	hasReason  bool   // BT-120 exemption reason or BT-121 exemption reason code present
	reasonCode string // BT-121 VAT exemption reason code
}

type docAllowanceCharge struct {
	amount     string // BT-92 allowance / BT-99 charge amount
	baseAmount string // BT-93 allowance / BT-100 charge base amount
	category   string // BT-95 allowance / BT-102 charge VAT category code
	rate       string // BT-96 allowance / BT-103 charge VAT rate
	hasReason  bool   // BT-97/BT-98 or BT-104/BT-105 present
	reasonCode string // BT-98 allowance / BT-105 charge reason code
	isCharge   bool   // true = charge (BG-21); false = allowance (BG-20)
}

type invoiceLine struct {
	lineID        string // BT-126 Invoice line identifier
	parentLineID  string // EXTENDED sub-invoice-line parent reference
	itemName      string // BT-153 Item name
	netAmount     string // BT-131 Invoice line net amount
	quantity      string // BT-129 Invoiced quantity (trimmed; "" if absent/empty)
	unitCode      string // BT-130 Invoiced quantity unit of measure
	price         string // BT-146 Item net price
	vatCategory   string // BT-151 Invoiced item VAT category code
	vatRate       string // BT-152 Invoiced item VAT rate ("" if absent)
	originCountry string // BT-159 Item country of origin
	allowCharges  []lineAllowanceCharge
	period        invoicePeriod // BG-26 Invoice line period

	grossPrice   string // BT-148 Item gross price
	baseQty      string // BT-149 Item price base quantity
	baseQtyUnit  string // BT-150 Item price base quantity unit of measure
	itemAttrBad  bool   // an Item attribute (BG-32) missing its name or value
	stdIDPresent bool   // BT-157 Item standard identifier present
	stdIDScheme  string // BT-157 scheme identifier
	classPresent bool   // BT-158 Item classification identifier present
	classListID  string // BT-158 classification scheme (list) identifier
}

// docReference is an Additional supporting document (BG-24).
type docReference struct {
	hasID         bool   // BT-122 Supporting document reference present
	binaryPresent bool   // an embedded binary attachment is present
	mimeCode      string // attachment MIME code
	filename      string // attachment file name
}

// invoicePeriod is a billing period (BG-14 at document level, BG-26 per line).
type invoicePeriod struct {
	present bool   // whether the period group is present
	start   string // start date (BT-73 / BT-134)
	end     string // end date (BT-74 / BT-135)
	desc    string // BT-8 VAT point date code (UBL carries it in the period group)
}

// lineAllowanceCharge is an Invoice line allowance (BG-27) or charge (BG-28).
type lineAllowanceCharge struct {
	amount     string // BT-136 allowance / BT-141 charge amount
	baseAmount string // BT-137 allowance / BT-142 charge base amount
	hasReason  bool   // BT-139/BT-140 or BT-144/BT-145 present
	isCharge   bool   // true = charge (BG-28); false = allowance (BG-27)
}

// vatAmountTolerance is the EN 16931 tolerance (±1 of the invoice currency) the
// per-VAT-breakdown amount checks (BR-CO-17, BR-S-09) allow, absorbing the
// per-line VAT rounding that accumulates into a breakdown's tax amount. The other
// monetary-total rules require exact two-decimal equality, which the 0.005 epsilon
// expresses for two-decimal amounts.
const vatAmountTolerance = 1.0

// validateEN16931 applies the EN 16931 core business rules to a parsed invoice.
// The rule identifiers, messages and tolerances match the values validators and
// the EN 16931 Schematron report, so the same output holds for either syntax.
//
// It takes the parsed document rather than the model alone because EN 16931 is
// two kinds of statement, not one. The BR-* rules are about business terms and
// are evaluated on the syntax-neutral model, which is the whole point of the
// model. CEN also publishes a syntax binding per syntax — UBL-SR-* for UBL,
// CII-SR-* and CII-DT-* for CII — whose rules are about the shape of the XML,
// and those are evaluated on the tree, in en16931_ubl_rules.go and
// en16931_cii_rules.go, which say why at greater length.
//
// binding names whose CII binding the caller wants, because the CII binding is
// the one part of this that is not CEN's everywhere. Every CIUS in this package
// imports CEN's — KoSIT's released Schematron and OpenPEPPOL's both include
// EN16931-CII-validation — and Factur-X does not, publishing one of its own
// instead. Naming it at each call site rather than deriving it from profile is
// deliberate: eight callers pass ciiBindingCEN and mean it, and that is a fact
// about those authorities worth reading at the call. See facturx.go.
func validateEN16931(r *run, p *parsed, profile Profile, binding ciiBinding) []Violation {
	inv := p.inv
	var out []Violation
	add := adder(&out, SourceEN16931)
	req := func(rule, msg, val string) {
		if val == "" {
			add(rule, msg)
		}
	}

	// Mandatory document-level business terms (present in every profile).
	req("BR-01", "An Invoice shall have a Specification identifier (BT-24)", inv.specID)
	req("BR-02", "An Invoice shall have an Invoice number (BT-1)", inv.number)
	req("BR-03", "An Invoice shall have an Invoice issue date (BT-2)", inv.issueDate)
	req("BR-04", "An Invoice shall have an Invoice type code (BT-3)", inv.typeCode)
	req("BR-05", "An Invoice shall have an Invoice currency code (BT-5)", inv.currency)
	req("BR-06", "An Invoice shall contain the Seller name (BT-27)", inv.sellerName)
	req("BR-07", "An Invoice shall contain the Buyer name (BT-44)", inv.buyerName)
	// BR-08 is a group-presence test: the Seller postal address (BG-5) element
	// must be present, even if empty. Its content (the country code) is BR-09.
	if !inv.sellerAddressPresent {
		add("BR-08", "An Invoice shall contain the Seller postal address (BG-5)")
	}
	req("BR-09", "The Seller postal address shall contain a Seller country code (BT-40)", inv.sellerCountry)
	// The Buyer postal address (BG-8) is not mandatory in the reduced MINIMUM CIUS.
	if profile != ProfileMinimum {
		if !inv.buyerAddressPresent {
			add("BR-10", "An Invoice shall contain the Buyer postal address (BG-8)")
		}
		req("BR-11", "The Buyer postal address shall contain a Buyer country code (BT-55)", inv.buyerCountry)
	}

	// Conditional party groups. A Seller tax representative (BG-11), Payee (BG-10)
	// or Deliver-to address (BG-15), when present, must carry its mandatory terms.
	if inv.taxRepPresent {
		req("BR-18", "The Seller tax representative name (BT-62) shall be provided when a tax representative party is present", inv.taxRepName)
		if !inv.taxRepAddressPresent {
			add("BR-19", "The Seller tax representative postal address (BG-12) shall be provided when a tax representative party is present")
		}
		if !inv.taxRepVATID {
			add("BR-56", "Each Seller tax representative party (BG-11) shall have a Seller tax representative VAT identifier (BT-63)")
		}
	}
	if inv.taxRepAddressPresent {
		req("BR-20", "The Seller tax representative postal address (BG-12) shall contain a Tax representative country code (BT-69)", inv.taxRepCountry)
	}
	if inv.payeePresent {
		req("BR-17", "The Payee name (BT-59) shall be provided when the Payee (BG-10) is present", inv.payeeName)
	}
	if inv.deliverToPresent {
		req("BR-57", "Each Deliver to address (BG-15) shall contain a Deliver to country code (BT-80)", inv.deliverToCountry)
	}

	// Payment instructions (BG-16/17).
	if inv.paymentInstrPresent && len(inv.paymentMeans) == 0 {
		add("BR-49", "A Payment instruction (BG-16) shall specify the Payment means type code (BT-81)")
	}
	if inv.creditAccountPresent && inv.creditAccountID == "" {
		add("BR-50", "A Payment account identifier (BT-84) shall be present when Credit transfer (BG-17) information is provided")
	}
	for _, pm := range inv.paymentMeans {
		if (pm == "30" || pm == "58") && inv.creditAccountID == "" {
			add("BR-61", "A credit-transfer payment means (BT-81) requires a Payment account identifier (BT-84)")
			break
		}
	}
	// BR-51: an invoice shall not carry a full payment card number (BT-87). The
	// PCI Security Standards Council permits the first six and last four digits,
	// so the CEN test is a length one: at most ten characters after
	// normalisation.
	//
	// This is reported for CII only, and that asymmetry is CEN's rather than this
	// package's. The rule is one assertion in the abstract model, but the two
	// bindings give it two different severities: EN16931-CII-model.sch flags it
	// fatal, EN16931-UBL-model.sch flags it warning, and the CEN unit-test suite
	// tags the failing UBL fragment <warning>, not <error>. This package reports
	// what an authority makes fatal and names its advisory rules in Coverage
	// instead (NLCIUS's BR-NL-19..35 are the precedent), so the UBL half stays in
	// the table and the CII half is checked. Reporting a UBL invoice for a rule
	// KoSIT and the CEN validators merely warn on would be an over-report on a
	// document those validators pass.
	if inv.syntax == "CII" {
		for _, pan := range inv.cardPANs {
			if len(strings.Join(strings.Fields(pan), " ")) > 10 {
				add("BR-51", "An Invoice shall not contain a full payment card primary account number (BT-87); at most the first 6 and last 4 digits may be shown")
			}
		}
	}
	// BR-53: a VAT accounting currency (BT-6) requires the VAT total in that
	// currency (BT-111); BR-CL-05 validates the code itself.
	if inv.taxCurrency != "" {
		if !inv.vatInTaxCurrency {
			add("BR-53", "When a VAT accounting currency code (BT-6) is present, the Invoice total VAT amount in accounting currency (BT-111) shall be provided")
		}
		if !en16931Currencies[inv.taxCurrency] {
			add("BR-CL-05", fmt.Sprintf("VAT accounting currency code (BT-6=%q) shall be a valid ISO 4217 code", inv.taxCurrency))
		}
	}

	// Item detail: gross price (BR-28), item attributes (BR-54), and the item
	// standard/classification identifiers and their scheme code lists (BR-64/65,
	// BR-CL-21/13). Each identifier check is conditional on the element's presence.
	// Between rule groups: one pass over the invoice's lines or breakdowns is
	// the coarsest unit of work here, so this is the granularity of
	// cancellation. The findings already gathered are returned — they are
	// true, just incomplete — and r.finish adds the RuleLimit trip.
	if r.stopped() {
		return out
	}
	for _, li := range inv.lines {
		if gp := li.grossPrice; gp != "" {
			if p, ok := parseAmount(gp); ok && p < 0 {
				add("BR-28", "The Item gross price (BT-148) shall not be negative")
			}
		}
		if li.itemAttrBad {
			add("BR-54", "Each Item attribute (BG-32) shall contain an Item attribute name (BT-160) and value (BT-161)")
		}
		if li.stdIDPresent && li.stdIDScheme == "" {
			add("BR-64", "The Item standard identifier (BT-157) shall have a Scheme identifier")
		}
		if s := li.stdIDScheme; s != "" && !en16931ICD[s] {
			add("BR-CL-21", fmt.Sprintf("Item standard identifier scheme (%q) shall belong to the ISO 6523 ICD list", s))
		}
		if li.classPresent && li.classListID == "" {
			add("BR-65", "The Item classification identifier (BT-158) shall have a Scheme identifier")
		}
		if l := li.classListID; l != "" && !en16931ItemClassCodes[l] {
			add("BR-CL-13", fmt.Sprintf("Item classification scheme (%q) shall be a valid UNTDID 7143 value", l))
		}
		if u := li.baseQtyUnit; u != "" && !en16931Units[u] {
			add("BR-CL-23", fmt.Sprintf("Item price base quantity unit code (BT-150=%q) is not a valid UNECE Rec 20/21 code", u))
		}
	}

	// Supporting documents (BR-52, mime BR-CL-24), preceding invoice references
	// (BR-55) and the VAT point date code (BR-CL-06).
	for _, d := range inv.docRefs {
		if !d.hasID {
			add("BR-52", "Each Additional supporting document (BG-24) shall contain a Supporting document reference (BT-122)")
		}
		if m := d.mimeCode; m != "" && !en16931MIME[m] {
			add("BR-CL-24", fmt.Sprintf("Attachment MIME code (%q) is not a permitted value", m))
		}
	}
	if inv.billingRefNoID {
		add("BR-55", "Each Preceding Invoice reference (BG-3) shall contain a Preceding Invoice reference (BT-25)")
	}
	if c := inv.period.desc; c != "" && c != "3" && c != "35" && c != "432" {
		add("BR-CL-06", fmt.Sprintf("Value added tax point date code (BT-8=%q) shall be a restriction of UNTDID 2005", c))
	}

	// Electronic addresses (BR-62/63) and their and other scheme identifiers
	// against the ISO 6523 / UNTDID 1153 code lists (BR-CL-10/11/07); amount
	// currency identifiers against ISO 4217 (BR-CL-03).
	if inv.sellerEndpointPresent && inv.sellerEndpointScheme == "" {
		add("BR-62", "The Seller electronic address (BT-34) shall have a Scheme identifier")
	}
	if inv.buyerEndpointPresent && inv.buyerEndpointScheme == "" {
		add("BR-63", "The Buyer electronic address (BT-49) shall have a Scheme identifier")
	}
	for _, s := range inv.partySchemes {
		if !en16931ICD[s] && s != "SEPA" {
			add("BR-CL-10", fmt.Sprintf("Party identifier scheme (%q) shall belong to the ISO 6523 ICD list", s))
		}
	}
	for _, s := range inv.legalSchemes {
		if !en16931ICD[s] {
			add("BR-CL-11", fmt.Sprintf("Party legal registration scheme (%q) shall belong to the ISO 6523 ICD list", s))
		}
	}
	for _, s := range inv.objectSchemes {
		if !en16931RefTypeCodes[s] {
			add("BR-CL-07", fmt.Sprintf("Object identifier scheme (%q) shall be a restriction of UNTDID 1153", s))
		}
	}
	// BR-CL-08: the Invoice note subject code (BT-21) against UNTDID 4451. Both
	// bindings carry the same code list; the mappers already resolved the very
	// different ways they spell the term.
	//
	// The list is the CEN genericode, which is what the CII binding embeds. The
	// UBL binding embeds a copy that is eighteen codes short of it (BAT..BBB,
	// BMF..BMH, CCJ..CCO), so a UBL invoice using one of those newer UNTDID 4451
	// codes is reported by the published UBL Schematron and not here. Following
	// the stale copy would mean this package minting a finding against a code the
	// authority's own code list says is valid.
	for _, c := range inv.noteSubjectCodes {
		if !en16931TextSubjectCodes[c] {
			add("BR-CL-08", fmt.Sprintf("Invoice note subject code (BT-21=%q) shall be coded using UNTDID 4451", c))
		}
	}
	// BR-CL-26: the Deliver-to location identifier scheme (BT-71) against the
	// ISO 6523 ICD list — the same list BR-CL-10/11/21 use.
	for _, s := range inv.deliverToLocSchemes {
		if !en16931ICD[s] {
			add("BR-CL-26", fmt.Sprintf("Deliver to location identifier scheme (BT-71=%q) shall belong to the ISO 6523 ICD list", s))
		}
	}
	for _, c := range inv.currencyIDs {
		if !en16931Currencies[c] {
			add("BR-CL-03", fmt.Sprintf("Amount currency identifier (%q) shall be a valid ISO 4217 code", c))
			break
		}
	}

	// BR-CO-03: the VAT point date (BT-7) and its code (BT-8) are mutually
	// exclusive.
	if inv.vatPointDate != "" && inv.period.desc != "" {
		add("BR-CO-03", "the Value added tax point date (BT-7) and code (BT-8) are mutually exclusive")
	}
	// BR-CO-09: each VAT identifier (BT-31/63/48) shall carry a country-code
	// prefix — an ISO 3166-1 alpha-2 code, plus "EL" for Greece, which uses a VAT
	// prefix that is not its country code.
	//
	// An identifier too short to hold a two-character prefix is a violation, not a
	// reason to look away: "D" is not a country-prefixed VAT identifier under any
	// reading of the rule, and the guard that skipped it reported nothing while
	// "XX123", "123456789" and "de123" were all correctly reported. An absent
	// identifier stays a no-op — presence is BR-CO-26's and the CIUS layer's
	// business, not this rule's.
	for _, v := range []string{inv.sellerVATIDValue, inv.taxRepVATIDValue, inv.buyerVATIDValue} {
		if v == "" {
			continue
		}
		if len(v) < 2 || (!en16931Countries[v[:2]] && v[:2] != "EL") {
			add("BR-CO-09", fmt.Sprintf("VAT identifier (%q) shall have a country-code prefix", v))
		}
	}
	// BR-CO-26: the buyer must be able to identify the seller (BT-29/30/31).
	if inv.sellerID == "" && inv.sellerLegalReg == "" && inv.sellerVATIDValue == "" {
		add("BR-CO-26", "the Seller identifier (BT-29), legal registration (BT-30) or VAT identifier (BT-31) shall be present")
	}

	// BR-IC-11/12: an intra-community supply (category K) breakdown requires a
	// delivery date or invoicing period, and a Deliver-to country.
	hasIC := false
	if r.stopped() {
		return out
	}
	for _, b := range inv.vatBreakdowns {
		if b.category == "K" {
			hasIC = true
		}
	}
	if hasIC {
		hasDelivery := inv.deliveryDate != "" || (inv.period.present && (inv.period.start != "" || inv.period.end != ""))
		if !hasDelivery {
			add("BR-IC-11", "an intra-community supply requires an Actual delivery date (BT-72) or an Invoicing period (BG-14)")
		}
		if inv.deliverToCountry == "" {
			add("BR-IC-12", "an intra-community supply requires a Deliver to country code (BT-80)")
		}
	}

	// Datatype rules from the UBL binding that the model can answer. These are
	// UBL-only: the reported identifier is a UBL one, and a CII invoice does not
	// report them, because CII bounds decimals through the shared BR-DEC rules
	// and states its attribute requirements as CII-DT-* rules, which are
	// evaluated against the tree in en16931_cii_rules.go.
	//
	// The rest of the UBL binding — the 54 fatal UBL-SR-* cardinality rules — is
	// not here. Those are statements about the UBL document tree rather than
	// about business terms, and they are evaluated against the tree in
	// en16931_ubl_rules.go.
	ublOnly := func(rule, msg string) {
		if inv.syntax != "CII" {
			add(rule, msg)
		}
	}
	if inv.amountDecimalsBad {
		ublOnly("UBL-DT-01", "Amounts shall be decimal up to two fraction digits")
	}
	for _, d := range inv.docRefs {
		if d.binaryPresent && d.mimeCode == "" {
			ublOnly("UBL-DT-06", "A binary object (attachment) shall carry a MIME code attribute")
		}
		if d.binaryPresent && d.filename == "" {
			ublOnly("UBL-DT-07", "A binary object (attachment) shall carry a file name attribute")
		}
	}
	// Full-invoice profiles carry lines and a line-net total; the head-only
	// Factur-X CIUS (MINIMUM, BASIC WL) legitimately omit both, so gate the
	// line-presence rules to profiles that carry lines.
	headOnly := profile == ProfileMinimum || profile == ProfileBasicWL
	if !headOnly {
		if len(inv.lines) == 0 {
			add("BR-16", "An Invoice shall have at least one Invoice line (BG-25)")
		}
		req("BR-12", "An Invoice shall have the Sum of Invoice line net amount (BT-106)", inv.totals.lineTotal)
	}
	// BR-CO-18: at least one VAT breakdown group. MINIMUM carries only totals, no
	// breakdown, so it is exempt.
	if profile != ProfileMinimum && len(inv.vatBreakdowns) == 0 {
		add("BR-CO-18", "An Invoice shall at least have one VAT breakdown group (BG-23)")
	}

	req("BR-13", "An Invoice shall have the Invoice total amount without VAT (BT-109)", inv.totals.taxBasisTotal)
	req("BR-14", "An Invoice shall have the Invoice total amount with VAT (BT-112)", inv.totals.grandTotal)
	req("BR-15", "An Invoice shall have the Amount due for payment (BT-115)", inv.totals.duePayable)

	// Code lists (BR-CL-*). The Invoice currency code (BT-5, BR-CL-04) and the
	// Invoice type code (BT-3, BR-CL-01) are checked against the exact EN 16931
	// code lists; the country codes (BT-40/55) against ISO 3166-1 alpha-2 shape.
	if cur := inv.currency; cur != "" && !en16931Currencies[cur] {
		add("BR-CL-04", fmt.Sprintf("Invoice currency code (BT-5=%q) shall be a valid ISO 4217 alpha-3 code", cur))
	}
	if tc := inv.typeCode; tc != "" && !en16931TypeCodes[tc] {
		add("BR-CL-01", fmt.Sprintf("Invoice type code (BT-3=%q) is not a permitted UNTDID 1001 value", tc))
	}
	// BR-CL-14 covers every postal-address country code (a cac:Country); BR-CL-15
	// is the Item country of origin (BT-159, a cac:OriginCountry).
	for _, c := range []struct{ term, val string }{
		{"Seller country code (BT-40)", inv.sellerCountry},
		{"Buyer country code (BT-55)", inv.buyerCountry},
	} {
		if c.val != "" && !en16931Countries[c.val] {
			add("BR-CL-14", fmt.Sprintf("%s=%q shall be a valid ISO 3166-1 code", c.term, c.val))
		}
	}
	if r.stopped() {
		return out
	}
	for _, li := range inv.lines {
		if oc := li.originCountry; oc != "" && !en16931Countries[oc] {
			add("BR-CL-15", fmt.Sprintf("Item country of origin (BT-159=%q) shall be a valid ISO 3166-1 code", oc))
		}
	}
	// BR-CL-16: payment means (BT-81) against UNCL 4461.
	for _, pm := range inv.paymentMeans {
		if !en16931PaymentMeans[pm] {
			add("BR-CL-16", fmt.Sprintf("Payment means type code (BT-81=%q) is not a valid UNCL 4461 value", pm))
		}
	}
	// BR-CL-25: the Seller/Buyer electronic address scheme (BT-34/49) against the
	// CEF Electronic Address Scheme code list.
	if s := inv.sellerEndpointScheme; s != "" && !en16931EAS[s] {
		add("BR-CL-25", fmt.Sprintf("Seller electronic address scheme (BT-34=%q) is not a valid EAS value", s))
	}
	if s := inv.buyerEndpointScheme; s != "" && !en16931EAS[s] {
		add("BR-CL-25", fmt.Sprintf("Buyer electronic address scheme (BT-49=%q) is not a valid EAS value", s))
	}
	// BR-CL-22: the VAT exemption reason code (BT-121) against the CEF VATEX list.
	if r.stopped() {
		return out
	}
	for _, tt := range inv.vatBreakdowns {
		if rc := tt.reasonCode; rc != "" && !en16931VATEX[rc] {
			add("BR-CL-22", fmt.Sprintf("VAT exemption reason code (BT-121=%q) is not a valid VATEX value", rc))
		}
	}

	// Decimals (BR-DEC-*): monetary amounts shall have at most two decimal places.
	dec := func(rule, name, val string) {
		if val != "" && decimalCount(val) > 2 {
			add(rule, fmt.Sprintf("amount %s (%q) shall have at most two decimals", name, val))
		}
	}
	if inv.hasTotals {
		dec("BR-DEC-09", "Sum of line net amounts (BT-106)", inv.totals.lineTotal)
		dec("BR-DEC-10", "Sum of allowances on document level (BT-107)", inv.totals.allowanceTotal)
		dec("BR-DEC-11", "Sum of charges on document level (BT-108)", inv.totals.chargeTotal)
		dec("BR-DEC-12", "Invoice total without VAT (BT-109)", inv.totals.taxBasisTotal)
		dec("BR-DEC-13", "Invoice total VAT amount (BT-110)", inv.totals.taxTotal)
		dec("BR-DEC-14", "Invoice total with VAT (BT-112)", inv.totals.grandTotal)
		dec("BR-DEC-16", "Paid amount (BT-113)", inv.totals.paidAmount)
		dec("BR-DEC-17", "Rounding amount (BT-114)", inv.totals.payableRounding)
		dec("BR-DEC-18", "Amount due for payment (BT-115)", inv.totals.duePayable)
	}
	if r.stopped() {
		return out
	}
	for _, tt := range inv.vatBreakdowns {
		dec("BR-DEC-19", "VAT category taxable amount (BT-116)", tt.basis)
		dec("BR-DEC-20", "VAT category tax amount (BT-117)", tt.calc)
	}
	if r.stopped() {
		return out
	}
	for _, li := range inv.lines {
		dec("BR-DEC-23", "Invoice line net amount (BT-131)", li.netAmount)
	}
	if r.stopped() {
		return out
	}
	for _, ac := range inv.allowCharges {
		if ac.isCharge {
			dec("BR-DEC-05", "Document level charge amount (BT-99)", ac.amount)
			dec("BR-DEC-06", "Document level charge base amount (BT-100)", ac.baseAmount)
		} else {
			dec("BR-DEC-01", "Document level allowance amount (BT-92)", ac.amount)
			dec("BR-DEC-02", "Document level allowance base amount (BT-93)", ac.baseAmount)
		}
	}
	// BR-DEC-15: the Invoice total VAT amount in accounting currency (BT-111).
	// Checked only when the amount was found, which is where the two bindings
	// agree. A VAT accounting currency (BT-6) declared without a BT-111 at all is
	// BR-53's finding, not a decimal one — the CII binding's phrasing does report
	// BR-DEC-15 for that shape as well, and duplicating BR-53 under a decimal
	// rule's identifier would say something the document's defect does not.
	dec("BR-DEC-15", "Invoice total VAT amount in accounting currency (BT-111)", inv.vatInTaxCurrencyAmt)

	// BR-CO-15: Invoice total with VAT (BT-112) = total without VAT (BT-109) +
	// total VAT amount (BT-110).
	if inv.hasTotals {
		basis, okB := parseAmount(inv.totals.taxBasisTotal)
		grand, okG := parseAmount(inv.totals.grandTotal)
		tax, okT := parseAmount(inv.totals.taxTotal)
		if !okT {
			tax = 0 // BT-110 is optional when there is no VAT
		}
		if okB && okG && math.Abs((basis+tax)-grand) > 0.005 {
			add("BR-CO-15", fmt.Sprintf("Invoice total with VAT (BT-112=%.2f) shall equal total without VAT (BT-109=%.2f) + VAT total (BT-110=%.2f)", grand, basis, tax))
		}
	}

	// VAT breakdown (BG-23) rules, applied to each entry present, so profiles
	// without a breakdown (MINIMUM) are naturally skipped.
	var vatTotal float64
	if r.stopped() {
		return out
	}
	for _, tt := range inv.vatBreakdowns {
		if tt.basis == "" {
			add("BR-45", "Each VAT breakdown (BG-23) shall have a VAT category taxable amount (BT-116)")
		}
		if tt.calc == "" {
			add("BR-46", "Each VAT breakdown (BG-23) shall have a VAT category tax amount (BT-117)")
		}
		if tt.category == "" {
			add("BR-47", "Each VAT breakdown (BG-23) shall be defined through a VAT category code (BT-118)")
		} else if !validEN16931VATCategory(tt.category) {
			add("BR-CL-17", fmt.Sprintf("VAT category code (BT-118=%q) is not a valid UNCL 5305 value", tt.category))
		}
		// BR-48: a rate is required except for the "Not subject to VAT" category (O).
		if tt.rate == "" && tt.category != "O" {
			add("BR-48", "Each VAT breakdown (BG-23) shall have a VAT category rate (BT-119)")
		}
		// BR-CO-17: BT-117 = BT-116 x (BT-119 / 100), rounded to two decimals. The
		// EN 16931 tolerance is ±1 (vatAmountTolerance), not exact, because per-line
		// VAT rounding accumulates into the breakdown amount.
		// Named pct, not r: r is the *run in this scope, and every other
		// per-collection loop in this function polls r.stopped(). Shadowing it with
		// a float64 makes adding that poll here — the natural next change — a
		// compile error at best, and go vet does not flag the shadow.
		b, okB := parseAmount(tt.basis)
		c, okC := parseAmount(tt.calc)
		pct, okR := parseAmount(tt.rate)
		if okB && okC && okR && math.Abs(round2(b*pct/100)-c) >= vatAmountTolerance {
			add("BR-CO-17", fmt.Sprintf("VAT category tax amount (BT-117=%.2f) shall equal taxable amount (BT-116=%.2f) x rate (BT-119=%.2f%%)", c, b, pct))
		}
		if okC {
			vatTotal += c
		}
	}
	// VAT category rules (BR-S/Z/E/AE/IC/G/O-*): breakdown existence, per-line and
	// per-allowance/charge rate constraints, taxable-amount sums, tax-zero, and
	// exemption-reason presence.
	validateVATCategories(r, inv, add)
	// BR-CO-14: Invoice total VAT amount (BT-110) = sum of VAT category tax
	// amounts (BT-117), when a breakdown is present.
	if len(inv.vatBreakdowns) > 0 {
		if tax, ok := parseAmount(inv.totals.taxTotal); ok && math.Abs(vatTotal-tax) > 0.005 {
			add("BR-CO-14", fmt.Sprintf("Invoice total VAT (BT-110=%.2f) shall equal the sum of VAT breakdown tax amounts (%.2f)", tax, vatTotal))
		}
	}

	// Document-level allowance (BG-20) and charge (BG-21) rules.
	if r.stopped() {
		return out
	}
	for _, ac := range inv.allowCharges {
		if ac.isCharge {
			if ac.amount == "" {
				add("BR-36", "Each Document level charge (BG-21) shall have a Document level charge amount (BT-99)")
			}
			if ac.category == "" {
				add("BR-37", "Each Document level charge (BG-21) shall have a Document level charge VAT category code (BT-102)")
			}
			if !ac.hasReason {
				add("BR-38", "Each Document level charge (BG-21) shall have a Document level charge reason (BT-104) or reason code (BT-105)")
				add("BR-CO-22", "Each Document level charge (BG-21) shall contain a Document level charge reason (BT-104) or reason code (BT-105)")
			}
			if rc := ac.reasonCode; rc != "" && !en16931ChargeReasons[rc] {
				add("BR-CL-20", fmt.Sprintf("Document level charge reason code (BT-105=%q) is not a valid UNCL 7161 value", rc))
			}
		} else {
			if ac.amount == "" {
				add("BR-31", "Each Document level allowance (BG-20) shall have a Document level allowance amount (BT-92)")
			}
			if ac.category == "" {
				add("BR-32", "Each Document level allowance (BG-20) shall have a Document level allowance VAT category code (BT-95)")
			}
			if !ac.hasReason {
				add("BR-33", "Each Document level allowance (BG-20) shall have a Document level allowance reason (BT-97) or reason code (BT-98)")
				add("BR-CO-21", "Each Document level allowance (BG-20) shall contain a Document level allowance reason (BT-97) or reason code (BT-98)")
			}
			if rc := ac.reasonCode; rc != "" && !en16931AllowanceReasons[rc] {
				add("BR-CL-19", fmt.Sprintf("Document level allowance reason code (BT-98=%q) is not a valid UNCL 5189 value", rc))
			}
		}
	}

	// Line-item rules (BG-25). Includes EXTENDED sub-invoice lines (flat siblings
	// distinguished by a parent line reference). A grouping line — one referenced
	// as another line's parent — aggregates its sub-lines and need not carry an
	// invoiced quantity or item price, so those two are checked only on leaves.
	grouping := map[string]bool{}
	if r.stopped() {
		return out
	}
	for _, li := range inv.lines {
		if li.parentLineID != "" {
			grouping[li.parentLineID] = true
		}
	}
	if r.stopped() {
		return out
	}
	for i, li := range inv.lines {
		if li.lineID == "" {
			add("BR-21", fmt.Sprintf("Invoice line %d shall have an Invoice line identifier (BT-126)", i+1))
		}
		if li.itemName == "" {
			add("BR-25", "Each Invoice line shall have an Item name (BT-153)")
		}
		if li.netAmount == "" {
			add("BR-24", "Each Invoice line shall have an Invoice line net amount (BT-131)")
		}
		if grouping[li.lineID] {
			continue // grouping line: no direct quantity, price or item VAT category
		}
		if li.vatCategory == "" {
			add("BR-CO-04", "Each Invoice line (BG-25) shall be categorized with an Invoiced item VAT category code (BT-151)")
		}
		if li.quantity == "" {
			add("BR-22", "Each Invoice line shall have an Invoiced quantity (BT-129)")
		} else if li.unitCode == "" {
			add("BR-23", "Each Invoice line shall have an Invoiced quantity unit of measure (BT-130)")
		} else if !en16931Units[li.unitCode] {
			add("BR-CL-23", fmt.Sprintf("Invoiced quantity unit code (BT-130=%q) is not a valid UNECE Rec 20/21 code", li.unitCode))
		}
		if li.price == "" {
			add("BR-26", "Each Invoice line shall have an Item net price (BT-146)")
		} else if p, ok := parseAmount(li.price); ok && p < 0 {
			add("BR-27", "The Item net price (BT-146) shall not be negative")
		}
		// Invoice line allowances (BG-27) and charges (BG-28).
		for _, ac := range li.allowCharges {
			if ac.isCharge {
				if ac.amount == "" {
					add("BR-43", "Each Invoice line charge (BG-28) shall have an Invoice line charge amount (BT-141)")
				} else {
					dec("BR-DEC-27", "Invoice line charge amount (BT-141)", ac.amount)
				}
				dec("BR-DEC-28", "Invoice line charge base amount (BT-142)", ac.baseAmount)
				if !ac.hasReason {
					add("BR-44", "Each Invoice line charge (BG-28) shall have an Invoice line charge reason (BT-144) or reason code (BT-145)")
					add("BR-CO-24", "Each Invoice line charge (BG-28) shall contain an Invoice line charge reason (BT-144) or reason code (BT-145)")
				}
			} else {
				if ac.amount == "" {
					add("BR-41", "Each Invoice line allowance (BG-27) shall have an Invoice line allowance amount (BT-136)")
				} else {
					dec("BR-DEC-24", "Invoice line allowance amount (BT-136)", ac.amount)
				}
				dec("BR-DEC-25", "Invoice line allowance base amount (BT-137)", ac.baseAmount)
				if !ac.hasReason {
					add("BR-42", "Each Invoice line allowance (BG-27) shall have an Invoice line allowance reason (BT-139) or reason code (BT-140)")
					add("BR-CO-23", "Each Invoice line allowance (BG-27) shall contain an Invoice line allowance reason (BT-139) or reason code (BT-140)")
				}
			}
		}
	}

	// BR-CO-10: Sum of Invoice line net amounts (BT-106) = sum of line net
	// amounts (BT-131), over every line the document carries.
	//
	// This used to skip a line that named another as its parent, on the premise
	// that a sub-invoice line's amount is rolled up into its parent's BT-131. That
	// premise came in with the initial import, carries no argument, and is not
	// CEN's: EN16931-CII-model.sch sums ../../ram:IncludedSupplyChainTradeLineItem,
	// and a CII sub-line is a sibling of the line it references rather than a
	// child of it, so CEN counts both. The corpus settles it —
	// cii-br-dex-15-test-on-sub-invoice-lines.xml, one of KoSIT's own instances,
	// has a parent of 288,79, a sub-line of 26,07 and a BT-106 of 314,86 — and the
	// carve-out was reporting that document for a rule CEN's validator passes it
	// on. Nothing is rolled up there.
	//
	// A producer whose GROUP lines *do* roll their children up gets a finding here
	// and gets the same one from any validator that imports CEN's CII binding.
	// Factur-X reads the same summation over the DETAIL lines only, which is
	// BR-FXEXT-CO-10, and on a document being validated as Factur-X that reading
	// governs — see facturXAuthorityParity.
	if len(inv.lines) > 0 && inv.hasTotals {
		var lineSum float64
		if r.stopped() {
			return out
		}
		for _, li := range inv.lines {
			if v, ok := parseAmount(li.netAmount); ok {
				lineSum += v
			}
		}
		if lt, ok := parseAmount(inv.totals.lineTotal); ok && math.Abs(round2(lineSum)-lt) > 0.005 {
			add("BR-CO-10", fmt.Sprintf("Sum of Invoice line net amount (BT-106=%.2f) shall equal the sum of line net amounts (%.2f)", lt, lineSum))
		}
	}

	// BR-CO-13: Invoice total without VAT (BT-109) = line total (BT-106) minus
	// the allowance total (BT-107) plus the charge total (BT-108).
	if inv.hasTotals {
		if lt, ok := parseAmount(inv.totals.lineTotal); ok {
			allowances, _ := parseAmount(inv.totals.allowanceTotal)
			charges, _ := parseAmount(inv.totals.chargeTotal)
			if basis, ok := parseAmount(inv.totals.taxBasisTotal); ok &&
				math.Abs(round2(lt-allowances+charges)-basis) > 0.005 {
				add("BR-CO-13", fmt.Sprintf("Invoice total without VAT (BT-109=%.2f) shall equal line total (%.2f) - allowances (%.2f) + charges (%.2f)", basis, lt, allowances, charges))
			}
		}
	}

	// BR-CO-11/12: the allowance (BT-107) and charge (BT-108) totals equal the sum
	// of the document-level allowance (BT-92) and charge (BT-99) amounts.
	//
	// There is no profile in this condition and there must not be one. It used to
	// read `profile != ProfileExtended`, with the stated reason that some EXTENDED
	// producers carry amounts in the totals without an itemizable BG-20/21 entry —
	// a producer habit written as a profile test, which is C45. Measured over the
	// corpus the two do not coincide: of the documents carrying BT-107 or BT-108
	// that no itemized entry accounts for, 2 are Factur-X EXTENDED and 3 are UBL,
	// while 23 EXTENDED documents were exempted that never needed it. So the gate
	// removed the rule for 23 documents it had nothing to protect and left it in
	// place for the 3 it did.
	//
	// It is removed rather than narrowed, because there is nothing left for a
	// narrower document test to say. CEN's binding states BR-CO-11 and BR-CO-12
	// unconditionally; "the entries do not account for the total" is not a reason
	// to stay silent, it is the finding. The two documents the gate was actually
	// protecting are Factur-X EXTENDED invoices whose BT-108 folds in a
	// ram:SpecifiedLogisticsServiceCharge (BT-X-272), which FNFE's own
	// BR-FXEXT-CO-12 adds to the sum and CEN's rule does not know about — and that
	// is not a CEN question at all. It is answered where it belongs, by the
	// authority-parity pass in facturx_restatements.go, which drops a CEN finding
	// on the Factur-X path when FNFE's restatement of that same rule is satisfied.
	if inv.hasTotals {
		var allowSum, chargeSum float64
		if r.stopped() {
			return out
		}
		for _, ac := range inv.allowCharges {
			if v, ok := parseAmount(ac.amount); ok {
				if ac.isCharge {
					chargeSum += v
				} else {
					allowSum += v
				}
			}
		}
		if at, ok := parseAmount(inv.totals.allowanceTotal); ok && math.Abs(round2(allowSum)-at) > 0.005 {
			add("BR-CO-11", fmt.Sprintf("Sum of allowances on document level (BT-107=%.2f) shall equal the sum of Document level allowance amounts (%.2f)", at, allowSum))
		}
		if ct, ok := parseAmount(inv.totals.chargeTotal); ok && math.Abs(round2(chargeSum)-ct) > 0.005 {
			add("BR-CO-12", fmt.Sprintf("Sum of charges on document level (BT-108=%.2f) shall equal the sum of Document level charge amounts (%.2f)", ct, chargeSum))
		}
	}

	// Invoicing period (BG-14) and Invoice line period (BG-26): when present, a
	// start or end date is required (BR-CO-19/20), and a given end must not precede
	// a given start (BR-29/30).
	checkPeriod := func(present, order string, p invoicePeriod) {
		if !p.present {
			return
		}
		// A period group carrying only a VAT point date code (BT-8) is not an
		// invoicing period in the BR-CO-19/20 sense.
		if p.start == "" && p.end == "" && p.desc == "" {
			add(present, "if an invoicing period (BG-14/BG-26) is used, its start or end date shall be present")
		}
		// Only order two dates this package could actually read as dates. A value
		// it cannot parse says nothing about the period's order, and reporting an
		// ordering violation from it would be an accusation built on a guess.
		start, okStart := normDate(p.start)
		end, okEnd := normDate(p.end)
		if okStart && okEnd && end < start {
			add(order, "the invoicing period end date shall not precede its start date")
		}
	}
	checkPeriod("BR-CO-19", "BR-29", inv.period)
	if r.stopped() {
		return out
	}
	for _, li := range inv.lines {
		checkPeriod("BR-CO-20", "BR-30", li.period)
	}

	// BR-CO-16: Amount due for payment (BT-115) = Invoice total with VAT (BT-112)
	// - Paid amount (BT-113) + Rounding amount (BT-114). MINIMUM omits the paid
	// amount from its reduced summation, so its due may differ without a modeled
	// prepaid; exempt it.
	if inv.hasTotals && profile != ProfileMinimum {
		grand, okG := parseAmount(inv.totals.grandTotal)
		due, okD := parseAmount(inv.totals.duePayable)
		paid, _ := parseAmount(inv.totals.paidAmount)
		rounding, _ := parseAmount(inv.totals.payableRounding)
		if okG && okD && math.Abs(round2(grand-paid+rounding)-due) > 0.005 {
			add("BR-CO-16", fmt.Sprintf("Amount due for payment (BT-115=%.2f) shall equal total with VAT (BT-112=%.2f) - paid (BT-113=%.2f) + rounding (BT-114=%.2f)", due, grand, paid, rounding))
		}
	}

	// The syntax bindings. Each reads the tree and each is a no-op on a document
	// in the other syntax.
	//
	// The UBL half is CEN's under either binding, and that is not an omission:
	// Factur-X is a CII format and publishes no UBL binding at all, so there is
	// nothing of its own for a UBL document to be judged by. The CII half is the
	// one the binding argument chooses between.
	out = append(out, validateUBLSyntaxRules(r, p.root)...)
	if r.stopped() {
		return out
	}
	switch binding {
	case ciiBindingFacturX:
		out = append(out, validateFacturXRules(r, p, profile)...)
	default:
		out = append(out, validateCIISyntaxRules(r, p.root)...)
	}
	if r.stopped() {
		return out
	}
	// And the advisory halves, generated from CEN's Schematron and reported as
	// the warnings CEN flags them. They come last because they are the only
	// findings here that no authority rejects a document for: a caller reading
	// Violations in order meets everything blocking before anything advisory.
	//
	// Under Factur-X's binding the CII half of that table is not CEN's to apply
	// either — the 471 advisory CII-SR-*/CII-DT-* rules are the same binding as
	// the 109 fatal ones — so a CII document under a Factur-X profile skips it.
	// advisorySyntaxRules dispatches on the root, so a UBL document reaching here
	// still gets CEN's advisory UBL binding, which is the same reasoning as above.
	if binding != ciiBindingFacturX || p.root == nil || p.root.name != "CrossIndustryInvoice" {
		out = append(out, advisorySyntaxRules(r, p.root)...)
	}

	// Last, and only under Factur-X's binding: drop the CEN findings this
	// document's own authority would not have made. facturXAuthorityParity says
	// what that means and why it is narrower than "suppress the duplicates".
	if binding == ciiBindingFacturX {
		out = facturXAuthorityParity(r, p, profile, out)
	}

	return out
}
