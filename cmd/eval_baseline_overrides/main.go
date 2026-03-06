package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type overrideRecord struct {
	RawTitle    string   `json:"raw_title"`
	Key         string   `json:"key"`
	Reasons     []string `json:"reasons"`
	Recommended struct {
		Choice     string  `json:"choice"`
		Episode    string  `json:"episode"`
		EpisodeIdx int     `json:"episode_idx"`
		TitleStart int     `json:"title_start"`
		TitleEnd   int     `json:"title_end"`
		Confidence float64 `json:"confidence"`
	} `json:"recommended"`
}

type overrideHint struct {
	Choice     string
	Episode    string
	EpisodeIdx int
	TitleStart int
	TitleEnd   int
	Confidence float64
	Reasons    []string
}

type evalReport struct {
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

type diffSummary struct {
	Compared int `json:"compared"`

	MissingInNew  int `json:"missing_in_new"`
	MissingInBase int `json:"missing_in_base"`

	FieldChanges       map[string]int      `json:"field_changes"`
	FieldChangeTopN    map[string][]string `json:"field_change_top_n"`
	ContentKindChanges map[string]int      `json:"content_kind_changes"`
	ContentKindTopN    map[string][]string `json:"content_kind_top_n"`
	TopN               int                 `json:"top_n"`
}

type rawTitleMap map[string][]model.ParseResult

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	titlesPath := flag.String("titles", "tests/baselines/regression_set_v1.titles.txt", "input titles (one title per line, supports continuation lines)")
	basePath := flag.String("base", "tests/baselines/baseline.v2.20260122.regression_set_v1.jsonl", "baseline ParseResult jsonl")
	overridesPath := flag.String("overrides", "", "override proposal jsonl(s). comma-separated")
	outDir := flag.String("out_dir", "/tmp/title_parser_eval", "output dir")
	limit := flag.Int("limit", 0, "max number of titles to process (0 = no limit)")
	minConfidence := flag.Float64("min_confidence", 0.15, "only apply overrides when recommended.confidence >= this threshold")
	allowReasons := flag.String("allow_reasons", "", "comma-separated allowlist of proposal reasons. empty=allow all")
	riskOnly := flag.Bool("risk_only", true, "only consider applying overrides for baseline samples that are hardcase risk")
	strictGate := flag.Bool("strict_gate", true, "accept override only when it improves hardcase reasons and introduces no new risk reasons")
	titleSanityGate := flag.Bool("title_sanity_gate", true, "reject overrides when parsed title is obviously low quality (stopwords/too-short/tech leakage)")
	baselineFallback := flag.Bool("baseline_fallback", true, "when override is not applied or rejected, write the baseline record (not current parse)")
	topN := flag.Int("top", 30, "top-N samples per reason/field")
	flag.Parse()

	if strings.TrimSpace(*overridesPath) == "" {
		fmt.Fprintln(os.Stderr, "missing -overrides")
		os.Exit(2)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir out_dir: %v\n", err)
		os.Exit(1)
	}

	overriddenPath := filepath.Join(*outDir, "overridden.jsonl")
	evalPath := filepath.Join(*outDir, "eval_overrides.json")
	diffPath := filepath.Join(*outDir, "diff_summary.json")

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	p := parser.New(cfg)

	titles, err := readTitlesWithContinuation(*titlesPath, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read titles: %v\n", err)
		os.Exit(1)
	}

	overrides, err := loadOverrides(splitCSVPaths(*overridesPath), *minConfidence, *allowReasons)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load overrides: %v\n", err)
		os.Exit(1)
	}

	baseMap, baseLines, err := loadByRawTitle(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load base: %v\n", err)
		os.Exit(1)
	}

	if err := writeOverriddenWithGate(p, titles, baseMap, overrides, overriddenPath, *riskOnly, *strictGate, *titleSanityGate, *baselineFallback); err != nil {
		fmt.Fprintf(os.Stderr, "write overridden: %v\n", err)
		os.Exit(1)
	}

	newMap, newLines, err := loadByRawTitle(overriddenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load overridden: %v\n", err)
		os.Exit(1)
	}

	eval := computeEval(baseMap, baseLines, newMap, newLines, *topN)
	if err := writeJSON(evalPath, eval); err != nil {
		fmt.Fprintf(os.Stderr, "write eval: %v\n", err)
		os.Exit(1)
	}

	ds := computeDiffSummary(baseMap, newMap, *topN)
	if err := writeJSON(diffPath, ds); err != nil {
		fmt.Fprintf(os.Stderr, "write diff summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "wrote overridden=%s\n", overriddenPath)
	fmt.Fprintf(os.Stderr, "wrote eval=%s\n", evalPath)
	fmt.Fprintf(os.Stderr, "wrote diff_summary=%s\n", diffPath)
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func splitCSVPaths(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func mergeHint(dst overrideHint, src overrideHint) overrideHint {
	if strings.EqualFold(strings.TrimSpace(src.Choice), "alt") {
		dst.Choice = "alt"
	} else if strings.TrimSpace(dst.Choice) == "" {
		dst.Choice = strings.TrimSpace(src.Choice)
	}
	if strings.TrimSpace(src.Episode) != "" {
		dst.Episode = strings.TrimSpace(src.Episode)
	}
	if src.EpisodeIdx >= 0 {
		dst.EpisodeIdx = src.EpisodeIdx
	}
	if src.TitleEnd > src.TitleStart {
		dst.TitleStart = src.TitleStart
		dst.TitleEnd = src.TitleEnd
	}
	if src.Confidence != 0 {
		dst.Confidence = src.Confidence
	}
	if len(src.Reasons) > 0 {
		dst.Reasons = uniqueSortedStrings(append(dst.Reasons, src.Reasons...))
	}
	return dst
}

func parseAllowReasonsCSV(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out[p] = true
	}
	return out
}

func hasAnyAllowedReason(reasons []string, allow map[string]bool) bool {
	if len(allow) == 0 {
		return true
	}
	for _, r := range reasons {
		if allow[strings.TrimSpace(r)] {
			return true
		}
	}
	return false
}

func uniqueSortedStrings(xs []string) []string {
	m := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if m[x] {
			continue
		}
		m[x] = true
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func loadOverrides(paths []string, minConfidence float64, allowReasonsCSV string) (map[string]overrideHint, error) {
	allow := parseAllowReasonsCSV(allowReasonsCSV)
	m := map[string]overrideHint{}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var r overrideRecord
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				continue
			}
			key := strings.TrimSpace(r.Key)
			if key == "" {
				if strings.TrimSpace(r.RawTitle) != "" {
					key = sha1Hex(r.RawTitle)
				}
			}
			if key == "" {
				continue
			}
			src := overrideHint{
				Choice:     strings.TrimSpace(r.Recommended.Choice),
				Episode:    strings.TrimSpace(r.Recommended.Episode),
				EpisodeIdx: r.Recommended.EpisodeIdx,
				TitleStart: r.Recommended.TitleStart,
				TitleEnd:   r.Recommended.TitleEnd,
				Confidence: r.Recommended.Confidence,
				Reasons:    r.Reasons,
			}
			if minConfidence > 0 {
				if src.Confidence < minConfidence {
					continue
				}
			}
			if !hasAnyAllowedReason(src.Reasons, allow) {
				continue
			}

			if cur, ok := m[key]; ok {
				m[key] = mergeHint(cur, src)
			} else {
				m[key] = src
			}
		}
		if err := sc.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		_ = f.Close()
	}
	return m, nil
}

