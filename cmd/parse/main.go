package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/parser"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	titleArg := flag.String("title", "", "title to parse")
	overrideTitleStart := flag.Int("override_title_start", -1, "override title span start (token index)")
	overrideTitleEnd := flag.Int("override_title_end", -1, "override title span end (token index, exclusive)")
	overrideEpisode := flag.String("override_episode", "", "override episode")
	overrideEpisodeIdx := flag.Int("override_episode_idx", -1, "override episode index")
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

	var hints *parser.ParseHints
	if *overrideTitleStart >= 0 && *overrideTitleEnd > *overrideTitleStart {
		ph := &parser.ParseHints{}
		ph.OverrideTitleStart = *overrideTitleStart
		ph.OverrideTitleEnd = *overrideTitleEnd
		hints = ph
	}
	if strings.TrimSpace(*overrideEpisode) != "" && *overrideEpisodeIdx >= 0 {
		if hints == nil {
			hints = &parser.ParseHints{}
		}
		hints.OverrideEpisode = strings.TrimSpace(*overrideEpisode)
		hints.OverrideEpisodeIdx = *overrideEpisodeIdx
	}

	res, err := p.ParseWithHints(raw, hints)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}
