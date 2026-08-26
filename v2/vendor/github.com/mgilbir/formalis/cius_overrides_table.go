package formalis

// Code generated from the CIUS Schematrons and CEN/TC 434's own git history by
// testdata/cius-condition-overrides/gen.py; DO NOT EDIT. Regenerate with
// `make cius-condition-overrides`.
//
// Five of the CIUS this package validates against ship a *copy* of CEN's Schematron
// rather than referencing it, and a copy can be edited. This file is the answer to
// "which CEN conditions did each authority write itself", derived by resolving each
// copy and asking CEN's repository whether it ever published what the copy carries.
// A condition CEN published at some release is a stale vendored copy and not a
// national reading; one CEN never published is the authority's own, and only those
// are overridden. The reasoning is in cius_overrides.go and the derivation in the
// generator's docstring.

// ciusCENCopyVerdicts records, per authority, what its copy of CEN's Schematron
// turned out to be. Derived by testdata/cius-condition-overrides/gen.py from the
// copy and from CEN's own git history, and re-derived by
// TestCIUSCopiesOfCENAreClassifiedFromTheArtefacts on every run.
//
// stale is the count that matters for reading this table: a condition the copy
// carries, CEN's current file does not, and some CEN commit did. It is a measure of
// how old the vendored copy is and not of anything national. own is the count this
// package acts on.
var ciusCENCopyVerdicts = []ciusCENCopyVerdict{
	{
		authority: "CIUS-PT 2.1.1 (UBL)",
		source:    SourceCIUSPT,
		ships:     true,
		shared:    771,
		same:      27,
		stale:     735,
		applied:   true,
		own: map[string]string{
			"BR-23":   "polarity, test",
			"BR-E-02": "test",
			"BR-E-03": "test",
			"BR-E-04": "test",
			"BR-E-10": "context",
			"BR-S-02": "test",
			"BR-S-03": "test",
			"BR-S-04": "test",
			"BR-S-10": "context",
		},
	},
	{
		authority: "CIUS-RO 1.0.9 (UBL)",
		source:    SourceCIUSRO,
		ships:     true,
		shared:    930,
		same:      904,
		stale:     26,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS SI-UBL 2.0.3.2 (UBL)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    929,
		same:      866,
		stale:     63,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS SI-UBL G-account 1.0.2 (UBL)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    745,
		same:      700,
		stale:     45,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS 1.0.3 (CII)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    733,
		same:      628,
		stale:     105,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority:  "UBL.BE v1.31 (UBL)",
		source:     SourceUBLBE,
		ships:      true,
		shared:     250,
		same:       205,
		stale:      38,
		applied:    false,
		notApplied: "the five BR-*-08 conditions sum xs:decimal amounts across a node population reached through the parent axis, and the two BR-CL-* ones sit in <rule> elements whose contexts carry two predicates on one step, which neither this generator's context grammar nor ptDTCtxStep describes. Applying them would mean extending both and re-measuring an arithmetic comparison over the whole corpus; reported here rather than applied half-checked",
		own: map[string]string{
			"BR-AE-08": "test",
			"BR-CL-11": "test",
			"BR-CL-15": "test",
			"BR-G-08":  "test",
			"BR-IC-08": "test",
			"BR-O-08":  "test",
			"BR-Z-08":  "test",
		},
	},
	{
		authority: "SRBDT 1.0.0 (UBL)",
		source:    SourceSRBDT,
		ships:     false,
		shared:    0,
		same:      0,
		stale:     0,
		applied:   true,
		own:       map[string]string{},
	},
}

