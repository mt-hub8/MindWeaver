package formalis

import "strings"

// The PEPPOL-COMMON-* family: twelve format checks on a participant identifier,
// one per ISO 6523 scheme OpenPEPPOL validates the shape of.
//
// They are the same twelve rules in both bindings, over different element names —
// cbc:EndpointID, cac:PartyIdentification/cbc:ID and cbc:CompanyID in UBL,
// ram:URIID, ram:ID and ram:GlobalID in CII — so the rule bodies are shared and
// each binding's walk hands over the node population it found.
//
// The check-digit algorithms are transcribed from the xsl:function declarations of
// the same Schematron files, which is where OpenPEPPOL defines them: u:gln,
// u:mod11, u:mod97-0208, u:checkCodiceIPA, u:checkCF (and u:checkCF16),
// u:checkPIVAseIT (and u:checkPIVA, u:addPIVA), u:checkSEOrgnr and u:abn. Each Go
// function names the one it came from, because several of them are not the standard
// algorithm they resemble: u:checkCF16's twelfth to fourteenth characters are
// "castable as xs:string", which is true of anything, and u:abn subtracts 49 rather
// than 48 from its first codepoint.
//
// The whole family fires on a value and never on its absence: a document that uses
// none of the twelve schemes answers to none of these rules. That is why 7 of the
// 12 are silent over all 1,690 corpus documents, and it is not mis-wiring —
// OpenPEPPOL ships a test set for each, and TestEveryPublishedPeppolRuleHasBothVerdicts
// requires every one of them to have a document that trips it.

// peppolSchemeCheck is one PEPPOL-COMMON-* rule: the ISO 6523 scheme identifier
// it is bound to, the identifier it reports, whether it is bound to the electronic
// address alone, and the format test.
type peppolSchemeCheck struct {
	scheme string
	rule   string
	// endpointOnly marks the two rules whose context is the electronic address
	// element by itself — cbc:EndpointID / ram:URIID — rather than the union of
	// the three identifier elements the rest share.
	endpointOnly bool
	// raw is whether the test reads the element's string value untouched. Three of
	// them use string() and the rest normalize-space(), and the difference decides
	// whether a padded identifier passes.
	raw bool
	ok  func(string) bool
}

// peppolSchemeChecks is the PEPPOL-COMMON-* family, one entry per identifier
// scheme Peppol validates the format of. The check digit algorithms are
// transcribed from the xsl:function declarations of the same Schematron files —
// u:gln, u:mod11, u:mod97-0208, u:checkCodiceIPA, u:checkCF, u:checkPIVAseIT,
// u:checkSEOrgnr and u:abn — and each function's Go counterpart cites it.
var peppolSchemeChecks = []peppolSchemeCheck{
	{scheme: "0088", rule: "PEPPOL-COMMON-R040", ok: func(s string) bool { return peppolAllDigits(s) && peppolGLN(s) }},
	{scheme: "0192", rule: "PEPPOL-COMMON-R041", ok: func(s string) bool { return len(s) == 9 && peppolAllDigits(s) && peppolMod11(s) }},
	{scheme: "0184", rule: "PEPPOL-COMMON-R042", raw: true, ok: peppolDanishCVR},
	{scheme: "0208", rule: "PEPPOL-COMMON-R043", ok: func(s string) bool { return len(s) == 10 && peppolAllDigits(s) && peppolMod97BE(s) }},
	{scheme: "0201", rule: "PEPPOL-COMMON-R044", ok: peppolCodiceIPA},
	{scheme: "0210", rule: "PEPPOL-COMMON-R045", ok: peppolCodiceFiscale},
	{scheme: "9907", rule: "PEPPOL-COMMON-R046", endpointOnly: true, ok: peppolCodiceFiscale},
	{scheme: "0211", rule: "PEPPOL-COMMON-R047", ok: peppolPartitaIVA},
	{scheme: "0007", rule: "PEPPOL-COMMON-R049", ok: func(s string) bool { return len(s) == 10 && peppolAllDigits(s) && peppolSwedishOrgNr(s) }},
	{scheme: "0151", rule: "PEPPOL-COMMON-R050", ok: peppolABN},
	{scheme: "0096", rule: "PEPPOL-COMMON-R052", raw: true, ok: func(s string) bool { return len(s) == 10 && peppolAllDigits(s) }},
	{scheme: "0198", rule: "PEPPOL-COMMON-R053", raw: true, ok: peppolDanishSE},
}

// peppolCommonIdentifierRules evaluates the PEPPOL-COMMON-* family. schemeIDs is
// the union of the three identifier elements each binding names — cbc:EndpointID,
// cac:PartyIdentification/cbc:ID and cbc:CompanyID in UBL, ram:URIID, ram:ID and
// ram:GlobalID in CII — and endpoints is the electronic address element alone,
// which two of the rules are bound to instead.
//
// Every rule in the family is a format check on an identifier declared under one
// scheme, so a document that uses none of the twelve schemes answers to none of
// them: the family fires on the value, never on its absence.
func peppolCommonIdentifierRules(e *peppolEval, schemeIDs, endpoints []*ciiNode) {
	for _, c := range peppolSchemeChecks {
		if !e.has(c.rule) {
			continue
		}
		nodes := schemeIDs
		if c.endpointOnly {
			nodes = endpoints
		}
		for _, n := range nodes {
			if n.attr("schemeID") != c.scheme {
				continue
			}
			val := strings.Join(strings.Fields(n.text), " ")
			if c.raw {
				val = n.rawText()
			}
			if !c.ok(val) {
				e.addf(c.rule, "The identifier %q declared under scheme %s is not in the format that scheme requires", val, c.scheme)
			}
		}
	}
}

