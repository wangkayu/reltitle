package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type diffCounts struct {
	Total int

	FieldChanges map[string]int
	Examples     map[string][]example

	StreamingMetaAliasViolations int
	StreamingMetaAliasBadTopN    []string
}

type example struct {
	RawTitle string
	Field    string
	Base     string
	New      string
}

var streamingMetaRe = regexp.MustCompile(`(?i)\b(?:multi[-_ ]?subs?|multisub|dual[-_ ]?audio|multi[-_ ]?audio|english\s*dub|dub)\b`)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	basePath := flag.String("base", "", "baseline v2 jsonl (one ParseResult per line)")
	configDir := flag.String("config", "configs", "config directory")
	limit := flag.Int("limit", 0, "limit number of records to process (0 = no limit)")
	topN := flag.Int("top", 20, "max examples per changed field")
	progressEvery := flag.Int("progress", 0, "print progress to stderr every N records (0 = disabled)")
	flag.Parse()

	if strings.TrimSpace(*basePath) == "" {
		fmt.Fprintln(os.Stderr, "usage: regression_check -base BASELINE.jsonl [-config configs]")
		os.Exit(2)
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	p := parser.New(cfg)

	f, err := os.Open(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open baseline: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	d := &diffCounts{
		FieldChanges: map[string]int{},
		Examples:     map[string][]example{},
	}

	lineNo := 0
	for scanner.Scan() {
		if *limit > 0 && d.Total >= *limit {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++
		d.Total++
		if *progressEvery > 0 && d.Total%*progressEvery == 0 {
			fmt.Fprintf(os.Stderr, "processed=%d\n", d.Total)
		}

		var base model.ParseResult
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			fmt.Fprintf(os.Stderr, "bad json at line %d: %v\n", lineNo, err)
			os.Exit(1)
		}
		rt := strings.TrimSpace(base.RawTitle)
		if rt == "" {
			fmt.Fprintf(os.Stderr, "missing raw_title at line %d\n", lineNo)
			os.Exit(1)
		}

		cur, err := p.Parse(rt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error at line %d: %v | %s\n", lineNo, err, rt)
			os.Exit(1)
		}

		if isStreamingReleaseStrict(cur) {
			if hasStreamingMetaInAliases(cur.Aliases) {
				d.StreamingMetaAliasViolations++
				if *topN > 0 && len(d.StreamingMetaAliasBadTopN) < *topN {
					d.StreamingMetaAliasBadTopN = append(d.StreamingMetaAliasBadTopN, rt)
				}
			}
		}

		diffOne(d, base, *cur, *topN)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}

	printReport(d)

	changed := 0
	for _, v := range d.FieldChanges {
		changed += v
	}
	if changed > 0 || d.StreamingMetaAliasViolations > 0 {
		os.Exit(1)
	}
}

func hasStreamingMetaInAliases(aliases []string) bool {
	for _, a := range aliases {
		if streamingMetaRe.MatchString(a) {
			return true
		}
	}
	return false
}

func diffOne(d *diffCounts, b model.ParseResult, n model.ParseResult, topN int) {
	checkFieldChange(d, "ep_route", b.EpRoute, n.EpRoute, b.RawTitle, topN)
	checkFieldChange(d, "content_kind", b.ContentKind, n.ContentKind, b.RawTitle, topN)
	checkFieldChange(d, "template_id", b.TemplateID, n.TemplateID, b.RawTitle, topN)

	checkFieldChange(d, "group", b.Group, n.Group, b.RawTitle, topN)
	checkFieldChange(d, "title", b.Title, n.Title, b.RawTitle, topN)
	checkFieldChange(d, "episode", b.Episode, n.Episode, b.RawTitle, topN)
	checkFieldChange(d, "episode_range", b.EpisodeRange, n.EpisodeRange, b.RawTitle, topN)
	checkFieldChange(d, "season", b.Season, n.Season, b.RawTitle, topN)
	checkFieldChange(d, "pack_kind", b.PackKind, n.PackKind, b.RawTitle, topN)
	checkFieldChange(d, "sp_kind", b.SPKind, n.SPKind, b.RawTitle, topN)
	checkFieldChange(d, "movie_index", b.MovieIndex, n.MovieIndex, b.RawTitle, topN)
}

func checkFieldChange(d *diffCounts, field string, b string, n string, rawTitle string, topN int) {
	bs := strings.TrimSpace(b)
	ns := strings.TrimSpace(n)
	if bs == ns {
		return
	}
	d.FieldChanges[field]++
	if topN <= 0 {
		return
	}
	if len(d.Examples[field]) >= topN {
		return
	}
	d.Examples[field] = append(d.Examples[field], example{RawTitle: strings.TrimSpace(rawTitle), Field: field, Base: bs, New: ns})
}

func printReport(d *diffCounts) {
	fmt.Printf("total=%d\n", d.Total)

	fmt.Println()
	fmt.Println("field_changes:")
	fields := make([]string, 0, len(d.FieldChanges))
	for k := range d.FieldChanges {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	for _, f := range fields {
		fmt.Printf("  %s: %d\n", f, d.FieldChanges[f])
		if len(d.Examples[f]) > 0 {
			for _, ex := range d.Examples[f] {
				fmt.Printf("    %s\n", ex.RawTitle)
				fmt.Printf("      base=%s\n", ex.Base)
				fmt.Printf("      new =%s\n", ex.New)
			}
		}
	}

	fmt.Println()
	fmt.Println("streaming_release invariants:")
	fmt.Printf("  meta_in_alias_violations: %d\n", d.StreamingMetaAliasViolations)
	if len(d.StreamingMetaAliasBadTopN) > 0 {
		fmt.Printf("  top_meta_in_alias_violations:\n")
		for _, s := range d.StreamingMetaAliasBadTopN {
			fmt.Printf("    %s\n", s)
		}
	}
}

func isStreamingReleaseStrict(res *model.ParseResult) bool {
	if res == nil {
		return false
	}
	plat := ""
	src := ""
	for _, a := range res.Attributes {
		if a.Type == model.AttrPlatform && plat == "" {
			plat = strings.TrimSpace(a.Normalized)
		}
		if a.Type == model.AttrSourceType && src == "" {
			src = strings.TrimSpace(a.Normalized)
		}
	}
	if plat == "" {
		return false
	}
	up := strings.ToUpper(src)
	return up == "WEB-DL" || up == "WEBRIP" || up == "WEB-RIP"
}