// ciusCENCopyOmissions records, per authority that ships a copy of CEN's
// Schematron, which CEN release the copy was taken from and which CEN identifiers
// the copy does not carry. Derived by testdata/cius-condition-overrides/gen.py and
// re-derived by TestCIUSCopyOmissionsAreClassifiedFromTheArtefacts on every run.
//
// It is the absence half of the question ciusCENCopyVerdicts answers for differing
// conditions, and it is split by the same discriminator: CEN's own history. An
// identifier the copy lacks that CEN had published when the copy was taken is one
// the authority dropped; one CEN had not published yet is the copy's age.
var ciusCENCopyOmissions = []ciusCENCopyOmission{
	{
		authority:      "CIUS-PT 2.1.1 (UBL)",
		source:         SourceCIUSPT,
		classified:     true,
		release:        "validation-1.1.0",
		releaseThrough: "",
		releaseDate:    "2018-06-26",
		carried:        774,
		differing:      26,
		overlay:        false,
		files: []ciusCENFileOmission{
			{
				cenFile: "EN16931-UBL-codes.sch",
				copied:  false,
				fetched: true,
				dropped: []string{
					"BR-CL-01", "BR-CL-03", "BR-CL-04", "BR-CL-05", "BR-CL-06", "BR-CL-07", "BR-CL-10",
					"BR-CL-11", "BR-CL-13", "BR-CL-14", "BR-CL-15", "BR-CL-16", "BR-CL-17", "BR-CL-18",
					"BR-CL-19", "BR-CL-20", "BR-CL-21", "BR-CL-23", "BR-CL-24",
				},
				postdates: []string{
					"BR-CL-22", "BR-CL-25", "BR-CL-26",
				},
			},
			{
				cenFile: "EN16931-model.sch",
				copied:  true,
				fetched: true,
				dropped: []string{
					"BR-51", "BR-53", "BR-AE-01", "BR-AE-02", "BR-AE-03", "BR-AE-04", "BR-AE-05", "BR-AE-06",
					"BR-AE-07", "BR-AE-08", "BR-AE-09", "BR-AE-10", "BR-CL-08", "BR-CO-05", "BR-CO-06",
					"BR-CO-09", "BR-CO-10", "BR-CO-11", "BR-CO-12", "BR-CO-13", "BR-CO-14", "BR-CO-15",
					"BR-CO-16", "BR-CO-17", "BR-DEC-01", "BR-DEC-02", "BR-DEC-05", "BR-DEC-06", "BR-DEC-09",
					"BR-DEC-10", "BR-DEC-11", "BR-DEC-12", "BR-DEC-13", "BR-DEC-14", "BR-DEC-15", "BR-DEC-16",
					"BR-DEC-17", "BR-DEC-18", "BR-DEC-19", "BR-DEC-20", "BR-DEC-23", "BR-DEC-24", "BR-DEC-25",
					"BR-DEC-27", "BR-DEC-28", "BR-E-08", "BR-G-01", "BR-G-02", "BR-G-03", "BR-G-04",
					"BR-G-05", "BR-G-06", "BR-G-07", "BR-G-08", "BR-G-09", "BR-G-10", "BR-IC-01", "BR-IC-02",
					"BR-IC-03", "BR-IC-04", "BR-IC-05", "BR-IC-06", "BR-IC-07", "BR-IC-08", "BR-IC-09",
					"BR-IC-10", "BR-IC-11", "BR-IC-12", "BR-O-01", "BR-O-02", "BR-O-03", "BR-O-04", "BR-O-05",
					"BR-O-06", "BR-O-07", "BR-O-08", "BR-O-09", "BR-O-10", "BR-O-11", "BR-O-12", "BR-O-13",
					"BR-O-14", "BR-S-08", "BR-S-09", "BR-Z-01", "BR-Z-02", "BR-Z-03", "BR-Z-04", "BR-Z-05",
					"BR-Z-06", "BR-Z-07", "BR-Z-08", "BR-Z-09", "BR-Z-10",
				},
				postdates: []string{
					"BR-AF-01", "BR-AF-02", "BR-AF-03", "BR-AF-04", "BR-AF-05", "BR-AF-06", "BR-AF-07",
					"BR-AF-08", "BR-AF-09", "BR-AF-10", "BR-AG-01", "BR-AG-02", "BR-AG-03", "BR-AG-04",
					"BR-AG-05", "BR-AG-06", "BR-AG-07", "BR-AG-08", "BR-AG-09", "BR-AG-10", "BR-B-01",
					"BR-B-02",
				},
			},
			{
				cenFile: "EN16931-syntax.sch",
				copied:  true,
				fetched: true,
				dropped: []string{
					"UBL-DT-01",
				},
				postdates: []string{
					"UBL-CR-002", "UBL-CR-648", "UBL-CR-649", "UBL-CR-650", "UBL-CR-651", "UBL-CR-652",
					"UBL-CR-653", "UBL-CR-654", "UBL-CR-655", "UBL-CR-656", "UBL-CR-657", "UBL-CR-658",
					"UBL-CR-659", "UBL-CR-660", "UBL-CR-661", "UBL-CR-662", "UBL-CR-663", "UBL-CR-664",
					"UBL-CR-665", "UBL-CR-666", "UBL-CR-667", "UBL-CR-668", "UBL-CR-669", "UBL-CR-670",
					"UBL-CR-671", "UBL-CR-672", "UBL-CR-673", "UBL-CR-674", "UBL-CR-675", "UBL-CR-676",
					"UBL-CR-677", "UBL-CR-678", "UBL-CR-679", "UBL-CR-680", "UBL-CR-681", "UBL-CR-682",
					"UBL-DT-27", "UBL-DT-28", "UBL-SR-42", "UBL-SR-43", "UBL-SR-44", "UBL-SR-45", "UBL-SR-46",
					"UBL-SR-47", "UBL-SR-48", "UBL-SR-49", "UBL-SR-50", "UBL-SR-51", "UBL-SR-52", "UBL-SR-53",
					"UBL-SR-54", "UBL-SR-55", "UBL-SR-56",
				},
			},
		},
	},
	{
		authority:      "CIUS-RO 1.0.9 (UBL)",
		source:         SourceCIUSRO,
		classified:     true,
		release:        "validation-1.3.8",
		releaseThrough: "",
		releaseDate:    "2022-04-08",
		carried:        954,
		differing:      0,
		overlay:        false,
		files: []ciusCENFileOmission{
			{
				cenFile: "EN16931-UBL-codes.sch",
				copied:  false,
				fetched: false,
			},
			{
				cenFile: "EN16931-model.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"BR-AF-01", "BR-AF-02", "BR-AF-03", "BR-AF-04", "BR-AF-05", "BR-AF-06", "BR-AF-07",
					"BR-AF-08", "BR-AF-09", "BR-AF-10", "BR-AG-01", "BR-AG-02", "BR-AG-03", "BR-AG-04",
					"BR-AG-05", "BR-AG-06", "BR-AG-07", "BR-AG-08", "BR-AG-09", "BR-AG-10",
				},
			},
			{
				cenFile: "EN16931-syntax.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"UBL-SR-51", "UBL-SR-52", "UBL-SR-53", "UBL-SR-54", "UBL-SR-55", "UBL-SR-56",
				},
			},
		},
	},
	{
		authority:      "NLCIUS SI-UBL 2.0.3.2 (UBL)",
		source:         SourceNLCIUS,
		classified:     true,
		release:        "validation-1.3.6",
		releaseThrough: "",
		releaseDate:    "2021-05-30",
		carried:        954,
		differing:      1,
		overlay:        false,
		files: []ciusCENFileOmission{
			{
				cenFile: "EN16931-UBL-codes.sch",
				copied:  false,
				fetched: false,
			},
			{
				cenFile: "EN16931-model.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"BR-AF-01", "BR-AF-02", "BR-AF-03", "BR-AF-04", "BR-AF-05", "BR-AF-06", "BR-AF-07",
					"BR-AF-08", "BR-AF-09", "BR-AF-10", "BR-AG-01", "BR-AG-02", "BR-AG-03", "BR-AG-04",
					"BR-AG-05", "BR-AG-06", "BR-AG-07", "BR-AG-08", "BR-AG-09", "BR-AG-10",
				},
			},
			{
				cenFile: "EN16931-syntax.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"UBL-CR-681", "UBL-CR-682", "UBL-SR-51", "UBL-SR-52", "UBL-SR-53", "UBL-SR-54",
					"UBL-SR-55", "UBL-SR-56",
				},
			},
		},
	},
	{
		authority:     "NLCIUS SI-UBL G-account 1.0.2 (UBL)",
		source:        SourceNLCIUS,
		classified:    false,
		notClassified: "an overlay on NLCIUS SI-UBL 2.0.3.2: it replaces CEN's abstract syntax file and <include>s the whole of SI-UBL 2.0 for the rest, so what it omits is what that entry omits. Recorded there",
	},
	{
		authority:      "NLCIUS 1.0.3 (CII)",
		source:         SourceNLCIUS,
		classified:     true,
		release:        "validation-1.3.1",
		releaseThrough: "validation-1.3.4",
		releaseDate:    "2020-02-25",
		carried:        735,
		differing:      0,
		overlay:        false,
		files: []ciusCENFileOmission{
			{
				cenFile: "EN16931-CII-codes.sch",
				copied:  false,
				fetched: false,
			},
			{
				cenFile: "EN16931-CII-model.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"BR-B-01", "BR-B-02",
				},
			},
			{
				cenFile: "EN16931-CII-syntax.sch",
				copied:  true,
				fetched: true,
				postdates: []string{
					"CII-DT-097", "CII-DT-101", "CII-DT-102", "CII-DT-103", "CII-DT-104", "CII-SR-452",
					"CII-SR-453", "CII-SR-454", "CII-SR-455", "CII-SR-456", "CII-SR-457", "CII-SR-458",
					"CII-SR-459", "CII-SR-460", "CII-SR-461", "CII-SR-462", "CII-SR-463", "CII-SR-464",
					"CII-SR-465", "CII-SR-466", "CII-SR-467", "CII-SR-468", "CII-SR-469", "CII-SR-470",
					"CII-SR-471", "CII-SR-472", "CII-SR-473", "CII-SR-474", "CII-SR-475", "CII-SR-476",
					"CII-SR-477", "CII-SR-478", "CII-SR-479", "CII-SR-480", "CII-SR-481", "CII-SR-482",
					"CII-SR-483", "CII-SR-484", "CII-SR-485", "CII-SR-486", "CII-SR-487", "CII-SR-488",
					"CII-SR-489", "CII-SR-490", "CII-SR-491", "CII-SR-492", "CII-SR-493", "CII-SR-494",
				},
			},
		},
	},
	{
		authority:     "UBL.BE v1.31 (UBL)",
		source:        SourceUBLBE,
		classified:    false,
		notClassified: "its file re-cases 671 of CEN's identifiers (UBL-CR-001 as ubl-CR-001 and so on), so an omission set computed on exact identifiers would report a family the file carries in full as dropped. Classifying it means deciding whether a re-cased identifier is CEN's, which is a question about this authority's identifier namespace and not about absence",
	},
	{
		authority:     "SRBDT 1.0.0 (UBL)",
		source:        SourceSRBDT,
		classified:    false,
		notClassified: "ships no copy of CEN's files, so it omits all of them; EN16931-UBL-srbdt-validation.sch says so itself, in a comment, and ciusCENCopyVerdicts records the same fact as ships: false",
	},
}

