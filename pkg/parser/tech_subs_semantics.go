package parser

import (
	"strings"

	"title-parser/pkg/model"
)

func hasSplitSCTCMarker(s string) bool {
	noSpace := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if noSpace == "" {
		return false
	}
	for _, pat := range []string{
		"简/繁", "简／繁", "簡/繁", "簡／繁",
		"繁/简", "繁／简", "繁/簡", "繁／簡",
		"简體/繁體", "简体/繁体", "簡體/繁體", "繁體/簡體",
		"简中/繁中", "簡中/繁中", "繁中/简中", "繁中/簡中",
	} {
		if strings.Contains(noSpace, pat) {
			return true
		}
	}
	return false
}

func containsASCIIWordLike(s string, markers ...string) bool {
	if strings.TrimSpace(s) == "" || len(markers) == 0 {
		return false
	}
	low := strings.ToLower(s)
	isAlphaNum := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
	}
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" {
			continue
		}
		searchFrom := 0
		for searchFrom < len(low) {
			idx := strings.Index(low[searchFrom:], marker)
			if idx < 0 {
				break
			}
			idx += searchFrom
			beforeOK := idx == 0 || !isAlphaNum(low[idx-1])
			after := idx + len(marker)
			afterOK := after >= len(low) || !isAlphaNum(low[after])
			if beforeOK && afterOK {
				return true
			}
			searchFrom = idx + 1
		}
	}
	return false
}

func extractTechSubsSemanticsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if len(tokens) == 0 || attrs == nil {
		return
	}
	techStart, hasTechStart := firstTechTokenIndexFromAttrs(*attrs)
	if !hasTechStart {
		techStart = 0
	}
	headGroupStart := -1
	headGroupEnd := -1
	if g, span := extractHeadBracketGroup(tokens); strings.TrimSpace(g) != "" {
		headGroupStart = span[0]
		headGroupEnd = span[1]
	}

	addAttr := func(t model.AttrType, val string, start int, end int, conf float64) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		*attrs = append(*attrs, model.Attribute{Type: t, Normalized: val, TokenStart: start, TokenEnd: end, Confidence: conf, Source: "tech_subs"})
	}

	addLang := func(set map[string]bool, code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		set[code] = true
	}
	addFmt := func(set map[string]bool, fmt string) {
		fmt = strings.TrimSpace(strings.ToUpper(fmt))
		if fmt == "" {
			return
		}
		set[fmt] = true
	}

	emitFromSegment := func(start int, end int, conf float64) bool {
		if start < 0 {
			start = 0
		}
		if end > len(tokens) {
			end = len(tokens)
		}
		if end <= start {
			return false
		}

		isBracketOrParenBlock := false
		if tokens[start].Text == "[" || tokens[start].Text == "【" || tokens[start].Text == "(" || tokens[start].Text == "（" {
			isBracketOrParenBlock = true
		}

		isSubsMarkerToken := func(s string) bool {
			s = strings.TrimSpace(s)
			if s == "" {
				return false
			}
			low := strings.ToLower(s)
			if strings.Contains(s, "字幕") || strings.Contains(s, "内封") || strings.Contains(s, "內封") || strings.Contains(s, "内嵌") || strings.Contains(s, "內嵌") || strings.Contains(s, "外挂") || strings.Contains(s, "外掛") {
				return true
			}
			if strings.Contains(s, "简") || strings.Contains(s, "簡") || strings.Contains(s, "繁") || strings.Contains(s, "雙語") || strings.Contains(s, "双语") {
				if strings.Contains(s, "简") || strings.Contains(s, "簡") || strings.Contains(s, "繁") {
					return true
				}
			}
			if strings.Contains(low, "sub") || strings.Contains(low, "soft") || strings.Contains(low, "hardsub") || strings.Contains(low, "nosub") {
				return true
			}
			if strings.Contains(low, "chs") || strings.Contains(low, "cht") || strings.Contains(low, "jpn") || strings.Contains(low, "jp") || strings.Contains(low, "eng") || strings.Contains(low, "kor") {
				return true
			}
			if strings.Contains(low, "ass") || strings.Contains(low, "srt") || strings.Contains(low, "pgs") || strings.Contains(low, "sup") || strings.Contains(low, "vobsub") {
				return true
			}
			if low == "ass" || low == "srt" || low == "ssa" || low == "sub" || low == "pgs" || low == "sup" {
				return true
			}
			return false
		}

		cutStart := start
		if !isBracketOrParenBlock {
			for k := start; k < end; k++ {
				if tokens[k].Removed {
					continue
				}
				if isSubsMarkerToken(tokens[k].Text) {
					cutStart = k
					break
				}
			}
			if cutStart == start {
				// no early cut, but still may be a subs-only segment; proceed.
			} else if cutStart >= end {
				return false
			}
		}
		if hasTechStart && start < techStart {
			inBr := false
			for k := start; k < end; k++ {
				t := strings.TrimSpace(tokens[k].Text)
				if t == "[" || t == "【" {
					inBr = true
					break
				}
			}
			if !inBr {
				return false
			}
		}

		partsLower := make([]string, 0, end-cutStart)
		noSpace := ""
		noSpaceLower := ""
		for k := cutStart; k < end; k++ {
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			partsLower = append(partsLower, strings.ToLower(t))
			noSpace += t
			noSpaceLower += strings.ToLower(t)
		}
		if noSpace == "" {
			return false
		}
		isYueAudioBilingual := (strings.Contains(noSpace, "粵") || strings.Contains(noSpace, "粤")) &&
			strings.Contains(noSpace, "日") &&
			(strings.Contains(noSpace, "雙語") || strings.Contains(noSpace, "双语"))

		isSubsLike := false
		if strings.Contains(noSpace, "字幕") || strings.Contains(noSpaceLower, "sub") {
			isSubsLike = true
		}
		if strings.Contains(noSpace, "内封") || strings.Contains(noSpace, "內封") || strings.Contains(noSpace, "内嵌") || strings.Contains(noSpace, "內嵌") || strings.Contains(noSpace, "外挂") || strings.Contains(noSpace, "外掛") {
			isSubsLike = true
		}
		if strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "简繁日") || strings.Contains(noSpace, "簡繁日") || strings.Contains(noSpace, "繁簡日") {
			isSubsLike = true
		}
		if strings.Contains(noSpace, "双语") || strings.Contains(noSpace, "雙語") || strings.Contains(noSpace, "多语") || strings.Contains(noSpace, "多語") {
			if strings.Contains(noSpace, "简") || strings.Contains(noSpace, "簡") || strings.Contains(noSpace, "繁") {
				isSubsLike = true
			}
		}
		if hasSplitSCTCMarker(noSpace) {
			isSubsLike = true
		}
		if strings.Contains(noSpaceLower, "chs") || strings.Contains(noSpaceLower, "cht") || strings.Contains(noSpaceLower, "jpn") {
			isSubsLike = true
		}
		if strings.Contains(noSpaceLower, "ass") || strings.Contains(noSpaceLower, "srt") || strings.Contains(noSpaceLower, "pgs") || strings.Contains(noSpaceLower, "sup") || strings.Contains(noSpaceLower, "vobsub") {
			isSubsLike = true
		}
		if !isSubsLike {
			if isBracketOrParenBlock {
				trimmed := strings.TrimSpace(noSpace)
				if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "【") || strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "（") {
					trimmed = strings.TrimSpace(strings.Trim(trimmed, "[]【】()（）"))
				}
				if strings.HasPrefix(trimmed, "附") {
					if strings.Contains(trimmed, "访谈") || strings.Contains(trimmed, "熟肉") || strings.Contains(trimmed, "流媒体") || strings.Contains(trimmed, "流媒") {
						addAttr(model.AttrNote, "BONUS", cutStart, end, conf)
						for k := cutStart; k < end; k++ {
							tokens[k].Removed = true
						}
						return true
					}
				}
			}
			return false
		}

		flag := ""
		rawLike := false
		if strings.Contains(noSpace, "无字幕") || strings.Contains(noSpace, "無字幕") || strings.Contains(noSpace, "生肉") {
			rawLike = true
		}
		if strings.Contains(noSpaceLower, "nosub") || strings.Contains(noSpaceLower, "no-sub") || strings.Contains(noSpaceLower, "no_sub") {
			rawLike = true
		}
		if strings.Contains(noSpace, "外挂") || strings.Contains(noSpace, "外掛") || strings.Contains(noSpaceLower, "external") {
			flag = "ex"
		}
		if strings.Contains(noSpace, "内嵌") || strings.Contains(noSpace, "內嵌") || strings.Contains(noSpaceLower, "hardsub") || strings.Contains(noSpaceLower, "hardsub") {
			flag = "hard"
		}
		if strings.Contains(noSpace, "内封") || strings.Contains(noSpace, "內封") || strings.Contains(noSpaceLower, "softsub") || strings.Contains(noSpaceLower, "soft") {
			flag = "soft"
		}

		langs := map[string]bool{}
		if strings.Contains(noSpace, "简繁") || strings.Contains(noSpace, "簡繁") || strings.Contains(noSpace, "繁简") || strings.Contains(noSpace, "繁簡") {
			addLang(langs, "SC")
			addLang(langs, "TC")
		}
		if hasSplitSCTCMarker(noSpace) {
			addLang(langs, "SC")
			addLang(langs, "TC")
		}
		if strings.Contains(noSpace, "简中") || strings.Contains(noSpace, "簡中") || strings.Contains(noSpace, "简体") || strings.Contains(noSpace, "簡體") {
			addLang(langs, "SC")
		}
		if strings.Contains(noSpace, "繁中") || strings.Contains(noSpace, "繁体") || strings.Contains(noSpace, "繁體") {
			addLang(langs, "TC")
		}
		if strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "简繁日") || strings.Contains(noSpace, "簡繁日") || strings.Contains(noSpace, "繁簡日") {
			if !isYueAudioBilingual {
				addLang(langs, "JPN")
			}
			if strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") {
				addLang(langs, "SC")
			}
			if strings.Contains(noSpace, "繁日") {
				addLang(langs, "TC")
			}
		}
		if containsASCIIWordLike(noSpaceLower, "jp", "jpn", "japanese") {
			addLang(langs, "JPN")
		}
		if containsASCIIWordLike(noSpaceLower, "chs", "gb", "sc", "zh-hans", "zh-cn") {
			addLang(langs, "SC")
		}
		if containsASCIIWordLike(noSpaceLower, "cht", "big5", "tc", "zh-hant", "zh-tw", "zh-hk") {
			addLang(langs, "TC")
		}
		if strings.Contains(noSpace, "英") || containsASCIIWordLike(noSpaceLower, "eng", "english") {
			addLang(langs, "ENG")
		}
		if containsASCIIWordLike(noSpaceLower, "español", "espanol", "spanish") {
			addLang(langs, "SPA")
		}
		if containsASCIIWordLike(noSpaceLower, "valenci") {
			addLang(langs, "CAT")
		}
		if strings.Contains(noSpace, "韩") || strings.Contains(noSpace, "韓") || containsASCIIWordLike(noSpaceLower, "kor", "korean") {
			addLang(langs, "KOR")
		}
		if strings.Contains(noSpace, "日") {
			if strings.Contains(noSpace, "字幕") || strings.Contains(noSpace, "内封") || strings.Contains(noSpace, "內封") || strings.Contains(noSpace, "内嵌") || strings.Contains(noSpace, "內嵌") || strings.Contains(noSpace, "外挂") || strings.Contains(noSpace, "外掛") {
				if !isYueAudioBilingual {
					addLang(langs, "JPN")
				}
			}
		}

		fmts := map[string]bool{}
		for _, pl := range partsLower {
			s := strings.TrimSpace(pl)
			if s == "" {
				continue
			}
			su := strings.ToUpper(s)
			if su == "ASS" || su == "SRT" || su == "SSA" {
				addFmt(fmts, su)
				continue
			}
			if su == "PGS" || su == "SUP" {
				addFmt(fmts, "PGS")
				continue
			}
			if su == "VOBSUB" {
				addFmt(fmts, "VOBSUB")
				continue
			}
		}
		if strings.Contains(noSpaceLower, "vobsub") {
			addFmt(fmts, "VOBSUB")
		}
		if strings.Contains(noSpaceLower, "pgs") || strings.Contains(noSpaceLower, "sup") {
			addFmt(fmts, "PGS")
		}
		if strings.Contains(noSpaceLower, "ass") {
			addFmt(fmts, "ASS")
		}
		if strings.Contains(noSpaceLower, "srt") {
			addFmt(fmts, "SRT")
		}
		// "SUB" 在场景标题里大多表示 "有字幕"(例如 Sub_Esp)，不是字幕封装格式。
		// 强收敛模式下不再使用 sub_flag 表示“有字幕”，只输出 hard/soft/ex。

		groups := map[string]bool{}
		if langs["JPN"] && langs["SC"] {
			groups["JPSC"] = true
		}
		if langs["JPN"] && langs["TC"] {
			groups["JPTC"] = true
		}
		sawBilingual := strings.Contains(noSpace, "双语") || strings.Contains(noSpace, "雙語") || strings.Contains(noSpaceLower, "bilingual")
		if sawBilingual && langs["SC"] && langs["ENG"] {
			groups["SC+ENG"] = true
		}
		if sawBilingual && langs["TC"] && langs["ENG"] {
			groups["TC+ENG"] = true
		}
		if rawLike {
			langs = map[string]bool{"RAW": true}
			groups = map[string]bool{}
			flag = ""
		}

		emitted := false
		if flag != "" {
			addAttr(model.AttrSubFlag, flag, cutStart, end, conf)
			emitted = true
		}

		langOrder := []string{"RAW", "SC", "TC", "JPN", "ENG", "KOR", "SPA", "CAT"}
		for _, l := range langOrder {
			if langs[l] {
				addAttr(model.AttrSubLang, l, cutStart, end, conf)
				emitted = true
			}
		}
		fmtOrder := []string{"ASS", "SRT", "PGS", "SSA", "SUB", "VOBSUB"}
		for _, f := range fmtOrder {
			if fmts[f] {
				addAttr(model.AttrSubFormat, f, cutStart, end, conf)
				emitted = true
			}
		}
		groupOrder := []string{"JPSC", "JPTC", "SC+ENG", "TC+ENG"}
		for _, g := range groupOrder {
			if groups[g] {
				addAttr(model.AttrSubLangGroup, g, cutStart, end, conf)
				emitted = true
			}
		}
		if !emitted {
			return false
		}
		for k := cutStart; k < end; k++ {
			tokens[k].Removed = true
		}
		return true
	}

	for i := 0; i < len(tokens); i++ {
		if hasTechStart && i < techStart {
			if tokens[i].Text != "[" && tokens[i].Text != "【" && tokens[i].Text != "(" && tokens[i].Text != "（" {
				continue
			}
		}
		if tokens[i].Text == "[" || tokens[i].Text == "【" {
			open := tokens[i].Text
			close := "]"
			if open == "【" {
				close = "】"
			}
			depth := 0
			end := -1
			for j := i; j < len(tokens); j++ {
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
			if end >= 0 {
				if i == headGroupStart && end+1 == headGroupEnd {
					i = end
					continue
				}
				_ = emitFromSegment(i, end+1, 0.75)
				i = end
				continue
			}
		}
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			_, closeIdx, _, ok := parseParenBlock(tokens, i, len(tokens))
			if ok {
				_ = emitFromSegment(i, closeIdx+1, 0.75)
				i = closeIdx
				continue
			}
		}
		if !hasTechStart {
			if isInBracket(tokens, i) || isInParen(tokens, i) {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if strings.EqualFold(t, "sub") || strings.EqualFold(t, "subs") {
				_ = emitFromSegment(i, len(tokens), 0.75)
				break
			}
			continue
		}
		if !hasTechStart {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if t == "|" || t == "｜" || t == ";" || t == "；" {
			continue
		}
		start := i
		j := i
		for j < len(tokens) {
			if isInBracket(tokens, j) || isInParen(tokens, j) {
				break
			}
			jt := strings.TrimSpace(tokens[j].Text)
			if jt == "" {
				j++
				continue
			}
			if jt == "[" || jt == "【" || jt == "(" || jt == "（" {
				break
			}
			if jt == "|" || jt == "｜" || jt == ";" || jt == "；" {
				break
			}
			j++
		}
		if j > start {
			_ = emitFromSegment(start, j, 0.75)
			i = j - 1
		}
	}
}
