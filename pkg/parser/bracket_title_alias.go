package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/model"
)

var bracketStandaloneMIndexPattern = regexp.MustCompile(`(?i)^m0*(\d{1,3})$`)
var bracketStandaloneMovieIndexPattern = regexp.MustCompile(`(?i)^movie\s*0*(\d{1,3})$`)
var bracketInlineMovieRangePattern = regexp.MustCompile(`(?i)\b(?:movie|film)\s*0*\d{1,3}\s*[-~+]\s*0*\d{1,3}\b`)
var bracketInlineMovieIndexPattern = regexp.MustCompile(`(?i)\b(?:movie|film)\s*0*(\d{1,3})\b`)

func extractBracketTitleAliasesAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute, aliasValid func(string) bool) (string, []string, int, int) {
	if len(tokens) == 0 {
		return "", nil, -1, -1
	}
	if attrs == nil {
		return "", nil, -1, -1
	}

	groupText := ""
	bestGroupConf := -1.0
	for _, a := range *attrs {
		if a.Type != model.AttrGroup {
			continue
		}
		g := strings.TrimSpace(a.Normalized)
		if g == "" {
			continue
		}
		if a.Confidence > bestGroupConf {
			bestGroupConf = a.Confidence
			groupText = g
		}
	}
	compactKey := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "&", "")
		s = strings.ReplaceAll(s, "-", "")
		s = strings.ReplaceAll(s, "_", "")
		return s
	}

	techStart, hasTechStart := firstTechTokenIndexFromAttrs(*attrs)
	if !hasTechStart {
		techStart = len(tokens)
	}
	if techStart < 0 {
		techStart = len(tokens)
	}
	if techStart > len(tokens) {
		techStart = len(tokens)
	}

	start := titleStartAfterLeadingEmptyBracketBlocks(tokens)
	if start < 0 {
		start = 0
	}
	if start-1 >= 0 {
		prev := strings.TrimSpace(tokens[start-1].Text)
		if prev == "[" || prev == "【" || prev == "(" || prev == "（" {
			start = start - 1
		}
	}
	if start >= techStart {
		start = 0
	}

	normalizeDigitRange := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		s = regexp.MustCompile(`\b(\d{1,4})\s*-\s*(\d{1,4})\b`).ReplaceAllString(s, "$1-$2")
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
		return strings.TrimSpace(s)
	}

	parseStandaloneMovieIndex := func(s string) (string, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", false
		}
		if strings.ContainsAny(s, "+-/~") {
			return "", false
		}
		if m := bracketStandaloneMIndexPattern.FindStringSubmatch(s); len(m) == 2 {
			n := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if n == "" {
				n = "0"
			}
			return n, true
		}
		if m := bracketStandaloneMovieIndexPattern.FindStringSubmatch(s); len(m) == 2 {
			n := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if n == "" {
				n = "0"
			}
			return n, true
		}
		return "", false
	}

	parseInlineMovieIndex := func(s string) (string, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", false
		}
		if bracketInlineMovieRangePattern.MatchString(s) {
			return "", false
		}
		if m := bracketInlineMovieIndexPattern.FindStringSubmatch(s); len(m) == 2 {
			n := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				return n, true
			}
		}
		return "", false
	}

	containsEpisodeMarker := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		if regexp.MustCompile(`(?i)^0?\d{1,4}\s*(?:end|fin|final)$`).MatchString(s) {
			return true
		}
		// Batch range forms should stay in token stream for episode_range detection.
		// Examples: "01-24TV全集", "01-12 全集", "01-12FIN", "01-12TV全集+SP+特典映像".
		if regexp.MustCompile(`(?i)^0?\d{1,4}\s*[-~]\s*0?\d{1,4}(?:\.\d{1,2})?.*$`).MatchString(s) {
			return true
		}
		// Multi-episode list forms: "804&805", "07+08".
		if regexp.MustCompile(`(?i)^\d{1,4}(?:\s*(?:&|＆|\+|＋|/|／|,|，|、)\s*\d{1,4})+$`).MatchString(s) {
			return true
		}
		// Any embedded episode marker like "第687集" (even with extra words) should be kept
		// for episode detection and should not be used as main title/alias.
		if regexp.MustCompile(`第\d{1,4}(?:集|话|話)`).FindStringIndex(s) != nil {
			return true
		}
		// Summary/special blocks like "总集篇01_12.5" should be treated as episode markers.
		// Tokenizer may split it into "总集篇01_12 . 5".
		if regexp.MustCompile(`总集篇\s*0*\d{1,3}\s*[_\s-]+\s*\d{1,4}(?:\s*\.\s*\d{1,2})?`).FindStringIndex(s) != nil {
			return true
		}
		// Tagged TV ranges like "TV01-25Fin" should be treated as episode/range markers.
		if regexp.MustCompile(`(?i)\bTV\s*0*\d{1,4}\s*[-~]\s*\d{1,4}\s*(?:FIN|FINAL)?\b`).FindStringIndex(s) != nil {
			return true
		}
		if regexp.MustCompile(`(?i)\bS\d{1,2}E\d{1,4}\b`).FindStringIndex(s) != nil {
			return true
		}
		if regexp.MustCompile(`(?i)\b(?:ep|e)\s*\d{1,4}(?:\.\d{1,2})?\b`).FindStringIndex(s) != nil {
			return true
		}
		return false
	}

	isLikelyTechLike := func(s string) bool {
		low := strings.ToLower(strings.TrimSpace(s))
		if low == "" {
			return false
		}
		if hasCJK(s) || hasLatin(s) {
		} else {
			if isLikelyMetaBracketText(s) {
				return true
			}
		}
		if strings.Contains(low, "webrip") || strings.Contains(low, "web-dl") || strings.Contains(low, "webdl") {
			return true
		}
		if strings.Contains(low, "hevc") || strings.Contains(low, "x265") || strings.Contains(low, "h265") || strings.Contains(low, "x264") || strings.Contains(low, "h264") || strings.Contains(low, "avc") {
			return true
		}
		if strings.Contains(low, "8bit") || strings.Contains(low, "10bit") || strings.Contains(low, "bit") {
			return true
		}
		if regexp.MustCompile(`(?i)\b(?:v\d{1,2}|ver\.?\d{1,2}|rev\d{1,2}|repack|proper|rerip|remaster)\b`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`(?i)\b\d{3,4}p\b`).MatchString(s) {
			return true
		}
		return false
	}

	looksLikeEpOnly := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		if regexp.MustCompile(`(?i)^0?\d{1,4}\s*(?:end|fin|final)$`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`(?i)^\d{1,4}(?:\s*(?:&|＆|\+|＋|/|／|,|，|、)\s*\d{1,4})+$`).MatchString(s) {
			return true
		}
		// Composite EP + special index like "07+ES07" / "07 + ES07".
		if regexp.MustCompile(`(?i)^\d{1,4}\s*\+\s*(?:es|sp|ova|oad|ona|ex)0*\d{1,3}$`).MatchString(s) {
			return true
		}
		// Standalone special index like "ES07".
		if regexp.MustCompile(`(?i)^(?:es|sp|ova|oad|ona|ex)0*\d{1,3}$`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`(?i)^0?\d{1,4}[vV]\d{1,2}$`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`(?i)^第?\d{1,4}(?:\s*-\s*\d{1,4})?(?:集|话|話|回|章)(?:\s|$)`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`^\d{1,4}$`).MatchString(s) {
			return true
		}
		if episodeOfTotalPattern.MatchString(s) {
			return true
		}
		if episodeRangePattern.MatchString(normalizeDigitRange(s)) {
			return true
		}
		if regexp.MustCompile(`(?i)^ep\s*\d{1,4}$`).MatchString(s) {
			return true
		}
		if regexp.MustCompile(`(?i)^0?\d{1,4}\s*(?:&|＆)\s*(?:sp|ova|oad|oav|pv|op|ed)(?:\s|$)`).MatchString(s) {
			return true
		}
		// Patterns like "01_标题" are often episode markers. Keep this strict (underscore only)
		// to avoid misclassifying titles like "86 - 不存在的战区".
		if m := regexp.MustCompile(`^0?(\d{1,4})_+(.+)$`).FindStringSubmatch(s); len(m) == 3 {
			n := strings.TrimLeft(m[1], "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				if v := regexp.MustCompile(`^\d+$`).FindString(n); v != "" {
					if hasCJK(m[2]) {
						return true
					}
				}
			}
		}
		return false
	}

	findSearchHintAlias := func() []string {
		out := make([]string, 0, 6)
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if t == "" {
				continue
			}
			if !strings.Contains(t, "检索") && !strings.Contains(t, "檢索") && !strings.Contains(t, "索引") && !strings.Contains(t, "搜索") && !strings.Contains(t, "搜尋") && !strings.Contains(strings.ToLower(t), "search") {
				continue
			}

			if i+2 < len(tokens) {
				sep := strings.TrimSpace(tokens[i+1].Text)
				if sep == ":" || sep == "：" {
					end := len(tokens)
					inParen := isInParen(tokens, i)
					if inParen {
						depth := 0
						for j := i; j < len(tokens); j++ {
							if tokens[j].Text == "(" || tokens[j].Text == "（" {
								depth++
								continue
							}
							if tokens[j].Text == ")" || tokens[j].Text == "）" {
								if depth == 0 {
									end = j
									break
								}
								depth--
							}
						}
					} else {
						depth := 0
						for j := i; j < len(tokens); j++ {
							if tokens[j].Text == "[" || tokens[j].Text == "【" {
								depth++
								continue
							}
							if tokens[j].Text == "]" || tokens[j].Text == "】" {
								if depth == 0 {
									end = j
									break
								}
								depth--
							}
						}
					}

					parts := make([]string, 0, 16)
					for j := i + 2; j < end && j < len(tokens); j++ {
						if tokens[j].Removed {
							continue
						}
						p := strings.TrimSpace(tokens[j].Text)
						if p == "" {
							continue
						}
						if p == "(" || p == ")" || p == "（" || p == "）" || p == "[" || p == "]" || p == "【" || p == "】" {
							continue
						}
						parts = append(parts, p)
					}
					cand := strings.TrimSpace(strings.Join(parts, " "))
					cand = strings.Trim(cand, "()（）")
					cand = strings.TrimSpace(cand)
					if cand != "" {
						cands, isHint, hintKeyword := splitAliasCandidatesFromParenContent(strings.TrimSpace(t + ":" + cand))
						if isHint {
							hk := strings.TrimSpace(hintKeyword)
							if hk != "" {
								*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: hk, Confidence: 0.6, Source: "hint_keyword"})
							}
							for _, a := range cands {
								a = cleanAliasToken(a)
								if a == "" {
									continue
								}
								compact := strings.ReplaceAll(a, " ", "")
								if len([]rune(compact)) < 4 {
									continue
								}
								if isLikelyMetaBracketText(a) {
									continue
								}
								if !hasCJK(a) && len([]rune(a)) < 3 {
									continue
								}
								if len([]rune(a)) > 400 {
									continue
								}
								out = append(out, a)
							}
							openIdx := i
							if inParen {
								depth := 0
								for j := i - 1; j >= 0; j-- {
									tj := tokens[j].Text
									if tj == ")" || tj == "）" {
										depth++
										continue
									}
									if tj == "(" || tj == "（" {
										if depth == 0 {
											openIdx = j
											break
										}
										depth--
									}
								}
							} else {
								depth := 0
								for j := i - 1; j >= 0; j-- {
									tj := tokens[j].Text
									if tj == "]" || tj == "】" {
										depth++
										continue
									}
									if tj == "[" || tj == "【" {
										if depth == 0 {
											openIdx = j
											break
										}
										depth--
									}
								}
							}
							for j := openIdx; j <= end && j >= 0 && j < len(tokens); j++ {
								tokens[j].Removed = true
							}
							continue
						}
					}
				}
			}

			if idx := strings.IndexAny(t, ":："); idx >= 0 {
				inner := strings.TrimSpace(t[idx+1:])
				inner = strings.Trim(inner, "()（）")
				inner = strings.TrimSpace(inner)
				if inner == "" {
					continue
				}
				cands, isHint, hintKeyword := splitAliasCandidatesFromParenContent(strings.TrimSpace(t[:idx] + ":" + inner))
				if isHint {
					hk := strings.TrimSpace(hintKeyword)
					if hk != "" {
						*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: hk, Confidence: 0.6, Source: "hint_keyword"})
					}
					for _, a := range cands {
						a = cleanAliasToken(a)
						if a == "" {
							continue
						}
						if isLikelyMetaBracketText(a) {
							continue
						}
						if !hasCJK(a) && len([]rune(a)) < 3 {
							continue
						}
						if len([]rune(a)) > 400 {
							continue
						}
						out = append(out, a)
					}
					tokens[i].Removed = true
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	searchHintAliases := findSearchHintAlias()

	type blk struct {
		open  int
		close int
		inner string
	}

	blocks := make([]blk, 0, 4)
	for i := start; i < techStart && i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		open, close, ok := headAliasBracketPair(tokens[i].Text)
		if !ok {
			break
		}
		end := -1
		depth := 0
		for j := i; j < techStart && j < len(tokens); j++ {
			if tokens[j].Text == open {
				depth++
				continue
			}
			if tokens[j].Text == close {
				if depth > 0 {
					depth--
					if depth == 0 {
						end = j
						break
					}
				}
			}
		}
		if end < 0 {
			break
		}
		parts := make([]string, 0, end-i)
		partsRaw := make([]string, 0, end-i)
		for k := i + 1; k < end; k++ {
			p := strings.TrimSpace(tokens[k].Text)
			if p == "" {
				continue
			}
			if p == "[" || p == "]" || p == "【" || p == "】" {
				continue
			}
			partsRaw = append(partsRaw, p)
			if tokens[k].Removed {
				continue
			}
			parts = append(parts, p)
		}
		inner := strings.TrimSpace(strings.Join(parts, " "))
		inner = normalizeDigitRange(inner)
		innerRaw := strings.TrimSpace(strings.Join(partsRaw, " "))
		innerRaw = normalizeDigitRange(innerRaw)
		if inner == "" {
			i = end
			continue
		}
		if compactKey(inner) != "" && compactKey(groupText) != "" {
			if compactKey(inner) == compactKey(groupText) {
				i = end
				continue
			}
		}
		if mi, ok := parseInlineMovieIndex(innerRaw); ok {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrMovieIndex,
				Normalized: mi,
				TokenStart: i + 1,
				TokenEnd:   end,
				Confidence: 0.72,
				Source:     "bracket_inline_movie_index",
			})
		}
		if looksLikeLeadingTitleMeta(inner) {
			i = end
			continue
		}
		if isLikelyMetaBracketText(inner) {
			i = end
			continue
		}
		if containsEpisodeMarker(inner) {
			// Keep this block in tokens for episode detection. Do not consume it
			// into title/alias candidates.
			i = end
			continue
		}
		compactSP := regexp.MustCompile(`[^0-9A-Za-z]+`).ReplaceAllString(inner, "")
		if compactSP != "" {
			if m := spKindPattern.FindStringSubmatch(compactSP); len(m) >= 2 {
				i = end
				continue
			}
		}
		if mi, ok := parseStandaloneMovieIndex(innerRaw); ok {
			// [M21] / [Movie 21] after title blocks should be treated as movie_index.
			// Keep title/alias extraction deterministic and avoid leaking Mxx as alias.
			if len(blocks) > 0 {
				*attrs = append(*attrs, model.Attribute{
					Type:       model.AttrMovieIndex,
					Normalized: mi,
					TokenStart: i + 1,
					TokenEnd:   end,
					Confidence: 0.8,
					Source:     "bracket_m_index",
				})
				for k := i; k <= end && k >= 0 && k < len(tokens); k++ {
					tokens[k].Removed = true
				}
				i = end
				continue
			}
		}
		if isLikelyTechLike(inner) {
			break
		}
		if looksLikeEpOnly(inner) {
			if len(blocks) == 0 {
				i = end
				continue
			}
			break
		}
		blocks = append(blocks, blk{open: i, close: end, inner: inner})
		i = end
	}
	if len(blocks) < 2 {
		return "", searchHintAliases, -1, -1
	}

	extractExplicitSeasonNumber := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if m := regexp.MustCompile(`(?i)\bS(\d{1,2})\b`).FindStringSubmatch(s); len(m) == 2 {
			v := strings.TrimLeft(m[1], "0")
			if v == "" {
				v = m[1]
			}
			return v
		}
		if m := regexp.MustCompile(`(?i)\bSeason\s*(\d{1,2})\b`).FindStringSubmatch(s); len(m) == 2 {
			v := strings.TrimLeft(m[1], "0")
			if v == "" {
				v = m[1]
			}
			return v
		}
		if m := regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季$`).FindStringSubmatch(s); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" {
				return v
			}
		}
		return ""
	}

	mainIdx := 0
	bestScore := -1
	for i := range blocks {
		cand := blocks[i].inner
		score := 0
		if hasCJK(cand) {
			score += 50
		}
		if hasLatin(cand) {
			score += 10
		}
		l := len([]rune(cand))
		score += l
		if score > bestScore {
			bestScore = score
			mainIdx = i
		}
	}

	mainTitle := normalizeTitleMain(blocks[mainIdx].inner)
	if mainTitle == "" {
		return "", searchHintAliases, -1, -1
	}
	aliases := make([]string, 0, len(blocks))
	seen := map[string]bool{}
	seen[strings.ToLower(strings.TrimSpace(mainTitle))] = true
	for i := range blocks {
		if i == mainIdx {
			continue
		}
		// Extract season from the raw alias block before cleanAliasToken/stripExplicitSeasonSuffix drops it.
		if s := extractExplicitSeasonNumber(blocks[i].inner); s != "" {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrSeason, Normalized: s, TokenStart: -1, TokenEnd: -1, Confidence: 0.65, Source: "alias_season"})
		}
		a := cleanAliasToken(blocks[i].inner)
		if stripped := stripExplicitSeasonSuffix(a); stripped != "" {
			a = stripped
		}
		if !aliasValid(a) {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(a))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		aliases = append(aliases, a)
	}
	aliases = mergeAliases(aliases, searchHintAliases)

	mainStart := blocks[mainIdx].open + 1
	mainEnd := blocks[mainIdx].close
	for _, b := range blocks {
		for k := b.open; k <= b.close && k >= 0 && k < len(tokens); k++ {
			tokens[k].Removed = true
		}
	}

	if len(aliases) == 0 {
		return mainTitle, nil, mainStart, mainEnd
	}
	return mainTitle, aliases, mainStart, mainEnd
}
