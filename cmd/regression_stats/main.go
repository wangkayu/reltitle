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

type Stats struct {
	TotalAll    int
	Total       int
	ParsedOK    int
	ParsedError int

	ContentKind map[string]int
	EpRoute     map[string]int
	TemplateID  map[string]int

	NonEmpty map[string]int

	EpRouteTotals      map[string]int
	EpRouteUnknown     map[string]int
	UnknownDefaultTopN map[string][]string
	UnknownTokenFreq   map[string]int
	UnknownTechFreq    map[string]int
	AllTokenFreq       map[string]int
	AllTechFreq        map[string]int

	SampleToken        string
	SampleTokenTop     int
	SampleTokenHits    []string
	SampleTokenTechHit []string

	StreamingTotal          int
	StreamingTemplateHit    int
	StreamingTailParenEmpty int
	StreamingTailParenPure  int
	StreamingTailParenMeta  int
	StreamingTailParenMixed int
	StreamingMetaAliasBad   int
	StreamingMissTopN       []string
	StreamingMissTokenFreq  map[string]int
	StreamingMissTechFreq   map[string]int
	StreamingTotalByEpRoute map[string]int
	StreamingMissByEpRoute  map[string]int
	StreamingMissByKind     map[string]int

	NoiseSkipped         int
	NoiseSkippedByReason map[string]int
}

