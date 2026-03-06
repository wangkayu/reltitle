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

type v1Line struct {
	RawTitle string          `json:"raw_title"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type v1Result struct {
	TitleBlock struct {
		Title  string `json:"title"`
		Group  string `json:"group,omitempty"`
		Season string `json:"season"`
	} `json:"title_block"`
	EPBlock struct {
		Episode      string `json:"episode"`
		EpisodeRange string `json:"episode_range"`
		PackKind     string `json:"pack_kind"`
	} `json:"ep_block"`
	TailGroup  string `json:"tail_group,omitempty"`
	Attributes []struct {
		Type       string `json:"type"`
		Text       string `json:"text"`
		Normalized string `json:"normalized"`
	} `json:"attributes"`
}

type normRecord struct {
	RawTitle      string
	Group         string
	Title         string
	Episode       string
	EpisodeRange  string
	Season        string
	PackKind      string
	HasParseError bool
	ParseError    string
}

type diffStats struct {
	Total int

	V1Lines int
	V2Lines int

	MissingInV1 int
	MissingInV2 int

	V1ErrorLines int

	FieldChanges map[string]int
	Buckets      map[string]int

	Examples map[string][]example
}

type example struct {
	RawTitle string
	Field    string
	V1       string
	V2       string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	v1Path := flag.String("v1", "", "v1 jsonl (from training/go/titleparser/cmd/titleparser_cli)")
	v2Path := flag.String("v2", "", "v2 jsonl (from title-parser/cmd/parse_batch)")
	topN := flag.Int("top", 20, "max examples per bucket")
	flag.Parse()

	if strings.TrimSpace(*v1Path) == "" || strings.TrimSpace(*v2Path) == "" {
		fmt.Fprintln(os.Stderr, "usage: compare_v1_v2 -v1 v1.jsonl -v2 v2.jsonl")
		os.Exit(2)
	}

	v1m, v1Lines, v1ErrLines, err := loadV1(*v1Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load v1: %v\n", err)
		os.Exit(1)
	}
	v2m, v2Lines, err := loadV2(*v2Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load v2: %v\n", err)
		os.Exit(1)
	}

	st := &diffStats{
		V1Lines:      v1Lines,
		V2Lines:      v2Lines,
		V1ErrorLines: v1ErrLines,
		FieldChanges: map[string]int{},
		Buckets:      map[string]int{},
		Examples:     map[string][]example{},
	}

	allKeys := unionKeys(v1m, v2m)
	for _, key := range allKeys {
		v1s := v1m[key]
		v2s := v2m[key]
		if len(v1s) == 0 {
			st.MissingInV1 += len(v2s)
			continue
		}
		if len(v2s) == 0 {
			st.MissingInV2 += len(v1s)
			continue
		}

		min := len(v1s)
		if len(v2s) < min {
			min = len(v2s)
		}
		if len(v1s) > len(v2s) {
			st.MissingInV2 += len(v1s) - len(v2s)
		} else if len(v2s) > len(v1s) {
			st.MissingInV1 += len(v2s) - len(v1s)
		}

		for i := 0; i < min; i++ {
			st.Total++
			diffOne(st, v1s[i], v2s[i], *topN)
		}
	}

	printStats(st)
}

func unionKeys(a map[string][]normRecord, b map[string][]model.ParseResult) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for k := range a {
		seen[k] = true
		out = append(out, k)
	}
	for k := range b {
		if !seen[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func loadV1(path string) (map[string][]normRecord, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	m := map[string][]normRecord{}
	lines := 0
	errLines := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++

		var ln v1Line
		if err := json.Unmarshal([]byte(line), &ln); err != nil {
			return nil, 0, 0, fmt.Errorf("bad json at line %d: %w", lineNo, err)
		}
		key := strings.TrimSpace(ln.RawTitle)
		if key == "" {
			return nil, 0, 0, fmt.Errorf("missing raw_title at line %d", lineNo)
		}

		rec := normRecord{RawTitle: key}
		if strings.TrimSpace(ln.Error) != "" {
			errLines++
			rec.HasParseError = true
			rec.ParseError = ln.Error
			m[key] = append(m[key], rec)
			continue
		}
		if len(ln.Result) == 0 {
			m[key] = append(m[key], rec)
			continue
		}

		var res v1Result
		if err := json.Unmarshal(ln.Result, &res); err != nil {
			return nil, 0, 0, fmt.Errorf("bad result json at line %d: %w", lineNo, err)
		}

		group := strings.TrimSpace(res.TitleBlock.Group)
		if group == "" {
			group = strings.TrimSpace(res.TailGroup)
		}
		if group == "" {
			for _, a := range res.Attributes {
				if strings.EqualFold(strings.TrimSpace(a.Type), "group") {
					if g := strings.TrimSpace(a.Normalized); g != "" {
						group = g
						break
					}
					if g := strings.TrimSpace(a.Text); g != "" {
						group = g
						break
					}
				}
			}
		}

		rec.Group = group
		rec.Title = strings.TrimSpace(res.TitleBlock.Title)
		rec.Episode = normalizeEpisode(res.EPBlock.Episode)
		rec.EpisodeRange = strings.TrimSpace(res.EPBlock.EpisodeRange)
		rec.Season = strings.TrimSpace(res.TitleBlock.Season)
		if rec.Season == "" {
			for _, a := range res.Attributes {
				if strings.EqualFold(strings.TrimSpace(a.Type), "season") {
					if s := strings.TrimSpace(a.Normalized); s != "" {
						rec.Season = s
						break
					}
					if s := strings.TrimSpace(a.Text); s != "" {
						rec.Season = s
						break
					}
				}
			}
		}
		rec.PackKind = strings.TrimSpace(res.EPBlock.PackKind)

		m[key] = append(m[key], rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, 0, err
	}

	return m, lines, errLines, nil
}

func loadV2(path string) (map[string][]model.ParseResult, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	m := map[string][]model.ParseResult{}
	lines := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++

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

	return m, lines, nil
}

func normalizeEpisode(ep string) string {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return ""
	}
	if strings.Contains(ep, ".") {
		parts := strings.SplitN(ep, ".", 2)
		if len(parts) == 2 {
			p0 := strings.TrimLeft(parts[0], "0")
			if p0 == "" {
				p0 = parts[0]
			}
			return p0 + "." + parts[1]
		}
	}
	ep2 := strings.TrimLeft(ep, "0")
	if ep2 == "" {
		return ep
	}
	return ep2
}

func diffOne(st *diffStats, v1 normRecord, v2 model.ParseResult, topN int) {
	if v1.HasParseError {
		bucket(st, "v1_error", example{RawTitle: v1.RawTitle, Field: "error", V1: v1.ParseError, V2: ""}, topN)
		return
	}

	v2Group := strings.TrimSpace(v2.CanonicalGroup())
	v2Title := strings.TrimSpace(v2.CanonicalTitle())
	v2Episode := normalizeEpisode(v2.CanonicalEpisode())
	v2EpisodeRange := strings.TrimSpace(v2.CanonicalEpisodeRange())
	v2Season := strings.TrimSpace(v2.CanonicalSeason())

	cmpField(st, "group", v1.RawTitle, v1.Group, v2Group, topN)

	if v2Title == "" && v1.Title != "" {
		bucket(st, "title_empty_in_v2", example{RawTitle: v1.RawTitle, Field: "title", V1: v1.Title, V2: v2Title}, topN)
	}
	cmpField(st, "title", v1.RawTitle, v1.Title, v2Title, topN)

	if v2Episode == "" && v1.Episode != "" {
		bucket(st, "episode_empty_in_v2", example{RawTitle: v1.RawTitle, Field: "episode", V1: v1.Episode, V2: v2Episode}, topN)
	}
	cmpField(st, "episode", v1.RawTitle, v1.Episode, v2Episode, topN)

	cmpField(st, "episode_range", v1.RawTitle, v1.EpisodeRange, v2EpisodeRange, topN)
	cmpField(st, "season", v1.RawTitle, v1.Season, v2Season, topN)
}

func cmpField(st *diffStats, field string, rawTitle string, a string, b string, topN int) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return
	}
	st.FieldChanges[field]++
	bucket(st, field+"_mismatch", example{RawTitle: rawTitle, Field: field, V1: a, V2: b}, topN)
}

func bucket(st *diffStats, name string, ex example, topN int) {
	st.Buckets[name]++
	if topN <= 0 {
		return
	}
	if len(st.Examples[name]) >= topN {
		return
	}
	st.Examples[name] = append(st.Examples[name], ex)
}

func printStats(st *diffStats) {
	fmt.Printf("v1_lines=%d\n", st.V1Lines)
	fmt.Printf("v2_lines=%d\n", st.V2Lines)
	fmt.Printf("compared=%d\n", st.Total)
	fmt.Printf("missing_in_v1=%d\n", st.MissingInV1)
	fmt.Printf("missing_in_v2=%d\n", st.MissingInV2)
	fmt.Printf("v1_error_lines=%d\n", st.V1ErrorLines)

	fmt.Println()
	fmt.Println("field_changes:")
	fields := make([]string, 0, len(st.FieldChanges))
	for k := range st.FieldChanges {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	for _, f := range fields {
		fmt.Printf("  %s: %d\n", f, st.FieldChanges[f])
	}

	fmt.Println()
	fmt.Println("buckets:")
	{
		type kv struct {
			k string
			v int
		}
		items := make([]kv, 0, len(st.Buckets))
		for k, v := range st.Buckets {
			items = append(items, kv{k: k, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		for _, it := range items {
			fmt.Printf("  %s: %d\n", it.k, it.v)
		}
	}

	if len(st.Examples) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("examples:")
	buckets := make([]string, 0, len(st.Examples))
	for k := range st.Examples {
		buckets = append(buckets, k)
	}
	sort.Strings(buckets)
	for _, b := range buckets {
		ex := st.Examples[b]
		if len(ex) == 0 {
			continue
		}
		fmt.Printf("[%s]\n", b)
		for _, e := range ex {
			fmt.Printf("  raw_title: %s\n", e.RawTitle)
			fmt.Printf("  %s: %q -> %q\n", e.Field, e.V1, e.V2)
		}
	}
}
