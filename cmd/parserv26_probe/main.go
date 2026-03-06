package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
	"title-parser/pkg/parserv26"
)

type ProbeRecord struct {
	RawTitle string             `json:"raw_title"`
	V25      *model.ParseResult `json:"v25,omitempty"`
	V26      model.ParseResult  `json:"v26"`
	Trace    parserv26.Trace    `json:"trace"`
}

type ProbeSummary struct {
	Total              int            `json:"total"`
	WithAnyCandidates  int            `json:"with_any_candidates"`
	AvgCandidates      float64        `json:"avg_candidates"`
	SlotCounts         map[string]int `json:"slot_counts"`
	V25HasGroupCount   int            `json:"v25_has_group_count,omitempty"`
	V25HasTitleCount   int            `json:"v25_has_title_count,omitempty"`
	V25HasEpisodeCount int            `json:"v25_has_episode_count,omitempty"`
	V26HasGroupCount   int            `json:"v26_has_group_count,omitempty"`
	V26HasTitleCount   int            `json:"v26_has_title_count,omitempty"`
	V26HasEpisodeCount int            `json:"v26_has_episode_count,omitempty"`
	GroupAgreeCount    int            `json:"group_agree_count,omitempty"`
	TitleAgreeCount    int            `json:"title_agree_count,omitempty"`
	EpisodeAgreeCount  int            `json:"episode_agree_count,omitempty"`
	TopSignals         []SignalCount  `json:"top_signals"`
}

type SignalCount struct {
	Signal string `json:"signal"`
	Count  int    `json:"count"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	inPath := flag.String("in", "", "input file (one title per line), default stdin")
	outPath := flag.String("out", "", "output JSONL path, default stdout")
	summaryPath := flag.String("summary", "", "summary JSON output path")
	limit := flag.Int("limit", 0, "max lines to process (0=no limit)")
	withV25 := flag.Bool("with-v25", true, "include v2.5 parse result for comparison")
	flag.Parse()

	if strings.TrimSpace(*inPath) == "" && flag.NArg() >= 1 {
		*inPath = strings.TrimSpace(flag.Arg(0))
	}
	if strings.TrimSpace(*outPath) == "" && flag.NArg() >= 2 {
		*outPath = strings.TrimSpace(flag.Arg(1))
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	p26 := parserv26.New(cfg)
	var p25 *parser.Parser
	if *withV25 {
		p25 = parser.New(cfg)
	}

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

	summary := ProbeSummary{SlotCounts: map[string]int{}}
	signalCounts := map[string]int{}
	candTotal := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if *limit > 0 && summary.Total >= *limit {
			break
		}

		rec := ProbeRecord{RawTitle: line}
		if p25 != nil {
			v25, err := p25.Parse(line)
			if err == nil {
				rec.V25 = v25
				if strings.TrimSpace(v25.CanonicalGroup()) != "" {
					summary.V25HasGroupCount++
				}
				if strings.TrimSpace(v25.CanonicalTitle()) != "" {
					summary.V25HasTitleCount++
				}
				if strings.TrimSpace(v25.CanonicalEpisode()) != "" || strings.TrimSpace(v25.CanonicalEpisodeRange()) != "" {
					summary.V25HasEpisodeCount++
				}
			}
		}

		v26, trace, err := p26.Parse(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v26 parse error: %v | %s\n", err, line)
			continue
		}
		rec.V26 = v26
		rec.Trace = trace
		if strings.TrimSpace(v26.CanonicalGroup()) != "" {
			summary.V26HasGroupCount++
		}
		if strings.TrimSpace(v26.CanonicalTitle()) != "" {
			summary.V26HasTitleCount++
		}
		if strings.TrimSpace(v26.CanonicalEpisode()) != "" || strings.TrimSpace(v26.CanonicalEpisodeRange()) != "" {
			summary.V26HasEpisodeCount++
		}
		if rec.V25 != nil {
			v25Group := rec.V25.CanonicalGroup()
			v26Group := v26.CanonicalGroup()
			if normGroup(v25Group) == normGroup(v26Group) && normGroup(v26Group) != "" {
				summary.GroupAgreeCount++
			}
			v25Title := rec.V25.CanonicalTitle()
			v26Title := v26.CanonicalTitle()
			if normText(v25Title) == normText(v26Title) && normText(v26Title) != "" {
				summary.TitleAgreeCount++
			}
			v25ep := normText(firstEpisodeValue(rec.V25))
			v26ep := normText(firstEpisodeValue(&v26))
			if v25ep == v26ep && v26ep != "" {
				summary.EpisodeAgreeCount++
			}
		}

		if len(trace.Candidates) > 0 {
			summary.WithAnyCandidates++
		}
		candTotal += len(trace.Candidates)
		for _, c := range trace.Candidates {
			summary.SlotCounts[string(c.Slot)]++
		}
		for _, sig := range trace.Signals {
			signalCounts[sig]++
		}

		if err := enc.Encode(rec); err != nil {
			fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
			os.Exit(1)
		}
		summary.Total++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan input: %v\n", err)
		os.Exit(1)
	}

	if summary.Total > 0 {
		summary.AvgCandidates = float64(candTotal) / float64(summary.Total)
	}
	summary.TopSignals = topSignals(signalCounts, 20)

	if strings.TrimSpace(*summaryPath) != "" {
		f, err := os.Create(*summaryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create summary: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fmt.Fprintf(os.Stderr, "write summary: %v\n", err)
			os.Exit(1)
		}
	}
}

func topSignals(counter map[string]int, n int) []SignalCount {
	out := make([]SignalCount, 0, len(counter))
	for k, v := range counter {
		out = append(out, SignalCount{Signal: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Signal < out[j].Signal
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func normText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normGroup(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	repl := []string{"-", "_", ".", " ", "(", ")", "[", "]"}
	for _, r := range repl {
		s = strings.ReplaceAll(s, r, "")
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstEpisodeValue(res *model.ParseResult) string {
	if res == nil {
		return ""
	}
	if v := strings.TrimSpace(res.CanonicalEpisode()); v != "" {
		return v
	}
	if v := strings.TrimSpace(res.CanonicalEpisodeRange()); v != "" {
		return v
	}
	return ""
}
