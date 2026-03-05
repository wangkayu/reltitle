package parser

import (
	"strings"

	"title-parser/pkg/model"
)

func inferTitleAndAliasesFromTitle(title string) (string, []string) {
	s := strings.TrimSpace(title)
	if s == "" {
		return "", nil
	}
	s = normalizeSpacedUnderscore(s)
	if strings.Contains(s, "_") && !strings.ContainsAny(s, "/／") {
		return s, nil
	}
	rawHasSlash := strings.ContainsAny(s, "/／")

	parts := splitByStrongTitleDelims(s)
	if rawHasSlash && len(parts) >= 2 {
		first := strings.TrimSpace(parts[0])
		cjkParts := make([]string, 0, len(parts))
		kanaParts := make([]string, 0, len(parts))
		latinParts := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if hasKana(p) {
				kanaParts = append(kanaParts, p)
				continue
			}
			if hasCJK(p) {
				cjkParts = append(cjkParts, p)
				continue
			}
			if hasLatin(p) {
				latinParts = append(latinParts, p)
				continue
			}
		}
		addAlias := func(out *[]string, seen map[string]bool, cand string) {
			cand = cleanTitleToken(cand)
			if !isValidAliasToken(cand) {
				return
			}
			k := strings.ToLower(strings.TrimSpace(cand))
			if k == "" {
				return
			}
			if !seen[k] {
				seen[k] = true
				*out = append(*out, cand)
			}
			if stripped := stripSeasonOrPartSuffix(cand); stripped != "" {
				ks := strings.ToLower(strings.TrimSpace(stripped))
				if ks != "" && !seen[ks] {
					seen[ks] = true
					*out = append(*out, stripped)
				}
			}
		}
		if hasKana(first) {
			main := normalizeTitleMain(cleanTitleToken(first))
			if main == "" {
				main = normalizeTitleMain(first)
			}
			aliases := make([]string, 0, len(parts)-1)
			seen := map[string]bool{}
			seen[strings.ToLower(strings.TrimSpace(main))] = true
			for _, a := range parts[1:] {
				addAlias(&aliases, seen, a)
			}
			if len(aliases) == 0 {
				return main, nil
			}
			return main, aliases
		}
		if hasCJK(first) {
			main := normalizeTitleMain(cleanTitleToken(first))
			if main == "" {
				main = normalizeTitleMain(first)
			}
			aliases := make([]string, 0, len(parts))
			seen := map[string]bool{}
			seen[strings.ToLower(strings.TrimSpace(main))] = true
			for _, a := range cjkParts {
				addAlias(&aliases, seen, a)
			}
			for _, a := range kanaParts {
				addAlias(&aliases, seen, a)
			}
			for _, a := range latinParts {
				addAlias(&aliases, seen, a)
			}
			if len(aliases) == 0 {
				return main, nil
			}
			return main, aliases
		}
		if isMostlyLatinToken(first) {
			for _, p := range parts[1:] {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if hasCJK(p) || hasKana(p) {
					main := normalizeTitleMain(cleanTitleToken(p))
					if main == "" {
						main = normalizeTitleMain(p)
					}
					aliases := make([]string, 0, len(parts))
					seen := map[string]bool{}
					seen[strings.ToLower(strings.TrimSpace(main))] = true
					addAlias(&aliases, seen, first)
					addAlias(&aliases, seen, p)
					for _, a := range cjkParts {
						addAlias(&aliases, seen, a)
					}
					for _, a := range kanaParts {
						addAlias(&aliases, seen, a)
					}
					for _, a := range latinParts {
						addAlias(&aliases, seen, a)
					}
					if len(aliases) == 0 {
						return main, nil
					}
					return main, aliases
				}
			}
		}
		if len(cjkParts) >= 2 {
			mainParts := make([]string, 0, len(cjkParts)+1)
			mainParts = append(mainParts, cjkParts...)
			latinKeep := ""
			for _, lp := range latinParts {
				if isShortLatinTitleCandidate(lp) {
					latinKeep = lp
					break
				}
			}
			if latinKeep != "" {
				mainParts = append(mainParts, latinKeep)
			}
			main := normalizeTitleMain(strings.TrimSpace(strings.Join(mainParts, " / ")))
			aliases := make([]string, 0, len(parts))
			seen := map[string]bool{}
			seen[strings.ToLower(strings.TrimSpace(main))] = true
			for _, a := range cjkParts {
				addAlias(&aliases, seen, a)
			}
			for _, a := range latinParts {
				if latinKeep != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(latinKeep)) {
					continue
				}
				addAlias(&aliases, seen, a)
			}
			if len(aliases) == 0 {
				return main, nil
			}
			return main, aliases
		}
	}
	if len(parts) >= 2 {
		main := normalizeTitleMain(chooseMainTitlePart(parts))
		aliases := make([]string, 0, len(parts)-1)
		seen := map[string]bool{}
		seen[strings.ToLower(main)] = true
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.EqualFold(p, main) {
				continue
			}
			p = cleanAliasToken(p)
			if !isValidAliasToken(p) {
				continue
			}
			key := strings.ToLower(p)
			if !seen[key] {
				seen[key] = true
				aliases = append(aliases, p)
			}
		}
		if len(aliases) == 0 {
			return main, nil
		}
		return main, aliases
	}

	if hasCJK(s) && hasLatin(s) {
		fields := strings.Fields(s)
		cjkParts := make([]string, 0, 8)
		latinParts := make([]string, 0, 8)
		for _, f := range fields {
			ff := strings.TrimSpace(f)
			if ff == "" {
				continue
			}
			if hasCJK(ff) {
				cjkParts = append(cjkParts, ff)
				continue
			}
			if hasLatin(ff) {
				latinParts = append(latinParts, ff)
				continue
			}
		}
		if len(cjkParts) > 0 && len(latinParts) > 0 {
			main := normalizeTitleMain(strings.TrimSpace(strings.Join(cjkParts, " ")))
			mergedLatin := mergeConsecutiveLatin(latinParts)
			outAliases := make([]string, 0, len(mergedLatin))
			seen := map[string]bool{}
			seen[strings.ToLower(main)] = true
			for _, p := range mergedLatin {
				p = cleanAliasToken(p)
				if !isValidAliasToken(p) {
					continue
				}
				k := strings.ToLower(p)
				if !seen[k] {
					seen[k] = true
					outAliases = append(outAliases, p)
				}
			}
			if len(outAliases) == 0 {
				return main, nil
			}
			return main, outAliases
		}
	}

	return normalizeTitleMain(s), nil
}