// ptOverrideModelPattern is CIUS-PT 2.1.1 (UBL)'s copy of CEN's pattern, carrying the 9
// assertions whose condition CIUS-PT 2.1.1 (UBL) wrote itself. Every <rule> of the pattern is here
// in the authority's order, because under ISO Schematron a node goes to the first
// rule whose context matches it and a rule dropped for carrying nothing would hand
// its nodes to a rule below it. The rules that carry no assertion are here for
// their contexts alone.
var ptOverrideModelPattern = ptDTPattern{
	name: "model",
	rules: []ptDTRuleSrc{
		{
			context: "cac:AdditionalDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AdditionalDocumentReference", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:ContractDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ContractDocumentReference", ""}}}},
		},
		{
			context: "cac:Delivery",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"Delivery", ""}}}},
		},
		{
			context: "cac:Delivery/cac:DeliveryLocation/cac:Address",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}}}},
		},
		{
			context: "cac:DespatchDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"DespatchDocumentReference", ""}}}},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
		},
		{
			context: "cac:LegalMonetaryTotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"LegalMonetaryTotal", ""}}}},
		},
		{
			context: "//ubl:Invoice | //cn:CreditNote",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{}}, {"CreditNote", []ptDTCtxStep{}}},
			asserts: []ptDTAssertSrc{
				{"BR-S-02", "assert", "(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-S-03", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-S-04", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-02", "assert", "(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-03", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-04", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
			},
		},
		{
			context: "cac:InvoiceLine | cac:CreditNoteLine",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-23", "report", "(exists(cbc:InvoicedQuantity) and not(cbc:InvoicedQuantity/@unitCode)) or (exists(cbc:CreditedQuantity) and not(cbc:CreditedQuantity/@unitCode))", "An Invoice line (BG-25) shall have an Invoiced quantity unit of measure code (BT-130)."},
			},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:InvoicePeriod | cac:CreditNoteLine/cac:InvoicePeriod",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"InvoicePeriod", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"InvoicePeriod", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Price | cac:CreditNoteLine/cac:Price",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}}}},
		},
		{
			context: "cac:InvoicePeriod",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoicePeriod", ""}}}},
		},
		{
			context: "//cac:AdditionalItemProperty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AdditionalItemProperty", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode | cac:CreditNoteLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:StandardItemIdentification/cbc:ID | cac:CreditNoteLine/cac:Item/cac:StandardItemIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}},
		},
		{
			context: "cac:OrderReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"OrderReference", ""}}}},
		},
		{
			context: "cac:OriginatorDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"OriginatorDocumentReference", ""}}}},
		},
		{
			context: "cac:PayeeParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PayeeParty", ""}}}},
		},
		{
			context: "cac:PaymentMeans[cbc:PaymentMeansCode='30' or cbc:PaymentMeansCode='58']/cac:PayeeFinancialAccount",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentMeans", "cbc:PaymentMeansCode='30' or cbc:PaymentMeansCode='58'"}, {"PayeeFinancialAccount", ""}}}},
		},
		{
			context: "cac:PaymentMeans",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentMeans", ""}}}},
		},
		{
			context: "cac:PaymentTerms",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentTerms", ""}}}},
		},
		{
			context: "cac:BillingReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"BillingReference", ""}}}},
		},
		{
			context: "cac:ProjectReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ProjectReference", ""}}}},
		},
		{
			context: "cac:ReceiptDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ReceiptDocumentReference", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:TaxRepresentativeParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxRepresentativeParty", ""}}}},
		},
		{
			context: "cac:TaxRepresentativeParty/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'AA'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'AA'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-S-10", "assert", "not(cbc:TaxExemptionReason) and not(cbc:TaxExemptionReasonCode)", "A VATBReakdown (BG-23) with VAT Category code (BT-118) \"Standard rate\" shall not have a VAT exemption reason code (BT-121) or VAT exemption reason text (BT-120)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR']/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-E-10", "assert", "exists(cbc:TaxExemptionReason) or exists(cbc:TaxExemptionReasonCode)", "A VATBReakdown (BG-23) with VAT Category code (BT-118) \"Exempt from VAT\" shall have a VAT exemption reason code (BT-121) or a VAT exemption reason text (BT-120)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
	},
}

var ptConditionOverrides = &ciusOverrides{
	authority: SourceCIUSPT,
	syntax:    "UBL",
	rules: map[string]Severity{
		"BR-23":   SeverityFatal,
		"BR-E-02": SeverityFatal,
		"BR-E-03": SeverityFatal,
		"BR-E-04": SeverityFatal,
		"BR-E-10": SeverityFatal,
		"BR-S-02": SeverityFatal,
		"BR-S-03": SeverityFatal,
		"BR-S-04": SeverityFatal,
		"BR-S-10": SeverityFatal,
	},
	patterns: []ptDTPattern{ptOverrideModelPattern},
}
