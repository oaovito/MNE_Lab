// dtsinspect is a development-only, local interoperability inspector.
// Reports can contain private source metadata; they are never shipped fixtures.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/oaovito/mne_lab/internal/science/malvern/dts"
)

func main() {
	input := flag.String("input", "", "DTS original to read without modification")
	output := flag.String("output", "", "private JSON report destination (required)")
	flag.Parse()
	if *input == "" || *output == "" || *input == *output {
		fmt.Fprintln(os.Stderr, "Distinct --input and --output paths are required; reports contain private metadata.")
		os.Exit(2)
	}
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot open input.")
		os.Exit(1)
	}
	b, err := io.ReadAll(io.LimitReader(f, dts.MaxFileSize+1))
	f.Close()
	if err != nil || len(b) > dts.MaxFileSize {
		fmt.Fprintln(os.Stderr, "Input exceeds size limit or cannot be read.")
		os.Exit(1)
	}
	report, err := dts.Inspect(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot encode report.")
		os.Exit(1)
	}
	// O_EXCL protects the original and existing reports, including symlink aliases.
	f, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Output already exists or cannot be created.")
		os.Exit(1)
	}
	_, err = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "Cannot write report.")
		os.Exit(1)
	}
	fmt.Printf("Private local report written: %d streams, %d record envelopes; scientific results remain UNVALIDATED.\n", len(report.Container.Streams), len(report.Records))
}