func inferPostEpisodeSearchHintAliasesFromParens(tokens []model.Token, episodeIdx int, episodeRangeIdx int, attrs *[]model.Attribute) []string {
	epStart := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < epStart {
		epStart = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < epStart {
		epStart = episodeIdx
	}
	if epStart < 0 || epStart >= len(tokens) {
		return nil
	}
	aliases := make([]string, 0, 4)
	for i := epStart; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if tokens[i].Text != "(" && tokens[i].Text != "（" {
			continue
		}
		_, closeIdx, inner, ok := parseParenBlock(tokens, i, len(tokens))
		if !ok {
			continue
		}
		inner = strings.TrimSpace(inner)
		if inner == "" {
			i = closeIdx
			continue
		}
		cands, isSearchHint, hintKeyword := splitAliasCandidatesFromParenContent(inner)
		if !isSearchHint {
			i = closeIdx
			continue
		}
		if attrs != nil {
			hk := strings.TrimSpace(hintKeyword)
			if hk != "" {
				*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: hk, Confidence: 0.6, Source: "hint_keyword"})
			}
		}
		for _, c := range cands {
			c = cleanAliasToken(c)
			if c == "" {
				continue
			}
			if isLikelyMetaBracketText(c) {
				continue
			}
			if !hasCJK(c) && len([]rune(c)) < 3 {
				continue
			}
			if len([]rune(c)) > 80 {
				continue
			}
			aliases = append(aliases, c)
		}
		i = closeIdx
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}
