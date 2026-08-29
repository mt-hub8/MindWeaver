// pdfblocker is a qualification-only PDF helper. The first extraction call is
// held by a loopback coordinator so the real MindWeaver worker can be stopped
// at an externally observable running-job checkpoint. A later call returns one
// fixed synthetic text layer. It is never included in a product artifact.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const coordinatorEnvironment = "MWQ_RUNTIME_PDF_COORDINATOR"

func main() {
	if len(os.Args) == 2 && os.Args[1] == protocol.ProbeArgument {
		if protocol.WriteProbe(os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	flags := flag.NewFlagSet("pdfblocker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "ignored qualification source")
	if flags.Parse(os.Args[1:]) != nil || flags.NArg() != 0 || strings.TrimSpace(*input) == "" {
		os.Exit(64)
	}
	endpoint, address, err := coordinator(os.Getenv(coordinatorEnvironment))
	if err != nil {
		os.Exit(1)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, requested string) (net.Conn, error) {
				if requested != address {
					return nil, errors.New("qualification helper refused unexpected target")
				}
				return dialer.DialContext(ctx, network, address)
			},
		},
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(nil))
	if err != nil {
		os.Exit(1)
	}
	response, err := client.Do(request)
	if err != nil {
		os.Exit(1)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 32))
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil || closeErr != nil || string(body) != "release\n" {
		os.Exit(1)
	}
	if protocol.WriteResult(os.Stdout, protocol.Result{Text: "runtime qualification blocker searchable", Pages: 1}) != nil {
		os.Exit(1)
	}
}

func coordinator(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "/pdf" ||
		parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("invalid qualification coordinator")
	}
	address, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !address.IsLoopback() || address.Is4In6() || address.Zone() != "" || parsed.Port() == "" {
		return "", "", errors.New("qualification coordinator is not literal loopback")
	}
	dialAddress := net.JoinHostPort(address.String(), parsed.Port())
	return "http://" + dialAddress + "/pdf", dialAddress, nil
}
