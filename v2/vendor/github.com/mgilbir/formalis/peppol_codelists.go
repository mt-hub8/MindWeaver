package formalis

// The code lists OpenPEPPOL restricts EN 16931's to, transcribed from the
// `<let>` variables of the vendored Schematron.
//
// Peppol does not publish these as genericode the way CEN publishes EN 16931's
// (testdata/en16931-codelists/gen.py derives en16931_codelists.go from that
// bundle). It ships them inline, as XPath `tokenize('AED AFN ALL ...', '\s')`
// sequences inside PEPPOL-EN16931-UBL.sch and PEPPOL-EN16931-CII.sch — which
// makes the artefact this repository already vendors the source of truth, and
// TestPeppolCodeListsMatchTheSchematron reads every one of them back out with an
// XML decoder and fails on any divergence. That is the same fidelity contract
// en16931_codelists_test.go holds the generated tables to, and it is why these are
// hand-written rather than generated: the derivation is a `tokenize` argument in a
// file the test suite already opens.
//
// Both bindings carry byte-identical lists — the test asserts that too, so a
// binding that starts to diverge upstream cannot be absorbed silently into one
// table here.
//
// Three of the six lists Peppol restricts are the same set CEN publishes, and
// this file does not copy them: the MIME code list, the UNCL 5189 allowance
// reason list and the UNCL 7161 charge reason list are exactly en16931MIME,
// en16931AllowanceReasons and en16931ChargeReasons. PEPPOL-EN16931-CL001/CL002/
// CL003 therefore restrict nothing BR-CL-24/BR-CL-19/BR-CL-20 do not, and a
// document that trips one trips both, under both identifiers, because both
// authorities publish the rule. The reuse is asserted rather than assumed, in the
// same test, so an upstream change that made Peppol's list narrower would fail
// here instead of quietly widening what CL001 accepts.
//
// The other three are genuinely Peppol's own and differ from CEN's:
//
//   - the currency list has ANG, BGN and STD, which EN 16931's does not, and
//     lacks STN and XCG, which it does. So an invoice in São Tomé's current
//     currency (STN) satisfies BR-CL-03 and fails PEPPOL-EN16931-CL007, and one
//     in the withdrawn STD does the reverse. Peppol's list is the rule Peppol
//     publishes; the divergence is upstream's and is not this package's to
//     reconcile.
//   - the electronic address scheme list is narrower: EN 16931 has 0242, 0246,
//     0248 and the five two-letter legacy schemes (AN, AQ, AS, AU, EM) that
//     Peppol does not accept.
//   - the invoice period description code list (UNCL 2005) has no EN 16931
//     counterpart this package tabulates at all.

// peppolCurrencies is Peppol's ISO 4217 currency list (PEPPOL-EN16931-CL007).
var peppolCurrencies = map[string]bool{
	"AED": true, "AFN": true, "ALL": true, "AMD": true, "ANG": true, "AOA": true, "ARS": true, "AUD": true, "AWG": true, "AZN": true,
	"BAM": true, "BBD": true, "BDT": true, "BGN": true, "BHD": true, "BIF": true, "BMD": true, "BND": true, "BOB": true, "BOV": true,
	"BRL": true, "BSD": true, "BTN": true, "BWP": true, "BYN": true, "BZD": true, "CAD": true, "CDF": true, "CHE": true, "CHF": true,
	"CHW": true, "CLF": true, "CLP": true, "CNY": true, "COP": true, "COU": true, "CRC": true, "CUP": true, "CVE": true, "CZK": true,
	"DJF": true, "DKK": true, "DOP": true, "DZD": true, "EGP": true, "ERN": true, "ETB": true, "EUR": true, "FJD": true, "FKP": true,
	"GBP": true, "GEL": true, "GHS": true, "GIP": true, "GMD": true, "GNF": true, "GTQ": true, "GYD": true, "HKD": true, "HNL": true,
	"HTG": true, "HUF": true, "IDR": true, "ILS": true, "INR": true, "IQD": true, "IRR": true, "ISK": true, "JMD": true, "JOD": true,
	"JPY": true, "KES": true, "KGS": true, "KHR": true, "KMF": true, "KPW": true, "KRW": true, "KWD": true, "KYD": true, "KZT": true,
	"LAK": true, "LBP": true, "LKR": true, "LRD": true, "LSL": true, "LYD": true, "MAD": true, "MDL": true, "MGA": true, "MKD": true,
	"MMK": true, "MNT": true, "MOP": true, "MRU": true, "MUR": true, "MVR": true, "MWK": true, "MXN": true, "MXV": true, "MYR": true,
	"MZN": true, "NAD": true, "NGN": true, "NIO": true, "NOK": true, "NPR": true, "NZD": true, "OMR": true, "PAB": true, "PEN": true,
	"PGK": true, "PHP": true, "PKR": true, "PLN": true, "PYG": true, "QAR": true, "RON": true, "RSD": true, "RUB": true, "RWF": true,
	"SAR": true, "SBD": true, "SCR": true, "SDG": true, "SEK": true, "SGD": true, "SHP": true, "SLE": true, "SOS": true, "SRD": true,
	"SSP": true, "STD": true, "SVC": true, "SYP": true, "SZL": true, "THB": true, "TJS": true, "TMT": true, "TND": true, "TOP": true,
	"TRY": true, "TTD": true, "TWD": true, "TZS": true, "UAH": true, "UGX": true, "USD": true, "USN": true, "UYI": true, "UYU": true,
	"UYW": true, "UZS": true, "VED": true, "VES": true, "VND": true, "VUV": true, "WST": true, "XAF": true, "XAG": true, "XAU": true,
	"XBA": true, "XBB": true, "XBC": true, "XBD": true, "XCD": true, "XDR": true, "XOF": true, "XPD": true, "XPF": true, "XPT": true,
	"XSU": true, "XTS": true, "XUA": true, "YER": true, "ZAR": true, "ZMW": true, "ZWG": true, "XXX": true, "CNH": true,
}