func readTitlesWithContinuation(path string, limit int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	reCont := regexp.MustCompile(`^:?[-]\s*\d{1,4}\b`)

	out := make([]string, 0, 1024)
	pending := ""
	flush := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		out = append(out, line)
	}

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
			flush(pending)
			if limit > 0 && len(out) >= limit {
				pending = ""
				break
			}
		}
		pending = line
	}
	if strings.TrimSpace(pending) != "" {
		flush(pending)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func nextBaseline(baseMap rawTitleMap, cursors map[string]int, rawTitle string) (*model.ParseResult, bool) {
	rawTitle = strings.TrimSpace(rawTitle)
	if rawTitle == "" {
		return nil, false
	}
	idx := cursors[rawTitle]
	arr := baseMap[rawTitle]
	if idx < 0 || idx >= len(arr) {
		return nil, false
	}
	cursors[rawTitle] = idx + 1
	return &arr[idx], true
}

func strictlyImprovesHardcase(base *model.ParseResult, cur *model.ParseResult) bool {
	br := hardcaseReasons(base)
	nr := hardcaseReasons(cur)
	if !br["any_risk"] {
		return false
	}
	if nr["any_risk"] {
		// Must remove at least one risk reason.
		removed := false
		for k := range br {
			if k == "any_risk" {
				continue
			}
			if !nr[k] {
				removed = true
				break
			}
		}
		if !removed {
			return false
		}
	} else {
		// all risk removed
		return true
	}
	// Ensure no new risk reasons introduced.
	for k := range nr {
		if k == "any_risk" {
			continue
		}
		if !br[k] {
			return false
		}
	}
	return true
}

func writeOverriddenWithGate(
	p *parser.Parser,
	titles []string,
	baseMap rawTitleMap,
	overrides map[string]overrideHint,
	outPath string,
	riskOnly bool,
	strictGate bool,
	titleSanityGate bool,
	baselineFallback bool,
) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	defer bw.Flush()
	enc := json.NewEncoder(bw)

	baseCursors := map[string]int{}
	processed := 0
	withBaseline := 0
	withProposal := 0
	attempted := 0
	accepted := 0
	rejected := 0
	rejectedTitleSanity := 0
	skippedRiskOnly := 0
	skippedNoBaseStrict := 0
	parseErrors := 0
	for _, t := range titles {
		raw := strings.TrimSpace(t)
		if raw == "" {
			continue
		}
		processed++

		baseRes, hasBase := nextBaseline(baseMap, baseCursors, raw)
		baseIsRisk := false
		if hasBase {
			withBaseline++
			br := hardcaseReasons(baseRes)
			baseIsRisk = br["any_risk"]
		}

		// Decide output before encoding.
		var outRes *model.ParseResult
		if baselineFallback && hasBase {
			outRes = baseRes
		}

		h := sha1Hex(raw)
		ov, ok := overrides[h]
		if ok {
			withProposal++
			if (!riskOnly) || baseIsRisk {
				if strings.EqualFold(strings.TrimSpace(ov.Choice), "alt") {
					ph := &parser.ParseHints{}
					set := false
					if strings.TrimSpace(ov.Episode) != "" && ov.EpisodeIdx >= 0 {
						ph.OverrideEpisode = ov.Episode
						ph.OverrideEpisodeIdx = ov.EpisodeIdx
						set = true
					}
					if ov.TitleStart >= 0 && ov.TitleEnd > ov.TitleStart {
						ph.OverrideTitleStart = ov.TitleStart
						ph.OverrideTitleEnd = ov.TitleEnd
						set = true
					}
					if set {
						attempted++
						res, err := p.ParseWithHints(raw, ph)
						if err != nil {
							fmt.Fprintf(os.Stderr, "parse error: %v | %s\n", err, raw)
							parseErrors++
							res = nil
						}
						if res != nil {
							accept := true
							if strictGate {
								// Without baseline we can't compare for "strict" improvement.
								if !hasBase {
									skippedNoBaseStrict++
									accept = false
								} else {
									accept = strictlyImprovesHardcase(baseRes, res)
								}
							}
							if accept && titleSanityGate {
								if !isSaneTitle(strings.TrimSpace(res.CanonicalTitle())) {
									accept = false
									rejectedTitleSanity++
								}
							}
							if accept {
								outRes = res
								accepted++
							} else {
								rejected++
							}
						}
					}
				}
			} else {
				skippedRiskOnly++
			}
		}

		// If we don't have baseline fallback, always emit something.
		if outRes == nil {
			res, err := p.Parse(raw)
			if err != nil {
				fmt.Fprintf(os.Stderr, "parse error: %v | %s\n", err, raw)
				continue
			}
			outRes = res
		}
		if err := enc.Encode(outRes); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "processed=%d baseline=%d proposals=%d attempted=%d accepted=%d rejected=%d rejected_title_sanity=%d skipped_risk_only=%d skipped_no_base_strict=%d parse_errors=%d\n",
		processed, withBaseline, withProposal, attempted, accepted, rejected, rejectedTitleSanity, skippedRiskOnly, skippedNoBaseStrict, parseErrors)
	return nil
}

func isSaneTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	stop := map[string]bool{
		"国漫":   true,
		"國漫":   true,
		"日漫":   true,
		"日本动漫": true,
		"日本動畫": true,
		"韩漫":   true,
		"韓漫":   true,
		"美漫":   true,
		"欧美":   true,
		"歐美":   true,
	}
	if stop[t] {
		return false
	}
	// Reject very short titles, but avoid being too aggressive for CJK.
	if len([]rune(t)) <= 2 {
		return false
	}
	// Reject titles that look like an unfinished Latin fragment (e.g. "One").
	if regexp.MustCompile(`^[A-Za-z]{1,4}$`).MatchString(t) {
		return false
	}
	// Reject obvious tech leakage.
	up := strings.ToUpper(t)
	if strings.Contains(up, "1080") || strings.Contains(up, "2160") || strings.Contains(up, "720") {
		return false
	}
	if strings.Contains(up, "WEB") || strings.Contains(up, "HEVC") || strings.Contains(up, "AVC") || strings.Contains(up, "10BIT") {
		return false
	}
	return true
}

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

func computeEval(baseMap rawTitleMap, baseLines int, newMap rawTitleMap, newLines int, topN int) *evalReport {
	r := &evalReport{
		BaseLines:       baseLines,
		NewLines:        newLines,
		BaseCounts:      map[string]int{},
		NewCounts:       map[string]int{},
		Delta:           map[string]int{},
		FixedCounts:     map[string]int{},
		RegressedCounts: map[string]int{},
		FixedTopN:       map[string][]string{},
		RegressedTopN:   map[string][]string{},
		TopN:            topN,
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
			diffEvalOne(r, &bl[i], &nl[i])
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
	return r
}

func diffEvalOne(r *evalReport, base *model.ParseResult, cur *model.ParseResult) {
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
		if a.TokenStart < ted && tst < a.TokenEnd {
			return true
		}
	}
	return false
}

