package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"title-parser/pkg/model"
)

type DiffCounts struct {
	Total int

	BaseLines int
	NewLines  int

	MissingInNew  int
	MissingInBase int

	FieldChanges map[string]int

	ContentKindChanges map[string]int
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	basePath := flag.String("base", "", "base jsonl")
	newPath := flag.String("new", "", "new jsonl")
	flag.Parse()

	if strings.TrimSpace(*basePath) == "" || strings.TrimSpace(*newPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: compare_jsonl -base BASE.jsonl -new NEW.jsonl")
		os.Exit(2)
	}

	baseMap, baseLines, err := loadByRawTitle(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load base: %v\n", err)
		os.Exit(1)
	}
	newMap, newLines, err := loadByRawTitle(*newPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load new: %v\n", err)
		os.Exit(1)
	}

	d := &DiffCounts{
		BaseLines:          baseLines,
		NewLines:           newLines,
		FieldChanges:       map[string]int{},
		ContentKindChanges: map[string]int{},
	}

	allKeys := make([]string, 0, len(baseMap)+len(newMap))
	seen := map[string]bool{}
	for k := range baseMap {
		seen[k] = true
		allKeys = append(allKeys, k)
	}
	for k := range newMap {
		if !seen[k] {
			allKeys = append(allKeys, k)
		}
	}
	sort.Strings(allKeys)

	for _, key := range allKeys {
		bl := baseMap[key]
		nl := newMap[key]

		if len(bl) == 0 {
			d.MissingInBase += len(nl)
			continue
		}
		if len(nl) == 0 {
			d.MissingInNew += len(bl)
			continue
		}

		min := len(bl)
		if len(nl) < min {
			min = len(nl)
		}
		if len(bl) > len(nl) {
			d.MissingInNew += len(bl) - len(nl)
		} else if len(nl) > len(bl) {
			d.MissingInBase += len(nl) - len(bl)
		}

		for i := 0; i < min; i++ {
			d.Total++
			diffOne(d, bl[i], nl[i])
		}
	}

	printDiff(d)
}

func loadByRawTitle(path string) (map[string][]model.ParseResult, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	m := map[string][]model.ParseResult{}
	lineNo := 0
	total := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		total++

		var res model.ParseResult
		if err := json.Unmarshal([]byte(line), &res); err != nil {
			return nil, 0, fmt.Errorf("bad json at line %d: %w", lineNo, err)
		}
		key := strings.TrimSpace(res.RawTitle)
		if key == "" {
			return nil, 0, fmt.Errorf("missing raw_title at line %d", lineNo)
		}
		m[key] = append(m[key], res)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return m, total, nil
}

func diffOne(d *DiffCounts, b model.ParseResult, n model.ParseResult) {
	checkFieldChange(d, "content_kind", b.ContentKind, n.ContentKind)
	checkFieldChange(d, "template_id", b.TemplateID, n.TemplateID)
	checkFieldChange(d, "group", b.CanonicalGroup(), n.CanonicalGroup())
	checkFieldChange(d, "title", b.CanonicalTitle(), n.CanonicalTitle())
	checkFieldChange(d, "episode", b.CanonicalEpisode(), n.CanonicalEpisode())
	checkFieldChange(d, "episode_range", b.CanonicalEpisodeRange(), n.CanonicalEpisodeRange())
	checkFieldChange(d, "season", b.CanonicalSeason(), n.CanonicalSeason())
	checkFieldChange(d, "pack_kind", b.CanonicalPackKind(), n.CanonicalPackKind())
	checkFieldChange(d, "sp_kind", b.CanonicalSPKind(), n.CanonicalSPKind())
	checkFieldChange(d, "movie_index", b.CanonicalMovieIndex(), n.CanonicalMovieIndex())

	if strings.TrimSpace(b.ContentKind) != strings.TrimSpace(n.ContentKind) {
		from := strings.TrimSpace(b.ContentKind)
		to := strings.TrimSpace(n.ContentKind)
		key := from + " -> " + to
		d.ContentKindChanges[key]++
	}
}

func checkFieldChange(d *DiffCounts, field string, b string, n string) {
	if strings.TrimSpace(b) != strings.TrimSpace(n) {
		d.FieldChanges[field]++
	}
}

func printDiff(d *DiffCounts) {
	fmt.Printf("base_lines=%d\n", d.BaseLines)
	fmt.Printf("new_lines=%d\n", d.NewLines)
	fmt.Printf("compared=%d\n", d.Total)
	fmt.Printf("missing_in_new=%d\n", d.MissingInNew)
	fmt.Printf("missing_in_base=%d\n", d.MissingInBase)

	fmt.Println()
	fmt.Println("field_changes:")
	fields := make([]string, 0, len(d.FieldChanges))
	for k := range d.FieldChanges {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	for _, f := range fields {
		fmt.Printf("  %s: %d\n", f, d.FieldChanges[f])
	}

	fmt.Println()
	fmt.Println("content_kind_changes:")
	{
		type kv struct {
			k string
			v int
		}
		var items []kv
		for k, v := range d.ContentKindChanges {
			items = append(items, kv{k: k, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		for _, it := range items {
			fmt.Printf("  %s: %d\n", it.k, it.v)
		}
	}
}
