package formalis

// Code generated from ANAF's CIUS-RO 1.0.9 Schematron by
// testdata/cius-ro-rules/gen.py; DO NOT EDIT. Regenerate with `make cius-ro-rules`.
//
// These are the 90 fatal assertions of the mechanical half of
// cius-ro/RO16931-rules.sch: the BR-RO-L* length limits, the BR-DEC-RO-* decimal
// limits, the BR-RO-DT* date formats and the BR-RO-A* occurrence limits. Each
// assertion's test is ANAF's own XPath, whitespace-normalised and otherwise
// verbatim, so this file reads against the Schematron line by line;
// cius_pt_datatype.go is the parser it is written in, cius_pt_datatype_eval.go the
// evaluator, and cius_ro_rules_test.go re-derives these tables from the Schematron
// and fails if the committed ones have drifted.
//
// The 25 BR-RO-NNN business rules that share these <rule> elements are not here:
// they need judgement rather than transcription and are written by hand in
// cius_ro.go. Neither shadows the other — they are assertions of the same rules.

// roRulesPattern is ANAF's ROmodel pattern, mechanical half:
// 29 rules carrying 90 fatal assertions, in pattern order. A rule with no
// assertion is here for its context alone, which under ISO Schematron claims its nodes
// away from every rule below it — and three of these rules are here for no other reason.
var roRulesPattern = ptDTPattern{
	name: "ROmodel",
	rules: []ptDTRuleSrc{
		{
			context: "cbc:IssueDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"IssueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT001", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-2, BT-27) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:TaxPointDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"TaxPointDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT002", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-7) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:DueDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"DueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT003", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-9) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:PaymentDueDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"PaymentDueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT003_CN", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-9) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:ActualDeliveryDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"ActualDeliveryDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT004", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-73, BT-134) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:StartDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"StartDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT005", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-73, BT-134) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:EndDate",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"EndDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-DT006", "assert", "string-length(text()) = 10 and (string(.) castable as xs:date)", "A date (BT-74, BT-135) MUST be formatted YYYY-MM-DD."},
			},
		},
		{
			context: "cbc:PriceAmount",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"PriceAmount", ""}}}},
		},
		{
			context: "cbc:InvoiceTypeCode",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoiceTypeCode", ""}}}},
		},
		{
			context: "cbc:CreditNoteTypeCode",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"CreditNoteTypeCode", ""}}}},
		},
		{
			context: "/ubl:Invoice | /cn:CreditNote",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{}}, {"CreditNote", []ptDTCtxStep{}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-A020", "assert", "count(cbc:Note) <= 20", "The allowed maximum number of occurences of Invoice note (BG-1) is 20."},
				{"BR-RO-L0201", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:PostalZone)) <=20", "The allowed maximum number of characters for the Seller post code (BT-38) is 20."},
				{"BR-RO-L0202", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:PostalZone)) <=20", "The allowed maximum number of characters for the Buyer post code (BT-53) is 20."},
				{"BR-RO-L0203", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PostalAddress/cbc:PostalZone)) <= 20", "The allowed maximum number of characters for the Tax representative post code (BT-67) is 20."},
				{"BR-RO-L0204", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:PostalZone)) <= 20", "The allowed maximum number of characters for the Deliver to post code (BT-78) is 20."},
				{"BR-RO-L155", "assert", "string-length(normalize-space(cbc:ID)) <= 200", "The allowed maximum number of characters for the Invoice number (BT-1) is 200."},
				{"BR-RO-L0302", "assert", "string-length(normalize-space(cac:ContractDocumentReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Contract reference(BT-12) is 200."},
				{"BR-RO-L0303", "assert", "string-length(normalize-space(cac:OrderReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Purchase order reference(BT-13) is 200."},
				{"BR-RO-L0304", "assert", "string-length(normalize-space(cac:OrderReference/cbc:SalesOrderID)) <= 200", "The allowed maximum number of characters for the Sales order reference (BT-14) is 200."},
				{"BR-RO-L0305", "assert", "string-length(normalize-space(cac:ReceiptDocumentReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Receiving advice reference (BT-15) is 200."},
				{"BR-RO-L0306", "assert", "string-length(normalize-space(cac:DespatchDocumentReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Despatch advice reference (BT-16) is 200."},
				{"BR-RO-L0307", "assert", "string-length(normalize-space(cac:OriginatorDocumentReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Tender or lot reference (BT-17) is 200."},
				{"BR-RO-L0501", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName)) <= 50", "The allowed maximum number of characters for the Seller city (BT-37) is 50."},
				{"BR-RO-L0502", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName)) <= 50", "The allowed maximum number of characters for the Buyer city (BT-52) is 50."},
				{"BR-RO-L0503", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PostalAddress/cbc:CityName)) <= 50", "The allowed maximum number of characters for the Tax representative city (BT-66) is 50."},
				{"BR-RO-L0504", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:CityName)) <= 50", "The allowed maximum number of characters for the Deliver to city (BT-77) is 50."},
				{"BR-RO-L1001", "assert", "string-length(normalize-space(cbc:AccountingCost)) <= 100", "The allowed maximum number of characters for the Buyer accounting reference (BT-19) is 100."},
				{"BR-RO-L1002", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName)) <= 100", "The allowed maximum number of characters for the Seller address line 2 (BT-36) is 100."},
				{"BR-RO-L1003", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line)) <= 100", "The allowed maximum number of characters for the Seller address line 3 (BT-162) is 100."},
				{"BR-RO-L1004", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Name)) <= 100", "The allowed maximum number of characters for the Seller contact point (BT-41) is 100."},
				{"BR-RO-L1005", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Telephone)) <= 100", "The allowed maximum number of characters for the Seller contact telephone number (BT-42) is 100."},
				{"BR-RO-L1006", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:ElectronicMail)) <= 100", "The allowed maximum number of characters for the Seller contact email address (BT-43) is 100."},
				{"BR-RO-L1007", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName)) <= 100", "The allowed maximum number of characters for the Buyer address line 2 (BT-51) is 100."},
				{"BR-RO-L1008", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line)) <= 100", "The allowed maximum number of characters for the Buyer address line 3 (BT-163) is 100."},
				{"BR-RO-L1009", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Name)) <= 100", "The allowed maximum number of characters for the Buyer contact point (BT-56) is 100."},
				{"BR-RO-L1010", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Telephone)) <= 100", "The allowed maximum number of characters for the Buyer contact telephone number (BT-57) is 100."},
				{"BR-RO-L1011", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:ElectronicMail)) <= 100", "The allowed maximum number of characters for the Buyer contact email address (BT-58) is 100."},
				{"BR-RO-L1012", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PostalAddress/cbc:AdditionalStreetName)) <= 100", "The allowed maximum number of characters for the Tax representative address line 2 (BT-65) is 100."},
				{"BR-RO-L1013", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PostalAddress/cac:AddressLine/cbc:Line)) <= 100", "The allowed maximum number of characters for the Tax representative address line 3 (BT-164) is 100."},
				{"BR-RO-L1014", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:AdditionalStreetName)) <= 100", "The allowed maximum number of characters for the Deliver to address line 2 (BT-76) is 100."},
				{"BR-RO-L1015", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryLocation/cac:Address/cac:AddressLine/cbc:Line)) <= 100", "The allowed maximum number of characters for the Deliver to address line 3 (BT-165) is 100."},
				{"BR-RO-L151", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName)) <= 150", "The allowed maximum number of characters for the Seller address line 1 (BT-35) is 150."},
				{"BR-RO-L152", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:StreetName)) <= 150", "The allowed maximum number of characters for the Buyer address line 1 (BT-50) is 150."},
				{"BR-RO-L153", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PostalAddress/cbc:StreetName)) <= 150", "The allowed maximum number of characters for the Tax representative address line 1 (BT-64) is 150."},
				{"BR-RO-L154", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:StreetName)) <= 150", "The allowed maximum number of characters for the Deliver to address line 1(BT-75) is 150."},
				{"BR-RO-L201", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName)) <= 200", "The allowed maximum number of characters for the Seller name (BT-27) is 200."},
				{"BR-RO-L202", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name)) <= 200", "The allowed maximum number of characters for the Seller trading name (BT-28) is 200."},
				{"BR-RO-L203", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName)) <= 200", "The allowed maximum number of characters for the Buyer name (BT-44) is 200."},
				{"BR-RO-L204", "assert", "string-length(normalize-space(cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name)) <= 200", "The allowed maximum number of characters for the Buyer trading name (BT-45) is 200."},
				{"BR-RO-L205", "assert", "string-length(normalize-space(cac:PayeeParty/cac:PartyName/cbc:Name)) <= 200", "The allowed maximum number of characters for the Payee name (BT-59) is 200."},
				{"BR-RO-L206", "assert", "string-length(normalize-space(cac:TaxRepresentativeParty/cac:PartyName/cbc:Name)) <= 200", "The allowed maximum number of characters for the Seller tax representative name (BT-62) is 200."},
				{"BR-RO-L207", "assert", "string-length(normalize-space(cac:Delivery/cac:DeliveryParty/cac:PartyName/cbc:Name)) <= 200", "The allowed maximum number of characters for the Deliver to party name (BT-70) is 200."},
				{"BR-RO-L301", "assert", "string-length(normalize-space(cac:PaymentTerms/cbc:Note)) <= 300", "The allowed maximum number of characters for the Payment terms (BT-20) is 300."},
				{"BR-RO-L1000", "assert", "string-length(normalize-space(cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm)) <= 1000", "The allowed maximum number of characters for the Seller additional legal (BT-33) is 1000."},
			},
		},
		{
			context: "/ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress | /ubl:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "//cac:PaymentMeans",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"PaymentMeans", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-L208", "assert", "string-length(normalize-space(cac:PayeeFinancialAccount/cbc:Name)) <= 200", "The allowed maximum number of characters for the Payment account name (BT-85) is 200."},
				{"BR-RO-L209", "assert", "string-length(normalize-space(cac:CardAccount/cbc:HolderName)) <= 200", "The allowed maximum number of characters for the Payment card holder name(BT-88) is 200."},
				{"BR-RO-L1016", "assert", "string-length(normalize-space(cbc:PaymentMeansCode/@name)) <= 100", "The allowed maximum number of characters for the Payment means text (BT-82) is 100."},
				{"BR-RO-L140", "assert", "string-length(normalize-space(cbc:PaymentID)) <= 140", "The allowed maximum number of characters for the Remittance information (BT-83) is 140."},
			},
		},
		{
			context: "/ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address | /ubl:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}}}},
		},
		{
			context: "cac:InvoicePeriod/cbc:DescriptionCode",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoicePeriod", ""}, {"DescriptionCode", ""}}}},
		},
		{
			context: "cac:InvoiceLine | cac:CreditNoteLine",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoiceLine", ""}}}, {"//", []ptDTCtxStep{{"CreditNoteLine", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-L1024", "assert", "string-length(normalize-space(cac:Item/cbc:Name)) <= 100", "The allowed maximum number of characters for the Item name (BT-153) is 100."},
				{"BR-RO-L1021", "assert", "string-length(normalize-space(cbc:AccountingCost)) <= 100", "The allowed maximum number of characters for the Invoice line Buyer accounting reference (BT-133) is 100."},
				{"BR-RO-L212", "assert", "string-length(normalize-space(cac:Item/cbc:Description)) <= 200", "The allowed maximum number of characters for the Item description (BT-154) is 200."},
				{"BR-RO-L303", "assert", "string-length(normalize-space(cbc:Note)) <= 300", "The allowed maximum number of characters for the Invoice line note (BT-127) is 300."},
			},
		},
		{
			context: "//cac:Item/cac:AdditionalItemProperty",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"Item", ""}, {"AdditionalItemProperty", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-L0505", "assert", "string-length(normalize-space(cbc:Name)) <= 50", "The allowed maximum number of characters for the Item attribute name (BT-160) is 50."},
				{"BR-RO-L1025", "assert", "string-length(normalize-space(cbc:Value)) <= 100", "The allowed maximum number of characters for the Item attribute value (BT-161) is 100."},
			},
		},
		{
			context: "/ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = false()] | /ubl:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = false()]",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = false()"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-01", "assert", "string-length(substring-after(cbc:Amount,'.'))<=2", "The allowed maximum number of decimals for the Document level allowance amount(BT-92) is 2."},
				{"BR-DEC-RO-02", "assert", "string-length(substring-after(cbc:BaseAmount,'.'))<=2", "The allowed maximum number of decimals for the Document level allowance base amount(BT-93) is 2."},
				{"BR-RO-L1017", "assert", "string-length(normalize-space(cbc:AllowanceChargeReasonCode)) <= 100", "The allowed maximum number of characters for the Document level allowance reason (BT-97) is 100."},
			},
		},
		{
			context: "/ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = true()] | /ubl:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = true()]",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = true()"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-05", "assert", "string-length(substring-after(cbc:Amount,'.'))<=2", "The allowed maximum number of decimals for the Document level charge amount (BT-99) is 2."},
				{"BR-DEC-RO-06", "assert", "string-length(substring-after(cbc:BaseAmount,'.'))<=2", "The allowed maximum number of decimals for the Document level charge base amount (BT-100) is 2."},
				{"BR-RO-L1018", "assert", "string-length(normalize-space(cbc:AllowanceChargeReasonCode)) <= 100", "The allowed maximum number of characters for the Document level charge reason (BT-104) is 100."},
			},
		},
		{
			context: "cac:LegalMonetaryTotal",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"LegalMonetaryTotal", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-09", "assert", "string-length(substring-after(cbc:LineExtensionAmount,'.'))<=2", "The allowed maximum number of decimals for the Sum of Invoice line net amount (BT-106) is 2."},
				{"BR-DEC-RO-10", "assert", "string-length(substring-after(cbc:AllowanceTotalAmount,'.'))<=2", "The allowed maximum number of decimals for the Sum of allowances on document level(BT-107) is 2."},
				{"BR-DEC-RO-11", "assert", "string-length(substring-after(cbc:ChargeTotalAmount,'.'))<=2", "The allowed maximum number of decimals for the Sum of charges on document level(BT-108) is 2."},
				{"BR-DEC-RO-12", "assert", "string-length(substring-after(cbc:TaxExclusiveAmount,'.'))<=2", "The allowed maximum number of decimals for the Invoice total amount without VAT (BT-109) is 2."},
				{"BR-DEC-RO-14", "assert", "string-length(substring-after(cbc:TaxInclusiveAmount,'.'))<=2", "The allowed maximum number of decimals for the Invoice total amount with VAT (BT-112) is 2."},
				{"BR-DEC-RO-16", "assert", "string-length(substring-after(cbc:PrepaidAmount,'.'))<=2", "The allowed maximum number of decimals for the Paid amount(BT-113) is 2."},
				{"BR-DEC-RO-17", "assert", "string-length(substring-after(cbc:PayableRoundingAmount,'.'))<=2", "The allowed maximum number of decimals for the Rounding amount(BT-114) is 2."},
				{"BR-DEC-RO-18", "assert", "string-length(substring-after(cbc:PayableAmount,'.'))<=2", "The allowed maximum number of decimals for the Amount due for payment (BT-115) is 2."},
			},
		},
		{
			context: "/ubl:Invoice | cac:CreditNote",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-1009", "assert", "string-length(substring-after(cbc:TaxableAmount,'.'))<=2", "The allowed maximum number of decimals for the VAT category taxable amount (BT-116) is 2."},
				{"BR-DEC-RO-1010", "assert", "string-length(substring-after(cbc:TaxAmount,'.'))<=2", "The allowed maximum number of decimals for the VAT category tax amount (BT-117) is 2."},
			},
		},
		{
			context: "cac:InvoiceLine | cac:CreditNoteLine",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoiceLine", ""}}}, {"//", []ptDTCtxStep{{"CreditNoteLine", ""}}}},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = false()] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = false()]",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = false()"}}}, {"//", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = false()"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-24", "assert", "string-length(substring-after(cbc:Amount,'.'))<=2", "The allowed maximum number of decimals for the Invoice line allowance amount (BT-136) is 2."},
				{"BR-DEC-RO-25", "assert", "string-length(substring-after(cbc:BaseAmount,'.'))<=2", "The allowed maximum number of decimals for the Invoice line allowance base amount (BT-137) is 2."},
				{"BR-RO-L1022", "assert", "string-length(normalize-space(cbc:AllowanceChargeReason)) <= 100", "The allowed maximum number of characters for the Invoice line allowance reason (BT-139) is 100."},
			},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = true()] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = true()]",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = true()"}}}, {"//", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = true()"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-DEC-RO-27", "assert", "string-length(substring-after(cbc:Amount,'.'))<=2", "The allowed maximum number of decimals for the Invoice line charge amount (BT-141) is 2."},
				{"BR-DEC-RO-28", "assert", "string-length(substring-after(cbc:BaseAmount,'.'))<=2", "The allowed maximum number of decimals for the Invoice line charge base amount (BT-142) is 2."},
				{"BR-RO-L1023", "assert", "string-length(normalize-space(cbc:AllowanceChargeReason)) <= 100", "The allowed maximum number of characters for the Invoice line charge reason (BT-144) is 100."},
			},
		},
		{
			context: "cbc:Note",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"Note", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-L302", "assert", "string-length(normalize-space(.)) <= 300", "The allowed maximum number of characters for the Invoice note (BT-22) is 300."},
			},
		},
		{
			context: "//cac:AdditionalDocumentReference",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"AdditionalDocumentReference", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-L0308", "assert", "string-length(normalize-space(cbc:ID)) <= 200", "The allowed maximum number of characters for the Invoiced object identifier (BT-18) and the Supporting document reference(BT-122)is 200."},
				{"BR-RO-L1020", "assert", "string-length(normalize-space(cbc:DocumentDescription)) <= 100", "The allowed maximum number of characters for the Supporting document description (BT-123) is 100."},
				{"BR-RO-L210", "assert", "string-length(normalize-space(cac:Attachment/cac:ExternalReference/cbc:URI)) <= 200", "The allowed maximum number of characters for the External document location (BT-124) is 200."},
				{"BR-RO-L211", "assert", "string-length(normalize-space(cac:Attachment/cbc:EmbeddedDocumentBinaryObject/@filename)) <= 200", "The allowed maximum number of characters for the Attached document Filename (BT-125-2) is 200."},
			},
		},
		{
			context: "//cac:BillingReference",
			paths:   []ptDTCtxPath{{"//", []ptDTCtxStep{{"BillingReference", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-RO-A500", "assert", "count(cac:InvoiceDocumentReference) <= 500", "The allowed maximum number of occurences of Preceding invoice reference (BG-3) is 500."},
				{"BR-RO-L156", "assert", "string-length(normalize-space(cac:InvoiceDocumentReference/cbc:ID)) <= 200", "The allowed maximum number of characters for the Preceding Invoice number (BT-25) is 200."},
			},
		},
		{
			context: "/ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal | /ubl:CreditNote/cac:TaxTotal/cac:TaxSubtotal",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}}}},
		},
	},
}

