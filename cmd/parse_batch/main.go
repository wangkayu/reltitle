package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/parser"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	inPath := flag.String("in", "", "input file (one title per line). default: stdin")
	outPath := flag.String("out", "", "output file (jsonl). default: stdout")
	limit := flag.Int("limit", 0, "max number of titles to process (0 = no limit)")
	flag.Parse()

	// Backward/CLI convenience: allow passing input/output as positional args.
	// e.g. parse_batch input.txt > out.jsonl
	if strings.TrimSpace(*inPath) == "" {
		if flag.NArg() >= 1 {
			*inPath = strings.TrimSpace(flag.Arg(0))
		}
	}
	if strings.TrimSpace(*outPath) == "" {
		if flag.NArg() >= 2 {
			*outPath = strings.TrimSpace(flag.Arg(1))
		}
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	p := parser.New(cfg)

	var r io.Reader = os.Stdin
	if strings.TrimSpace(*inPath) != "" {
		f, err := os.Open(*inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open input: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	var w io.Writer = os.Stdout
	if strings.TrimSpace(*outPath) != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	enc := json.NewEncoder(w)

	reCont := regexp.MustCompile(`^:?-\s*\d{1,4}\b`)

	flush := func(line string) bool {
		line = strings.TrimSpace(line)
		if line == "" {
			return true
		}
		res, err := p.Parse(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error: %v | %s\n", err, line)
			return true
		}
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			os.Exit(1)
		}
		return true
	}

	count := 0
	pending := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if reCont.MatchString(line) {
			if strings.TrimSpace(pending) != "" {
				pending = strings.TrimSpace(pending + " " + line)
				continue
			}
		}
		if strings.TrimSpace(pending) != "" {
			_ = flush(pending)
			count++
			if *limit > 0 && count >= *limit {
				pending = ""
				break
			}
		}
		pending = line
	}
	if strings.TrimSpace(pending) != "" {
		_ = flush(pending)
		count++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}
}
