package formalis

import "context"

// This file validates the Finnish TEAPPS format (Tieto TEAPPS) — a proprietary
// batch invoice XML (root INVOICE_CENTER, carrying a transport frame and one or
// more INVOICE documents). It is XSD-validated rather than rule-validated and is
// deeply nested, so this checks only the mandatory structure common to the
// format: each invoice's type and customer information.
//
// Rule identifiers are TP-* (this package's own). Not vendored: the TEAPPS sample
// instances (phax/phive-rules) are used only as the oracle.

// IsTEAPPS reports whether the XML is a TEAPPS batch document.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition: several of them key
// on a root element name that four national formats, seven CIUS and the EN 16931
// UBL binding all share, and more than one can report true about one document.
// This one keys on a root no other format claims, so nothing overlaps it today.
// Detect owns the precedence for the whole set and returns a single answer; route
// with it.
func IsTEAPPS(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "INVOICE_CENTER", nil
}

// ValidateTEAPPS validates a Finnish TEAPPS batch against its mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean batch.
//
// This validator checks the mandatory structure and code lists rather than the
// whole schema its authority publishes, so the Report is never Conformant even
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceTEAPPS),
// says what was not checked.
func ValidateTEAPPS(ctx context.Context, xmlData []byte) (Report, error) {
	return teappsValidator.validate(ctx, xmlData)
}

var teappsValidator = treeValidator{
	source:   SourceTEAPPS,
	rootRule: "TP-root",
	rootMsg:  "the document root shall be INVOICE_CENTER",
	accepts:  rootNamed("INVOICE_CENTER"),
	check:    checkTEAPPS,
}

func checkTEAPPS(root *ciiNode, add func(rule, msg string)) {
	invoices := root.findAll("INVOICE")
	if len(invoices) == 0 {
		add("TP-invoice", "the batch shall contain at least one INVOICE")
	}
	for _, inv := range invoices {
		if len(inv.findAll("INVOICE_TYPE")) == 0 {
			add("TP-type", "each INVOICE shall contain an INVOICE_TYPE")
		}
		if len(inv.findAll("CUSTOMER_INFORMATION")) == 0 {
			add("TP-customer", "each INVOICE shall contain CUSTOMER_INFORMATION")
		}
	}
}
