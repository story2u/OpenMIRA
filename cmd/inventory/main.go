// Command inventory emits a read-only snapshot or report of an optional
// external reference app. It remains available as transitional evidence while
// the Go/Next.js IM project becomes independently verifiable.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"wework-go/internal/inventory"
)

func main() {
	referenceRoot := flag.String("reference-root", "", "external reference project root")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	format := flag.String("format", "json", "output format: json or markdown")
	flag.Parse()

	root := *referenceRoot
	if root == "" {
		fmt.Fprintln(os.Stderr, "inventory failed: -reference-root is required")
		os.Exit(1)
	}

	snapshot, err := inventory.Build(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inventory failed: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		encodeJSON(snapshot, *pretty)
	case "markdown":
		fmt.Print(inventory.MarkdownReport(snapshot))
	default:
		fmt.Fprintf(os.Stderr, "unsupported inventory format %q\n", *format)
		os.Exit(1)
	}
}

func encodeJSON(snapshot inventory.Snapshot, pretty bool) {
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "encode inventory failed: %v\n", err)
		os.Exit(1)
	}
}