// peppolEAS is Peppol's electronic address scheme list (PEPPOL-EN16931-CL008).
var peppolEAS = map[string]bool{
	"0002": true, "0007": true, "0009": true, "0037": true, "0060": true, "0088": true, "0096": true, "0097": true, "0106": true, "0130": true,
	"0135": true, "0142": true, "0147": true, "0151": true, "0154": true, "0158": true, "0170": true, "0177": true, "0183": true, "0184": true,
	"0188": true, "0190": true, "0191": true, "0192": true, "0193": true, "0194": true, "0195": true, "0196": true, "0198": true, "0199": true,
	"0200": true, "0201": true, "0202": true, "0203": true, "0204": true, "0205": true, "0208": true, "0209": true, "0210": true, "0211": true,
	"0212": true, "0213": true, "0215": true, "0216": true, "0217": true, "0218": true, "0221": true, "0225": true, "0230": true, "0235": true,
	"0240": true, "0244": true, "0245": true, "9910": true, "9913": true, "9914": true, "9915": true, "9918": true, "9919": true, "9920": true,
	"9922": true, "9923": true, "9924": true, "9925": true, "9926": true, "9927": true, "9928": true, "9929": true, "9930": true, "9931": true,
	"9932": true, "9933": true, "9934": true, "9935": true, "9936": true, "9937": true, "9938": true, "9939": true, "9940": true, "9941": true,
	"9942": true, "9943": true, "9944": true, "9945": true, "9946": true, "9947": true, "9948": true, "9949": true, "9950": true, "9951": true,
	"9952": true, "9953": true, "9957": true, "9959": true,
}

// peppolPeriodDescCodes is the UNCL 2005 invoice period description code list
// (PEPPOL-EN16931-CL006).
var peppolPeriodDescCodes = map[string]bool{"3": true, "35": true, "432": true}

// peppolInvoiceTypeCodes is the UNTDID 1001 set profile 01 permits on a UBL
// Invoice (PEPPOL-EN16931-P0100 in the UBL binding).
var peppolInvoiceTypeCodes = map[string]bool{
	"71": true, "80": true, "82": true, "84": true, "102": true, "218": true, "219": true, "326": true, "331": true, "380": true,
	"382": true, "383": true, "384": true, "386": true, "388": true, "393": true, "395": true, "553": true, "575": true, "623": true,
	"780": true, "817": true, "870": true, "875": true, "876": true, "877": true,
}

// peppolCreditNoteTypeCodes is the same for a UBL CreditNote
// (PEPPOL-EN16931-P0101, which the CII binding does not publish).
var peppolCreditNoteTypeCodes = map[string]bool{
	"381": true, "396": true, "81": true, "83": true, "532": true,
}

// peppolCIITypeCodes is the list the CII binding's PEPPOL-EN16931-P0100 uses. CII
// expresses an invoice and a credit note with one root and one BT-3, so Peppol
// checks the document type code against the union of the two UBL lists rather
// than against one of them.
var peppolCIITypeCodes = map[string]bool{
	"71": true, "102": true, "218": true, "219": true, "326": true, "331": true, "382": true, "553": true, "817": true, "870": true,
	"875": true, "876": true, "877": true, "380": true, "383": true, "384": true, "386": true, "388": true, "393": true, "82": true,
	"80": true, "84": true, "395": true, "575": true, "623": true, "780": true, "381": true, "396": true, "81": true, "83": true,
	"532": true,
}