func computeDiffSummary(baseMap rawTitleMap, newMap rawTitleMap, topN int) *diffSummary {
	d := &diffSummary{
		FieldChanges:       map[string]int{},
		FieldChangeTopN:    map[string][]string{},
		ContentKindChanges: map[string]int{},
		ContentKindTopN:    map[string][]string{},
		TopN:               topN,
	}

	allKeys := unionKeys(baseMap, newMap)
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
			d.Compared++
			diffOne(d, &bl[i], &nl[i])
		}
	}

	return d
}

func diffOne(d *diffSummary, b *model.ParseResult, n *model.ParseResult) {
	checkFieldChange(d, "content_kind", b.ContentKind, n.ContentKind, b.RawTitle)
	checkFieldChange(d, "template_id", b.TemplateID, n.TemplateID, b.RawTitle)
	checkFieldChange(d, "group", b.Group, n.Group, b.RawTitle)
	checkFieldChange(d, "title", b.Title, n.Title, b.RawTitle)
	checkFieldChange(d, "episode", b.Episode, n.Episode, b.RawTitle)
	checkFieldChange(d, "episode_range", b.EpisodeRange, n.EpisodeRange, b.RawTitle)
	checkFieldChange(d, "season", b.Season, n.Season, b.RawTitle)
	checkFieldChange(d, "pack_kind", b.PackKind, n.PackKind, b.RawTitle)
	checkFieldChange(d, "sp_kind", b.SPKind, n.SPKind, b.RawTitle)
	checkFieldChange(d, "movie_index", b.MovieIndex, n.MovieIndex, b.RawTitle)

	if strings.TrimSpace(b.ContentKind) != strings.TrimSpace(n.ContentKind) {
		from := strings.TrimSpace(b.ContentKind)
		to := strings.TrimSpace(n.ContentKind)
		k := from + " -> " + to
		d.ContentKindChanges[k]++
		addTopN(d.ContentKindTopN, k, b.RawTitle, d.TopN)
	}
}

func checkFieldChange(d *diffSummary, field string, b string, n string, rawTitle string) {
	if strings.TrimSpace(b) != strings.TrimSpace(n) {
		d.FieldChanges[field]++
		addTopN(d.FieldChangeTopN, field, rawTitle, d.TopN)
	}
}

func readAll(path string) (io.Reader, func() error, error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdin, func() error { return nil }, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
