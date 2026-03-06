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

func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	inPath := flag.String("in", "", "input title file (one title per line, supports continuation lines). default: stdin")
	overridesPath := flag.String("overrides", "", "override proposal jsonl(s). comma-separated. (e.g. from cmd/propose_rerank_episodes / cmd/propose_rerank_titlecut)")
	outPath := flag.String("out", "", "output jsonl (ParseResult). default: stdout")
	limit := flag.Int("limit", 0, "max number of titles to process (0 = no limit)")
	minConfidence := flag.Float64("min_confidence", 0.0, "only apply overrides when recommended.confidence >= this threshold")
	allowReasons := flag.String("allow_reasons", "", "comma-separated allowlist of proposal reasons. empty=allow all. (e.g. parser_title_empty,parser_title_contains_hard_tech)")
	riskOnly := flag.Bool("risk_only", true, "only consider applying overrides for samples that are hardcase risk")
	strictGate := flag.Bool("strict_gate", true, "accept override only when it improves hardcase reasons and introduces no new risk reasons")
	titleSanityGate := flag.Bool("title_sanity_gate", true, "reject overrides when parsed title is obviously low quality (stopwords/too-short/tech leakage)")
	flag.Parse()

	if strings.TrimSpace(*overridesPath) == "" {
		fmt.Fprintf(os.Stderr, "missing -overrides\n")
		os.Exit(1)
	}

	cfg, err := config.LoadAll(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	p := parser.New(cfg)

	paths := splitCSVPaths(*overridesPath)
	overrides, err := loadOverrides(paths, *minConfidence, *allowReasons)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load overrides: %v\n", err)
		os.Exit(1)
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
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir out dir: %v\n", err)
			os.Exit(1)
		}
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

	processed := 0
	withProposal := 0
	attempted := 0
	accepted := 0
	rejected := 0
	rejectedTitleSanity := 0
	skippedRiskOnly := 0
	parseErrors := 0

	flush := func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" {
			return nil
		}
		processed++
		baseRes, err := p.Parse(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error: %v | %s\n", err, line)
			parseErrors++
			return nil
		}

		outRes := baseRes

		h := sha1Hex(line)
		ov, ok := overrides[h]
		if ok {
			withProposal++
			baseIsRisk := hardcaseReasons(baseRes)["any_risk"]
			if (!*riskOnly) || baseIsRisk {
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
						altRes, err := p.ParseWithHints(line, ph)
						if err != nil {
							fmt.Fprintf(os.Stderr, "parse error: %v | %s\n", err, line)
							parseErrors++
							altRes = nil
						}
						if altRes != nil {
							accept := true
							if *strictGate {
								accept = strictlyImprovesHardcase(baseRes, altRes)
							}
							if accept && *titleSanityGate {
								if !isSaneTitle(strings.TrimSpace(altRes.CanonicalTitle())) {
									accept = false
									rejectedTitleSanity++
								}
							}
							if accept {
								outRes = altRes
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

		return enc.Encode(outRes)
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
			if err := flush(pending); err != nil {
				fmt.Fprintf(os.Stderr, "encode: %v\n", err)
				os.Exit(1)
			}
			count++
			if *limit > 0 && count >= *limit {
				pending = ""
				break
			}
		}
		pending = line
	}
	if strings.TrimSpace(pending) != "" {
		if err := flush(pending); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			os.Exit(1)
		}
		count++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "processed=%d overrides=%d proposals=%d attempted=%d accepted=%d rejected=%d rejected_title_sanity=%d skipped_risk_only=%d parse_errors=%d\n",
		count, len(overrides), withProposal, attempted, accepted, rejected, rejectedTitleSanity, skippedRiskOnly, parseErrors)
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
	// Choice: prefer alt if any source says alt.
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
	// Title span: allow start=0, so only apply when end>start.
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

func strictlyImprovesHardcase(base *model.ParseResult, cur *model.ParseResult) bool {
	br := hardcaseReasons(base)
	nr := hardcaseReasons(cur)
	if !br["any_risk"] {
		return false
	}
	if nr["any_risk"] {
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
		return true
	}
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
	if len([]rune(t)) <= 2 {
		return false
	}
	if regexp.MustCompile(`^[A-Za-z]{1,4}$`).MatchString(t) {
		return false
	}
	up := strings.ToUpper(t)
	if strings.Contains(up, "1080") || strings.Contains(up, "2160") || strings.Contains(up, "720") {
		return false
	}
	if strings.Contains(up, "WEB") || strings.Contains(up, "HEVC") || strings.Contains(up, "AVC") || strings.Contains(up, "10BIT") {
		return false
	}
	return true
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
