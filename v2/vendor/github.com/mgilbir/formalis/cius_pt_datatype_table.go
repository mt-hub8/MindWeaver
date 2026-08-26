package formalis

// Code generated from AT/eSPap's CIUS-PT 2.1.1 Schematron by
// testdata/cius-pt-rules/gen.py; DO NOT EDIT. Regenerate with `make cius-pt-rules`.
//
// These are the 291 fatal DT-CIUS-PT-* assertions the Portuguese profile publishes:
// 269 in datatype/urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-datatype.sch and 22 in the
// abstract condition pattern, resolved through its UBL binding the way a Schematron
// processor resolves it. Each assertion's test is AT's own XPath,
// whitespace-normalised and otherwise verbatim, so this file can be read against the
// Schematron line by line; cius_pt_datatype.go is the parser,
// cius_pt_datatype_eval.go the evaluator, and cius_pt_datatype_test.go re-derives
// these tables from the Schematron and fails if the committed ones have drifted.
//
// The BR-* assertions that share the condition pattern's <rule> elements are not
// here: they need judgement rather than transcription and are written by hand in
// cius_pt_rules.go.

// ptDatatypePattern is AT/eSPap's concrete UBL-datatype pattern:
// 152 rules carrying 269 fatal DT-CIUS-PT-* assertions, in pattern order. A rule
// with no assertion is here for its context alone, which under ISO Schematron claims its
// nodes away from every rule below it.
var ptDatatypePattern = ptDTPattern{
	name: "UBL-datatype",
	lets: []ptDTLetSrc{
		{"bt05", "//ubl:Invoice/cbc:DocumentCurrencyCode | //cn:CreditNote/cbc:DocumentCurrencyCode"},
		{"bt06", "//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode"},
	},
	rules: []ptDTRuleSrc{
		{
			context: "//ubl:Invoice/cbc:ID | //cn:CreditNote/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-001", "assert", "matches(.,'^(.{1,50})$')", "The BT-1 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:IssueDate | //cn:CreditNote/cbc:IssueDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"IssueDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"IssueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-002", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-2 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:InvoiceTypeCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceTypeCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-003_i", "assert", "(matches(.,'^([A-Z0-9]{2,3})$') and contains('380,383,386,389,FT,FS,FR,ND,RP,RE,CS,LD,RA,', concat(normalize-space(.), ',')))", "The BT-3 only allows the following values: 380, 383, 386, 389, FT, FS, FR, ND, RP, RE, CS, LD, RA."},
			},
		},
		{
			context: "//cn:CreditNote/cbc:CreditNoteTypeCode",
			paths:   []ptDTCtxPath{{"CreditNote", []ptDTCtxStep{{"CreditNoteTypeCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-003_cn", "assert", "(matches(.,'^([A-Z0-9]{2,3})$') and contains('381,NC,', concat(normalize-space(.), ',')))", "The BT-3 only allows the following values: 381, NC."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:DocumentCurrencyCode | //cn:CreditNote/cbc:DocumentCurrencyCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"DocumentCurrencyCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"DocumentCurrencyCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-151", "assert", "(not(contains(normalize-space(.), ' ')) and contains(' AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYR BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HRK HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRO MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLL SOS SRD SSP STD SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UZS VEF VND VUV WST XAF XAG XAU XBA XBB XBC XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWL ', concat(' ', normalize-space(.), ' ')))", "The BT-5 must be coded using ISO code list 4217 alpha-3."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxCurrencyCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxCurrencyCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-152", "assert", "(not(contains(normalize-space(.), ' ')) and contains(' AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYR BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HRK HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRO MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLL SOS SRD SSP STD SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UZS VEF VND VUV WST XAF XAG XAU XBA XBB XBC XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWL ', concat(' ', normalize-space(.), ' ')))", "the BT-6 must be coded using ISO code list 4217 alpha-3."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:TaxPointDate | //cn:CreditNote/cbc:TaxPointDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxPointDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxPointDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-006", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-7 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoicePeriod/cbc:DescriptionCode | //cn:CreditNote/cac:InvoicePeriod/cbc:DescriptionCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoicePeriod", ""}, {"DescriptionCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"InvoicePeriod", ""}, {"DescriptionCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-007", "assert", "(matches(.,'^(\\d{1,3})$') and contains('3,35,432,', concat(normalize-space(.), ',')))", "The BT-8 only allows the following values: 3, 35, 432."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:DueDate | //cn:CreditNote/cac:PaymentMeans/cbc:PaymentDueDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"DueDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentDueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-008", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-9 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:BuyerReference | //cn:CreditNote/cbc:BuyerReference",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"BuyerReference", ""}}}, {"CreditNote", []ptDTCtxStep{{"BuyerReference", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-009", "assert", "matches(.,'^(.{1,50})$')", "The BT-10 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:ProjectReference/cbc:ID | //cn:CreditNote/cac:ProjectReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"ProjectReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"ProjectReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-010", "assert", "matches(.,'^(.{1,50})$')", "The BT-11 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:ContractDocumentReference/cbc:ID | //cn:CreditNote/cac:ContractDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"ContractDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"ContractDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-011", "assert", "matches(.,'^(.{1,20})$')", "The BT-12 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:OrderReference/cbc:ID | //cn:CreditNote/cac:OrderReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"OrderReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"OrderReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-012", "assert", "matches(.,'^(.{1,20})$')", "The BT-13 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:OrderReference/cbc:SalesOrderID | //cn:CreditNote/cac:OrderReference/cbc:SalesOrderID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"OrderReference", ""}, {"SalesOrderID", ""}}}, {"CreditNote", []ptDTCtxStep{{"OrderReference", ""}, {"SalesOrderID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-013", "assert", "matches(.,'^(.{1,20})$')", "The BT-14 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:ReceiptDocumentReference/cbc:ID | //cn:CreditNote/cac:ReceiptDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"ReceiptDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"ReceiptDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-014", "assert", "matches(.,'^(.{1,20})$')", "The BT-15 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:DespatchDocumentReference/cbc:ID | //cn:CreditNote/cac:DespatchDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"DespatchDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"DespatchDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-015", "assert", "matches(.,'^(.{1,20})$')", "The BT-16 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:OriginatorDocumentReference/cbc:ID | //cn:CreditNote/cac:OriginatorDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"OriginatorDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"OriginatorDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-016", "assert", "matches(.,'^(.{1,20})$')", "The BT-17 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AdditionalDocumentReference/cbc:ID | //cn:CreditNote/cac:AdditionalDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-017.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-18 and BT-122 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-017.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' AAA AAB AAC AAD AAE AAF AAG AAH AAI AAJ AAK AAL AAM AAN AAO AAP AAQ AAR AAS AAT AAU AAV AAW AAX AAY AAZ ABA ABB ABC ABD ABE ABF ABG ABH ABI ABJ ABK ABL ABM ABN ABO ABP ABQ ABR ABS ABT ABU ABV ABW ABX ABY ABZ AC ACA ACB ACC ACD ACE ACF ACG ACH ACI ACJ ACK ACL ACN ACO ACP ACQ ACR ACT ACU ACV ACW ACX ACY ACZ ADA ADB ADC ADD ADE ADF ADG ADI ADJ ADK ADL ADM ADN ADO ADP ADQ ADT ADU ADV ADW ADX ADY ADZ AE AEA AEB AEC AED AEE AEF AEG AEH AEI AEJ AEK AEL AEM AEN AEO AEP AEQ AER AES AET AEU AEV AEW AEX AEY AEZ AF AFA AFB AFC AFD AFE AFF AFG AFH AFI AFJ AFK AFL AFM AFN AFO AFP AFQ AFR AFS AFT AFU AFV AFW AFX AFY AFZ AGA AGB AGC AGD AGE AGF AGG AGH AGI AGJ AGK AGL AGM AGN AGO AGP AGQ AGR AGS AGT AGU AGV AGW AGX AGY AGZ AHA AHB AHC AHD AHE AHF AHG AHH AHI AHJ AHK AHL AHM AHN AHO AHP AHQ AHR AHS AHT AHU AHV AHX AHY AHZ AIA AIB AIC AID AIE AIF AIG AIH AII AIJ AIK AIL AIM AIN AIO AIP AIQ AIR AIS AIT AIU AIV AIW AIX AIY AIZ AJA AJB AJC AJD AJE AJF AJG AJH AJI AJJ AJK AJL AJM AJN AJO AJP AJQ AJR AJS AJT AJU AJV AJW AJX AJY AJZ AKA AKB AKC AKD AKE AKF AKG AKH AKI AKJ AKK AKL AKM AKN AKO AKP AKQ AKR AKS AKT AKU AKV AKW AKX AKY AKZ ALA ALB ALC ALD ALE ALF ALG ALH ALI ALJ ALK ALL ALM ALN ALO ALP ALQ ALR ALS ALT ALU ALV ALW ALX ALY ALZ AMA AMB AMC AMD AME AMF AMG AMH AMI AMJ AMK AML AMM AMN AMO AMP AMQ AMR AMS AMT AMU AMV AMW AMX AMY AMZ ANA ANB ANC AND ANE ANF ANG ANH ANI ANJ ANK ANL ANM ANN ANO ANP ANQ ANR ANS ANT ANU ANV ANW ANX ANY AOA AOD AOE AOF AOG AOH AOI AOJ AOK AOL AOM AON AOO AOP AOQ AOR AOS AOT AOU AOV AOW AOX AOY AOZ AP APA APB APC APD APE APF APG APH API APJ APK APL APM APN APO APP APQ APR APS APT APU APV APW APX APY APZ AQA AQB AQC AQD AQE AQF AQG AQH AQI AQJ AQK AQL AQM AQN AQO AQP AQQ AQR AQS AQT AQU AQV AQW AQX AQY AQZ ARA ARB ARC ARD ARE ARF ARG ARH ARI ARJ ARK ARL ARM ARN ARO ARP ARQ ARR ARS ART ARU ARV ARW ARX ARY ARZ ASA ASB ASC ASD ASE ASF ASG ASH ASI ASJ ASK ASL ASM ASN ASO ASP ASQ ASR ASS AST ASU ASV ASW ASX ASY ASZ ATA ATB ATC ATD ATE ATF ATG ATH ATI ATJ ATK ATL ATM ATN ATO ATP ATQ ATR ATS ATT ATU ATV ATW ATX ATY ATZ AU AUA AUB AUC AUD AUE AUF AUG AUH AUI AUJ AUK AUL AUM AUN AUO AUP AUQ AUR AUS AUT AUU AUV AUW AUX AUY AUZ AV AVA AVB AVC AVD AVE AVF AVG AVH AVI AVJ AVK AVL AVM AVN AVO AVP AVQ AVR AVS AVT AVU AVV AVW AVX AVY AVZ AWA AWB AWC AWD AWE AWF AWG AWH AWI AWJ AWK AWL AWM AWN AWO AWP AWQ AWR AWS AWT AWU AWV AWW AWX AWY AWZ AXA AXB AXC AXD AXE AXF AXG AXH AXI AXJ AXK AXL AXM AXN AXO AXP AXQ AXR BA BC BD BE BH BM BN BO BR BT BW CAS CAT CAU CAV CAW CAX CAY CAZ CBA CBB CD CEC CED CFE CFF CFO CG CH CK CKN CM CMR CN CNO COF CP CR CRN CS CST CT CU CV CW CZ DA DAN DB DI DL DM DQ DR EA EB ED EE EI EN EQ ER ERN ET EX FC FF FI FLW FN FO FS FT FV FX GA GC GD GDN GN HS HWB IA IB ICA ICE ICO II IL INB INN INO IP IS IT IV JB JE LA LAN LAR LB LC LI LO LRC LS MA MB MF MG MH MR MRN MS MSS MWB NA NF OH OI ON OP OR PB PC PD PE PF PI PK PL POR PP PQ PR PS PW PY RA RC RCN RE REN RF RR RT SA SB SD SE SEA SF SH SI SM SN SP SQ SRN SS STA SW SZ TB TCR TE TF TI TIN TL TN TP UAR UC UCN UN UO URI VA VC VGR VM VN VON VOR VP VR VS VT VV WE WM WN WR WS WY XA XC XP ZZZ ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-18 and BT-122 identification scheme identifier MUST be coded using a restriction of UNTDID 1153."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AdditionalDocumentReference/cbc:DocumentTypeCode | //cn:CreditNote/cac:AdditionalDocumentReference/cbc:DocumentTypeCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"DocumentTypeCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"DocumentTypeCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-153", "assert", "(not(contains(normalize-space(.), ' ')) and contains(' 130 ', concat(' ', normalize-space(.), ' ')))", "The Document type code only allows the following values: 130."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:AccountingCost | //cn:CreditNote/cbc:AccountingCost",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCost", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCost", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-018", "assert", "matches(.,'^(.{1,50})$')", "The BT-19 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentTerms/cbc:Note | //cn:CreditNote/cac:PaymentTerms/cbc:Note",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentTerms", ""}, {"Note", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentTerms", ""}, {"Note", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-019", "assert", "matches(.,'^(.{1,200})$')", "The BT-20 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:Note | //cn:CreditNote/cbc:Note",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Note", ""}}}, {"CreditNote", []ptDTCtxStep{{"Note", ""}}}},
			lets: []ptDTLetSrc{
				{"cnt20_2", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(SOURCECURRENCYCODE)#(.{1,})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(SOURCECURRENCYCODE)#(.{1,})#)$')])"},
				{"cnt20_3", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(CALCULATIONRATE)#(.{1,})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(CALCULATIONRATE)#(.{1,})#)$')])"},
				{"cnt20_4", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(DESCRIPTION@INVOICEPERIOD)#(.{1,150})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(DESCRIPTION@INVOICEPERIOD)#(.{1,150})#)$')])"},
				{"cnt20_6", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(NUMBER@ATCERTIFIEDPROGRAM)#(.{1,})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(NUMBER@ATCERTIFIEDPROGRAM)#(.{1,})#)$')])"},
				{"cnt20_7", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(HASHCODE@ATCERTIFIEDPROGRAM)#(.{1,})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(HASHCODE@ATCERTIFIEDPROGRAM)#(.{1,})#)$')])"},
				{"cnt20_8", "count(ancestor::ubl:Invoice/cbc:Note[matches(.,'^(#(DESCRIPTION@ATCERTIFIEDPROGRAM)#(.{1,})#)$')] | ancestor::cn:CreditNote/cbc:Note[matches(.,'^(#(DESCRIPTION@ATCERTIFIEDPROGRAM)#(.{1,})#)$')])"},
				{"wht20_9", "concat('#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-'),'#')),'#')"},
				{"cnt20_9", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"wht20_10", "concat('#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-'),'#')),'#')"},
				{"cnt20_10", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"wht20_11", "concat('#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-'),'#')),'#')"},
				{"cnt20_11", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"atm20_12", "concat('#ENTITY@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#ENTITY@ATMPAYMENT-'),'#')),'#')"},
				{"cnt20_12", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"atm20_13", "concat('#REFERENCE@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#REFERENCE@ATMPAYMENT-'),'#')),'#')"},
				{"cnt20_13", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"atm20_14", "concat('#AMOUNT@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#AMOUNT@ATMPAYMENT-'),'#')),'#')"},
				{"cnt20_14", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
				{"atm20_15", "concat('#DESCRIPTION@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#DESCRIPTION@ATMPAYMENT-'),'#')),'#')"},
				{"cnt20_15", "count(index-of((ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note),.))"},
			},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-020", "assert", "matches(.,'^(.{1,500})$','s')", "The BT-22 does not meet the defined format: alphanumeric with size between 1 and 500."},
				{"DT-CIUS-PT-020_2.1", "report", "starts-with(normalize-space(.),'#SOURCECURRENCYCODE#') and not((not(contains(normalize-space(substring-before(substring-after(.,'#SOURCECURRENCYCODE#'),'#')), ' ')) and contains(' AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYR BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HRK HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRO MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLL SOS SRD SSP STD SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UZS VEF VND VUV WST XAF XAG XAU XBA XBB XBC XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWL ', concat(' ', normalize-space(substring-before(substring-after(.,'#SOURCECURRENCYCODE#'),'#')), ' '))))", "The BT-22 when prefixed with '#SOURCECURRENCYCODE#', MUST be coded using ISO code list 4217 alpha-3."},
				{"DT-CIUS-PT-020_2.2", "report", "starts-with(normalize-space(.),'#SOURCECURRENCYCODE#') and not(matches(.,'^(#(SOURCECURRENCYCODE)#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#SOURCECURRENCYCODE#' and suffice with '#'."},
				{"DT-CIUS-PT-020_2.3", "report", "starts-with(normalize-space(.),'#CALCULATIONRATE#') and $cnt20_2 < 1", "The BT-22 when prefixed with '#SOURCECURRENCYCODE#', is mandatory if an element with the prefix '#CALCULATIONRATE#' has been or will be referenced."},
				{"DT-CIUS-PT-020_2.4", "report", "starts-with(normalize-space(.),'#SOURCECURRENCYCODE#') and $cnt20_2 > 1", "The BT-22 when prefixed with '#SOURCECURRENCYCODE#', can only exist once."},
				{"DT-CIUS-PT-020_3.1", "report", "starts-with(normalize-space(.),'#CALCULATIONRATE#') and not(matches(normalize-space(substring-before(substring-after(.,'#CALCULATIONRATE#'),'#')),'^(\\d{1,9}\\.\\d{2,5})$'))", "The BT-22 when prefixed with '#CALCULATIONRATE#', have to meet the defined format: decimal value with 5 decimal places and 9 unit places."},
				{"DT-CIUS-PT-020_3.2", "report", "starts-with(normalize-space(.),'#CALCULATIONRATE#') and not(matches(.,'^(#(CALCULATIONRATE)#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#CALCULATIONRATE#' and suffice with '#'."},
				{"DT-CIUS-PT-020_3.3", "report", "starts-with(normalize-space(.),'#SOURCECURRENCYCODE#') and $cnt20_3 < 1", "The BT-22 when prefixed with '#CALCULATIONRATE#', is mandatory if an element with the prefix '#SOURCECURRENCYCODE#' has been or will be referenced."},
				{"DT-CIUS-PT-020_3.4", "report", "starts-with(normalize-space(.),'#CALCULATIONRATE#') and $cnt20_3 > 1", "The BT-22 when prefixed with '#CALCULATIONRATE#', can only exist once."},
				{"DT-CIUS-PT-020_4.1", "report", "starts-with(normalize-space(.),'#DESCRIPTION@INVOICEPERIOD#') and not(matches(normalize-space(substring-before(substring-after(.,'#DESCRIPTION@INVOICEPERIOD#'),'#')),'^(.{1,150})$'))", "The BT-22 when prefixed with '#DESCRIPTION@INVOICEPERIOD#', have to meet the defined format: alphanumeric with size between 1 and 150."},
				{"DT-CIUS-PT-020_4.2", "report", "starts-with(normalize-space(.),'#DESCRIPTION@INVOICEPERIOD#') and not(matches(.,'^(#(DESCRIPTION@INVOICEPERIOD)#(.{1,150})#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#DESCRIPTION@INVOICEPERIOD#' and suffice with '#'."},
				{"DT-CIUS-PT-020_4.3", "report", "starts-with(normalize-space(.),'#DESCRIPTION@INVOICEPERIOD#') and $cnt20_4 > 1", "The BT-22 when prefixed with '#DESCRIPTION@INVOICEPERIOD#', can only exist once."},
				{"DT-CIUS-PT-020_5.1", "report", "starts-with(normalize-space(.),'#ADDITIONALPROPERTY#') and not(matches(normalize-space(substring-before(substring-after(.,'#ADDITIONALPROPERTY#'),'#')),'^(.{1,50})$'))", "The BT-22 when prefixed with '#ADDITIONALPROPERTY#', have to meet the defined format for name: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-020_5.2", "report", "starts-with(normalize-space(.),'#ADDITIONALPROPERTY#') and not(matches(normalize-space(substring-before(substring-after(substring-after(.,'#ADDITIONALPROPERTY#'),'#'),'#')),'^(.{1,50})$'))", "The BT-22 when prefixed with '#ADDITIONALPROPERTY#', have to meet the defined format for value: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-020_5.3", "report", "starts-with(normalize-space(.),'#ADDITIONALPROPERTY#') and not(matches(.,'^(#(ADDITIONALPROPERTY)#(.{1,50})#(.{1,50})#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#ADDITIONALPROPERTY#' and suffice with '#'. The value must consist of a first part for the name, separated by \"#\", and for a second part for the value."},
				{"DT-CIUS-PT-020_6.1", "report", "starts-with(normalize-space(.),'#NUMBER@ATCERTIFIEDPROGRAM#') and not(matches(normalize-space(substring-before(substring-after(.,'#NUMBER@ATCERTIFIEDPROGRAM#'),'#')),'^(.{1,20})$'))", "The BT-22 when prefixed with '#NUMBER@ATCERTIFIEDPROGRAM#', have to meet the defined format: alphanumeric with size between 1 and 20."},
				{"DT-CIUS-PT-020_6.2", "report", "starts-with(normalize-space(.),'#NUMBER@ATCERTIFIEDPROGRAM#') and not(matches(.,'^(#(NUMBER@ATCERTIFIEDPROGRAM)#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#NUMBER@ATCERTIFIEDPROGRAM#' and suffice with '#'."},
				{"DT-CIUS-PT-020_6.3", "report", "(starts-with(normalize-space(.),'#HASHCODE@ATCERTIFIEDPROGRAM#') or starts-with(normalize-space(.),'#DESCRIPTION@ATCERTIFIEDPROGRAM#')) and $cnt20_6 < 1", "The BT-22 when prefixed with '#NUMBER@ATCERTIFIEDPROGRAM#', is mandatory if an element with the prefix '#HASHCODE@ATCERTIFIEDPROGRAM#' or '#DESCRIPTION@ATCERTIFIEDPROGRAM#' has been or will be referenced."},
				{"DT-CIUS-PT-020_6.4", "report", "starts-with(normalize-space(.),'#NUMBER@ATCERTIFIEDPROGRAM#') and $cnt20_6 > 1", "The BT-22 when prefixed with '#NUMBER@ATCERTIFIEDPROGRAM#', can only exist once."},
				{"DT-CIUS-PT-020_7.1", "report", "starts-with(normalize-space(.),'#HASHCODE@ATCERTIFIEDPROGRAM#') and not(matches(normalize-space(substring-before(substring-after(.,'#HASHCODE@ATCERTIFIEDPROGRAM#'),'#')),'^(.{1,20})$'))", "The BT-22 when prefixed with '#HASHCODE@ATCERTIFIEDPROGRAM#', have to meet the defined format: alphanumeric with size between 1 and 20."},
				{"DT-CIUS-PT-020_7.2", "report", "starts-with(normalize-space(.),'#HASHCODE@ATCERTIFIEDPROGRAM#') and not(matches(.,'^(#(HASHCODE@ATCERTIFIEDPROGRAM)#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#HASHCODE@ATCERTIFIEDPROGRAM#' and suffice with '#'."},
				{"DT-CIUS-PT-020_7.3", "report", "(starts-with(normalize-space(.),'#NUMBER@ATCERTIFIEDPROGRAM#') or starts-with(normalize-space(.),'#DESCRIPTION@ATCERTIFIEDPROGRAM#')) and $cnt20_7 < 1", "The BT-22 when prefixed with '#HASHCODE@ATCERTIFIEDPROGRAM#', is mandatory if an element with the prefix '#NUMBER@ATCERTIFIEDPROGRAM#' or '#DESCRIPTION@ATCERTIFIEDPROGRAM#' has been or will be referenced."},
				{"DT-CIUS-PT-020_7.4", "report", "starts-with(normalize-space(.),'#HASHCODE@ATCERTIFIEDPROGRAM#') and $cnt20_7 > 1", "The BT-22 when prefixed with '#HASHCODE@ATCERTIFIEDPROGRAM#', can only exist once."},
				{"DT-CIUS-PT-020_8.1", "report", "starts-with(normalize-space(.),'#DESCRIPTION@ATCERTIFIEDPROGRAM#') and not(matches(normalize-space(substring-before(substring-after(.,'#DESCRIPTION@ATCERTIFIEDPROGRAM#'),'#')),'^(.{1,150})$'))", "The BT-22 when prefixed with '#HASHCODE@ATCERTIFIEDPROGRAM#', have to meet the defined format: alphanumeric with size between 1 and 150."},
				{"DT-CIUS-PT-020_8.2", "report", "starts-with(normalize-space(.),'#DESCRIPTION@ATCERTIFIEDPROGRAM#') and not(matches(.,'^(#(DESCRIPTION@ATCERTIFIEDPROGRAM)#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#DESCRIPTION@ATCERTIFIEDPROGRAM#' and suffice with '#'."},
				{"DT-CIUS-PT-020_8.3", "report", "starts-with(normalize-space(.),'#DESCRIPTION@ATCERTIFIEDPROGRAM#') and $cnt20_8 > 1", "The BT-22 when prefixed with '#DESCRIPTION@ATCERTIFIEDPROGRAM#', can only exist once."},
				{"DT-CIUS-PT-020_9.1", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-') and not(matches(.,'^(#(WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_9.2", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-') and not(matches(normalize-space(substring-before(substring-after(.,$wht20_9),'#')),'^(.{1,150})$'))", "The BT-22 when prefixed with '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#', have to meet the defined format: alphanumeric with size between 1 and 150."},
				{"DT-CIUS-PT-020_9.3", "report", "starts-with(normalize-space(.),$wht20_9) and $cnt20_9 > 1", "The BT-22 when prefixed with '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#' must be unique."},
				{"DT-CIUS-PT-020_9.4", "report", "starts-with(normalize-space(.),$wht20_9) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_9.5", "report", "starts-with(normalize-space(.),$wht20_9) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_10.1", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-') and not(matches(.,'^(#(WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_10.2", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-') and not(matches(normalize-space(substring-before(substring-after(.,$wht20_10),'#')),'^([0-9]{1,13}.[0-9]{2})$'))", "The BT-22 when prefixed with '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#', have to meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-020_10.3", "report", "starts-with(normalize-space(.),$wht20_10) and $cnt20_10 > 1", "The BT-22 when prefixed with '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#' must be unique."},
				{"DT-CIUS-PT-020_10.4", "report", "starts-with(normalize-space(.),$wht20_10) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_10.5", "report", "starts-with(normalize-space(.),$wht20_10) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_11.1", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-') and not(matches(.,'^(#(WITHHOLDINGTAXTYPE@WITHHOLDINGTAX)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_11.2", "report", "starts-with(normalize-space(.),'#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-') and not(contains('IRF,', concat(normalize-space(substring-before(substring-after(.,$wht20_11),'#')), ',')))", "The BT-22 when prefixed with '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#', only admits the following values: IRF."},
				{"DT-CIUS-PT-020_11.3", "report", "starts-with(normalize-space(.),$wht20_11) and $cnt20_11 > 1", "The BT-22 when prefixed with '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#' must be unique."},
				{"DT-CIUS-PT-020_11.4", "report", "starts-with(normalize-space(.),$wht20_11) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXDESCRIPTION@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_11.5", "report", "starts-with(normalize-space(.),$wht20_11) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-',normalize-space(substring-before(substring-after(.,'#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-'),'#')),'#')))))", "The BT-22 when prefixed with '#WITHHOLDINGTAXTYPE@WITHHOLDINGTAX-000#' is mandatory an element with the prefix '#WITHHOLDINGTAXAMOUNT@WITHHOLDINGTAX-000#'."},
				{"DT-CIUS-PT-020_12.1", "report", "starts-with(normalize-space(.),'#ENTITY@ATMPAYMENT-') and not(matches(.,'^(#(ENTITY@ATMPAYMENT)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#ENTITY@ATMPAYMENT-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_12.2", "report", "starts-with(normalize-space(.),'#ENTITY@ATMPAYMENT-') and not(matches(normalize-space(substring-before(substring-after(.,$atm20_12),'#')),'^(.{1,20})$'))", "The BT-22 when prefixed with '#ENTITY@ATMPAYMENT-000#', have to meet the defined format: alphanumeric with size between 1 and 20."},
				{"DT-CIUS-PT-020_12.3", "report", "starts-with(normalize-space(.),$atm20_12) and $cnt20_12 > 1", "The BT-22 when prefixed with '#ENTITY@ATMPAYMENT-000#' must be unique."},
				{"DT-CIUS-PT-020_12.4", "report", "starts-with(normalize-space(.),$atm20_12) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#REFERENCE@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#ENTITY@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#ENTITY@ATMPAYMENT-000#' is mandatory an element with the prefix '#REFERENCE@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_12.5", "report", "starts-with(normalize-space(.),$atm20_12) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#AMOUNT@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#ENTITY@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#ENTITY@ATMPAYMENT-000#' is mandatory an element with the prefix '#AMOUNT@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_13.1", "report", "starts-with(normalize-space(.),'#REFERENCE@ATMPAYMENT-') and not(matches(.,'^(#(REFERENCE@ATMPAYMENT)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#REFERENCE@ATMPAYMENT-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_13.2", "report", "starts-with(normalize-space(.),'#REFERENCE@ATMPAYMENT-') and not(matches(normalize-space(substring-before(substring-after(.,$atm20_13),'#')),'^(.{1,20})$'))", "The BT-22 when prefixed with '#REFERENCE@ATMPAYMENT-000#', have to meet the defined format: alphanumeric with size between 1 and 20."},
				{"DT-CIUS-PT-020_13.3", "report", "starts-with(normalize-space(.),$atm20_13) and $cnt20_13 > 1", "The BT-22 when prefixed with '#REFERENCE@ATMPAYMENT-000#' must be unique."},
				{"DT-CIUS-PT-020_13.4", "report", "starts-with(normalize-space(.),$atm20_13) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#ENTITY@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#REFERENCE@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#REFERENCE@ATMPAYMENT-000#' is mandatory an element with the prefix '#ENTITY@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_13.5", "report", "starts-with(normalize-space(.),$atm20_13) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#AMOUNT@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#REFERENCE@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#REFERENCE@ATMPAYMENT-000#' is mandatory an element with the prefix '#AMOUNT@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_14.1", "report", "starts-with(normalize-space(.),'#AMOUNT@ATMPAYMENT-') and not(matches(.,'^(#(AMOUNT@ATMPAYMENT)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#AMOUNT@ATMPAYMENT-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_14.2", "report", "starts-with(normalize-space(.),'#AMOUNT@ATMPAYMENT-') and not(matches(normalize-space(substring-before(substring-after(.,$atm20_14),'#')),'^([0-9]{1,13}.[0-9]{2})$'))", "The BT-22 when prefixed with '#AMOUNT@ATMPAYMENT-000#', have to meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-020_14.3", "report", "starts-with(normalize-space(.),$atm20_14) and $cnt20_14 > 1", "The BT-22 when prefixed with '#AMOUNT@ATMPAYMENT-000#' must be unique."},
				{"DT-CIUS-PT-020_14.4", "report", "starts-with(normalize-space(.),$atm20_14) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#ENTITY@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#AMOUNT@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#AMOUNT@ATMPAYMENT-000#' is mandatory an element with the prefix '#ENTITY@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_14.5", "report", "starts-with(normalize-space(.),$atm20_14) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#REFERENCE@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#AMOUNT@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#AMOUNT@ATMPAYMENT-000#' is mandatory an element with the prefix '#REFERENCE@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_15.1", "report", "starts-with(normalize-space(.),'#DESCRIPTION@ATMPAYMENT-') and not(matches(.,'^(#(DESCRIPTION@ATMPAYMENT)-([0-9]{3})#(.+)#)$'))", "The BT-22 does not meet the defined format: it is mandatory to prefix the value with the following '#DESCRIPTION@ATMPAYMENT-000#' and suffice with '#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-020_15.2", "report", "starts-with(normalize-space(.),'#DESCRIPTION@ATMPAYMENT-') and not(matches(normalize-space(substring-before(substring-after(.,$atm20_15),'#')),'^(.{1,150})$'))", "The BT-22 when prefixed with '#DESCRIPTION@ATMPAYMENT-000#', have to meet the defined format: alphanumeric with size between 1 and 150."},
				{"DT-CIUS-PT-020_15.3", "report", "starts-with(normalize-space(.),$atm20_15) and $cnt20_15 > 1", "The BT-22 when prefixed with '#DESCRIPTION@ATMPAYMENT-000#' must be unique."},
				{"DT-CIUS-PT-020_15.4", "report", "starts-with(normalize-space(.),$atm20_15) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#ENTITY@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#DESCRIPTION@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#DESCRIPTION@ATMPAYMENT-000#' is mandatory an element with the prefix '#ENTITY@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_15.5", "report", "starts-with(normalize-space(.),$atm20_15) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#REFERENCE@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#DESCRIPTION@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#DESCRIPTION@ATMPAYMENT-000#' is mandatory an element with the prefix '#REFERENCE@ATMPAYMENT-000#'."},
				{"DT-CIUS-PT-020_15.6", "report", "starts-with(normalize-space(.),$atm20_15) and (every $note in (ancestor::ubl:Invoice/cbc:Note | ancestor::cn:CreditNote/cbc:Note) satisfies (not(starts-with(normalize-space($note),concat('#AMOUNT@ATMPAYMENT-',normalize-space(substring-before(substring-after(.,'#DESCRIPTION@ATMPAYMENT-'),'#')),'#')))))", "The BT-22 when prefixed with '#DESCRIPTION@ATMPAYMENT-000#' is mandatory an element with the prefix '#AMOUNT@ATMPAYMENT-000#'."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:ProfileID | //cn:CreditNote/cbc:ProfileID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"ProfileID", ""}}}, {"CreditNote", []ptDTCtxStep{{"ProfileID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-021", "assert", "matches(.,'^(.{1,50})$')", "The BT-23 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cbc:CustomizationID | //cn:CreditNote/cbc:CustomizationID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"CustomizationID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CustomizationID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-022", "assert", "matches(.,'^urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:(.{1,10})$')", "The BT-24 only allows the following value: urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT: followed by the version identifier."},
			},
		},
		{
			context: "//ubl:Invoice/cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID | //cn:CreditNote/cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"BillingReference", ""}, {"InvoiceDocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"BillingReference", ""}, {"InvoiceDocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-023", "assert", "matches(.,'^(.{1,50})$')", "The BT-25 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:BillingReference/cac:InvoiceDocumentReference/cbc:IssueDate | //cn:CreditNote/cac:BillingReference/cac:InvoiceDocumentReference/cbc:IssueDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"BillingReference", ""}, {"InvoiceDocumentReference", ""}, {"IssueDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"BillingReference", ""}, {"InvoiceDocumentReference", ""}, {"IssueDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-024", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-26 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"RegistrationName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"RegistrationName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-025", "assert", "matches(.,'^(.{1,200})$')", "The BT-27 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyName", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyName", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-026", "assert", "matches(.,'^(.{1,200})$')", "The BT-28 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-027.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-29 and BT-90 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-027.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 SEPA ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-29 and BT-90 identification scheme identifier must be coded using one of the ISO 6523 ICD list or SEPA."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-028.1", "assert", "matches(.,'^(.{1,200})$')", "The BT-30 does not meet the defined format: alphanumeric with size between 1 and 200."},
				{"DT-CIUS-PT-028.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-30 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-029", "assert", "((not(contains(substring(normalize-space(.),0,3), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', substring(normalize-space(.),0,3), ' '))))", "The BT-31 does not meet the defined format: value preceded by the country code, according to ISO code list 3166-1."},
				{"DT-CIUS-PT-030", "assert", "matches(.,'^(.{1,50})$')", "The BT-32 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyLegalForm", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyLegalForm", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-031", "assert", "matches(.,'^(.{1,200})$')", "The BT-33 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cbc:EndpointID | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"EndpointID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-032.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-34 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-032.2", "report", "(exists(@schemeID) and not(matches(@schemeID,'^(.{1,20})$')))", "The BT-34 identification scheme identifier does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-033", "assert", "matches(.,'^(.{1,200})$')", "The BT-35 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-034", "assert", "matches(.,'^(.{1,200})$')", "The BT-36 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-035", "assert", "matches(.,'^(.{1,200})$')", "The BT-162 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CityName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CityName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-036", "assert", "matches(.,'^(.{1,200})$')", "The BT-37 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:PostalZone | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:PostalZone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-037", "assert", "matches(.,'^(.{1,200})$')", "The BT-38 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-038", "assert", "matches(.,'^(.{1,200})$')", "The BT-39 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-141", "assert", "(not(contains(normalize-space(.), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', normalize-space(.), ' ')))", "The BT-40 must be coded using ISO code list 3166-1."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Name | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-039", "assert", "matches(.,'^(.{1,200})$')", "The BT-41 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Telephone | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:Telephone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"Telephone", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"Telephone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-040", "assert", "matches(.,'^(.{1,200})$')", "The BT-42 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:ElectronicMail | //cn:CreditNote/cac:AccountingSupplierParty/cac:Party/cac:Contact/cbc:ElectronicMail",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"ElectronicMail", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"Contact", ""}, {"ElectronicMail", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-041", "assert", "matches(.,'^(.{1,200})$')", "The BT-43 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"RegistrationName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"RegistrationName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-042", "assert", "matches(.,'^(.{1,200})$')", "The BT-44 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyName", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyName", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-043", "assert", "matches(.,'^(.{1,200})$')", "The BT-45 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification/cbc:ID | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-044.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-46 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-044.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 SEPA ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-46 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-045.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-47 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-045.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-47 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-046.1", "assert", "((not(contains(substring(normalize-space(.),0,3), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', substring(normalize-space(.),0,3), ' '))))", "The BT-48 does not meet the defined format: value preceded by the country code, according to ISO code list 3166-1."},
				{"DT-CIUS-PT-046.2", "assert", "matches(.,'^(.{1,50})$')", "The BT-48 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cbc:EndpointID | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"EndpointID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-047.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-49 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-047.2", "report", "(exists(@schemeID) and not(matches(@schemeID,'^(.{1,20})$')))", "The BT-49 identification scheme identifier does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:StreetName | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:StreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-048", "assert", "matches(.,'^(.{1,200})$')", "The BT-50 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:AdditionalStreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-049", "assert", "matches(.,'^(.{1,200})$')", "The BT-51 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:AddressLine/cbc:Line",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-050", "assert", "matches(.,'^(.{1,200})$')", "The BT-163 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CityName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CityName", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CityName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-051", "assert", "matches(.,'^(.{1,200})$')", "The BT-52 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:PostalZone | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:PostalZone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-052", "assert", "matches(.,'^(.{1,200})$')", "The BT-53 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cbc:CountrySubentity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-053", "assert", "matches(.,'^(.{1,200})$')", "The BT-54 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-142", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', normalize-space(.), ' '))))", "The BT-55 must be coded using ISO code list 3166-1."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Name | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-054", "assert", "matches(.,'^(.{1,200})$')", "The BT-56 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Telephone | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:Telephone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"Telephone", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"Telephone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-055", "assert", "matches(.,'^(.{1,200})$')", "The BT-57 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:ElectronicMail | //cn:CreditNote/cac:AccountingCustomerParty/cac:Party/cac:Contact/cbc:ElectronicMail",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"ElectronicMail", ""}}}, {"CreditNote", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"Contact", ""}, {"ElectronicMail", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-056", "assert", "matches(.,'^(.{1,200})$')", "The BT-58 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PayeeParty/cac:PartyName/cbc:Name | //cn:CreditNote/cac:PayeeParty/cac:PartyName/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyName", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyName", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-057", "assert", "matches(.,'^(.{1,200})$')", "The BT-59 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PayeeParty/cac:PartyIdentification/cbc:ID | //cn:CreditNote/cac:PayeeParty/cac:PartyIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-058.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-60 and BT-90 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-058.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 SEPA ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-60 and BT-90 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list or SEPA."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PayeeParty/cac:PartyLegalEntity/cbc:CompanyID | //cn:CreditNote/cac:PayeeParty/cac:PartyLegalEntity/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PayeeParty", ""}, {"PartyLegalEntity", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-059.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-61 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-059.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-61 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PartyName/cbc:Name | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PartyName/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PartyName", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PartyName", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-060", "assert", "matches(.,'^(.{1,200})$')", "The BT-62 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PartyTaxScheme", ""}, {"CompanyID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-061.1", "assert", "((not(contains(substring(normalize-space(.),0,3), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', substring(normalize-space(.),0,3), ' '))))", "The BT-63 does not meet the defined format: value preceded by the country code, according to ISO code list 3166-1."},
				{"DT-CIUS-PT-061.2", "assert", "matches(.,'^(.{1,50})$')", "The BT-63 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:StreetName | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:StreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"StreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-062", "assert", "matches(.,'^(.{1,200})$')", "The BT-64 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:AdditionalStreetName | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:AdditionalStreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"AdditionalStreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-063", "assert", "matches(.,'^(.{1,200})$')", "The BT-65 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cac:AddressLine/cbc:Line | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cac:AddressLine/cbc:Line",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"AddressLine", ""}, {"Line", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-064", "assert", "matches(.,'^(.{1,200})$')", "The BT-164 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:CityName | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:CityName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"CityName", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"CityName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-065", "assert", "matches(.,'^(.{1,200})$')", "The BT-66 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:PostalZone | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:PostalZone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"PostalZone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-066", "assert", "matches(.,'^(.{1,200})$')", "The BT-67 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:CountrySubentity | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cbc:CountrySubentity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"CountrySubentity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-067", "assert", "matches(.,'^(.{1,200})$')", "The BT-68 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country/cbc:IdentificationCode | //cn:CreditNote/cac:TaxRepresentativeParty/cac:PostalAddress/cac:Country/cbc:IdentificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}, {"Country", ""}, {"IdentificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-144", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', normalize-space(.), ' '))))", "The BT-69 must be coded using ISO code list 3166-1."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryParty/cac:PartyName/cbc:Name | //cn:CreditNote/cac:Delivery/cac:DeliveryParty/cac:PartyName/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryParty", ""}, {"PartyName", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryParty", ""}, {"PartyName", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-068", "assert", "matches(.,'^(.{1,200})$')", "The BT-70 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cbc:ID | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-069.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-71 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-069.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-71 identification scheme identifier MUST be coded using one of the ISO 6523 ICD list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cbc:ActualDeliveryDate | //cn:CreditNote/cac:Delivery/cbc:ActualDeliveryDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"ActualDeliveryDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"ActualDeliveryDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-070", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-72 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoicePeriod/cbc:StartDate | //cn:CreditNote/cac:InvoicePeriod/cbc:StartDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoicePeriod", ""}, {"StartDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"InvoicePeriod", ""}, {"StartDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-071", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-73 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoicePeriod/cbc:EndDate | //cn:CreditNote/cac:InvoicePeriod/cbc:EndDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoicePeriod", ""}, {"EndDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"InvoicePeriod", ""}, {"EndDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-072", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-74 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:StreetName | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:StreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"StreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"StreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-073", "assert", "matches(.,'^(.{1,200})$')", "The BT-75 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:AdditionalStreetName | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:AdditionalStreetName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"AdditionalStreetName", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"AdditionalStreetName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-074", "assert", "matches(.,'^(.{1,200})$')", "The BT-76 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:AddressLine/cbc:Line | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:AddressLine/cbc:Line",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"AddressLine", ""}, {"Line", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"AddressLine", ""}, {"Line", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-075", "assert", "matches(.,'^(.{1,200})$')", "The BT-165 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:CityName | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:CityName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"CityName", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"CityName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-076", "assert", "matches(.,'^(.{1,200})$')", "The BT-77 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:PostalZone | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:PostalZone",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"PostalZone", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"PostalZone", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-077", "assert", "matches(.,'^(.{1,200})$')", "The BT-78 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:CountrySubentity | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cbc:CountrySubentity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"CountrySubentity", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"CountrySubentity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-078", "assert", "matches(.,'^(.{1,200})$')", "The BT-79 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country/cbc:IdentificationCode | //cn:CreditNote/cac:Delivery/cac:DeliveryLocation/cac:Address/cac:Country/cbc:IdentificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"Country", ""}, {"IdentificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}, {"Country", ""}, {"IdentificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-145", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', normalize-space(.), ' '))))", "The BT-80 must be coded using ISO code list 3166-1."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cbc:PaymentMeansCode | //cn:CreditNote/cac:PaymentMeans/cbc:PaymentMeansCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMeansCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMeansCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-079.1", "report", "starts-with(normalize-space(.),'#ENTITY@ATMPAYMENT#') and not(matches(.,'^(#(ENTITY@ATMPAYMENT)#(.{1,20})#)$'))", "The BT-81 does not meet the defined format: it is mandatory to prefix the value with the following '#ENTITY@ATMPAYMENT#' and suffice with '#'."},
				{"DT-CIUS-PT-079.2", "report", "not(starts-with(normalize-space(.),'#ENTITY@ATMPAYMENT#')) and (contains(normalize-space(.),' ') or not(contains( ' 30 48 49 57 58 59 CC CD CH CI CO CS DE LC MB NU OU PR TB TR ',concat(' ',normalize-space(.),' '))))", "The BT-81 must be coded using UNCL4461 code list or the following values: CC, CD, CH, CI, CO, CS, DE, LC, MB, NU, OU, PR, TB, TR."},
				{"DT-CIUS-PT-079.3", "report", "(exists(@name) and not(matches(@name,'^(.{1,150})$')))", "The BT-82 identification name does not meet the defined format: alphanumeric with size between 1 and 150."},
				{"DT-CIUS-PT-079.4", "report", "exists(@name) and starts-with(normalize-space(@name),'#DESCRIPTION@ATMPAYMENT#') and not(matches(@name,'^(#(DESCRIPTION@ATMPAYMENT)#(.{1,125})#)$'))", "The BT-82 does not meet the defined format: it is mandatory to prefix the value with the following '#DESCRIPTION@ATMPAYMENT#' and suffice with '#'."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cbc:PaymentID | //cn:CreditNote/cac:PaymentMeans/cbc:PaymentID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-080", "assert", "(matches(.,'^(#(REFERENCE@ATMPAYMENT)#(.{1,20})#)$') or matches(.,'^(#(REFERENCE@DUCPAYMENT)#(.{1,20})#)$'))", "The BT-83 does not meet the defined format: it is mandatory to prefix the value with the following '#REFERENCE@ATMPAYMENT#' or '#REFERENCE@DUCPAYMENT#' and suffice with '#'."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:PayeeFinancialAccount/cbc:ID | //cn:CreditNote/cac:PaymentMeans/cac:PayeeFinancialAccount/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-081", "assert", "matches(.,'^(.{1,50})$')", "The BT-84 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:PayeeFinancialAccount/cbc:Name | //cn:CreditNote/cac:PaymentMeans/cac:PayeeFinancialAccount/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-082", "assert", "matches(.,'^(.{1,200})$')", "The BT-85 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID | //cn:CreditNote/cac:PaymentMeans/cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"FinancialInstitutionBranch", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PayeeFinancialAccount", ""}, {"FinancialInstitutionBranch", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-083", "assert", "matches(.,'^(.{1,50})$')", "The BT-86 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:CardAccount/cbc:PrimaryAccountNumberID | //cn:CreditNote/cac:PaymentMeans/cac:CardAccount/cbc:PrimaryAccountNumberID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"PrimaryAccountNumberID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"PrimaryAccountNumberID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-084.1", "assert", "matches(.,'^(.{1,200})$')", "The BT-87 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:CardAccount/cbc:NetworkID | //cn:CreditNote/cac:PaymentMeans/cac:CardAccount/cbc:NetworkID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"NetworkID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"NetworkID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-084.2", "assert", "matches(.,'^(.{1,200})$')", "The Network identifier does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:CardAccount/cbc:HolderName | //cn:CreditNote/cac:PaymentMeans/cac:CardAccount/cbc:HolderName",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"HolderName", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"CardAccount", ""}, {"HolderName", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-085", "assert", "matches(.,'^(.{1,200})$')", "The BT-88 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:PaymentMandate/cbc:ID | //cn:CreditNote/cac:PaymentMeans/cac:PaymentMandate/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMandate", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMandate", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-086", "assert", "matches(.,'^(.{1,50})$')", "The BT-89 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID | //cn:CreditNote/cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMandate", ""}, {"PayerFinancialAccount", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"PaymentMeans", ""}, {"PaymentMandate", ""}, {"PayerFinancialAccount", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-087", "assert", "matches(.,'^(.{1,50})$')", "The BT-91 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cbc:Amount | //cn:CreditNote/cac:AllowanceCharge/cbc:Amount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"Amount", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"Amount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-088.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-92 and BT-99 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-088.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-92 and BT-99 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cbc:BaseAmount | //cn:CreditNote/cac:AllowanceCharge/cbc:BaseAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"BaseAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"BaseAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-089.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-93 and BT-100 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-089.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-93 and BT-100 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cbc:MultiplierFactorNumeric | //cn:CreditNote/cac:AllowanceCharge/cbc:MultiplierFactorNumeric",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"MultiplierFactorNumeric", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"MultiplierFactorNumeric", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-090.1", "assert", "matches(.,'^(\\d{1,3}\\.\\d{2})$')", "The BT-94 and BT-101 does not meet the defined format: decimal value with 2 decimal places and 3 unit places."},
				{"DT-CIUS-PT-090.2", "report", "matches(.,'^(\\d{1,3}\\.\\d{2})$') and (xs:decimal(.) < 0.00 or xs:decimal(.) > 100.00)", "The BT-94 and BT-101 does not meet the defined format: value between 0.01 and 100.00."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cac:TaxCategory/cbc:ID | //cn:CreditNote/cac:AllowanceCharge/cac:TaxCategory/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-091", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AA S E RED INT NOR ISE OUT NA Z AE IC G O ',concat(' ',normalize-space(.),' ')))", "The BT-95 and BT-102 does not meet the defined format: only admits the following values: AA, S, E, RED, INT, NOR, ISE, OUT, NA, Z, AE, IC, G, O."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cac:TaxCategory/cbc:Percent | //cn:CreditNote/cac:AllowanceCharge/cac:TaxCategory/cbc:Percent",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"Percent", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"Percent", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-092.1", "assert", "matches(.,'^(\\d{1,3}\\.\\d{2})$')", "The BT-96 and BT-103 does not meet the defined format: decimal value with 2 decimal places and 3 unit places."},
				{"DT-CIUS-PT-092.2", "report", "matches(.,'^(\\d{1,3}\\.\\d{2})$') and (xs:decimal(.) < 0.00 or xs:decimal(.) > 100.00)", "The BT-96 and BT-103 does not meet the defined format: value between 0.01 and 100.00."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme/cbc:ID | //cn:CreditNote/cac:AllowanceCharge/cac:TaxCategory/cac:TaxScheme/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"TaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-156", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AAA AAB AAC AAD AAE AAF AAG AAH AAI AAJ AAK ADD BOL CAP CAR COC CST CUD CVD ENV EXC EXP FET FRE GCN GST ILL IMP IND LAC LCN LDP LOC LST MCA MCD OTH PDB PDC PRF SCN SSS STT SUP SUR SWT TAC TOT TOX TTA VAD VAT ',concat(' ',normalize-space(.),' ')))", "The Invoice allowance (BG-20) and/or charge (BG-21) tax scheme should be coded using a restriction of UN/ECE 5153."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge/cbc:AllowanceChargeReason | //cn:CreditNote/cac:AllowanceCharge/cbc:AllowanceChargeReason",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", ""}, {"AllowanceChargeReason", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", ""}, {"AllowanceChargeReason", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-093", "assert", "matches(.,'^(.{1,200})$')", "The BT-97 and BT-104 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']/cbc:AllowanceChargeReasonCode | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']/cbc:AllowanceChargeReasonCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}, {"AllowanceChargeReasonCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}, {"AllowanceChargeReasonCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-149", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' 41 42 60 62 63 64 65 66 67 68 70 71 88 95 100 102 103 104 105 ', concat(' ', normalize-space(.), ' '))))", "The BT-98 must belong to the UNCL 5189 code list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']/cbc:AllowanceChargeReasonCode | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']/cbc:AllowanceChargeReasonCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}, {"AllowanceChargeReasonCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}, {"AllowanceChargeReasonCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-150", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AA AAA AAC AAD AAE AAF AAH AAI AAS AAT AAV AAY AAZ ABA ABB ABC ABD ABF ABK ABL ABN ABR ABS ABT ABU ACF ACG ACH ACI ACJ ACK ACL ACM ACS ADC ADE ADJ ADK ADL ADM ADN ADO ADP ADQ ADR ADT ADW ADY ADZ AEA AEB AEC AED AEF AEH AEI AEJ AEK AEL AEM AEN AEO AEP AES AET AEU AEV AEW AEX AEY AEZ AJ AU CA CAB CAD CAE CAF CAI CAJ CAK CAL CAM CAN CAO CAP CAQ CAR CAS CAT CAU CAV CAW CD CG CS CT DAB DAD DL EG EP ER FAA FAB FAC FC FH FI GAA HAA HD HH IAA IAB ID IF IR IS KO L1 LA LAA LAB LF MAE MI ML NAA OA PA PAA PC PL RAB RAC RAD RAF RE RF RH RV SA SAA SAD SAE SAI SG SH SM ST SU TAB TAC TT TV V1 V2 WH XAA YY ZZZ ', concat(' ', normalize-space(.), ' '))))", "The BT-105 must belong to the UNCL 7161 code list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:LineExtensionAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:LineExtensionAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"LineExtensionAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"LineExtensionAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-094.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2})$')", "The BT-106 does not meet the defined format: decimal value with 2 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-094.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-106 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:AllowanceTotalAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:AllowanceTotalAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"AllowanceTotalAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"AllowanceTotalAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-095.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-107 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-095.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-107 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:ChargeTotalAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:ChargeTotalAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"ChargeTotalAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"ChargeTotalAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-096.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-108 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-096.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-108 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:TaxExclusiveAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:TaxExclusiveAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"TaxExclusiveAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"TaxExclusiveAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-097.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-109 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-097.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-109 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cbc:TaxAmount | //cn:CreditNote/cac:TaxTotal/cbc:TaxAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-098.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2})$')", "The BT-110 and BT-111 does not meet the defined format: decimal value with 2 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-098.2", "report", "exists(//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode) and normalize-space(@currencyID) != $bt06", "The BT-111 currencyID must be equal to BT-6."},
				{"DT-CIUS-PT-098.3", "report", "not(//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode) and normalize-space(@currencyID) != $bt05", "The BT-110 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:TaxInclusiveAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:TaxInclusiveAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"TaxInclusiveAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"TaxInclusiveAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-099.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-112 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-099.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-112 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:PrepaidAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:PrepaidAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PrepaidAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PrepaidAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-100.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-113 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-100.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-113 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:PayableRoundingAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:PayableRoundingAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PayableRoundingAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PayableRoundingAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-101.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2})$')", "The BT-114 does not meet the defined format: decimal value with 2 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-101.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-114 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:LegalMonetaryTotal/cbc:PayableAmount | //cn:CreditNote/cac:LegalMonetaryTotal/cbc:PayableAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PayableAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PayableAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-102.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-115 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-102.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-115 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cbc:TaxableAmount | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cbc:TaxableAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxableAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxableAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-103.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2,3})$')", "The BT-116 does not meet the defined format: decimal value with 3 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-103.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-116 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cbc:TaxAmount | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cbc:TaxAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-104.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2})$')", "The BT-117 does not meet the defined format: decimal value with 2 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-104.2", "report", "exists(//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode) and normalize-space(@currencyID) != $bt06", "The BT-117 currencyID must be equal to BT-6."},
				{"DT-CIUS-PT-104.3", "report", "not(//ubl:Invoice/cbc:TaxCurrencyCode | //cn:CreditNote/cbc:TaxCurrencyCode) and normalize-space(@currencyID) != $bt05", "The BT-117 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:ID | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-105", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AA S E RED INT NOR ISE OUT NA Z AE IC G O ',concat(' ',normalize-space(.),' ')))", "The BT-118 does not meet the defined format: only admits the following values: AA, S, E, RED, INT, NOR, ISE, OUT, NA, Z, AE, IC, G, O."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:Percent | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:Percent",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"Percent", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"Percent", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-106.1", "assert", "matches(.,'^(\\d{1,3}\\.\\d{2})$')", "The BT-119 does not meet the defined format: decimal value with 2 decimal places and 3 unit places."},
				{"DT-CIUS-PT-106.2", "report", "matches(.,'^(\\d{1,3}\\.\\d{2})$') and (xs:decimal(.) < 0.00 or xs:decimal(.) > 100.00)", "The BT-119 does not meet the defined format: value between 0.01 and 100.00."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:TaxExemptionReason | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:TaxExemptionReason",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxExemptionReason", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxExemptionReason", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-107", "assert", "matches(.,'^(.{1,200})$')", "The BT-120 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:TaxExemptionReasonCode | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cbc:TaxExemptionReasonCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxExemptionReasonCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxExemptionReasonCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-108", "assert", "matches(.,'^(.{1,5})$')", "The BT-121 does not meet the defined format: alphanumeric with size between 1 and 5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme/cbc:ID | //cn:CreditNote/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-154", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AAA AAB AAC AAD AAE AAF AAG AAH AAI AAJ AAK ADD BOL CAP CAR COC CST CUD CVD ENV EXC EXP FET FRE GCN GST ILL IMP IND LAC LCN LDP LOC LST MCA MCD OTH PDB PDC PRF SCN SSS STT SUP SUR SWT TAC TOT TOX TTA VAD VAT ',concat(' ',normalize-space(.),' ')))", "The VATBReakdown (BG-23) tax scheme should be coded using a restriction of UN/ECE 5153."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AdditionalDocumentReference/cbc:DocumentDescription | //cn:CreditNote/cac:AdditionalDocumentReference/cbc:DocumentDescription",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"DocumentDescription", ""}}}, {"CreditNote", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"DocumentDescription", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-109", "assert", "matches(.,'^(.{1,200})$')", "The BT-123 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference/cbc:URI | //cn:CreditNote/cac:AdditionalDocumentReference/cac:Attachment/cac:ExternalReference/cbc:URI",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"Attachment", ""}, {"ExternalReference", ""}, {"URI", ""}}}, {"CreditNote", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"Attachment", ""}, {"ExternalReference", ""}, {"URI", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-110", "assert", "matches(.,'^(.{1,200})$')", "The BT-124 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AdditionalDocumentReference/cac:Attachment/cbc:EmbeddedDocumentBinaryObject | //cn:CreditNote/cac:AdditionalDocumentReference/cac:Attachment/cbc:EmbeddedDocumentBinaryObject",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"Attachment", ""}, {"EmbeddedDocumentBinaryObject", ""}}}, {"CreditNote", []ptDTCtxStep{{"AdditionalDocumentReference", ""}, {"Attachment", ""}, {"EmbeddedDocumentBinaryObject", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-111.1", "assert", "matches(.,'^(.{1,6826666})$')", "The BT-125 does not meet the defined format: alphanumeric with size between 1 and 6826666."},
				{"DT-CIUS-PT-111.2", "assert", "((string-length(.)*0.75) div 1024) < 5000", "The BT-125 does not meet the defined format: the maximum size per file is 5 Mb."},
				{"DT-CIUS-PT-112", "assert", "(not(contains(normalize-space(@mimeCode),' ')) and contains(' text/plain image/png image/jpeg application/pdf application/msword application/vnd.openxmlformats-officedocument.wordprocessingml.document application/vnd.ms-excel application/vnd.openxmlformats-officedocument.spreadsheetml.sheet application/xml text/xml text/csv ',concat(' ',normalize-space(@mimeCode),' ')))", "The BT-125_1 does not meet the defined format: only admits the following values: text/plain, image/png, image/jpeg, application/pdf, application/msword, application/vnd.openxmlformats-officedocument.wordprocessingml.document, application/vnd.ms-excel, application/vnd.openxmlformats-officedocument.spreadsheetml.sheet, application/xml, text/xml, text/csv."},
				{"DT-CIUS-PT-113", "assert", "matches(@filename,'^(.{1,50})$')", "The BT-125_2 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-114.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-126 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-114.2", "assert", "count(//ubl:Invoice/cac:InvoiceLine/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cbc:ID) = count(distinct-values(//ubl:Invoice/cac:InvoiceLine/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cbc:ID))", "The BT-126 value must be unique."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cbc:Note | //cn:CreditNote/cac:CreditNoteLine/cbc:Note",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Note", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Note", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-115", "assert", "matches(.,'^(.{1,500})$')", "The BT-127 does not meet the defined format: alphanumeric with size between 1 and 500."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:DocumentReference/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:DocumentReference/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"DocumentReference", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"DocumentReference", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-116.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-128 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-116.2", "report", "(exists(@schemeID) and (contains(normalize-space(@schemeID), ' ') or not(contains(' AAA AAB AAC AAD AAE AAF AAG AAH AAI AAJ AAK AAL AAM AAN AAO AAP AAQ AAR AAS AAT AAU AAV AAW AAX AAY AAZ ABA ABB ABC ABD ABE ABF ABG ABH ABI ABJ ABK ABL ABM ABN ABO ABP ABQ ABR ABS ABT ABU ABV ABW ABX ABY ABZ AC ACA ACB ACC ACD ACE ACF ACG ACH ACI ACJ ACK ACL ACN ACO ACP ACQ ACR ACT ACU ACV ACW ACX ACY ACZ ADA ADB ADC ADD ADE ADF ADG ADI ADJ ADK ADL ADM ADN ADO ADP ADQ ADT ADU ADV ADW ADX ADY ADZ AE AEA AEB AEC AED AEE AEF AEG AEH AEI AEJ AEK AEL AEM AEN AEO AEP AEQ AER AES AET AEU AEV AEW AEX AEY AEZ AF AFA AFB AFC AFD AFE AFF AFG AFH AFI AFJ AFK AFL AFM AFN AFO AFP AFQ AFR AFS AFT AFU AFV AFW AFX AFY AFZ AGA AGB AGC AGD AGE AGF AGG AGH AGI AGJ AGK AGL AGM AGN AGO AGP AGQ AGR AGS AGT AGU AGV AGW AGX AGY AGZ AHA AHB AHC AHD AHE AHF AHG AHH AHI AHJ AHK AHL AHM AHN AHO AHP AHQ AHR AHS AHT AHU AHV AHX AHY AHZ AIA AIB AIC AID AIE AIF AIG AIH AII AIJ AIK AIL AIM AIN AIO AIP AIQ AIR AIS AIT AIU AIV AIW AIX AIY AIZ AJA AJB AJC AJD AJE AJF AJG AJH AJI AJJ AJK AJL AJM AJN AJO AJP AJQ AJR AJS AJT AJU AJV AJW AJX AJY AJZ AKA AKB AKC AKD AKE AKF AKG AKH AKI AKJ AKK AKL AKM AKN AKO AKP AKQ AKR AKS AKT AKU AKV AKW AKX AKY AKZ ALA ALB ALC ALD ALE ALF ALG ALH ALI ALJ ALK ALL ALM ALN ALO ALP ALQ ALR ALS ALT ALU ALV ALW ALX ALY ALZ AMA AMB AMC AMD AME AMF AMG AMH AMI AMJ AMK AML AMM AMN AMO AMP AMQ AMR AMS AMT AMU AMV AMW AMX AMY AMZ ANA ANB ANC AND ANE ANF ANG ANH ANI ANJ ANK ANL ANM ANN ANO ANP ANQ ANR ANS ANT ANU ANV ANW ANX ANY AOA AOD AOE AOF AOG AOH AOI AOJ AOK AOL AOM AON AOO AOP AOQ AOR AOS AOT AOU AOV AOW AOX AOY AOZ AP APA APB APC APD APE APF APG APH API APJ APK APL APM APN APO APP APQ APR APS APT APU APV APW APX APY APZ AQA AQB AQC AQD AQE AQF AQG AQH AQI AQJ AQK AQL AQM AQN AQO AQP AQQ AQR AQS AQT AQU AQV AQW AQX AQY AQZ ARA ARB ARC ARD ARE ARF ARG ARH ARI ARJ ARK ARL ARM ARN ARO ARP ARQ ARR ARS ART ARU ARV ARW ARX ARY ARZ ASA ASB ASC ASD ASE ASF ASG ASH ASI ASJ ASK ASL ASM ASN ASO ASP ASQ ASR ASS AST ASU ASV ASW ASX ASY ASZ ATA ATB ATC ATD ATE ATF ATG ATH ATI ATJ ATK ATL ATM ATN ATO ATP ATQ ATR ATS ATT ATU ATV ATW ATX ATY ATZ AU AUA AUB AUC AUD AUE AUF AUG AUH AUI AUJ AUK AUL AUM AUN AUO AUP AUQ AUR AUS AUT AUU AUV AUW AUX AUY AUZ AV AVA AVB AVC AVD AVE AVF AVG AVH AVI AVJ AVK AVL AVM AVN AVO AVP AVQ AVR AVS AVT AVU AVV AVW AVX AVY AVZ AWA AWB AWC AWD AWE AWF AWG AWH AWI AWJ AWK AWL AWM AWN AWO AWP AWQ AWR AWS AWT AWU AWV AWW AWX AWY AWZ AXA AXB AXC AXD AXE AXF AXG AXH AXI AXJ AXK AXL AXM AXN AXO AXP AXQ AXR BA BC BD BE BH BM BN BO BR BT BW CAS CAT CAU CAV CAW CAX CAY CAZ CBA CBB CD CEC CED CFE CFF CFO CG CH CK CKN CM CMR CN CNO COF CP CR CRN CS CST CT CU CV CW CZ DA DAN DB DI DL DM DQ DR EA EB ED EE EI EN EQ ER ERN ET EX FC FF FI FLW FN FO FS FT FV FX GA GC GD GDN GN HS HWB IA IB ICA ICE ICO II IL INB INN INO IP IS IT IV JB JE LA LAN LAR LB LC LI LO LRC LS MA MB MF MG MH MR MRN MS MSS MWB NA NF OH OI ON OP OR PB PC PD PE PF PI PK PL POR PP PQ PR PS PW PY RA RC RCN RE REN RF RR RT SA SB SD SE SEA SF SH SI SM SN SP SQ SRN SS STA SW SZ TB TCR TE TF TI TIN TL TN TP UAR UC UCN UN UO URI VA VC VGR VM VN VON VOR VP VR VS VT VV WE WM WN WR WS WY XA XC XP ZZZ ', concat(' ', normalize-space(@schemeID), ' ')))))", "The BT-128 identification scheme identifier MUST be coded using a restriction of UNTDID 1153."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cbc:InvoicedQuantity | //cn:CreditNote/cac:CreditNoteLine/cbc:CreditedQuantity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"InvoicedQuantity", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"CreditedQuantity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-117.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2,3})$')", "The BT-129 does not meet the defined format: decimal value with 3 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-143", "assert", "((not(contains(normalize-space(@unitCode), ' ')) and contains(' 10 11 13 14 15 20 21 22 23 24 25 27 28 33 34 35 37 38 40 41 56 57 58 59 60 61 64 66 74 76 77 78 80 81 84 85 87 89 91 1I 2A 2B 2C 2G 2H 2I 2J 2K 2L 2M 2N 2P 2Q 2R 2U 2X 2Y 2Z 3B 3C 4C 4G 4H 4K 4L 4M 4N 4O 4P 4Q 4R 4T 4U 4W 4X 5A 5B 5E 5J A1 A10 A11 A12 A13 A14 A15 A16 A17 A18 A19 A2 A20 A21 A22 A23 A24 A25 A26 A27 A28 A29 A3 A30 A31 A32 A33 A34 A35 A36 A37 A38 A39 A4 A40 A41 A42 A43 A44 A45 A47 A48 A49 A5 A50 A51 A52 A53 A54 A55 A56 A57 A58 A59 A6 A60 A61 A62 A63 A64 A65 A66 A67 A68 A69 A7 A70 A71 A73 A74 A75 A76 A77 A78 A79 A8 A80 A81 A82 A83 A84 A85 A86 A87 A88 A89 A9 A90 A91 A93 A94 A95 A96 A97 A98 A99 AA AB ACR ACT AD AE AH AI AK AL AMH AMP ANN APZ AQ ARE AS ASM ASU ATM ATT AY AZ B1 B10 B11 B12 B13 B14 B15 B16 B17 B18 B19 B20 B21 B22 B23 B24 B25 B26 B27 B28 B29 B3 B30 B31 B32 B33 B34 B35 B36 B37 B38 B39 B4 B40 B41 B42 B43 B44 B45 B46 B47 B48 B49 B50 B51 B52 B53 B54 B55 B56 B57 B58 B59 B60 B61 B62 B63 B64 B65 B66 B67 B68 B69 B7 B70 B71 B72 B73 B74 B75 B76 B77 B78 B79 B8 B80 B81 B82 B83 B84 B85 B86 B87 B88 B89 B90 B91 B92 B93 B94 B95 B96 B97 B98 B99 BAR BB BFT BHP BIL BLD BLL BP BQL BTU BUA BUI C0 C10 C11 C12 C13 C14 C15 C16 C17 C18 C19 C20 C21 C22 C23 C24 C25 C26 C27 C28 C29 C3 C30 C31 C32 C33 C34 C35 C36 C37 C38 C39 C40 C41 C42 C43 C44 C45 C46 C47 C48 C49 C50 C51 C52 C53 C54 C55 C56 C57 C58 C59 C60 C61 C62 C63 C64 C65 C66 C67 C68 C69 C7 C70 C71 C72 C73 C74 C75 C76 C78 C79 C8 C80 C81 C82 C83 C84 C85 C86 C87 C88 C89 C9 C90 C91 C92 C93 C94 C95 C96 C97 C99 CCT CDL CEL CEN CG CGM CKG CLF CLT CMK CMQ CMT CNP CNT COU CTG CTM CTN CUR CWA CWI D03 D04 D1 D10 D11 D12 D13 D15 D16 D17 D18 D19 D2 D20 D21 D22 D23 D24 D25 D26 D27 D29 D30 D31 D32 D33 D34 D35 D36 D37 D38 D39 D41 D42 D43 D44 D45 D46 D47 D48 D49 D5 D50 D51 D52 D53 D54 D55 D56 D57 D58 D59 D6 D60 D61 D62 D63 D65 D68 D69 D70 D71 D72 D73 D74 D75 D76 D77 D78 D80 D81 D82 D83 D85 D86 D87 D88 D89 D9 D91 D93 D94 D95 DAA DAD DAY DB DD DEC DG DJ DLT DMA DMK DMO DMQ DMT DN DPC DPR DPT DRA DRI DRL DT DTN DU DWT DX DZN DZP E01 E07 E08 E09 E10 E11 E12 E14 E15 E16 E17 E18 E19 E20 E21 E22 E23 E25 E27 E28 E30 E31 E32 E33 E34 E35 E36 E37 E38 E39 E4 E40 E41 E42 E43 E44 E45 E46 E47 E48 E49 E50 E51 E52 E53 E54 E55 E56 E57 E58 E59 E60 E61 E62 E63 E64 E65 E66 E67 E68 E69 E70 E71 E72 E73 E74 E75 E76 E77 E78 E79 E80 E81 E82 E83 E84 E85 E86 E87 E88 E89 E90 E91 E92 E93 E94 E95 E96 E97 E98 E99 EA EB EQ F01 F02 F03 F04 F05 F06 F07 F08 F10 F11 F12 F13 F14 F15 F16 F17 F18 F19 F20 F21 F22 F23 F24 F25 F26 F27 F28 F29 F30 F31 F32 F33 F34 F35 F36 F37 F38 F39 F40 F41 F42 F43 F44 F45 F46 F47 F48 F49 F50 F51 F52 F53 F54 F55 F56 F57 F58 F59 F60 F61 F62 F63 F64 F65 F66 F67 F68 F69 F70 F71 F72 F73 F74 F75 F76 F77 F78 F79 F80 F81 F82 F83 F84 F85 F86 F87 F88 F89 F90 F91 F92 F93 F94 F95 F96 F97 F98 F99 FAH FAR FBM FC FF FH FIT FL FOT FP FR FS FTK FTQ G01 G04 G05 G06 G08 G09 G10 G11 G12 G13 G14 G15 G16 G17 G18 G19 G2 G20 G21 G23 G24 G25 G26 G27 G28 G29 G3 G30 G31 G32 G33 G34 G35 G36 G37 G38 G39 G40 G41 G42 G43 G44 G45 G46 G47 G48 G49 G50 G51 G52 G53 G54 G55 G56 G57 G58 G59 G60 G61 G62 G63 G64 G65 G66 G67 G68 G69 G70 G71 G72 G73 G74 G75 G76 G77 G78 G79 G80 G81 G82 G83 G84 G85 G86 G87 G88 G89 G90 G91 G92 G93 G94 G95 G96 G97 G98 G99 GB GBQ GDW GE GF GFI GGR GIA GIC GII GIP GJ GL GLD GLI GLL GM GO GP GQ GRM GRN GRO GRT GT GV GWH H03 H04 H05 H06 H07 H08 H09 H10 H11 H12 H13 H14 H15 H16 H18 H19 H20 H21 H22 H23 H24 H25 H26 H27 H28 H29 H30 H31 H32 H33 H34 H35 H36 H37 H38 H39 H40 H41 H42 H43 H44 H45 H46 H47 H48 H49 H50 H51 H52 H53 H54 H55 H56 H57 H58 H59 H60 H61 H62 H63 H64 H65 H66 H67 H68 H69 H70 H71 H72 H73 H74 H75 H76 H77 H78 H79 H80 H81 H82 H83 H84 H85 H87 H88 H89 H90 H91 H92 H93 H94 H95 H96 H98 H99 HA HAR HBA HBX HC HDW HEA HGM HH HIU HJ HKM HLT HM HMQ HMT HN HP HPA HTZ HUR IA IE INH INK INQ ISD IU IV J10 J12 J13 J14 J15 J16 J17 J18 J19 J2 J20 J21 J22 J23 J24 J25 J26 J27 J28 J29 J30 J31 J32 J33 J34 J35 J36 J38 J39 J40 J41 J42 J43 J44 J45 J46 J47 J48 J49 J50 J51 J52 J53 J54 J55 J56 J57 J58 J59 J60 J61 J62 J63 J64 J65 J66 J67 J68 J69 J70 J71 J72 J73 J74 J75 J76 J78 J79 J81 J82 J83 J84 J85 J87 J89 J90 J91 J92 J93 J94 J95 J96 J97 J98 J99 JE JK JM JNT JOU JPS JWL K1 K10 K11 K12 K13 K14 K15 K16 K17 K18 K19 K2 K20 K21 K22 K23 K24 K25 K26 K27 K28 K3 K30 K31 K32 K33 K34 K35 K36 K37 K38 K39 K40 K41 K42 K43 K45 K46 K47 K48 K49 K5 K50 K51 K52 K53 K54 K55 K58 K59 K6 K60 K61 K62 K63 K64 K65 K66 K67 K68 K69 K70 K71 K73 K74 K75 K76 K77 K78 K79 K80 K81 K82 K83 K84 K85 K86 K87 K88 K89 K90 K91 K92 K93 K94 K95 K96 K97 K98 K99 KA KAT KB KBA KCC KDW KEL KGM KGS KHY KHZ KI KIC KIP KJ KJO KL KLK KLX KMA KMH KMK KMQ KMT KNI KNS KNT KO KPA KPH KPO KPP KR KSD KSH KT KTN KUR KVA KVR KVT KW KWH KWO KWT KX L10 L11 L12 L13 L14 L15 L16 L17 L18 L19 L2 L20 L21 L23 L24 L25 L26 L27 L28 L29 L30 L31 L32 L33 L34 L35 L36 L37 L38 L39 L40 L41 L42 L43 L44 L45 L46 L47 L48 L49 L50 L51 L52 L53 L54 L55 L56 L57 L58 L59 L60 L63 L64 L65 L66 L67 L68 L69 L70 L71 L72 L73 L74 L75 L76 L77 L78 L79 L80 L81 L82 L83 L84 L85 L86 L87 L88 L89 L90 L91 L92 L93 L94 L95 L96 L98 L99 LA LAC LBR LBT LD LEF LF LH LK LM LN LO LP LPA LR LS LTN LTR LUB LUM LUX LY M1 M10 M11 M12 M13 M14 M15 M16 M17 M18 M19 M20 M21 M22 M23 M24 M25 M26 M27 M29 M30 M31 M32 M33 M34 M35 M36 M37 M38 M39 M4 M40 M41 M42 M43 M44 M45 M46 M47 M48 M49 M5 M50 M51 M52 M53 M55 M56 M57 M58 M59 M60 M61 M62 M63 M64 M65 M66 M67 M68 M69 M7 M70 M71 M72 M73 M74 M75 M76 M77 M78 M79 M80 M81 M82 M83 M84 M85 M86 M87 M88 M89 M9 M90 M91 M92 M93 M94 M95 M96 M97 M98 M99 MAH MAL MAM MAR MAW MBE MBF MBR MC MCU MD MGM MHZ MIK MIL MIN MIO MIU MLD MLT MMK MMQ MMT MND MON MPA MQH MQS MSK MTK MTQ MTR MTS MVA MWH N1 N10 N11 N12 N13 N14 N15 N16 N17 N18 N19 N20 N21 N22 N23 N24 N25 N26 N27 N28 N29 N3 N30 N31 N32 N33 N34 N35 N36 N37 N38 N39 N40 N41 N42 N43 N44 N45 N46 N47 N48 N49 N50 N51 N52 N53 N54 N55 N56 N57 N58 N59 N60 N61 N62 N63 N64 N65 N66 N67 N68 N69 N70 N71 N72 N73 N74 N75 N76 N77 N78 N79 N80 N81 N82 N83 N84 N85 N86 N87 N88 N89 N90 N91 N92 N93 N94 N95 N96 N97 N98 N99 NA NAR NCL NEW NF NIL NIU NL NMI NMP NPR NPT NQ NR NT NTT NU NX OA ODE OHM ON ONZ OT OZ OZA OZI P1 P10 P11 P12 P13 P14 P15 P16 P17 P18 P19 P2 P20 P21 P22 P23 P24 P25 P26 P27 P28 P29 P30 P31 P32 P33 P34 P35 P36 P37 P38 P39 P40 P41 P42 P43 P44 P45 P46 P47 P48 P49 P5 P50 P51 P52 P53 P54 P55 P56 P57 P58 P59 P60 P61 P62 P63 P64 P65 P66 P67 P68 P69 P70 P71 P72 P73 P74 P75 P76 P77 P78 P79 P80 P81 P82 P83 P84 P85 P86 P87 P88 P89 P90 P91 P92 P93 P94 P95 P96 P97 P98 P99 PAL PD PFL PGL PI PLA PO PQ PR PS PT PTD PTI PTL Q10 Q11 Q12 Q13 Q14 Q15 Q16 Q17 Q18 Q19 Q20 Q21 Q22 Q23 Q24 Q25 Q26 Q27 Q28 Q3 QA QAN QB QR QT QTD QTI QTL QTR R1 R9 RH RM ROM RP RPM RPS RT S3 S4 SAN SCO SCR SEC SET SG SHT SIE SMI SQ SQR SR STC STI STK STL STN STW SW SX SYR T0 T3 TAH TAN TI TIC TIP TKM TMS TNE TP TPR TQD TRL TST TTS U1 U2 UA UB UC VA VLT VP W2 WA WB WCD WE WEB WEE WG WHR WM WSD WTT WW X1 YDK YDQ YRD Z11 ZP ZZ X43 X44 X1A X1B X1D X1F X1G X1W X2C X3A X3H X4A X4B X4C X4D X4F X4G X4H X5H X5L X5M X6H X6P X7A X7B X8A X8B X8C XAA XAB XAC XAD XAE XAF XAG XAH XAI XAJ XAL XAM XAP XAT XAV XB4 XBA XBB XBC XBD XBE XBF XBG XBH XBI XBJ XBK XBL XBM XBN XBO XBP XBQ XBR XBS XBT XBU XBV XBW XBX XBY XBZ XCA XCB XCC XCD XCE XCF XCG XCH XCI XCJ XCK XCL XCM XCN XCO XCP XCQ XCR XCS XCT XCU XCV XCW XCX XCY XCZ XDA XDB XDC XDG XDH XDI XDJ XDK XDL XDM XDN XDP XDR XDS XDT XDU XDV XDW XDX XDY XEC XED XEE XEF XEG XEH XEI XEN XFB XFC XFD XFE XFI XFL XFO XFP XFR XFT XFW XFX XGB XGI XGL XGR XGU XGY XGZ XHA XHB XHC XHG XHN XHR XIA XIB XIC XID XIE XIF XIG XIH XIK XIL XIN XIZ XJB XJC XJG XJR XJT XJY XKG XKI XLE XLG XLT XLU XLV XLZ XMA XMB XMC XME XMR XMS XMT XMW XMX XNA XNE XNF XNG XNS XNT XNU XNV XOA XOB XOC XOD XOE XOF XOK XOT XOU XP2 XPA XPB XPC XPD XPE XPF XPG XPH XPI XPJ XPK XPL XPN XPO XPP XPR XPT XPU XPV XPX XPY XPZ XQA XQB XQC XQD XQF XQG XQH XQJ XQK XQL XQM XQN XQP XQQ XQR XQS XRD XRG XRJ XRK XRL XRO XRT XRZ XSA XSB XSC XSD XSE XSH XSI XSK XSL XSM XSO XSP XSS XST XSU XSV XSW XSY XSZ XT1 XTB XTC XTD XTE XTG XTI XTK XTL XTN XTO XTR XTS XTT XTU XTV XTW XTY XTZ XUC XUN XVA XVG XVI XVK XVL XVN XVO XVP XVQ XVR XVS XVY XWA XWB XWC XWD XWF XWG XWH XWJ XWK XWL XWM XWN XWP XWQ XWR XWS XWT XWU XWV XWW XWX XWY XWZ XXA XXB XXC XXD XXF XXG XXH XXJ XXK XYA XYB XYC XYD XYF XYG XYH XYJ XYK XYL XYM XYN XYP XYQ XYR XYS XYT XYV XYW XYX XYY XYZ XZA XZB XZC XZD XZF XZG XZH XZJ XZK XZL XZM XZN XZP XZQ XZR XZS XZT XZU XZV XZW XZX XZY XZZ ', concat(' ', normalize-space(@unitCode), ' '))))", "The BT-130 Unit code must be coded according to the UN/ECE Recommendation 20 with Rec 21 extension."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cbc:LineExtensionAmount | //cn:CreditNote/cac:CreditNoteLine/cbc:LineExtensionAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"LineExtensionAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"LineExtensionAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-118.1", "assert", "matches(.,'^(-?\\d{1,13}\\.\\d{2,8})$')", "The BT-131 does not meet the defined format: decimal value with 8 decimal places and 13 unit places, than can be negative."},
				{"DT-CIUS-PT-118.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-131 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:OrderLineReference/cbc:LineID | //cn:CreditNote/cac:CreditNoteLine/cac:OrderLineReference/cbc:LineID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"OrderLineReference", ""}, {"LineID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"OrderLineReference", ""}, {"LineID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-119", "assert", "matches(.,'^(.{1,20})$')", "The BT-132 does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cbc:AccountingCost | //cn:CreditNote/cac:CreditNoteLine/cbc:AccountingCost",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AccountingCost", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AccountingCost", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-120", "assert", "matches(.,'^(.{1,200})$')", "The BT-133 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:InvoicePeriod/cbc:StartDate | //cn:CreditNote/cac:CreditNoteLine/cac:InvoicePeriod/cbc:StartDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"InvoicePeriod", ""}, {"StartDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"InvoicePeriod", ""}, {"StartDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-121", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-134 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:InvoicePeriod/cbc:EndDate | //cn:CreditNote/cac:CreditNoteLine/cac:InvoicePeriod/cbc:EndDate",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"InvoicePeriod", ""}, {"EndDate", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"InvoicePeriod", ""}, {"EndDate", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-122", "assert", "matches(.,'^((\\d{4})-(\\d{2})-(\\d{2}))$')", "The BT-135 does not meet the defined format: YYYY-MM-DD."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge/cbc:Amount | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge/cbc:Amount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", ""}, {"Amount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", ""}, {"Amount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-123.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-136 and BT-141 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-123.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-136 and BT-141 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge/cbc:BaseAmount | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge/cbc:BaseAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", ""}, {"BaseAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", ""}, {"BaseAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-124.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2})$')", "The BT-137 and BT-142 does not meet the defined format: decimal value with 2 decimal places and 13 unit places."},
				{"DT-CIUS-PT-124.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-137 and BT-142 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge/cbc:MultiplierFactorNumeric | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge/cbc:MultiplierFactorNumeric",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", ""}, {"MultiplierFactorNumeric", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", ""}, {"MultiplierFactorNumeric", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-125.1", "assert", "matches(.,'^(\\d{1,3}\\.\\d{2})$')", "The BT-138 and BT-143 does not meet the defined format: decimal value with 2 decimal places and 3 unit places."},
				{"DT-CIUS-PT-125.2", "report", "matches(.,'^(\\d{1,3}\\.\\d{2})$') and (xs:decimal(.) < 0.00 or xs:decimal(.) > 100.00)", "The BT-138 and BT-143 does not meet the defined format: value between 0.01 and 100.00."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge/cbc:AllowanceChargeReason | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge/cbc:AllowanceChargeReason",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", ""}, {"AllowanceChargeReason", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", ""}, {"AllowanceChargeReason", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-126", "assert", "matches(.,'^(.{1,200})$')", "The BT-139 and BT-144 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']/cbc:AllowanceChargeReasonCode | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']/cbc:AllowanceChargeReasonCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}, {"AllowanceChargeReasonCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}, {"AllowanceChargeReasonCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-147", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' 41 42 60 62 63 64 65 66 67 68 70 71 88 95 100 102 103 104 105 ', concat(' ', normalize-space(.), ' '))))", "The BT-140 must belong to the UNCL 5189 code list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']/cbc:AllowanceChargeReasonCode | //cn:CreditNote/cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']/cbc:AllowanceChargeReasonCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}, {"AllowanceChargeReasonCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}, {"AllowanceChargeReasonCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-148", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AA AAA AAC AAD AAE AAF AAH AAI AAS AAT AAV AAY AAZ ABA ABB ABC ABD ABF ABK ABL ABN ABR ABS ABT ABU ACF ACG ACH ACI ACJ ACK ACL ACM ACS ADC ADE ADF ADJ ADK ADL ADM ADN ADO ADP ADQ ADR ADT ADW ADY ADZ AEA AEB AEC AED AEF AEH AEI AEJ AEK AEL AEM AEN AEO AEP AES AET AEU AEV AEW AEX AEY AEZ AJ AU CA CAB CAD CAE CAF CAI CAJ CAK CAL CAM CAN CAO CAP CAQ CAR CAS CAT CAU CAV CAW CD CG CS CT DAB DAD DL EG EP ER FAA FAB FAC FC FH FI GAA HAA HD HH IAA IAB ID IF IR IS KO L1 LA LAA LAB LF MAE MI ML NAA OA PA PAA PC PL RAB RAC RAD RAF RE RF RH RV SA SAA SAD SAE SAI SG SH SM ST SU TAB TAC TT TV V1 V2 WH XAA YY ZZZ ', concat(' ', normalize-space(.), ' '))))", "The BT-145 must belong to the UNCL 7161 code list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Price/cbc:PriceAmount | //cn:CreditNote/cac:CreditNoteLine/cac:Price/cbc:PriceAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}, {"PriceAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}, {"PriceAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-127.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2,8})$')", "The BT-146 does not meet the defined format: decimal value with 8 decimal places and 13 unit places."},
				{"DT-CIUS-PT-127.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-146 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Price/cac:AllowanceCharge/cbc:Amount | //cn:CreditNote/cac:CreditNoteLine/cac:Price/cac:AllowanceCharge/cbc:Amount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}, {"AllowanceCharge", ""}, {"Amount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}, {"AllowanceCharge", ""}, {"Amount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-128.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2,8})$')", "The BT-147 does not meet the defined format: decimal value with 8 decimal places and 13 unit places."},
				{"DT-CIUS-PT-128.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-147 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Price/cac:AllowanceCharge/cbc:BaseAmount | //cn:CreditNote/cac:CreditNoteLine/cac:Price/cac:AllowanceCharge/cbc:BaseAmount",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}, {"AllowanceCharge", ""}, {"BaseAmount", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}, {"AllowanceCharge", ""}, {"BaseAmount", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-129.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2,8})$')", "The BT-148 does not meet the defined format: decimal value with 8 decimal places and 13 unit places."},
				{"DT-CIUS-PT-129.2", "assert", "normalize-space(@currencyID)= $bt05", "The BT-148 currencyID must be equal to BT-5."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Price/cbc:BaseQuantity | //cn:CreditNote/cac:CreditNoteLine/cac:Price/cbc:BaseQuantity",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}, {"BaseQuantity", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}, {"BaseQuantity", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-130.1", "assert", "matches(.,'^(\\d{1,13}\\.\\d{2,3})$')", "The BT-149 does not meet the defined format: decimal value with 3 decimal places and 13 unit places."},
				{"DT-CIUS-PT-130.2", "report", "(exists(@unitCode) and (contains(normalize-space(@unitCode), ' ') or not(contains(' 10 11 13 14 15 20 21 22 23 24 25 27 28 33 34 35 37 38 40 41 56 57 58 59 60 61 64 66 74 76 77 78 80 81 84 85 87 89 91 1I 2A 2B 2C 2G 2H 2I 2J 2K 2L 2M 2N 2P 2Q 2R 2U 2X 2Y 2Z 3B 3C 4C 4G 4H 4K 4L 4M 4N 4O 4P 4Q 4R 4T 4U 4W 4X 5A 5B 5E 5J A1 A10 A11 A12 A13 A14 A15 A16 A17 A18 A19 A2 A20 A21 A22 A23 A24 A25 A26 A27 A28 A29 A3 A30 A31 A32 A33 A34 A35 A36 A37 A38 A39 A4 A40 A41 A42 A43 A44 A45 A47 A48 A49 A5 A50 A51 A52 A53 A54 A55 A56 A57 A58 A59 A6 A60 A61 A62 A63 A64 A65 A66 A67 A68 A69 A7 A70 A71 A73 A74 A75 A76 A77 A78 A79 A8 A80 A81 A82 A83 A84 A85 A86 A87 A88 A89 A9 A90 A91 A93 A94 A95 A96 A97 A98 A99 AA AB ACR ACT AD AE AH AI AK AL AMH AMP ANN APZ AQ ARE AS ASM ASU ATM ATT AY AZ B1 B10 B11 B12 B13 B14 B15 B16 B17 B18 B19 B20 B21 B22 B23 B24 B25 B26 B27 B28 B29 B3 B30 B31 B32 B33 B34 B35 B36 B37 B38 B39 B4 B40 B41 B42 B43 B44 B45 B46 B47 B48 B49 B50 B51 B52 B53 B54 B55 B56 B57 B58 B59 B60 B61 B62 B63 B64 B65 B66 B67 B68 B69 B7 B70 B71 B72 B73 B74 B75 B76 B77 B78 B79 B8 B80 B81 B82 B83 B84 B85 B86 B87 B88 B89 B90 B91 B92 B93 B94 B95 B96 B97 B98 B99 BAR BB BFT BHP BIL BLD BLL BP BQL BTU BUA BUI C0 C10 C11 C12 C13 C14 C15 C16 C17 C18 C19 C20 C21 C22 C23 C24 C25 C26 C27 C28 C29 C3 C30 C31 C32 C33 C34 C35 C36 C37 C38 C39 C40 C41 C42 C43 C44 C45 C46 C47 C48 C49 C50 C51 C52 C53 C54 C55 C56 C57 C58 C59 C60 C61 C62 C63 C64 C65 C66 C67 C68 C69 C7 C70 C71 C72 C73 C74 C75 C76 C78 C79 C8 C80 C81 C82 C83 C84 C85 C86 C87 C88 C89 C9 C90 C91 C92 C93 C94 C95 C96 C97 C99 CCT CDL CEL CEN CG CGM CKG CLF CLT CMK CMQ CMT CNP CNT COU CTG CTM CTN CUR CWA CWI D03 D04 D1 D10 D11 D12 D13 D15 D16 D17 D18 D19 D2 D20 D21 D22 D23 D24 D25 D26 D27 D29 D30 D31 D32 D33 D34 D35 D36 D37 D38 D39 D41 D42 D43 D44 D45 D46 D47 D48 D49 D5 D50 D51 D52 D53 D54 D55 D56 D57 D58 D59 D6 D60 D61 D62 D63 D65 D68 D69 D70 D71 D72 D73 D74 D75 D76 D77 D78 D80 D81 D82 D83 D85 D86 D87 D88 D89 D9 D91 D93 D94 D95 DAA DAD DAY DB DD DEC DG DJ DLT DMA DMK DMO DMQ DMT DN DPC DPR DPT DRA DRI DRL DT DTN DU DWT DX DZN DZP E01 E07 E08 E09 E10 E11 E12 E14 E15 E16 E17 E18 E19 E20 E21 E22 E23 E25 E27 E28 E30 E31 E32 E33 E34 E35 E36 E37 E38 E39 E4 E40 E41 E42 E43 E44 E45 E46 E47 E48 E49 E50 E51 E52 E53 E54 E55 E56 E57 E58 E59 E60 E61 E62 E63 E64 E65 E66 E67 E68 E69 E70 E71 E72 E73 E74 E75 E76 E77 E78 E79 E80 E81 E82 E83 E84 E85 E86 E87 E88 E89 E90 E91 E92 E93 E94 E95 E96 E97 E98 E99 EA EB EQ F01 F02 F03 F04 F05 F06 F07 F08 F10 F11 F12 F13 F14 F15 F16 F17 F18 F19 F20 F21 F22 F23 F24 F25 F26 F27 F28 F29 F30 F31 F32 F33 F34 F35 F36 F37 F38 F39 F40 F41 F42 F43 F44 F45 F46 F47 F48 F49 F50 F51 F52 F53 F54 F55 F56 F57 F58 F59 F60 F61 F62 F63 F64 F65 F66 F67 F68 F69 F70 F71 F72 F73 F74 F75 F76 F77 F78 F79 F80 F81 F82 F83 F84 F85 F86 F87 F88 F89 F90 F91 F92 F93 F94 F95 F96 F97 F98 F99 FAH FAR FBM FC FF FH FIT FL FOT FP FR FS FTK FTQ G01 G04 G05 G06 G08 G09 G10 G11 G12 G13 G14 G15 G16 G17 G18 G19 G2 G20 G21 G23 G24 G25 G26 G27 G28 G29 G3 G30 G31 G32 G33 G34 G35 G36 G37 G38 G39 G40 G41 G42 G43 G44 G45 G46 G47 G48 G49 G50 G51 G52 G53 G54 G55 G56 G57 G58 G59 G60 G61 G62 G63 G64 G65 G66 G67 G68 G69 G70 G71 G72 G73 G74 G75 G76 G77 G78 G79 G80 G81 G82 G83 G84 G85 G86 G87 G88 G89 G90 G91 G92 G93 G94 G95 G96 G97 G98 G99 GB GBQ GDW GE GF GFI GGR GIA GIC GII GIP GJ GL GLD GLI GLL GM GO GP GQ GRM GRN GRO GRT GT GV GWH H03 H04 H05 H06 H07 H08 H09 H10 H11 H12 H13 H14 H15 H16 H18 H19 H20 H21 H22 H23 H24 H25 H26 H27 H28 H29 H30 H31 H32 H33 H34 H35 H36 H37 H38 H39 H40 H41 H42 H43 H44 H45 H46 H47 H48 H49 H50 H51 H52 H53 H54 H55 H56 H57 H58 H59 H60 H61 H62 H63 H64 H65 H66 H67 H68 H69 H70 H71 H72 H73 H74 H75 H76 H77 H78 H79 H80 H81 H82 H83 H84 H85 H87 H88 H89 H90 H91 H92 H93 H94 H95 H96 H98 H99 HA HAR HBA HBX HC HDW HEA HGM HH HIU HJ HKM HLT HM HMQ HMT HN HP HPA HTZ HUR IA IE INH INK INQ ISD IU IV J10 J12 J13 J14 J15 J16 J17 J18 J19 J2 J20 J21 J22 J23 J24 J25 J26 J27 J28 J29 J30 J31 J32 J33 J34 J35 J36 J38 J39 J40 J41 J42 J43 J44 J45 J46 J47 J48 J49 J50 J51 J52 J53 J54 J55 J56 J57 J58 J59 J60 J61 J62 J63 J64 J65 J66 J67 J68 J69 J70 J71 J72 J73 J74 J75 J76 J78 J79 J81 J82 J83 J84 J85 J87 J89 J90 J91 J92 J93 J94 J95 J96 J97 J98 J99 JE JK JM JNT JOU JPS JWL K1 K10 K11 K12 K13 K14 K15 K16 K17 K18 K19 K2 K20 K21 K22 K23 K24 K25 K26 K27 K28 K3 K30 K31 K32 K33 K34 K35 K36 K37 K38 K39 K40 K41 K42 K43 K45 K46 K47 K48 K49 K5 K50 K51 K52 K53 K54 K55 K58 K59 K6 K60 K61 K62 K63 K64 K65 K66 K67 K68 K69 K70 K71 K73 K74 K75 K76 K77 K78 K79 K80 K81 K82 K83 K84 K85 K86 K87 K88 K89 K90 K91 K92 K93 K94 K95 K96 K97 K98 K99 KA KAT KB KBA KCC KDW KEL KGM KGS KHY KHZ KI KIC KIP KJ KJO KL KLK KLX KMA KMH KMK KMQ KMT KNI KNS KNT KO KPA KPH KPO KPP KR KSD KSH KT KTN KUR KVA KVR KVT KW KWH KWO KWT KX L10 L11 L12 L13 L14 L15 L16 L17 L18 L19 L2 L20 L21 L23 L24 L25 L26 L27 L28 L29 L30 L31 L32 L33 L34 L35 L36 L37 L38 L39 L40 L41 L42 L43 L44 L45 L46 L47 L48 L49 L50 L51 L52 L53 L54 L55 L56 L57 L58 L59 L60 L63 L64 L65 L66 L67 L68 L69 L70 L71 L72 L73 L74 L75 L76 L77 L78 L79 L80 L81 L82 L83 L84 L85 L86 L87 L88 L89 L90 L91 L92 L93 L94 L95 L96 L98 L99 LA LAC LBR LBT LD LEF LF LH LK LM LN LO LP LPA LR LS LTN LTR LUB LUM LUX LY M1 M10 M11 M12 M13 M14 M15 M16 M17 M18 M19 M20 M21 M22 M23 M24 M25 M26 M27 M29 M30 M31 M32 M33 M34 M35 M36 M37 M38 M39 M4 M40 M41 M42 M43 M44 M45 M46 M47 M48 M49 M5 M50 M51 M52 M53 M55 M56 M57 M58 M59 M60 M61 M62 M63 M64 M65 M66 M67 M68 M69 M7 M70 M71 M72 M73 M74 M75 M76 M77 M78 M79 M80 M81 M82 M83 M84 M85 M86 M87 M88 M89 M9 M90 M91 M92 M93 M94 M95 M96 M97 M98 M99 MAH MAL MAM MAR MAW MBE MBF MBR MC MCU MD MGM MHZ MIK MIL MIN MIO MIU MLD MLT MMK MMQ MMT MND MON MPA MQH MQS MSK MTK MTQ MTR MTS MVA MWH N1 N10 N11 N12 N13 N14 N15 N16 N17 N18 N19 N20 N21 N22 N23 N24 N25 N26 N27 N28 N29 N3 N30 N31 N32 N33 N34 N35 N36 N37 N38 N39 N40 N41 N42 N43 N44 N45 N46 N47 N48 N49 N50 N51 N52 N53 N54 N55 N56 N57 N58 N59 N60 N61 N62 N63 N64 N65 N66 N67 N68 N69 N70 N71 N72 N73 N74 N75 N76 N77 N78 N79 N80 N81 N82 N83 N84 N85 N86 N87 N88 N89 N90 N91 N92 N93 N94 N95 N96 N97 N98 N99 NA NAR NCL NEW NF NIL NIU NL NMI NMP NPR NPT NQ NR NT NTT NU NX OA ODE OHM ON ONZ OT OZ OZA OZI P1 P10 P11 P12 P13 P14 P15 P16 P17 P18 P19 P2 P20 P21 P22 P23 P24 P25 P26 P27 P28 P29 P30 P31 P32 P33 P34 P35 P36 P37 P38 P39 P40 P41 P42 P43 P44 P45 P46 P47 P48 P49 P5 P50 P51 P52 P53 P54 P55 P56 P57 P58 P59 P60 P61 P62 P63 P64 P65 P66 P67 P68 P69 P70 P71 P72 P73 P74 P75 P76 P77 P78 P79 P80 P81 P82 P83 P84 P85 P86 P87 P88 P89 P90 P91 P92 P93 P94 P95 P96 P97 P98 P99 PAL PD PFL PGL PI PLA PO PQ PR PS PT PTD PTI PTL Q10 Q11 Q12 Q13 Q14 Q15 Q16 Q17 Q18 Q19 Q20 Q21 Q22 Q23 Q24 Q25 Q26 Q27 Q28 Q3 QA QAN QB QR QT QTD QTI QTL QTR R1 R9 RH RM ROM RP RPM RPS RT S3 S4 SAN SCO SCR SEC SET SG SHT SIE SMI SQ SQR SR STC STI STK STL STN STW SW SX SYR T0 T3 TAH TAN TI TIC TIP TKM TMS TNE TP TPR TQD TRL TST TTS U1 U2 UA UB UC VA VLT VP W2 WA WB WCD WE WEB WEE WG WHR WM WSD WTT WW X1 YDK YDQ YRD Z11 ZP ZZ X43 X44 X1A X1B X1D X1F X1G X1W X2C X3A X3H X4A X4B X4C X4D X4F X4G X4H X5H X5L X5M X6H X6P X7A X7B X8A X8B X8C XAA XAB XAC XAD XAE XAF XAG XAH XAI XAJ XAL XAM XAP XAT XAV XB4 XBA XBB XBC XBD XBE XBF XBG XBH XBI XBJ XBK XBL XBM XBN XBO XBP XBQ XBR XBS XBT XBU XBV XBW XBX XBY XBZ XCA XCB XCC XCD XCE XCF XCG XCH XCI XCJ XCK XCL XCM XCN XCO XCP XCQ XCR XCS XCT XCU XCV XCW XCX XCY XCZ XDA XDB XDC XDG XDH XDI XDJ XDK XDL XDM XDN XDP XDR XDS XDT XDU XDV XDW XDX XDY XEC XED XEE XEF XEG XEH XEI XEN XFB XFC XFD XFE XFI XFL XFO XFP XFR XFT XFW XFX XGB XGI XGL XGR XGU XGY XGZ XHA XHB XHC XHG XHN XHR XIA XIB XIC XID XIE XIF XIG XIH XIK XIL XIN XIZ XJB XJC XJG XJR XJT XJY XKG XKI XLE XLG XLT XLU XLV XLZ XMA XMB XMC XME XMR XMS XMT XMW XMX XNA XNE XNF XNG XNS XNT XNU XNV XOA XOB XOC XOD XOE XOF XOK XOT XOU XP2 XPA XPB XPC XPD XPE XPF XPG XPH XPI XPJ XPK XPL XPN XPO XPP XPR XPT XPU XPV XPX XPY XPZ XQA XQB XQC XQD XQF XQG XQH XQJ XQK XQL XQM XQN XQP XQQ XQR XQS XRD XRG XRJ XRK XRL XRO XRT XRZ XSA XSB XSC XSD XSE XSH XSI XSK XSL XSM XSO XSP XSS XST XSU XSV XSW XSY XSZ XT1 XTB XTC XTD XTE XTG XTI XTK XTL XTN XTO XTR XTS XTT XTU XTV XTW XTY XTZ XUC XUN XVA XVG XVI XVK XVL XVN XVO XVP XVQ XVR XVS XVY XWA XWB XWC XWD XWF XWG XWH XWJ XWK XWL XWM XWN XWP XWQ XWR XWS XWT XWU XWV XWW XWX XWY XWZ XXA XXB XXC XXD XXF XXG XXH XXJ XXK XYA XYB XYC XYD XYF XYG XYH XYJ XYK XYL XYM XYN XYP XYQ XYR XYS XYT XYV XYW XYX XYY XYZ XZA XZB XZC XZD XZF XZG XZH XZJ XZK XZL XZM XZN XZP XZQ XZR XZS XZT XZU XZV XZW XZX XZY XZZ ', concat(' ', normalize-space(@unitCode), ' ')))))", "The BT-149 Unit code must be coded according to the UN/ECE Recommendation 20 with Rec 21 extension."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-131", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AA S E RED INT NOR ISE OUT NA Z AE IC G O ',concat(' ',normalize-space(.),' ')))", "The BT-151 does not meet the defined format: only admits the following values: AA, S, E, RED, INT, NOR, ISE, OUT, NA, Z, AE, IC, G, O."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cbc:Percent | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory/cbc:Percent",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"Percent", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"Percent", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-132.1", "assert", "matches(.,'^(\\d{1,3}\\.\\d{2})$')", "The BT-152 does not meet the defined format: decimal value with 2 decimal places and 3 unit places."},
				{"DT-CIUS-PT-132.2", "report", "matches(.,'^(\\d{1,3}\\.\\d{2})$') and (xs:decimal(.) < 0.00 or xs:decimal(.) > 100.00)", "The BT-152 does not meet the defined format: value between 0.01 and 100.00."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", ""}, {"TaxScheme", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-155", "assert", "(not(contains(normalize-space(.),' ')) and contains(' AAA AAB AAC AAD AAE AAF AAG AAH AAI AAJ AAK ADD BOL CAP CAR COC CST CUD CVD ENV EXC EXP FET FRE GCN GST ILL IMP IND LAC LCN LDP LOC LST MCA MCD OTH PDB PDC PRF SCN SSS STT SUP SUR SWT TAC TOT TOX TTA VAD VAT ',concat(' ',normalize-space(.),' ')))", "The Invoice line (BG-25) tax scheme should be coded using a restriction of UN/ECE 5153."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cbc:Name | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"Name", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-133", "assert", "matches(.,'^(.{1,200})$')", "The BT-153 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cbc:Description | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cbc:Description",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"Description", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"Description", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-134", "assert", "matches(.,'^(.{1,200})$')", "The BT-154 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:SellersItemIdentification/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:SellersItemIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"SellersItemIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"SellersItemIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-135", "assert", "matches(.,'^(.{1,50})$')", "The BT-155 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:BuyersItemIdentification/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:BuyersItemIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"BuyersItemIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"BuyersItemIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-136", "assert", "matches(.,'^(.{1,50})$')", "The BT-156 does not meet the defined format: alphanumeric with size between 1 and 50."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:StandardItemIdentification/cbc:ID | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:StandardItemIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-137.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-157 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-137.2", "assert", "((not(contains(normalize-space(@schemeID), ' ')) and contains(' 0001 0002 0003 0004 0005 0006 0007 0008 0009 0010 0011 0012 0013 0014 0015 0016 0017 0018 0019 0020 0021 0022 0023 0024 0025 0026 0027 0028 0029 0030 0031 0032 0033 0034 0035 0036 0037 0038 0039 0040 0041 0042 0043 0044 0045 0046 0047 0048 0049 0050 0051 0052 0053 0054 0055 0056 0057 0058 0059 0060 0061 0062 0063 0064 0065 0066 0067 0068 0069 0070 0071 0072 0073 0074 0075 0076 0077 0078 0079 0080 0081 0082 0083 0084 0085 0086 0087 0088 0089 0090 0091 0092 0093 0094 0095 0096 0097 0098 0099 0100 0101 0102 0103 0104 0105 0106 0107 0108 0109 0110 0111 0112 0113 0114 0115 0116 0117 0118 0119 0120 0121 0122 0123 0124 0125 0126 0127 0128 0129 0130 0131 0132 0133 0134 0135 0136 0137 0138 0139 0140 0141 0142 0143 0144 0145 0146 0147 0148 0149 0150 0151 0152 0153 0154 0155 0156 0157 0158 0159 0160 0161 0162 0163 0164 0165 0166 0167 0168 0169 0170 0171 0172 0173 0174 0175 0176 0177 0178 0179 0180 0183 0184 0191 0192 ', concat(' ', normalize-space(@schemeID), ' '))))", "The BT-157 identifier scheme identifier MUST belong to the ISO 6523 ICD code list."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-138.1", "assert", "matches(.,'^(.{1,50})$')", "The BT-158 does not meet the defined format: alphanumeric with size between 1 and 50."},
				{"DT-CIUS-PT-138.2", "assert", "((not(contains(normalize-space(@listID), ' ')) and contains(' AA AB AC AD AE AF AG AH AI AJ AK AL AM AN AO AP AQ AR AS AT AU AV AW AX AY AZ BA BB BC BD BE BF BG BH BI BJ BK BL BM BN BO BP BQ BR BS BT BU BV BW BX BY BZ CC CG CL CR CV DR DW EC EF EN FS GB GN GS HS IB IN IS IT IZ MA MF MN MP NB ON PD PL PO PV QS RC RN RU RY SA SG SK SN SRS SRT SRU SRV SRW SRX SRY SRZ SS SSA SSB SSC SSD SSE SSF SSG SSH SSI SSJ SSK SSL SSM SSN SSO SSP SSQ SSR SSS SST SSU SSV SSW SSX SSY SSZ ST STA STB STC STD STE STF STG STH STI STJ STK STL STM STN STO STP STQ STR STS STT STU STV STW STX STY STZ SUA SUB SUC SUD SUE SUF SUG SUH SUI SUJ SUK SUL SUM TG TSN TSO TSP UA UP VN VP VS VX ZZZ ', concat(' ', normalize-space(@listID), ' '))))", "The BT-158 identification list must be coded using one of the UNTDID 7143 list."},
				{"DT-CIUS-PT-138.3", "report", "(exists(@listVersionID) and not(matches(@listVersionID,'^(.{1,20})$')))", "The BT-158 identification list version identifier does not meet the defined format: alphanumeric with size between 1 and 20."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:OriginCountry/cbc:IdentificationCode | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:OriginCountry/cbc:IdentificationCode",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"OriginCountry", ""}, {"IdentificationCode", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"OriginCountry", ""}, {"IdentificationCode", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-146", "assert", "((not(contains(normalize-space(.), ' ')) and contains(' AD AE AF AG AI AL AM AN AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BL BJ BM BN BO BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR ST SV SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW ', concat(' ', normalize-space(.), ' '))))", "The BT-159 must be coded using ISO code list 3166-1."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty/cbc:Name | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
			lets: []ptDTLetSrc{
				{"cntLine", "count(ancestor::ubl:Invoice/cac:InvoiceLine | ancestor::cn:CreditNote/cac:CreditNoteLine)"},
				{"cnt139_2", "count(ancestor::ubl:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY)#)$')] | ancestor::cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY)#)$')])"},
				{"cnt139_3", "count(ancestor::ubl:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY)#)$')] | ancestor::cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY)#)$')])"},
				{"cnt139_5", "count(ancestor::ubl:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(LINEID@COMMITMENTLINEREFERENCE)#)$')] | ancestor::cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty/cbc:Name[matches(.,'^(#(LINEID@COMMITMENTLINEREFERENCE)#)$')])"},
			},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-139", "assert", "matches(.,'^(.{1,200})$')", "The BT-160 does not meet the defined format: alphanumeric with size between 1 and 200."},
				{"DT-CIUS-PT-139_1", "report", "starts-with(normalize-space(.),'#ID@INVOICEDOCUMENTREFERENCE@BILLINGREFERENCE-') and not(matches(.,'^(#(ID@INVOICEDOCUMENTREFERENCE@BILLINGREFERENCE)-([0-9]{3})#)$'))", "The BT-160 does not meet the defined format: it is mandatory to fill out with '#ID@INVOICEDOCUMENTREFERENCE@BILLINGREFERENCE-000#'. The sub-suffix \"-000\" must be used as a group of elements."},
				{"DT-CIUS-PT-139_2", "report", "starts-with(normalize-space(.),'#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#') and $cnt139_2 > $cntLine", "The BT-160 when fill out with '#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#', can only exist once."},
				{"DT-CIUS-PT-139_3", "report", "starts-with(normalize-space(.),'#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#') and $cnt139_3 > $cntLine", "The BT-160 when fill out with '#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#', can only exist once."},
				{"DT-CIUS-PT-139_5", "report", "starts-with(normalize-space(.),'#LINEID@COMMITMENTLINEREFERENCE#') and $cnt139_5 > $cntLine", "The BT-160 when fill out with '#LINEID@COMMITMENTLINEREFERENCE#', can only exist once."},
			},
		},
		{
			context: "//ubl:Invoice/cac:InvoiceLine/cac:Item/cac:AdditionalItemProperty/cbc:Value | //cn:CreditNote/cac:CreditNoteLine/cac:Item/cac:AdditionalItemProperty/cbc:Value",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"AdditionalItemProperty", ""}, {"Value", ""}}}, {"CreditNote", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"AdditionalItemProperty", ""}, {"Value", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-140", "assert", "matches(.,'^(.{1,200})$')", "The BT-161 does not meet the defined format: alphanumeric with size between 1 and 200."},
			},
		},
	},
}

// ptConditionPattern is the DT-CIUS-PT-* half of AT/eSPap's condition pattern, resolved through its UBL binding:
// 24 rules carrying 22 fatal DT-CIUS-PT-* assertions, in pattern order. A rule
// with no assertion is here for its context alone, which under ISO Schematron claims its
// nodes away from every rule below it.
var ptConditionPattern = ptDTPattern{
	name: "UBL-condition",
	rules: []ptDTRuleSrc{
		{
			context: "cac:LegalMonetaryTotal/cbc:PayableAmount",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"LegalMonetaryTotal", ""}, {"PayableAmount", ""}}}},
		},
		{
			context: "//ubl:Invoice | //cn:CreditNote",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{}}, {"CreditNote", []ptDTCtxStep{}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-165", "assert", "every $Currency in cbc:DocumentCurrencyCode satisfies ((round( (cac:LegalMonetaryTotal/xs:decimal(cbc:TaxExclusiveAmount) + cac:TaxTotal/xs:decimal(cbc:TaxAmount[@currencyID=$Currency])) * 10 * 10) div 100) <= (cac:LegalMonetaryTotal/xs:decimal(cbc:TaxInclusiveAmount) + 1.00) and (round( (cac:LegalMonetaryTotal/xs:decimal(cbc:TaxExclusiveAmount) + cac:TaxTotal/xs:decimal(cbc:TaxAmount[@currencyID=$Currency])) * 10 * 10) div 100) >= (cac:LegalMonetaryTotal/xs:decimal(cbc:TaxInclusiveAmount) - 1.00))", "Invoice total amount with VAT (BT-112) = Invoice total amount without VAT (BT-109) + Invoice total VAT amount (BT-110), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-176", "assert", "every $taxExemptionReasonCode in //cac:InvoiceLine[cac:Item/cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:Item/cac:ClassifiedTaxCategory/cbc:ID = 'ISE']/cac:Item/cac:AdditionalItemProperty[cbc:Name='#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#']/cbc:Value satisfies ( count(//cac:TaxTotal/cac:TaxSubtotal[(cac:TaxCategory/cbc:ID='E' or cac:TaxCategory/cbc:ID='ISE') and cac:TaxCategory/cbc:TaxExemptionReasonCode = $taxExemptionReasonCode]) = 1)", "An Invoice that contains Invoice lines (BG-25) with diferent VAT exemption reason codes, where the VAT category code (BT-151) is “Exempt from VAT”, shall contain exactly one VATBReakdown (BG-23) with the VAT category code (BT-118) equal to \"Exempt from VAT\", for each VAT exemption reason code (BT-121)."},
				{"DT-CIUS-PT-176", "assert", "every $taxExemptionReasonCode in //cac:CreditNoteLine[cac:Item/cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:Item/cac:ClassifiedTaxCategory/cbc:ID = 'ISE']/cac:Item/cac:AdditionalItemProperty[cbc:Name='#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#']/cbc:Value satisfies ( count(//cac:TaxTotal/cac:TaxSubtotal[(cac:TaxCategory/cbc:ID='E' or cac:TaxCategory/cbc:ID='ISE') and cac:TaxCategory/cbc:TaxExemptionReasonCode = $taxExemptionReasonCode]) = 1)", "An Invoice that contains Invoice lines (BG-25) with diferent VAT exemption reason codes, where the VAT category code (BT-151) is “Exempt from VAT”, shall contain exactly one VATBReakdown (BG-23) with the VAT category code (BT-118) equal to \"Exempt from VAT\", for each VAT exemption reason code (BT-121)."},
			},
		},
		{
			context: "cac:InvoiceLine | cac:CreditNoteLine",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-157", "assert", "((not(exists(cac:Price/cbc:BaseQuantity)) or ((exists(cac:Price/cbc:BaseQuantity)) and cac:Price/xs:decimal(cbc:BaseQuantity) = 0 )) and ((exists(cac:AllowanceCharge[cbc:ChargeIndicator='false']) and exists(cac:AllowanceCharge[cbc:ChargeIndicator='true']) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='false'])) and exists(cac:AllowanceCharge[cbc:ChargeIndicator='true']) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (exists(cac:AllowanceCharge[cbc:ChargeIndicator='false']) and not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='true'])) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='false'])) and not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='true'])) and (round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and (round(xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) * 100) div 100) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)))) or (exists(cac:Price/cbc:BaseQuantity) and (cac:Price/xs:decimal(cbc:BaseQuantity) > 0) and ((exists(cac:AllowanceCharge[cbc:ChargeIndicator='false']) and exists(cac:AllowanceCharge[cbc:ChargeIndicator='true']) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='false'])) and exists(cac:AllowanceCharge[cbc:ChargeIndicator='true']) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) + (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (exists(cac:AllowanceCharge[cbc:ChargeIndicator='false']) and not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='true'])) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and ((round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) - (round(sum(cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100)) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or (not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='false'])) and not(exists(cac:AllowanceCharge[cbc:ChargeIndicator='true'])) and (round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and (round((xs:decimal(cbc:InvoicedQuantity | cbc:CreditedQuantity) * cac:Price/xs:decimal(cbc:PriceAmount) div cac:Price/xs:decimal(cbc:BaseQuantity)) * 100) div 100) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00))))", "The BT-131 must equal the multiplication between the quantity (InvoicedQuantity) and the price (PriceAmount) divided by the base quantity (BaseQuantity) subtracted by the sum of the discounts and added the sum of the charges of the line, with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-177", "assert", "((cac:Price/cbc:BaseQuantity) > 0) or not(exists(cac:Price/cbc:BaseQuantity))", "The Item price base quantity (BT-149) shall be greater than zero."},
			},
		},
		{
			context: "cac:PayeeParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PayeeParty", ""}}}},
		},
		{
			context: "//ubl:Invoice/cac:TaxTotal | //cn:CreditNote/cac:TaxTotal",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"TaxTotal", ""}}}, {"CreditNote", []ptDTCtxStep{{"TaxTotal", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-164", "assert", "((round((sum(cac:TaxSubtotal/xs:decimal(cbc:TaxAmount)) * 10 * 10)) div 100) <= (xs:decimal(child::cbc:TaxAmount) + 1.00) and (round((sum(cac:TaxSubtotal/xs:decimal(cbc:TaxAmount)) * 10 * 10)) div 100) >= (xs:decimal(child::cbc:TaxAmount) - 1.00)) or not(cac:TaxSubtotal)", "Invoice total VAT amount (BT-110) = Σ VAT category tax amount (BT-117), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-175", "assert", "(exists(//cac:InvoiceLine) and ((sum(//cac:TaxTotal/cac:TaxSubtotal[normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:TaxableAmount)) - 1.00) <= (round((sum(//cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='E' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='ISE']/xs:decimal(cbc:LineExtensionAmount)) + sum(//cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount)) - sum(//cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount))) * 100) div 100)) and ((sum(//cac:TaxTotal/cac:TaxSubtotal[normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:TaxableAmount)) + 1.00) >= (round((sum(//cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='E' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='ISE']/xs:decimal(cbc:LineExtensionAmount)) + sum(//cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount)) - sum(//cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount))) * 100) div 100))) or (exists(//cac:CreditNoteLine) and ((sum(//cac:TaxTotal/cac:TaxSubtotal[normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:TaxableAmount)) - 1.00) <= (round((sum(//cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='E' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='ISE']/xs:decimal(cbc:LineExtensionAmount)) + sum(//cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount)) - sum(//cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount))) * 100) div 100)) and ((sum(//cac:TaxTotal/cac:TaxSubtotal[normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:TaxableAmount)) + 1.00) >= (round((sum(//cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='E' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='ISE']/xs:decimal(cbc:LineExtensionAmount)) + sum(//cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount)) - sum(//cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID) = 'E' or normalize-space(cac:TaxCategory/cbc:ID) = 'ISE']/xs:decimal(cbc:Amount))) * 100) div 100)))", "In a VATBReakdown (BG-23) where the VAT category code (BT-118) is \"Exempt from VAT\" the VAT category taxable amount (BT-116) shall equal the sum of Invoice line net amounts (BT-131) minus the sum of Document level allowance amounts (BT-92) plus the sum of Document level charge amounts (BT-99) where the VAT category codes (BT-151, BT-95, BT-102) are “Exempt from VAT\", with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:LegalMonetaryTotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"LegalMonetaryTotal", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-160", "assert", "((round(sum(//cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) * 100) div 100) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and (round(sum(//cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) * 100) div 100) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00)) or ((round(sum(//cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) * 100) div 100) <= (xs:decimal(cbc:LineExtensionAmount) + 1.00) and (round(sum(//cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) * 100) div 100) >= (xs:decimal(cbc:LineExtensionAmount) - 1.00))", "Sum of Invoice line net amount (BT-106) = Σ Invoice line net amount (BT-131), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-161", "assert", "(exists(../cac:AllowanceCharge[cbc:ChargeIndicator='false']) and (round(sum(../cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) <= (xs:decimal(cbc:AllowanceTotalAmount) + 1.00) and (round(sum(../cac:AllowanceCharge[cbc:ChargeIndicator='false']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) >= (xs:decimal(cbc:AllowanceTotalAmount) - 1.00)) or (not(exists(../cac:AllowanceCharge[cbc:ChargeIndicator='false'])) and xs:decimal(cbc:AllowanceTotalAmount) = 0.00) or not(cbc:AllowanceTotalAmount)", "Sum of allowances on document level (BT-107) = Σ Document level allowance amount (BT-92), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-162", "assert", "(exists(../cac:AllowanceCharge[cbc:ChargeIndicator='true']) and ((round(sum(../cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) <= (xs:decimal(cbc:ChargeTotalAmount) + 1.00) and (round(sum(../cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount)) * 10 * 10) div 100) >= (xs:decimal(cbc:ChargeTotalAmount) - 1.00))) or (not(exists(../cac:AllowanceCharge[cbc:ChargeIndicator='true'])) and xs:decimal(cbc:ChargeTotalAmount) = 0.00) or not(cbc:ChargeTotalAmount)", "Sum of charges on document level (BT-108) = Σ Document level charge amount (BT-99), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-163", "assert", "(exists(../cac:InvoiceLine) and ((cbc:ChargeTotalAmount) and (cbc:AllowanceTotalAmount) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10) div 100 ) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10) div 100 ) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00)) or ( not(cbc:ChargeTotalAmount) and (cbc:AllowanceTotalAmount) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10 ) div 100) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10 ) div 100) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00) ) or ( (cbc:ChargeTotalAmount) and not(cbc:AllowanceTotalAmount) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount)) * 10 * 10 ) div 100) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount)) * 10 * 10 ) div 100) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00)) or ( not(cbc:ChargeTotalAmount) and not(cbc:AllowanceTotalAmount) and sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and sum(../cac:InvoiceLine/xs:decimal(cbc:LineExtensionAmount)) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00))) or (exists(../cac:CreditNoteLine) and ((cbc:ChargeTotalAmount) and (cbc:AllowanceTotalAmount) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10) div 100 ) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10) div 100 ) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00)) or ( not(cbc:ChargeTotalAmount) and (cbc:AllowanceTotalAmount) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10 ) div 100) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) - xs:decimal(cbc:AllowanceTotalAmount)) * 10 * 10 ) div 100) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00) ) or ( (cbc:ChargeTotalAmount) and not(cbc:AllowanceTotalAmount) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount)) * 10 * 10 ) div 100) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and (round((sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) + xs:decimal(cbc:ChargeTotalAmount)) * 10 * 10 ) div 100) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00)) or ( not(cbc:ChargeTotalAmount) and not(cbc:AllowanceTotalAmount) and sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) <= (xs:decimal(cbc:TaxExclusiveAmount) + 1.00) and sum(../cac:CreditNoteLine/xs:decimal(cbc:LineExtensionAmount)) >= (xs:decimal(cbc:TaxExclusiveAmount) - 1.00)))", "Invoice total amount without VAT (BT-109) = Σ Invoice line net amount (BT-131) - Sum of allowances on document level (BT-107) + Sum of charges on document level (BT-108), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-166", "assert", "(xs:decimal(cbc:PrepaidAmount) and not(xs:decimal(cbc:PayableRoundingAmount)) and (round((xs:decimal(cbc:TaxInclusiveAmount) - xs:decimal(cbc:PrepaidAmount)) * 10 * 10) div 100) <= (xs:decimal(cbc:PayableAmount) + 1.00) and (round((xs:decimal(cbc:TaxInclusiveAmount) - xs:decimal(cbc:PrepaidAmount)) * 10 * 10) div 100) >= (xs:decimal(cbc:PayableAmount) - 1.00)) or (not(xs:decimal(cbc:PrepaidAmount)) and not(xs:decimal(cbc:PayableRoundingAmount)) and xs:decimal(cbc:TaxInclusiveAmount) <= (xs:decimal(cbc:PayableAmount) + 1.00) and xs:decimal(cbc:TaxInclusiveAmount) >= (xs:decimal(cbc:PayableAmount) - 1.00)) or (xs:decimal(cbc:PrepaidAmount) and xs:decimal(cbc:PayableRoundingAmount) and ((round((xs:decimal(cbc:PayableAmount) - xs:decimal(cbc:PayableRoundingAmount)) * 10 * 10) div 100) <= ((round((xs:decimal(cbc:TaxInclusiveAmount) - xs:decimal(cbc:PrepaidAmount)) * 10 * 10) div 100) + 1.00)) and ((round((xs:decimal(cbc:PayableAmount) - xs:decimal(cbc:PayableRoundingAmount)) * 10 * 10) div 100) >= ((round((xs:decimal(cbc:TaxInclusiveAmount) - xs:decimal(cbc:PrepaidAmount)) * 10 * 10) div 100) - 1.00))) or (not(xs:decimal(cbc:PrepaidAmount)) and xs:decimal(cbc:PayableRoundingAmount) and (round((xs:decimal(cbc:PayableAmount) - xs:decimal(cbc:PayableRoundingAmount)) * 10 * 10) div 100) <= (xs:decimal(cbc:TaxInclusiveAmount) + 1.00) and (round((xs:decimal(cbc:PayableAmount) - xs:decimal(cbc:PayableRoundingAmount)) * 10 * 10) div 100) >= (xs:decimal(cbc:TaxInclusiveAmount) - 1.00))", "Amount due for payment (BT-115) = Invoice total amount with VAT (BT-112) -Paid amount (BT-113) +Rounding amount (BT-114), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-159", "assert", "(not(exists(xs:decimal(cbc:MultiplierFactorNumeric))) and not(exists(xs:decimal(cbc:BaseAmount)))) or (((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) <= (xs:decimal(cbc:Amount) + 1.00) and ((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) >= (xs:decimal(cbc:Amount) - 1.00))", "The BT-99 must equal the multiplication between the Document level charge base amount (BT-100) and the Document level charge percentage (BT-101), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-158", "assert", "(not(exists(xs:decimal(cbc:MultiplierFactorNumeric))) and not(exists(xs:decimal(cbc:BaseAmount)))) or (((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) <= (xs:decimal(cbc:Amount) + 1.00) and ((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) >= (xs:decimal(cbc:Amount) - 1.00))", "The BT-92 must equal the multiplication between the Document level allowance base amount (BT-93) and the Document level allowance percentage (BT-94), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-167", "assert", "(round(cac:TaxCategory/xs:decimal(cbc:Percent)) = 0 and (round(xs:decimal(cbc:TaxAmount)) = 0)) or (round(cac:TaxCategory/xs:decimal(cbc:Percent)) != 0 and ((round(xs:decimal(cbc:TaxableAmount) * (cac:TaxCategory/xs:decimal(cbc:Percent) div 100) * 10 * 10) div 100 ) <= (xs:decimal(cbc:TaxAmount) + 1.00)) and ((round(xs:decimal(cbc:TaxableAmount) * (cac:TaxCategory/xs:decimal(cbc:Percent) div 100) * 10 * 10) div 100 ) >= (xs:decimal(cbc:TaxAmount) - 1.00))) or (not(exists(cac:TaxCategory/xs:decimal(cbc:Percent))) and (round(xs:decimal(cbc:TaxAmount)) = 0))", "VAT category tax amount (BT-117) = VAT category taxable amount (BT-116) x (VAT category rate (BT-119) / 100), rounded to two decimals and with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-171", "assert", "every $rate in round(cbc:Percent) satisfies ((exists(//cac:InvoiceLine) and ((round((sum(../../../cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='AA' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='RED' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='INT'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) <= (xs:decimal(../cbc:TaxableAmount) + 1.00) and (round((sum(../../../cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='AA' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='RED' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='INT'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) >=(xs:decimal(../cbc:TaxableAmount) - 1.00))) or (exists(//cac:CreditNoteLine) and ( ((round((sum(../../../cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='AA' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='RED' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='INT'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) <= (xs:decimal(../cbc:TaxableAmount) + 1.00) and (round((sum(../../../cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='AA' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='RED' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='INT'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='AA' or normalize-space(cac:TaxCategory/cbc:ID)='RED' or normalize-space(cac:TaxCategory/cbc:ID)='INT'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) >=(xs:decimal(../cbc:TaxableAmount) - 1.00)))))", "For each different value of VAT category rate (BT-119) where the VAT category code (BT-118) is \"Lower rate\", the VAT category taxable amount (BT-116) in a VATBReakdown (BG-23) shall equal the sum of Invoice line net amounts (BT-131) plus the sum of document level charge amounts (BT-99) minus the sum of document level allowance amounts (BT-92) where the VAT category code (BT-151, BT-102, BT-95) is “Lower rate” and the VAT rate (BT-152, BT-103, BT-96) equals the VAT category rate (BT-119), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-172", "assert", "((round((xs:decimal(../cbc:TaxableAmount) * (xs:decimal(cbc:Percent) div 100)) * 10 * 10) div 100) <= (xs:decimal(../cbc:TaxAmount) + 1.00) and (round((xs:decimal(../cbc:TaxableAmount) * (xs:decimal(cbc:Percent) div 100)) * 10 * 10) div 100) >= (xs:decimal(../cbc:TaxAmount) - 1.00))", "The VAT category tax amount (BT-117) in a VATBReakdown (BG-23) where VAT category code (BT-118) is \"Lower rate\" shall equal the VAT category taxable amount (BT-116) multiplied by the VAT category rate (BT-119), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='false'"}, {"TaxCategory", "normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'"}}}},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='true'"}, {"TaxCategory", "normalize-space(cbc:ID)='AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'"}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-173", "assert", "every $rate in round(cbc:Percent) satisfies ((exists(//cac:InvoiceLine) and ((round((sum(../../../cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='S' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='NOR'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) <= (xs:decimal(../cbc:TaxableAmount) + 1.00) and (round((sum(../../../cac:InvoiceLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='S' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='NOR'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) >=(xs:decimal(../cbc:TaxableAmount) - 1.00))) or (exists(//cac:CreditNoteLine) and ( ((round((sum(../../../cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='S' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='NOR'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) <= (xs:decimal(../cbc:TaxableAmount) + 1.00) and (round((sum(../../../cac:CreditNoteLine[normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='S' or normalize-space(cac:Item/cac:ClassifiedTaxCategory/cbc:ID)='NOR'][cac:Item/cac:ClassifiedTaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:LineExtensionAmount)) + sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='true'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount)) - sum(../../../cac:AllowanceCharge[cbc:ChargeIndicator='false'][normalize-space(cac:TaxCategory/cbc:ID)='S' or normalize-space(cac:TaxCategory/cbc:ID)='NOR'][cac:TaxCategory/cbc:Percent=$rate]/xs:decimal(cbc:Amount))) * 100) div 100) >=(xs:decimal(../cbc:TaxableAmount) - 1.00)))))", "For each different value of VAT category rate (BT-119) where the VAT category code (BT-118) is \"Standard rated\", the VAT category taxable amount (BT-116) in a VATBReakdown (BG-23) shall equal the sum of Invoice line net amounts (BT-131) plus the sum of document level charge amounts (BT-99) minus the sum of document level allowance amounts (BT-92) where the VAT category code (BT-151, BT-102, BT-95) is “Standard rated” and the VAT rate (BT-152, BT-103, BT-96) equals the VAT category rate (BT-119), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
				{"DT-CIUS-PT-174", "assert", "((round((xs:decimal(../cbc:TaxableAmount) * (xs:decimal(cbc:Percent) div 100)) * 10 * 10) div 100) <= (xs:decimal(../cbc:TaxAmount) + 1.00) and (round((xs:decimal(../cbc:TaxableAmount) * (xs:decimal(cbc:Percent) div 100)) * 10 * 10) div 100) >= (xs:decimal(../cbc:TaxAmount) - 1.00))", "The VAT category tax amount (BT-117) in a VATBReakdown (BG-23) where VAT category code (BT-118) is \"Standard rated\" shall equal the VAT category taxable amount (BT-116) multiplied by the VAT category rate (BT-119), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[normalize-space(cbc:ID)='S' or normalize-space(cbc:ID) = 'NOR']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='false'"}, {"TaxCategory", "normalize-space(cbc:ID)='S' or normalize-space(cbc:ID) = 'NOR'"}}}},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[normalize-space(cbc:ID)='S' or normalize-space(cbc:ID) = 'NOR']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='true'"}, {"TaxCategory", "normalize-space(cbc:ID)='S' or normalize-space(cbc:ID) = 'NOR'"}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'"}}}},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='false'"}, {"TaxCategory", "normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'"}}}},
		},
		{
			context: "cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator='true'"}, {"TaxCategory", "normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-169", "assert", "(not(exists(xs:decimal(cbc:MultiplierFactorNumeric))) and not(exists(xs:decimal(cbc:BaseAmount)))) or (((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) <= (xs:decimal(cbc:Amount) + 1.00) and ((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) >= (xs:decimal(cbc:Amount) - 1.00))", "The BT-141 must equal the multiplication between the Invoice line charge base amount (BT-142) and the Invoice line charge percentage (BT-143), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-168", "assert", "(not(exists(xs:decimal(cbc:MultiplierFactorNumeric))) and not(exists(xs:decimal(cbc:BaseAmount)))) or (((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) <= (xs:decimal(cbc:Amount) + 1.00) and ((xs:decimal(cbc:BaseAmount) * (xs:decimal(cbc:MultiplierFactorNumeric) div 100) * 10 * 10) div 100) >= (xs:decimal(cbc:Amount) - 1.00))", "The BT-136 must equal the multiplication between the Invoice line allowance base amount (BT-137) and the Invoice line allowance percentage (BT-138), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Price | cac:CreditNoteLine/cac:Price",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}}}},
			asserts: []ptDTAssertSrc{
				{"DT-CIUS-PT-170", "assert", "(not(exists(cac:AllowanceCharge/xs:decimal(cbc:BaseAmount))) and not(exists(cac:AllowanceCharge/xs:decimal(cbc:Amount)))) or ((cac:AllowanceCharge/xs:decimal(cbc:BaseAmount) - (cac:AllowanceCharge/xs:decimal(cbc:Amount))) <= (xs:decimal(cbc:PriceAmount) + 1.00) and (cac:AllowanceCharge/xs:decimal(cbc:BaseAmount) - (cac:AllowanceCharge/xs:decimal(cbc:Amount))) >= (xs:decimal(cbc:PriceAmount) - 1.00))", "The BT-146 must equal the grossPrice (Item gross price) minus the discount (Item price discount), with an acceptance range of 1.00 € (it does not mean that this tolerance is accepted by the customer)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'] | cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT'"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR'] | cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR'"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'] | cac:CreditNoteLine/cac:Item/cac:ClassifiedTaxCategory[normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"ClassifiedTaxCategory", "normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE'"}}}},
		},
	},
}