// peppolAllDigits reports whether s is one or more ASCII digits, which is the
// `matches(normalize-space(), '^[0-9]+$')` guard three of the checks carry and the
// `castable as xs:integer` test two more use.
func peppolAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// peppolGLN is u:gln: the GS1 check digit over every character but the last,
// weighted 3 and 1 alternately from the right.
func peppolGLN(v string) bool {
	if v == "" {
		return false
	}
	body, check := v[:len(v)-1], int(v[len(v)-1]-'0')
	sum := 0
	for i := 0; i < len(body); i++ {
		d := int(body[len(body)-1-i] - '0')
		weight := 1
		if (i+1)%2 == 1 {
			weight = 3
		}
		sum += d * weight
	}
	return (10-sum%10)%10 == check
}

// peppolMod11 is u:mod11, the Norwegian organisation number check: the body
// weighted 2,3,4,5,6,7 cyclically from the right, and the value must be positive.
func peppolMod11(v string) bool {
	if v == "" {
		return false
	}
	body, check := v[:len(v)-1], int(v[len(v)-1]-'0')
	sum := 0
	for i := 0; i < len(body); i++ {
		sum += int(body[len(body)-1-i]-'0') * ((i % 6) + 2)
	}
	if strings.Trim(v, "0") == "" {
		return false // number($val) > 0
	}
	return (11-sum%11)%11 == check
}

// peppolMod97BE is u:mod97-0208, the Belgian enterprise number check: the last two
// digits are 97 less the first eight taken modulo 97.
func peppolMod97BE(v string) bool {
	if len(v) != 10 {
		return false
	}
	head := 0
	for i := 0; i < 8; i++ {
		head = head*10 + int(v[i]-'0')
	}
	check := (int(v[8]-'0'))*10 + int(v[9]-'0')
	return 97-head%97 == check
}

// peppolDanishCVR is PEPPOL-COMMON-R042's own test, which has no helper function:
// ten characters beginning "DK" followed by eight digits, or eight digits.
func peppolDanishCVR(v string) bool {
	if len(v) == 10 && v[:2] == "DK" && peppolAllDigits(v[2:]) {
		return true
	}
	return len(v) == 8 && peppolAllDigits(v)
}

// peppolDanishSE is PEPPOL-COMMON-R053's test: ten characters, "DK" and eight
// digits. Unlike R042 there is no bare-eight-digit alternative.
func peppolDanishSE(v string) bool {
	return len(v) == 10 && v[:2] == "DK" && peppolAllDigits(v[2:])
}

// peppolCodiceIPA is u:checkCodiceIPA: six ASCII alphanumerics.
func peppolCodiceIPA(v string) bool {
	if len(v) != 6 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}

// peppolCodiceFiscale is u:checkCF: eleven digits, or the sixteen-character form
// u:checkCF16 describes — six letters, two digits, a letter, two digits, three
// characters of any kind, a digit and a letter.
func peppolCodiceFiscale(v string) bool {
	letters := func(s string) bool {
		for i := 0; i < len(s); i++ {
			c := s[i]
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
				return false
			}
		}
		return len(s) > 0
	}
	switch len(v) {
	case 11:
		return peppolCastableAsInteger(v)
	case 16:
		return letters(v[0:6]) && peppolCastableAsInteger(v[6:8]) && letters(v[8:9]) &&
			peppolCastableAsInteger(v[9:11]) && peppolCastableAsInteger(v[14:15]) && letters(v[15:16])
	}
	return false
}

// peppolCastableAsInteger is XPath's `castable as xs:integer`: an optional sign
// and one or more digits.
func peppolCastableAsInteger(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	return peppolAllDigits(s)
}

// peppolPartitaIVA is u:checkPIVAseIT: a value that does not begin "IT" is
// accepted unchanged, and one that does must carry eleven digits passing the
// Luhn-style check u:addPIVA computes.
func peppolPartitaIVA(v string) bool {
	if len(v) < 2 || !strings.EqualFold(v[:2], "IT") {
		return true
	}
	code := v[2:]
	if len(code) != 11 || !peppolCastableAsInteger(code) {
		return false
	}
	// u:addPIVA doubles every second digit from the left and folds it with the
	// mapping '0246813579', which is doubling with the tens digit added back.
	const mapped = "0246813579"
	sum := 0
	for i := 0; i < len(code); i++ {
		d := int(code[i] - '0')
		if i%2 == 1 {
			d = int(mapped[d] - '0')
		}
		sum += d
	}
	return sum%10 == 0
}

// peppolSwedishOrgNr is u:checkSEOrgnr: the Luhn check over the first nine digits
// against the tenth.
func peppolSwedishOrgNr(v string) bool {
	if len(v) != 10 || !peppolAllDigits(v) {
		return false
	}
	main, check := v[:9], int(v[9]-'0')
	sum := 0
	for pos := 1; pos <= len(main); pos++ {
		d := int(main[len(main)-pos] - '0')
		if pos%2 == 1 {
			sum += (d*2)%10 + (d*2)/10
		} else {
			sum += d
		}
	}
	return (10-sum%10)%10 == check
}

// peppolABNWeights are u:abn's weights. The first digit is also reduced by one,
// which the function expresses by subtracting 49 rather than 48 from its
// codepoint.
var peppolABNWeights = []int{10, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19}

// peppolABN is u:abn, the Australian Business Number check. It reads eleven
// characters by codepoint, so a value of any other length yields an empty sequence
// in XPath and fails.
func peppolABN(v string) bool {
	r := []rune(v)
	if len(r) != 11 {
		return false
	}
	sum := (int(r[0]) - 49) * peppolABNWeights[0]
	for i := 1; i < 11; i++ {
		sum += (int(r[i]) - 48) * peppolABNWeights[i]
	}
	return sum%89 == 0
}