// roUnevaluableAsserts are the assertions ANAF publishes that no conforming
// Schematron processor can report, with the reason derived from the artefact rather
// than asserted. They are RuleFamily.Unevaluable entries in Coverage(SourceCIUSRO),
// and cius_ro_rules_test.go checks the two agree.
var roUnevaluableAsserts = map[string]string{
	"BR-DEC-RO-13": "unreachable: every node the rule context /ubl:Invoice | cac:CreditNote selects is claimed by the earlier rule /ubl:Invoice | /cn:CreditNote, and under ISO Schematron a node goes to the first matching rule only",
	"BR-DEC-RO-15": "unreachable: every node the rule context /ubl:Invoice | cac:CreditNote selects is claimed by the earlier rule /ubl:Invoice | /cn:CreditNote, and under ISO Schematron a node goes to the first matching rule only",
	"BR-DEC-RO-23": "unreachable: every node the rule context cac:InvoiceLine | cac:CreditNoteLine selects is claimed by the earlier rule cac:InvoiceLine | cac:CreditNoteLine, and under ISO Schematron a node goes to the first matching rule only",
	"BR-RO-A051":   "count(.) counts the context node, so count(.) <= 50 is true for every document; the rule was written for a document-wide count and cannot fail as bound",
	"BR-RO-A052":   "count(.) counts the context node, so count(.) <= 50 is true for every document; the rule was written for a document-wide count and cannot fail as bound",
	"BR-RO-L1019":  "unreachable: every node the rule context /ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal | /ubl:CreditNote/cac:TaxTotal/cac:TaxSubtotal selects is claimed by the earlier rule cac:TaxTotal/cac:TaxSubtotal, and under ISO Schematron a node goes to the first matching rule only",
}
