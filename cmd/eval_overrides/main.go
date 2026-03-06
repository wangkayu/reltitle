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

	"title-parser/pkg/model"
)

type report struct {
	BaseLines int `json:"base_lines"`
	NewLines  int `json:"new_lines"`
	Compared  int `json:"compared"`

	MissingInNew  int `json:"missing_in_new"`
	MissingInBase int `json:"missing_in_base"`

	BaseCounts map[string]int `json:"base_counts"`
	NewCounts  map[string]int `json:"new_counts"`
	Delta      map[string]int `json:"delta"`

	FixedCounts     map[string]int      `json:"fixed_counts"`
	RegressedCounts map[string]int      `json:"regressed_counts"`
	FixedTopN       map[string][]string `json:"fixed_top_n"`
	RegressedTopN   map[string][]string `json:"regressed_top_n"`

	TopN int `json:"top_n"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	basePath := flag.String("base", "", "baseline ParseResult jsonl")
	newPath := flag.String("new", "", "new ParseResult jsonl (e.g. after applying overrides)")
	outPath := flag.String("out", "", "output json (default: stdout)")
	topN := flag.Int("top", 30, "top-N samples per reason")
	flag.Parse()

	if strings.TrimSpace(*basePath) == "" || strings.TrimSpace(*newPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: eval_overrides -base BASE.jsonl -new NEW.jsonl")
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

	r := &report{
		BaseLines:       baseLines,
		NewLines:        newLines,
		BaseCounts:      map[string]int{},
		NewCounts:       map[string]int{},
		Delta:           map[string]int{},
		FixedCounts:     map[string]int{},
		RegressedCounts: map[string]int{},
		FixedTopN:       map[string][]string{},
		RegressedTopN:   map[string][]string{},
		TopN:            *topN,
	}

	allKeys := unionKeys(baseMap, newMap)
	for _, key := range allKeys {
		bl := baseMap[key]
		nl := newMap[key]
		if len(bl) == 0 {
			r.MissingInBase += len(nl)
			continue
		}
		if len(nl) == 0 {
			r.MissingInNew += len(bl)
			continue
		}

		min := len(bl)
		if len(nl) < min {
			min = len(nl)
		}
		if len(bl) > len(nl) {
			r.MissingInNew += len(bl) - len(nl)
		} else if len(nl) > len(bl) {
			r.MissingInBase += len(nl) - len(bl)
		}

		for i := 0; i < min; i++ {
			r.Compared++
			diffOne(r, &bl[i], &nl[i])
		}
	}

	for k, v := range r.NewCounts {
		r.Delta[k] = v - r.BaseCounts[k]
	}
	for k, v := range r.BaseCounts {
		if _, ok := r.Delta[k]; !ok {
			r.Delta[k] = -v
		}
	}

	var outW io.Writer = os.Stdout
	if strings.TrimSpace(*outPath) != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create out: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		outW = f
	}

	enc := json.NewEncoder(outW)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}

type rawTitleMap map[string][]model.ParseResult

func loadByRawTitle(path string) (rawTitleMap, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	m := rawTitleMap{}
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

func unionKeys(a rawTitleMap, b rawTitleMap) []string {
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

func diffOne(r *report, base *model.ParseResult, cur *model.ParseResult) {
	br := hardcaseReasons(base)
	nr := hardcaseReasons(cur)

	for k := range br {
		r.BaseCounts[k]++
	}
	for k := range nr {
		r.NewCounts[k]++
	}

	for reason := range br {
		if !nr[reason] {
			r.FixedCounts[reason]++
			addTopN(r.FixedTopN, reason, base.RawTitle, r.TopN)
		}
	}
	for reason := range nr {
		if !br[reason] {
			r.RegressedCounts[reason]++
			addTopN(r.RegressedTopN, reason, base.RawTitle, r.TopN)
		}
	}
}

func addTopN(dst map[string][]string, key string, rawTitle string, topN int) {
	if topN <= 0 {
		return
	}
	cur := dst[key]
	if len(cur) >= topN {
		return
	}
	rt := strings.TrimSpace(rawTitle)
	if rt == "" {
		return
	}
	dst[key] = append(cur, rt)
}

func hardcaseReasons(res *model.ParseResult) map[string]bool {
	out := map[string]bool{}
	if res == nil {
		return out
	}
	if strings.TrimSpace(res.CanonicalTitle()) == "" {
		out["parser_title_empty"] = true
	}
	if strings.EqualFold(strings.TrimSpace(res.TemplateID), "TF_UNKNOWN_DEFAULT") {
		out["parser_unknown_default"] = true
	}
	if hasTitleTechOverlap(res.Attributes) {
		out["parser_title_tech_overlap"] = true
	}
	if len(out) > 0 {
		out["any_risk"] = true
	}
	return out
}

func titleSpanFromAttrs(attrs []model.Attribute) (start int, end int, ok bool) {
	minS := 1 << 30
	maxE := -1
	for _, a := range attrs {
		if a.Type != model.AttrTitle {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		if a.TokenStart < minS {
			minS = a.TokenStart
		}
		if a.TokenEnd > maxE {
			maxE = a.TokenEnd
		}
	}
	if minS == 1<<30 || maxE < 0 {
		return 0, 0, false
	}
	return minS, maxE, true
}

func isHardTechAttrType(t model.AttrType) bool {
	switch t {
	case model.AttrSourceType,
		model.AttrResolution,
		model.AttrVideoCodec,
		model.AttrAudioCodec,
		model.AttrBitDepth,
		model.AttrContainer,
		model.AttrCRC:
		return true
	default:
		return false
	}
}

func hasTitleTechOverlap(attrs []model.Attribute) bool {
	tst, ted, ok := titleSpanFromAttrs(attrs)
	if !ok {
		return false
	}
	if tst < 0 {
		tst = 0
	}
	if ted <= tst {
		return false
	}
	for _, a := range attrs {
		if !isHardTechAttrType(a.Type) {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		// interval overlap
		if a.TokenStart < ted && tst < a.TokenEnd {
			return true
		}
	}
	return false
}
