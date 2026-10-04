package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"reltitle/pkg/config"
	"reltitle/pkg/parser"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	titleArg := flag.String("title", "", "title to parse")
	flag.Parse()

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	p := parser.New(cfg)

	raw := strings.TrimSpace(*titleArg)
	if raw == "" {
		if flag.NArg() > 0 {
			raw = strings.TrimSpace(flag.Arg(0))
		}
	}
	if raw == "" {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			raw = strings.TrimSpace(scanner.Text())
		}
	}
	if raw == "" {
		fmt.Fprintln(os.Stderr, "no title provided")
		os.Exit(2)
	}

	res, err := p.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}
