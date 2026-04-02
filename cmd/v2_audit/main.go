package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type AuditRecord struct {
	RawTitle string             `json:"raw_title"`
	V2       *model.ParseResult `json:"v2"`
	Audit    AuditResult        `json:"audit"`
}

type AuditResult struct {
	Issues        []string  `json:"issues,omitempty"`
	Warnings      []string  `json:"warnings,omitempty"`
	AttrHits      []AttrHit `json:"attr_hits,omitempty"`
	EPSummary     EPSummary `json:"ep_summary,omitempty"`
	AliasMetaHits []string  `json:"alias_meta_hits,omitempty"`
}

type AttrHit struct {
	Type       string  `json:"type"`
	Normalized string  `json:"normalized,omitempty"`
	TokenStart int     `json:"token_start,omitempty"`
	TokenEnd   int     `json:"token_end,omitempty"`
	TokenText  string  `json:"token_text,omitempty"`
	Source     string  `json:"source,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type EPSummary struct {
	PrimaryKind          string   `json:"primary_kind,omitempty"`
	PrimarySeason        string   `json:"primary_season,omitempty"`
	PrimaryEpisode       string   `json:"primary_episode,omitempty"`
	PrimaryEpisodeRange  string   `json:"primary_episode_range,omitempty"`
	UniqueEpisodes       []string `json:"unique_episodes,omitempty"`
	UniqueRanges         []string `json:"unique_ranges,omitempty"`
	SuspiciousCandidates []string `json:"suspicious_candidates,omitempty"`
}

type SummaryTokenStat struct {
	Token    string   `json:"token"`
	Count    int      `json:"count"`
	Examples []string `json:"examples,omitempty"`
}

type SummaryKVStat struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type AuditSummary struct {
	Processed   int `json:"processed"`
	ParseErrors int `json:"parse_errors"`

	UnknownTokens []SummaryTokenStat `json:"unknown_tokens,omitempty"`
	IssueCounts   []SummaryKVStat    `json:"issue_counts,omitempty"`
	WarningCounts []SummaryKVStat    `json:"warning_counts,omitempty"`
}

type summaryCollector struct {
	unknownTokenCount map[string]int
	unknownExamples   map[string][]string
	issueCount        map[string]int
	warningCount      map[string]int
	parseErrors       int
	processed         int

	maxUnknown  int
	maxExamples int
}

func newSummaryCollector(maxUnknown int, maxExamples int) *summaryCollector {
	if maxUnknown <= 0 {
		maxUnknown = 50
	}
	if maxExamples <= 0 {
		maxExamples = 3
	}
	return &summaryCollector{
		unknownTokenCount: map[string]int{},
		unknownExamples:   map[string][]string{},
		issueCount:        map[string]int{},
		warningCount:      map[string]int{},
		maxUnknown:        maxUnknown,
		maxExamples:       maxExamples,
	}
}

var rePureNumber = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

func normalizeUnknownToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s == "[" || s == "]" || s == "【" || s == "】" || s == "(" || s == ")" || s == "（" || s == "）" {
		return ""
	}
	if s == "/" || s == "／" || s == "-" || s == "_" || s == ":" || s == "：" || s == "+" || s == "＋" || s == "|" || s == "｜" || s == ";" || s == "；" || s == "," {
		return ""
	}
	if rePureNumber.MatchString(s) {
		return ""
	}
	low := strings.ToLower(s)
	low = strings.TrimSpace(low)
	return low
}

func (sc *summaryCollector) addExample(key string, rawTitle string) {
	if key == "" || strings.TrimSpace(rawTitle) == "" {
		return
	}
	ex := sc.unknownExamples[key]
	if len(ex) >= sc.maxExamples {
		return
	}
	for _, e := range ex {
		if e == rawTitle {
			return
		}
	}
	sc.unknownExamples[key] = append(ex, rawTitle)
}

func (sc *summaryCollector) collectUnknownTokens(res *model.ParseResult, rawTitle string) {
	if res == nil {
		return
	}
	ignored := make([]bool, len(res.Tokens))
	for _, a := range res.Attributes {
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		if a.TokenStart >= len(ignored) {
			continue
		}
		end := a.TokenEnd
		if end > len(ignored) {
			end = len(ignored)
		}
		for i := a.TokenStart; i < end; i++ {
			ignored[i] = true
		}
	}

	seen := map[string]bool{}
	for i, t := range res.Tokens {
		if i >= 0 && i < len(ignored) && ignored[i] {
			continue
		}
		if t.Removed {
			continue
		}
		key := normalizeUnknownToken(t.Text)
		if key == "" {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		sc.unknownTokenCount[key]++
		sc.addExample(key, rawTitle)
	}
}

func (sc *summaryCollector) collectIssuesWarnings(a AuditResult) {
	for _, it := range a.Issues {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		sc.issueCount[it]++
	}
	for _, wt := range a.Warnings {
		wt = strings.TrimSpace(wt)
		if wt == "" {
			continue
		}
		sc.warningCount[wt]++
	}
}

func (sc *summaryCollector) buildSummary() AuditSummary {
	s := AuditSummary{Processed: sc.processed, ParseErrors: sc.parseErrors}

	// unknown tokens
	toks := make([]SummaryTokenStat, 0, len(sc.unknownTokenCount))
	for k, c := range sc.unknownTokenCount {
		st := SummaryTokenStat{Token: k, Count: c}
		if ex := sc.unknownExamples[k]; len(ex) > 0 {
			st.Examples = ex
		}
		toks = append(toks, st)
	}
	sort.Slice(toks, func(i, j int) bool {
		if toks[i].Count == toks[j].Count {
			return toks[i].Token < toks[j].Token
		}
		return toks[i].Count > toks[j].Count
	})
	if sc.maxUnknown > 0 && len(toks) > sc.maxUnknown {
		toks = toks[:sc.maxUnknown]
	}
	s.UnknownTokens = toks

	// issues
	issues := make([]SummaryKVStat, 0, len(sc.issueCount))
	for k, c := range sc.issueCount {
		issues = append(issues, SummaryKVStat{Key: k, Count: c})
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Count == issues[j].Count {
			return issues[i].Key < issues[j].Key
		}
		return issues[i].Count > issues[j].Count
	})
	s.IssueCounts = issues

	// warnings
	warns := make([]SummaryKVStat, 0, len(sc.warningCount))
	for k, c := range sc.warningCount {
		warns = append(warns, SummaryKVStat{Key: k, Count: c})
	}
	sort.Slice(warns, func(i, j int) bool {
		if warns[i].Count == warns[j].Count {
			return warns[i].Key < warns[j].Key
		}
		return warns[i].Count > warns[j].Count
	})
	s.WarningCounts = warns

	return s
}

var reStreamMetaFlag = regexp.MustCompile(`(?i)\b(?:multi[-_ ]?subs?|multisub|dual[-_ ]?audio|multi[-_ ]?audio|english\s*dub|dub)\b`)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	inPath := flag.String("in", "", "input titles file (one title per line) or jsonl with raw_title")
	outPath := flag.String("out", "", "output jsonl (default: stdout)")
	limit := flag.Int("limit", 0, "max records to process (0=all)")
	progressEvery := flag.Int("progress", 0, "print progress to stderr every N records (0=disabled)")
	full := flag.Bool("full", true, "include full v2 ParseResult in output")
	records := flag.Bool("records", true, "emit per-record jsonl records")
	summary := flag.Bool("summary", false, "print summary json to stderr after processing")
	summaryOut := flag.String("summary_out", "", "write summary json to file instead of stderr")
	summaryTop := flag.Int("summary_top", 50, "top N unknown tokens to include in summary")
	summaryExamples := flag.Int("summary_examples", 3, "max examples per unknown token")
	flag.Parse()

	if strings.TrimSpace(*inPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: v2_audit -in INPUT [-out OUT.jsonl] [-config configs]")
		os.Exit(2)
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	p := parser.New(cfg)

	inF, err := os.Open(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer inF.Close()

	var outW *bufio.Writer
	if strings.TrimSpace(*outPath) == "" {
		if !*records {
			outW = bufio.NewWriter(io.Discard)
		} else {
			outW = bufio.NewWriter(os.Stdout)
		}
	} else {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open out: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		outW = bufio.NewWriter(f)
	}
	defer outW.Flush()

	scanner := bufio.NewScanner(inF)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var enc *json.Encoder
	if *records || strings.TrimSpace(*outPath) != "" {
		enc = json.NewEncoder(outW)
	}

	var sc *summaryCollector
	if *summary || strings.TrimSpace(*summaryOut) != "" {
		sc = newSummaryCollector(*summaryTop, *summaryExamples)
	}

	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if *limit > 0 && count >= *limit {
			break
		}
		count++
		if sc != nil {
			sc.processed = count
		}
		if *progressEvery > 0 && count%*progressEvery == 0 {
			fmt.Fprintf(os.Stderr, "processed=%d\n", count)
		}

		rawTitle := extractRawTitle(line)
		if rawTitle == "" {
			continue
		}

		res, err := p.Parse(rawTitle)
		if err != nil {
			if sc != nil {
				sc.parseErrors++
			}
			a := AuditRecord{RawTitle: rawTitle, V2: nil, Audit: AuditResult{Issues: []string{"parse_error"}}}
			if enc != nil {
				_ = enc.Encode(a)
			}
			continue
		}

		audit := auditOne(res)
		if sc != nil {
			sc.collectIssuesWarnings(audit)
			sc.collectUnknownTokens(res, rawTitle)
		}
		var v2 *model.ParseResult
		if *full {
			v2 = res
		} else {
			v2 = &model.ParseResult{
				RawTitle:           res.RawTitle,
				ContentKind:        res.ContentKind,
				ContentConfidence:  res.ContentConfidence,
				TemplateID:         res.TemplateID,
				TemplateConfidence: res.TemplateConfidence,
				EPSemantics:        res.CanonicalEPSemantics(),
				AudioSlot:          res.AudioSlot,
				Attributes:         res.Attributes,
				Tokens:             res.Tokens,
			}
		}

		rec := AuditRecord{RawTitle: rawTitle, V2: v2, Audit: audit}
		if enc != nil {
			_ = enc.Encode(rec)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}

	if sc != nil {
		sum := sc.buildSummary()
		bs, err := json.Marshal(sum)
		if err == nil {
			if strings.TrimSpace(*summaryOut) != "" {
				_ = os.WriteFile(*summaryOut, append(bs, '\n'), 0644)
			} else if *summary {
				fmt.Fprintln(os.Stderr, string(bs))
			}
		}
	}
}

func extractRawTitle(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err == nil {
		if rt, ok := m["raw_title"]; ok {
			if s, ok := rt.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return line
}

func auditOne(res *model.ParseResult) AuditResult {
	var out AuditResult
	if res == nil {
		out.Issues = append(out.Issues, "nil_result")
		return out
	}

	if strings.TrimSpace(res.TemplateID) == "TF_UNKNOWN_DEFAULT" {
		out.Warnings = append(out.Warnings, "unknown_default_template")
	}
	epRoute := strings.TrimSpace(res.CanonicalEpRoute())
	season := strings.TrimSpace(res.CanonicalSeason())
	episode := strings.TrimSpace(res.CanonicalEpisode())
	episodeRange := strings.TrimSpace(res.CanonicalEpisodeRange())
	if epRoute == "SXXEXX" {
		if season == "" || episode == "" {
			out.Issues = append(out.Issues, "sxxexx_route_missing_season_or_episode")
		}
	}
	if epRoute == "RANGE" {
		if episodeRange == "" {
			out.Issues = append(out.Issues, "range_route_missing_episode_range")
		}
	}

	out.AttrHits = buildAttrHits(res)
	out.EPSummary = buildEPSummary(res)
	out.AliasMetaHits = aliasMetaHits(res.CanonicalTitleAliases())
	if len(out.AliasMetaHits) > 0 {
		out.Warnings = append(out.Warnings, "alias_contains_streaming_meta")
	}

	if strings.TrimSpace(res.CanonicalTitle()) == "" {
		out.Issues = append(out.Issues, "missing_title")
	}
	if strings.TrimSpace(res.CanonicalGroup()) == "" {
		out.Warnings = append(out.Warnings, "missing_group")
	}

	if len(out.EPSummary.UniqueEpisodes) > 1 {
		out.Warnings = append(out.Warnings, "multiple_episode_candidates")
	}
	if len(out.EPSummary.UniqueRanges) > 1 {
		out.Warnings = append(out.Warnings, "multiple_episode_range_candidates")
	}
	if len(out.EPSummary.SuspiciousCandidates) > 0 {
		out.Warnings = append(out.Warnings, "suspicious_episode_candidates")
	}

	for _, h := range out.AttrHits {
		if isSuspiciousAttrHit(h) {
			out.Warnings = append(out.Warnings, "suspicious_lexicon_hit")
			break
		}
	}

	out.Issues = uniqueStrings(out.Issues)
	out.Warnings = uniqueStrings(out.Warnings)
	return out
}

func buildAttrHits(res *model.ParseResult) []AttrHit {
	if res == nil {
		return nil
	}
	out := make([]AttrHit, 0, len(res.Attributes))
	for _, a := range res.Attributes {
		start := a.TokenStart
		end := a.TokenEnd
		text := ""
		if start >= 0 && end > start && end <= len(res.Tokens) {
			parts := make([]string, 0, end-start)
			for i := start; i < end; i++ {
				parts = append(parts, res.Tokens[i].Text)
			}
			text = strings.TrimSpace(strings.Join(parts, " "))
		}
		out = append(out, AttrHit{Type: string(a.Type), Normalized: a.Normalized, TokenStart: a.TokenStart, TokenEnd: a.TokenEnd, TokenText: text, Source: a.Source, Confidence: a.Confidence})
	}
	return out
}

func buildEPSummary(res *model.ParseResult) EPSummary {
	var s EPSummary
	if res == nil {
		return s
	}
	sem := res.CanonicalEPSemantics()
	if sem == nil {
		return s
	}
	s.PrimaryKind = sem.Kind
	s.PrimarySeason = sem.Season
	s.PrimaryEpisode = sem.Episode
	if strings.TrimSpace(sem.RangeStart) != "" && strings.TrimSpace(sem.RangeEnd) != "" {
		s.PrimaryEpisodeRange = strings.TrimSpace(sem.RangeStart) + "-" + strings.TrimSpace(sem.RangeEnd)
	}

	epSet := map[string]bool{}
	rSet := map[string]bool{}
	susp := make([]string, 0, 2)

	addEP := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		epSet[v] = true
		if n, err := strconv.Atoi(strings.Split(v, ".")[0]); err == nil && (n >= 100 || v == "0") {
			susp = append(susp, v)
		}
	}
	addR := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		rSet[v] = true
	}

	addEP(sem.Episode)
	for _, item := range sem.EpisodeList {
		if strings.Contains(item, "-") || strings.Contains(item, "~") {
			addR(item)
			continue
		}
		addEP(item)
	}
	addR(s.PrimaryEpisodeRange)

	s.UniqueEpisodes = keysSorted(epSet)
	s.UniqueRanges = keysSorted(rSet)
	s.SuspiciousCandidates = uniqueStrings(susp)
	return s
}

func keysSorted(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func aliasMetaHits(aliases []string) []string {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	for _, a := range aliases {
		if reStreamMetaFlag.MatchString(a) {
			out = append(out, a)
		}
	}
	return uniqueStrings(out)
}

func isSuspiciousAttrHit(h AttrHit) bool {
	if strings.TrimSpace(h.Source) != "lexicon" {
		return false
	}
	t := strings.TrimSpace(strings.ToLower(h.Type))
	n := strings.TrimSpace(h.Normalized)
	tok := strings.TrimSpace(h.TokenText)
	if t == "sub_lang" {
		if tok == "" {
			return true
		}
		upTok := strings.ToUpper(tok)
		safe := map[string]bool{"CHS": true, "CHT": true, "CHST": true, "JPN": true, "ENG": true, "FRA": true, "VOSTFR": true, "VOSTEN": true}
		if safe[n] {
			return false
		}
		if strings.Contains(upTok, "MP4") || strings.Contains(upTok, "MKV") {
			return true
		}
	}
	return false
}
