// mindweaver-pdf is an internal, state-free parser helper. It is intentionally
// a separate process so malformed PDFs cannot crash the Vault-owning process.
package main

import (
	"fmt"
	"os"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/parser"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

func main() {
	if err := parser.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(protocol.ExitCode(err))
	}
}
