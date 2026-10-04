package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"reltitle/pkg/config"
	"reltitle/pkg/parser"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run parses one title and writes the result as JSON to stdout. The title comes from
// -title, then the first positional argument, then the first line of stdin.
// Exit codes: 0 success, 1 config or parse failure, 2 usage error or no title.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("reltitle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config", "configs", "config directory")
	titleArg := fs.String("title", "", "title to parse")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	p := parser.New(cfg)

	raw := strings.TrimSpace(*titleArg)
	if raw == "" {
		if fs.NArg() > 0 {
			raw = strings.TrimSpace(fs.Arg(0))
		}
	}
	if raw == "" {
		scanner := bufio.NewScanner(stdin)
		if scanner.Scan() {
			raw = strings.TrimSpace(scanner.Text())
		}
	}
	if raw == "" {
		fmt.Fprintln(stderr, "no title provided")
		return 2
	}

	res, err := p.Parse(raw)
	if err != nil {
		fmt.Fprintf(stderr, "parse: %v\n", err)
		return 1
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return 0
}
