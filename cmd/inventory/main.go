// Command inventory emits a read-only snapshot or report of the legacy Python app.
// It is used in phase one to keep route, contract, feature, and runtime
// coverage visible before any business migration starts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"wework-go/internal/inventory"
)

func main() {
	pythonRoot := flag.String("python-root", "../Python", "legacy Python project root")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	format := flag.String("format", "json", "output format: json or markdown")
	flag.Parse()

	snapshot, err := inventory.Build(*pythonRoot)
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