type inputRecord struct {
	RawTitle string `json:"raw_title"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	inPath := flag.String("in", "", "input jsonl file (default: stdin)")
	configDir := flag.String("config", "configs", "config directory (used when re-parsing titles)")
	reparse := flag.Bool("reparse", false, "re-parse raw_title using v2 parser instead of trusting json fields")
	plainTitles := flag.Bool("plain_titles", false, "treat input as plain text titles (one raw_title per line) instead of jsonl")
	sampleToken := flag.String("sample_token", "", "print sample raw_title lines whose residual tokens include this token (case-insensitive; uses same counting rules as token stats)")
	sampleTokenTop := flag.Int("sample_token_top", 20, "max number of sample raw_title lines to print for -sample_token")
	topN := flag.Int("top", 20, "print top N raw_title samples when template_id is TF_UNKNOWN_DEFAULT (grouped by ep_route)")
	topTplN := flag.Int("top_templates", 25, "print top N template_id distribution with signature")
	streamingMissTopN := flag.Int("top_streaming_miss", 10, "print top N raw_title samples where streaming_release(strict attrs) is true but template_id is not TF_STREAMING_*")
	topUnknownTokens := flag.Int("top_unknown_tokens", 30, "print top N non-removed token frequencies when template_id is TF_UNKNOWN_DEFAULT")
	topUnknownTechTokens := flag.Int("top_unknown_tech_tokens", 30, "print top N non-removed token frequencies after techStart when template_id is TF_UNKNOWN_DEFAULT")
	topAllTokens := flag.Int("top_tokens_all", 0, "print top N non-removed token frequencies across all parsed titles")
	topAllTechTokens := flag.Int("top_tech_tokens_all", 0, "print top N non-removed token frequencies after techStart across all parsed titles")
	topStreamingMissTokens := flag.Int("top_streaming_miss_tokens", 30, "print top N non-removed token frequencies for streaming misses")
	topStreamingMissTechTokens := flag.Int("top_streaming_miss_tech_tokens", 30, "print top N non-removed token frequencies after techStart for streaming misses")
	limit := flag.Int("limit", 0, "limit number of records to process (0 = no limit)")
	progressEvery := flag.Int("progress", 0, "print progress to stderr every N records (0 = disabled)")
	flag.Parse()

	var f *os.File
	var err error
	if strings.TrimSpace(*inPath) == "" {
		f = os.Stdin
	} else {
		f, err = os.Open(*inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open input: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
	}

	st := &Stats{
		ContentKind:             map[string]int{},
		EpRoute:                 map[string]int{},
		TemplateID:              map[string]int{},
		NonEmpty:                map[string]int{},
		EpRouteTotals:           map[string]int{},
		EpRouteUnknown:          map[string]int{},
		UnknownDefaultTopN:      map[string][]string{},
		UnknownTokenFreq:        map[string]int{},
		UnknownTechFreq:         map[string]int{},
		AllTokenFreq:            map[string]int{},
		AllTechFreq:             map[string]int{},
		StreamingMissTokenFreq:  map[string]int{},
		StreamingMissTechFreq:   map[string]int{},
		StreamingTotalByEpRoute: map[string]int{},
		StreamingMissByEpRoute:  map[string]int{},
		StreamingMissByKind:     map[string]int{},
		NoiseSkippedByReason:    map[string]int{},
	}
	st.SampleToken = strings.ToLower(strings.TrimSpace(*sampleToken))
	st.SampleTokenTop = *sampleTokenTop

	var p *parser.Parser
	var loadParserOnce func() error
	loadParserOnce = func() error {
		if p != nil {
			return nil
		}
		cfg, err := config.LoadAll(*configDir)
		if err != nil {
			return err
		}
		p = parser.New(cfg)
		return nil
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		if *limit > 0 && st.TotalAll >= *limit {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		st.TotalAll++
		if *progressEvery > 0 && st.TotalAll%*progressEvery == 0 {
			fmt.Fprintf(os.Stderr, "processed=%d\n", st.TotalAll)
		}

		var in inputRecord
		var res model.ParseResult
		needReparse := *reparse
		if *plainTitles {
			in.RawTitle = line
			res.RawTitle = line
			needReparse = true
		} else {
			_ = json.Unmarshal([]byte(line), &in)
			_ = json.Unmarshal([]byte(line), &res)
			if !needReparse {
				// Auto-reparse when input record looks like a foreign schema (e.g. training artifacts)
				// but still provides raw_title.
				if strings.TrimSpace(in.RawTitle) != "" {
					if strings.TrimSpace(res.ContentKind) == "" &&
						strings.TrimSpace(res.TemplateID) == "" &&
						strings.TrimSpace(res.Group) == "" &&
						strings.TrimSpace(res.Title) == "" &&
						strings.TrimSpace(res.Episode) == "" &&
						strings.TrimSpace(res.EpisodeRange) == "" &&
						strings.TrimSpace(res.Season) == "" &&
						strings.TrimSpace(res.PackKind) == "" &&
						strings.TrimSpace(res.SPKind) == "" &&
						strings.TrimSpace(res.MovieIndex) == "" {
						needReparse = true
					}
				}
			}
		}
		if needReparse {
			if strings.TrimSpace(in.RawTitle) == "" {
				st.ParsedError++
				continue
			}
			if err := loadParserOnce(); err != nil {
				fmt.Fprintf(os.Stderr, "load config: %v\n", err)
				os.Exit(1)
			}
			out, err := p.Parse(in.RawTitle)
			if err != nil {
				st.ParsedError++
				continue
			}
			st.ParsedOK++
			res = *out
		}

		rawTitle := strings.TrimSpace(in.RawTitle)
		if rawTitle == "" {
			rawTitle = strings.TrimSpace(res.RawTitle)
		}

		isUnknownDefault := strings.EqualFold(strings.TrimSpace(res.TemplateID), "TF_UNKNOWN_DEFAULT")
		if isUnknownDefault {
			if reason, ok := noiseReason(rawTitle); ok {
				st.NoiseSkipped++
				st.NoiseSkippedByReason[reason]++
				continue
			}
		}

		st.Total++

		st.ContentKind[res.ContentKind]++
		st.EpRoute[res.EpRoute]++
		st.TemplateID[strings.TrimSpace(res.TemplateID)]++
		epKey := strings.TrimSpace(res.EpRoute)
		if epKey == "" {
			epKey = "(empty)"
		}
		st.EpRouteTotals[epKey]++
		if isUnknownDefault {
			st.EpRouteUnknown[epKey]++
			if *topN > 0 {
				if len(st.UnknownDefaultTopN[epKey]) < *topN {
					if rawTitle != "" {
						st.UnknownDefaultTopN[epKey] = append(st.UnknownDefaultTopN[epKey], rawTitle)
					}
				}
			}
			if *topUnknownTokens > 0 || *topUnknownTechTokens > 0 {
				accumulateTokenFreq(st.UnknownTokenFreq, st.UnknownTechFreq, &res)
			}
		}
		if *topAllTokens > 0 || *topAllTechTokens > 0 {
			accumulateTokenFreq(st.AllTokenFreq, st.AllTechFreq, &res)
		}
		if st.SampleToken != "" && st.SampleTokenTop > 0 {
			collectSampleTokenHits(st, rawTitle, &res)
		}

		countNonEmpty(st.NonEmpty, "template_id", res.TemplateID)
		countNonEmpty(st.NonEmpty, "group", res.Group)
		countNonEmpty(st.NonEmpty, "title", res.Title)
		countNonEmpty(st.NonEmpty, "episode", res.Episode)
		countNonEmpty(st.NonEmpty, "episode_range", res.EpisodeRange)
		countNonEmpty(st.NonEmpty, "season", res.Season)
		countNonEmpty(st.NonEmpty, "pack_kind", res.PackKind)
		countNonEmpty(st.NonEmpty, "sp_kind", res.SPKind)
		countNonEmpty(st.NonEmpty, "movie_index", res.MovieIndex)

		isStreaming := isStreamingReleaseStrict(&res)
		if isStreaming {
			st.StreamingTotal++
			epKey2 := strings.TrimSpace(res.EpRoute)
			if epKey2 == "" {
				epKey2 = "(empty)"
			}
			st.StreamingTotalByEpRoute[epKey2]++
			isStreamingTpl := strings.HasPrefix(strings.TrimSpace(res.TemplateID), "TF_STREAMING_")
			if isStreamingTpl {
				st.StreamingTemplateHit++
			} else {
				st.StreamingMissByEpRoute[epKey2]++
				ck := strings.TrimSpace(res.ContentKind)
				if ck == "" {
					ck = "(empty)"
				}
				st.StreamingMissByKind[ck]++
				if *streamingMissTopN > 0 && len(st.StreamingMissTopN) < *streamingMissTopN {
					title := strings.TrimSpace(in.RawTitle)
					if title == "" {
						title = strings.TrimSpace(res.RawTitle)
					}
					if title != "" {
						st.StreamingMissTopN = append(st.StreamingMissTopN, title)
					}
				}
				if *topStreamingMissTokens > 0 || *topStreamingMissTechTokens > 0 {
					accumulateTokenFreq(st.StreamingMissTokenFreq, st.StreamingMissTechFreq, &res)
				}
			}
			bucket := tailParenBucket(strings.TrimSpace(res.TailParenBlock))
			switch bucket {
			case "empty":
				st.StreamingTailParenEmpty++
			case "pure_alias":
				st.StreamingTailParenPure++
			case "pure_meta":
				st.StreamingTailParenMeta++
			case "mixed":
				st.StreamingTailParenMixed++
			}
			if hasStreamingMetaInAliases(res.Aliases) {
				st.StreamingMetaAliasBad++
			}
		}
		if res.EPSlot != nil {
			st.NonEmpty["epslot"]++
			if res.EPSlot.Primary != nil {
				st.NonEmpty["epslot_primary"]++
			}
			if len(res.EPSlot.Candidates) > 0 {
				st.NonEmpty["epslot_candidates"]++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}

	printStats(st, *topN, *streamingMissTopN, *topTplN, *topUnknownTokens, *topUnknownTechTokens, *topAllTokens, *topAllTechTokens, *topStreamingMissTokens, *topStreamingMissTechTokens, *configDir)
}

func collectSampleTokenHits(st *Stats, rawTitle string, res *model.ParseResult) {
	if st == nil || res == nil {
		return
	}
	if strings.TrimSpace(rawTitle) == "" {
		return
	}
	if st.SampleToken == "" || st.SampleTokenTop <= 0 {
		return
	}
	if len(st.SampleTokenHits) >= st.SampleTokenTop && len(st.SampleTokenTechHit) >= st.SampleTokenTop {
		return
	}

	tokens := res.Tokens
	if len(tokens) == 0 {
		return
	}
	name := st.SampleToken
	titleStart, titleEnd, hasTitleSpan := titleSpanFromAttrs(res.Attributes)
	techStart, _ := techStartFromAttrs(res.Attributes, len(tokens))

	hasAll := false
	hasTech := false
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Removed {
			continue
		}
		if hasTitleSpan {
			if i >= titleStart && i < titleEnd {
				continue
			}
		}
		t := strings.TrimSpace(tok.Text)
		if isPunctToken(t) {
			continue
		}
		k := strings.ToLower(t)
		if k != name {
			continue
		}
		hasAll = true
		if i >= techStart {
			hasTech = true
		}
		// One hit per title is enough.
		break
	}

	if hasAll && len(st.SampleTokenHits) < st.SampleTokenTop {
		st.SampleTokenHits = append(st.SampleTokenHits, rawTitle)
	}
	if hasTech && len(st.SampleTokenTechHit) < st.SampleTokenTop {
		st.SampleTokenTechHit = append(st.SampleTokenTechHit, rawTitle)
	}
}

func noiseReason(rawTitle string) (string, bool) {
	low := strings.ToLower(strings.TrimSpace(rawTitle))
	if low == "" {
		return "", false
	}
	if strings.Contains(low, "kara.moe") {
		return "kara_moe", true
	}
	if strings.Contains(low, "karaoke") || strings.Contains(low, "karaokes") {
		return "karaoke", true
	}
	if strings.Contains(low, "backup") {
		if strings.Contains(low, "kara") || strings.Contains(low, "openings") || strings.Contains(low, "endings") {
			return "backup_collection", true
		}
	}
	if strings.Contains(low, "openings") && strings.Contains(low, "endings") && (strings.Contains(low, "jpop") || strings.Contains(low, "kpop")) {
		return "opening_ending_collection", true
	}
	return "", false
}

func countNonEmpty(m map[string]int, key string, s string) {
	if strings.TrimSpace(s) != "" {
		m[key]++
	}
}

type tplMeta struct {
	ID        string
	Name      string
	Signature string
}

func collectTemplateMeta(cfg *config.Config) map[string]tplMeta {
	out := map[string]tplMeta{}
	if cfg == nil {
		return out
	}
	add := func(t config.TemplateDef) {
		id := strings.TrimSpace(t.ID)
		if id == "" {
			return
		}
		if _, ok := out[id]; ok {
			return
		}
		out[id] = tplMeta{ID: id, Name: strings.TrimSpace(t.Name), Signature: strings.TrimSpace(t.Signature)}
	}
	for _, t := range cfg.TemplateCatalog.TVEpisodeTemplates {
		add(t)
	}
	for _, t := range cfg.TemplateCatalog.BatchTemplates {
		add(t)
	}
	for _, t := range cfg.TemplateCatalog.MovieTemplates {
		add(t)
	}
	for _, t := range cfg.TemplateCatalog.SpecialTemplates {
		add(t)
	}
	return out
}

func isPunctToken(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return true
	}
	switch t {
	case "[", "]", "【", "】", "(", ")", "（", "）", "{", "}", "<", ">":
		return true
	case "-", "_", ".", "|", "｜", "/", "／", ",", "，", ";", "；", ":", "：":
		return true
	default:
		return false
	}
}

func isTechAttrType(t model.AttrType) bool {
	s := string(t)
	switch s {
	case string(model.AttrSourceType), string(model.AttrResolution), string(model.AttrVideoCodec), string(model.AttrAudioCodec), string(model.AttrBitDepth), string(model.AttrContainer), string(model.AttrPlatform), string(model.AttrPlatformRegion), string(model.AttrCRC), string(model.AttrTech), string(model.AttrTailGroup):
		return true
	case string(model.AttrSubFlag), string(model.AttrSubLang), string(model.AttrSubFormat), string(model.AttrSubLangGroup):
		return true
	case string(model.AttrAudioFlag), string(model.AttrAudioLang), string(model.AttrAudioChannels), string(model.AttrAudioObject):
		return true
	case string(model.AttrContentRating):
		return true
	default:
		return false
	}
}

func techStartFromAttrs(attrs []model.Attribute, tokenCount int) (int, bool) {
	minIdx := 1 << 30
	for _, a := range attrs {
		if !isTechAttrType(a.Type) {
			continue
		}
		if a.TokenStart >= 0 && a.TokenStart < minIdx {
			minIdx = a.TokenStart
		}
	}
	if minIdx == 1<<30 {
		return tokenCount, false
	}
	if minIdx < 0 {
		minIdx = 0
	}
	if tokenCount > 0 && minIdx > tokenCount {
		minIdx = tokenCount
	}
	return minIdx, true
}

func accumulateTokenFreq(all map[string]int, tech map[string]int, res *model.ParseResult) {
	if res == nil {
		return
	}
	tokens := res.Tokens
	titleStart, titleEnd, hasTitleSpan := titleSpanFromAttrs(res.Attributes)
	techStart, _ := techStartFromAttrs(res.Attributes, len(tokens))
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Removed {
			continue
		}
		if hasTitleSpan {
			if i >= titleStart && i < titleEnd {
				continue
			}
		}
		t := strings.TrimSpace(tok.Text)
		if isPunctToken(t) {
			continue
		}
		k := strings.ToLower(t)
		all[k]++
		if i >= techStart {
			tech[k]++
		}
	}
}

func titleSpanFromAttrs(attrs []model.Attribute) (start int, end int, ok bool) {
	minS := 1 << 30
	maxE := -1
	for _, a := range attrs {
		if a.Type != model.AttrType("title") {
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

func printStats(st *Stats, topN int, streamingMissTopN int, topTplN int, topUnknownTokens int, topUnknownTechTokens int, topAllTokens int, topAllTechTokens int, topStreamingMissTokens int, topStreamingMissTechTokens int, configDir string) {
	fmt.Printf("total_all=%d\n", st.TotalAll)
	fmt.Printf("total=%d\n", st.Total)
	if st.NoiseSkipped > 0 {
		fmt.Printf("noise_skipped=%d\n", st.NoiseSkipped)
		if len(st.NoiseSkippedByReason) > 0 {
			keys := make([]string, 0, len(st.NoiseSkippedByReason))
			for k := range st.NoiseSkippedByReason {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Printf("noise_skipped_by_reason:\n")
			for _, k := range keys {
				fmt.Printf("  %s: %d\n", k, st.NoiseSkippedByReason[k])
			}
		}
	}
	if st.ParsedOK > 0 || st.ParsedError > 0 {
		fmt.Printf("parsed_ok=%d\n", st.ParsedOK)
		fmt.Printf("parsed_error=%d\n", st.ParsedError)
	}

	// Optional: print top template distribution with signature.
	if topTplN > 0 {
		var cfg *config.Config
		// Best-effort load config for mapping template_id -> signature.
		cfg2, err := config.LoadAll(configDir)
		if err == nil {
			cfg = cfg2
		}
		meta := collectTemplateMeta(cfg)
		fmt.Println()
		fmt.Printf("top template_id distribution (top=%d):\n", topTplN)
		type kv struct {
			k string
			v int
		}
		items := make([]kv, 0, len(st.TemplateID))
		for k, v := range st.TemplateID {
			kk := strings.TrimSpace(k)
			if kk == "" {
				kk = "(empty)"
			}
			items = append(items, kv{k: kk, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		if len(items) > topTplN {
			items = items[:topTplN]
		}
		for _, it := range items {
			pct := 0.0
			if st.Total > 0 {
				pct = float64(it.v) / float64(st.Total) * 100
			}
			m, ok := meta[it.k]
			if ok {
				fmt.Printf("  %s: %d (%.1f%%)  sig=%q  name=%q\n", it.k, it.v, pct, m.Signature, m.Name)
			} else {
				fmt.Printf("  %s: %d (%.1f%%)\n", it.k, it.v, pct)
			}
		}
	}

	fmt.Println()
	fmt.Println("content_kind distribution:")
	{
		type kv struct {
			k string
			v int
		}
		var items []kv
		for k, v := range st.ContentKind {
			items = append(items, kv{k: k, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		for _, it := range items {
			k := it.k
			if strings.TrimSpace(k) == "" {
				k = "(empty)"
			}
			pct := 0.0
			if st.Total > 0 {
				pct = float64(it.v) / float64(st.Total) * 100
			}
			fmt.Printf("  %s: %d (%.1f%%)\n", k, it.v, pct)
		}
	}

	fmt.Println()
	fmt.Println("ep_route distribution:")
	{
		type kv struct {
			k string
			v int
		}
		var items []kv
		for k, v := range st.EpRoute {
			kk := strings.TrimSpace(k)
			if kk == "" {
				kk = "(empty)"
			}
			items = append(items, kv{k: kk, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		for _, it := range items {
			pct := 0.0
			if st.Total > 0 {
				pct = float64(it.v) / float64(st.Total) * 100
			}
			fmt.Printf("  %s: %d (%.1f%%)\n", it.k, it.v, pct)
		}
	}

	fmt.Println()
	fmt.Println("TF_UNKNOWN_DEFAULT rate by ep_route:")
	{
		type kv struct {
			k string
			v int
		}
		var items []kv
		for k, v := range st.EpRouteTotals {
			items = append(items, kv{k: k, v: v})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
		for _, it := range items {
			unk := st.EpRouteUnknown[it.k]
			pct := 0.0
			if it.v > 0 {
				pct = float64(unk) / float64(it.v) * 100
			}
			fmt.Printf("  %s: %d/%d (%.1f%%)\n", it.k, unk, it.v, pct)
		}
	}

	if topN > 0 {
		fmt.Println()
		fmt.Printf("top TF_UNKNOWN_DEFAULT samples by ep_route (top=%d):\n", topN)
		{
			type kv struct {
				k string
				v int
			}
			var items []kv
			for k := range st.EpRouteTotals {
				unk := st.EpRouteUnknown[k]
				items = append(items, kv{k: k, v: unk})
			}
			sort.Slice(items, func(i, j int) bool { return items[i].v > items[j].v })
			for _, it := range items {
				if it.v <= 0 {
					continue
				}
				fmt.Printf("  %s: unknown_default=%d\n", it.k, it.v)
				samples := st.UnknownDefaultTopN[it.k]
				for _, s := range samples {
					fmt.Printf("    %s\n", s)
				}
			}
		}
	}

	printTopMap := func(title string, m map[string]int, top int) {
		if top <= 0 {
			return
		}
		if len(m) == 0 {
			return
		}
		type kv struct {
			k string
			v int
		}
		items := make([]kv, 0, len(m))
		for k, v := range m {
			kk := strings.TrimSpace(k)
			if kk == "" {
				continue
			}
			items = append(items, kv{k: kk, v: v})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].v == items[j].v {
				return items[i].k < items[j].k
			}
			return items[i].v > items[j].v
		})
		if len(items) > top {
			items = items[:top]
		}
		fmt.Println()
		fmt.Printf("%s (top=%d):\n", title, top)
		for _, it := range items {
			fmt.Printf("  %s: %d\n", it.k, it.v)
		}
	}

	printTopMap("top TF_UNKNOWN_DEFAULT tokens (non-removed)", st.UnknownTokenFreq, topUnknownTokens)
	printTopMap("top TF_UNKNOWN_DEFAULT tech-tail tokens (after techStart)", st.UnknownTechFreq, topUnknownTechTokens)
	printTopMap("top tokens all (non-removed)", st.AllTokenFreq, topAllTokens)
	printTopMap("top tech-tail tokens all (after techStart)", st.AllTechFreq, topAllTechTokens)
	printTopMap("top streaming_miss tokens (non-removed)", st.StreamingMissTokenFreq, topStreamingMissTokens)
	printTopMap("top streaming_miss tech-tail tokens (after techStart)", st.StreamingMissTechFreq, topStreamingMissTechTokens)
	if strings.TrimSpace(st.SampleToken) != "" {
		fmt.Println()
		fmt.Printf("sample_token=%q (top=%d):\n", st.SampleToken, st.SampleTokenTop)
		if len(st.SampleTokenHits) == 0 {
			fmt.Printf("  hits: 0\n")
		} else {
			fmt.Printf("  hits: %d\n", len(st.SampleTokenHits))
			for _, s := range st.SampleTokenHits {
				fmt.Printf("    %s\n", s)
			}
		}
		if len(st.SampleTokenTechHit) == 0 {
			fmt.Printf("  tech_tail_hits: 0\n")
		} else {
			fmt.Printf("  tech_tail_hits: %d\n", len(st.SampleTokenTechHit))
			for _, s := range st.SampleTokenTechHit {
				fmt.Printf("    %s\n", s)
			}
		}
	}

	fmt.Println()
	fmt.Println("streaming_release metrics (strict attrs):")
	fmt.Printf("  streaming_total: %d\n", st.StreamingTotal)
	fmt.Printf("  streaming_template_hit(TF_STREAMING_*): %d\n", st.StreamingTemplateHit)
	fmt.Printf("  tail_paren_bucket: empty=%d pure_alias=%d pure_meta=%d mixed=%d\n", st.StreamingTailParenEmpty, st.StreamingTailParenPure, st.StreamingTailParenMeta, st.StreamingTailParenMixed)
	fmt.Printf("  meta_in_alias_violations: %d\n", st.StreamingMetaAliasBad)
	{
		keys := make([]string, 0, len(st.StreamingTotalByEpRoute))
		for k := range st.StreamingTotalByEpRoute {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("  streaming_total_by_ep_route:\n")
		for _, k := range keys {
			fmt.Printf("    %s: %d\n", k, st.StreamingTotalByEpRoute[k])
		}
		keys2 := make([]string, 0, len(st.StreamingMissByEpRoute))
		for k := range st.StreamingMissByEpRoute {
			keys2 = append(keys2, k)
		}
		sort.Strings(keys2)
		fmt.Printf("  streaming_miss_by_ep_route:\n")
		for _, k := range keys2 {
			fmt.Printf("    %s: %d\n", k, st.StreamingMissByEpRoute[k])
		}
		keys3 := make([]string, 0, len(st.StreamingMissByKind))
		for k := range st.StreamingMissByKind {
			keys3 = append(keys3, k)
		}
		sort.Strings(keys3)
		fmt.Printf("  streaming_miss_by_content_kind:\n")
		for _, k := range keys3 {
			fmt.Printf("    %s: %d\n", k, st.StreamingMissByKind[k])
		}
	}
	if streamingMissTopN > 0 {
		fmt.Printf("  top_streaming_miss (top=%d):\n", streamingMissTopN)
		for _, s := range st.StreamingMissTopN {
			fmt.Printf("    %s\n", s)
		}
	}

	fmt.Println()
	fmt.Println("field non-empty rate:")
	fields := []string{"template_id", "group", "title", "episode", "episode_range", "season", "pack_kind", "sp_kind", "movie_index", "epslot", "epslot_primary", "epslot_candidates"}
	for _, f := range fields {
		v := st.NonEmpty[f]
		pct := 0.0
		if st.Total > 0 {
			pct = float64(v) / float64(st.Total) * 100
		}
		fmt.Printf("  %s: %d/%d (%.1f%%)\n", f, v, st.Total, pct)
	}
}

var streamingMetaRe = regexp.MustCompile(`(?i)\b(?:multi[-_ ]?subs?|multisub|dual[-_ ]?audio|multi[-_ ]?audio|english\s*dub|dub)\b`)

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

func hasStreamingMetaInAliases(aliases []string) bool {
	for _, a := range aliases {
		if streamingMetaRe.MatchString(a) {
			return true
		}
	}
	return false
}

func tailParenBucket(tail string) string {
	if strings.TrimSpace(tail) == "" {
		return "empty"
	}
	parts := regexp.MustCompile(`[,，;；\|/]+`).Split(tail, -1)
	hasAlias := false
	hasMeta := false
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		if streamingMetaRe.MatchString(s) {
			hasMeta = true
			continue
		}
		hasAlias = true
	}
	if hasAlias && hasMeta {
		return "mixed"
	}
	if hasMeta {
		return "pure_meta"
	}
	return "pure_alias"
}
