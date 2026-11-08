package parser

import (
	"strings"

	"github.com/wangkayu/reltitle/pkg/model"
)

// techSubsSemanticsExtract carries the shared state of
// extractTechSubsSemanticsAndRemoveFromTokens: the token stream, the
// attribute sink, the tech-section start, and the head group span. The
// methods below were closures of the original monolithic function; behavior
// is unchanged.
type techSubsSemanticsExtract struct {
	toonshubCN     bool
	tokens         []model.Token
	attrs          *[]model.Attribute
	techStart      int
	hasTechStart   bool
	headGroupStart int
	headGroupEnd   int
}

func (e *techSubsSemanticsExtract) addAttr(t model.AttrType, val string, start int, end int, conf float64) {
	attrs := e.attrs
	val = strings.TrimSpace(val)
	if val == "" {
		return
	}
	*attrs = append(*attrs, model.Attribute{Type: t, Normalized: val, TokenStart: start, TokenEnd: end, Confidence: conf, Source: model.AttrSourceTechSubs})
}

func (e *techSubsSemanticsExtract) addLang(set map[string]bool, code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	set[code] = true
}

func (e *techSubsSemanticsExtract) addFmt(set map[string]bool, fmt string) {
	fmt = strings.TrimSpace(strings.ToUpper(fmt))
	if fmt == "" {
		return
	}
	set[fmt] = true
}

func (e *techSubsSemanticsExtract) isSubsMarkerToken(s string) bool {
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
	// GB/Big5 是明确的简/繁文字编码标记，语义等同于 CHS/CHT；
	// 用词边界匹配避免 rgb、5GB 之类的误报。
	if containsASCIIWordLike(low, "gb", "big5") {
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

// emitFromSegment extracts subtitle semantics (flag / languages / formats /
// language groups) from the token range [start, end), appends the
// corresponding attributes, marks the range removed on success, and reports
// whether anything was emitted.
var subtitleListCodes = map[string]string{
	"chs": "SC", "sc": "SC", "gb": "SC", "cht": "TC", "tc": "TC", "big5": "TC",
	"zh": "ZH", "zho": "ZH", "chi": "ZH", "jpsc": "JPSC", "jptc": "JPTC",
	"ja": "JPN", "jp": "JPN", "jpn": "JPN", "en": "ENG", "eng": "ENG",
	"ko": "KOR", "kor": "KOR", "es": "SPA", "pt": "POR", "fr": "FRA",
	"de": "DEU", "it": "ITA", "ru": "RUS", "ar": "ARA", "id": "IND",
	"th": "THA", "pl": "POL", "my": "MSA", "vi": "VIE", "ca": "CAT",
}

// subtitleTrackCodes maps the media-server track markers used in subtitle
// file names to the language each one denotes.
var subtitleTrackCodes = map[string]string{
	"track2": "ENG", "track3": "TC", "track4": "SC", "track5": "JPN",
}

// subtitleStandaloneTags are the tags a block may consist of on its own and
// still read as a subtitle tag. It is deliberately narrower than
// subtitleListCodes: two-letter codes that are common English words (it, id,
// my, no) are absent, so a bracketed title such as [It] stays a title. Those
// codes still resolve inside an & or _ enumeration.
var subtitleStandaloneTags = map[string]bool{
	"zh": true, "zho": true, "chi": true, "ja": true, "jp": true, "jpn": true, "en": true, "eng": true,
	"ko": true, "kor": true, "chs": true, "cht": true, "sc": true, "tc": true,
	"gb": true, "big5": true, "jpsc": true, "jptc": true,
	"english": true, "japanese": true, "korean": true,
	"ass": true, "srt": true, "ssa": true, "pgs": true, "sup": true,
	"vobsub": true, "vtt": true,
}

// qualifiedZHHead reports whether the token at i is the bare "zh" head of a
// BCP 47 sequence such as zh-Hans or zh_cn. The script/region part carries the
// SC/TC precision, so the head must not also emit the generic ZH value.
func qualifiedZHHead(tokens []model.Token, i, end int) bool {
	if !strings.EqualFold(strings.TrimSpace(tokens[i].Text), "zh") {
		return false
	}
	sep := false
	for k := i + 1; k < end; k++ {
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		if !sep {
			if t != "-" && t != "_" {
				return false
			}
			sep = true
			continue
		}
		for _, r := range t {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				return true
			}
		}
		return false
	}
	return false
}

// subtitleLangTag resolves a BCP 47-style language tag used as a subtitle
// file-name suffix (zh-Hant, zh-Hans-CN, zh-TW, en-US, ja-JP) to its sub_lang
// value. Chinese needs the script or region to choose SC or TC; any other
// language resolves by its primary subtag. A lone primary subtag is not a
// qualified tag — those are handled by the standalone code list.
func subtitleLangTag(tag string) (string, bool) {
	tag = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tag)), "_", "-")
	parts := strings.Split(tag, "-")
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	if parts[0] == "zh" || parts[0] == "zho" || parts[0] == "chi" {
		return subtitleChineseScript(parts[1:]), true
	}
	if len(parts) < 2 {
		return "", false
	}
	for _, p := range parts[1:] {
		if !isLangSubtag(p) {
			return "", false
		}
	}
	code, ok := subtitleListCodes[parts[0]]
	if !ok {
		return "", false
	}
	return code, true
}

// subtitleChineseScript picks the script of a Chinese tag. An explicit Hans or
// Hant subtag wins; otherwise the region decides (CN/SG/MY are simplified,
// TW/HK/MO traditional). A bare or unknown region keeps the generic ZH.
func subtitleChineseScript(subtags []string) string {
	script := ""
	for _, p := range subtags {
		if script != "" {
			break
		}
		switch p {
		case "hans", "cn", "sg", "my":
			script = "SC"
		case "hant", "tw", "hk", "mo":
			script = "TC"
		}
	}
	if script == "" {
		return "ZH"
	}
	return script
}

// isLangSubtag reports whether p looks like a BCP 47 script (4 letters) or
// region (2 letters or 3 digits) subtag.
func isLangSubtag(p string) bool {
	if len(p) == 2 || len(p) == 4 {
		for _, r := range p {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
				return false
			}
		}
		return true
	}
	if len(p) == 3 {
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

// subtitleBCP47Tag reports whether token is a BCP 47 subtitle tag with a
// script or region subtag (zh_cn, zh-Hans, zh-SG, en-US, ja-JP).
func subtitleBCP47Tag(token string) bool {
	_, ok := subtitleLangTag(token)
	return ok
}

// subtitleStandaloneTag reports whether one block text is a subtitle tag that
// may stand alone, so the pass can claim it before the audio pass reads it as
// an audio language.
func subtitleStandaloneTag(text string) bool {
	text = strings.Trim(strings.TrimSpace(strings.ToLower(text)), "[]【】()（）")
	if text == "" {
		return false
	}
	if subtitleStandaloneTags[text] || subtitleBCP47Tag(text) {
		return true
	}
	_, track := subtitleTrackCodes[text]
	return track
}

// subtitleEnumerationToken is the wider check used once a block is known to
// be a joined list: every listed code is a language, format or track marker.
func subtitleEnumerationToken(token string) bool {
	token = strings.TrimSpace(strings.ToLower(token))
	if token == "" {
		return false
	}
	if subtitleStandaloneTags[token] || subtitleBCP47Tag(token) {
		return true
	}
	if _, ok := subtitleListCodes[token]; ok {
		return true
	}
	_, track := subtitleTrackCodes[token]
	return track
}

// subtitleTagOnlyBlock reports whether the whole block is subtitle
// language/format tags joined by &, +, _, - or a dot, e.g. "chs&jpn",
// "chs_jpn", "zh-cn", "chs.vtt", "track3". Such a block names subtitles, not
// an audio track, even when it is the only content of the bracket.
func subtitleTagOnlyBlock(text string) bool {
	text = strings.Trim(strings.TrimSpace(strings.ToLower(text)), "[]【】()（）")
	if text == "" {
		return false
	}
	if subtitleStandaloneTag(text) {
		return true
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '&', '+', ',', '、', '.', ' ':
			return true
		}
		return false
	})
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if subtitleEnumerationToken(part) {
			continue
		}
		sub := strings.FieldsFunc(part, func(r rune) bool { return r == '_' || r == '-' })
		if len(sub) < 2 {
			return false
		}
		for _, item := range sub {
			if !subtitleEnumerationToken(item) {
				return false
			}
		}
	}
	return true
}

func (e *techSubsSemanticsExtract) emitFromSegment(start int, end int, conf float64) bool {
	tokens := e.tokens
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

	cutStart := start
	if isBracketOrParenBlock && (tokens[start].Text == "[" || tokens[start].Text == "【") {
		for k := start + 1; k < end; k++ {
			text := strings.ToLower(strings.Trim(tokens[k].Text, ","))
			if text == "sub" || text == "subs" || text == "subbed" || text == "subtitle" || text == "subtitles" || text == "hardsub" || text == "softsub" {
				cutStart = k
				if k > start+1 {
					prev := strings.ToLower(tokens[k-1].Text)
					if prev == "hard" || prev == "soft" {
						cutStart--
					}
				}
				break
			}
		}
	}
	if !isBracketOrParenBlock {
		for k := start; k < end; k++ {
			if tokens[k].Removed {
				continue
			}
			if e.isSubsMarkerToken(tokens[k].Text) {
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
	if e.hasTechStart && start < e.techStart {
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
	// A bare 简繁/繁简 block names the two Chinese scripts and nothing else.
	if strings.Contains(noSpace, "简繁") || strings.Contains(noSpace, "簡繁") || strings.Contains(noSpace, "繁简") || strings.Contains(noSpace, "繁簡") {
		isSubsLike = true
	}
	if strings.Contains(noSpaceLower, "chs") || strings.Contains(noSpaceLower, "cht") || strings.Contains(noSpaceLower, "jpn") ||
		strings.Contains(noSpaceLower, "jpsc") || strings.Contains(noSpaceLower, "jptc") {
		isSubsLike = true
	}
	// A block made only of subtitle tags (zh-cn, ja, en-US, track3, vtt,
	// chs&jpn) is subtitle semantics even when it stands alone. This pass runs
	// before the audio pass, so the tag is claimed here instead of becoming an
	// audio language or a title alias. ToonsHub keeps its own routing: a lone
	// [EN] there is an audio language unless the title is the CN variant.
	if !isToonsHubTitle(e.tokens) && subtitleTagOnlyBlock(noSpaceLower) {
		isSubsLike = true
	}
	// GB/Big5 是明确的简/繁编码标记，等同 CHS/CHT；词边界匹配防误报。
	if containsASCIIWordLike(noSpaceLower, "gb", "big5") {
		isSubsLike = true
	}
	if strings.Contains(noSpaceLower, "ass") || strings.Contains(noSpaceLower, "srt") || strings.Contains(noSpaceLower, "pgs") || strings.Contains(noSpaceLower, "sup") || strings.Contains(noSpaceLower, "vobsub") {
		isSubsLike = true
	}
	listText := strings.Trim(noSpaceLower, "[]【】()（）")
	if e.toonshubCN && listText == "multi" {
		e.addAttr(model.AttrSubLang, "MULTI", cutStart, end, conf)
		e.addAttr(model.AttrSubFlag, "soft", cutStart, end, conf)
		for k := cutStart; k < end; k++ {
			tokens[k].Removed = true
		}
		return true
	}
	if e.toonshubCN {
		codes := strings.Split(listText, "&")
		allLanguages := len(codes) > 0
		for _, code := range codes {
			if subtitleListCodes[code] == "" {
				allLanguages = false
				break
			}
		}
		isSubsLike = isSubsLike || allLanguages
	}
	if !isSubsLike {
		if isBracketOrParenBlock {
			trimmed := strings.TrimSpace(noSpace)
			if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "【") || strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "（") {
				trimmed = strings.TrimSpace(strings.Trim(trimmed, "[]【】()（）"))
			}
			if strings.HasPrefix(trimmed, "附") {
				if strings.Contains(trimmed, "访谈") || strings.Contains(trimmed, "熟肉") || strings.Contains(trimmed, "流媒体") || strings.Contains(trimmed, "流媒") {
					e.addAttr(model.AttrNote, "BONUS", cutStart, end, conf)
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
	languageList := strings.Contains(noSpaceLower, "&")
	if cutStart > start {
		for k := cutStart; k < end; k++ {
			code := strings.ToLower(strings.Trim(tokens[k].Text, ","))
			if code == "zh" && qualifiedZHHead(tokens, k, end) {
				continue
			}
			if canonical := subtitleListCodes[code]; canonical != "" && code != "ca" {
				e.addLang(langs, canonical)
			}
		}
	}
	if languageList || e.toonshubCN {
		for _, code := range strings.Split(listText, "&") {
			if canonical := subtitleListCodes[code]; canonical != "" && (code != "ca" || e.toonshubCN) {
				e.addLang(langs, canonical)
			}
		}
	}
	if strings.Contains(noSpace, "简繁") || strings.Contains(noSpace, "簡繁") || strings.Contains(noSpace, "繁简") || strings.Contains(noSpace, "繁簡") {
		e.addLang(langs, "SC")
		e.addLang(langs, "TC")
	}
	if hasSplitSCTCMarker(noSpace) {
		e.addLang(langs, "SC")
		e.addLang(langs, "TC")
	}
	if strings.Contains(noSpace, "简中") || strings.Contains(noSpace, "簡中") || strings.Contains(noSpace, "简体") || strings.Contains(noSpace, "簡體") {
		e.addLang(langs, "SC")
	}
	if strings.Contains(noSpace, "繁中") || strings.Contains(noSpace, "繁体") || strings.Contains(noSpace, "繁體") {
		e.addLang(langs, "TC")
	}
	if strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "简繁日") || strings.Contains(noSpace, "簡繁日") || strings.Contains(noSpace, "繁簡日") {
		if !isYueAudioBilingual {
			e.addLang(langs, "JPN")
		}
		if strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") {
			e.addLang(langs, "SC")
		}
		if strings.Contains(noSpace, "繁日") {
			e.addLang(langs, "TC")
		}
	}
	tagText := strings.NewReplacer("_", "-").Replace(noSpaceLower)
	if containsASCIIWordLike(tagText, "ja", "jp", "jpn", "japanese") {
		e.addLang(langs, "JPN")
	}
	if code, ok := subtitleLangTag(listText); ok {
		e.addLang(langs, code)
	} else {
		if containsASCIIWordLike(tagText, "chs", "gb", "sc", "zh-hans", "zh-cn", "zh-sg", "zh-my") {
			e.addLang(langs, "SC")
		}
		if containsASCIIWordLike(tagText, "cht", "big5", "tc", "zh-hant", "zh-tw", "zh-hk", "zh-mo") {
			e.addLang(langs, "TC")
		}
		if containsASCIIWordLike(tagText, "zho", "chi") {
			e.addLang(langs, "ZH")
		}
	}
	if code, ok := subtitleTrackCodes[listText]; ok {
		e.addLang(langs, code)
	}
	if strings.Contains(noSpace, "英") || containsASCIIWordLike(noSpaceLower, "eng", "english", "en") {
		e.addLang(langs, "ENG")
	}
	if containsASCIIWordLike(noSpaceLower, "español", "espanol", "spanish") {
		e.addLang(langs, "SPA")
	}
	if containsASCIIWordLike(noSpaceLower, "valenci") {
		e.addLang(langs, "CAT")
	}
	if strings.Contains(noSpace, "韩") || strings.Contains(noSpace, "韓") || containsASCIIWordLike(noSpaceLower, "kor", "korean") {
		e.addLang(langs, "KOR")
	}
	if containsASCIIWordLike(noSpaceLower, "ko") {
		e.addLang(langs, "KOR")
	}
	if strings.Contains(noSpace, "日") {
		if strings.Contains(noSpace, "字幕") || strings.Contains(noSpace, "内封") || strings.Contains(noSpace, "內封") || strings.Contains(noSpace, "内嵌") || strings.Contains(noSpace, "內嵌") || strings.Contains(noSpace, "外挂") || strings.Contains(noSpace, "外掛") {
			if !isYueAudioBilingual {
				e.addLang(langs, "JPN")
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
		if su == "ASS" || su == "SRT" || su == "SSA" || su == "VTT" {
			e.addFmt(fmts, su)
			continue
		}
		if su == "PGS" || su == "SUP" {
			e.addFmt(fmts, "PGS")
			continue
		}
		if su == "VOBSUB" {
			e.addFmt(fmts, "VOBSUB")
			continue
		}
	}
	if strings.Contains(noSpaceLower, "vobsub") {
		e.addFmt(fmts, "VOBSUB")
	}
	if strings.Contains(noSpaceLower, "pgs") || strings.Contains(noSpaceLower, "sup") {
		e.addFmt(fmts, "PGS")
	}
	if strings.Contains(noSpaceLower, "ass") {
		e.addFmt(fmts, "ASS")
	}
	if strings.Contains(noSpaceLower, "srt") {
		e.addFmt(fmts, "SRT")
	}
	if strings.Contains(noSpaceLower, "vtt") {
		e.addFmt(fmts, "VTT")
	}
	// "SUB" 在场景标题里大多表示 "有字幕"(例如 Sub_Esp)，不是字幕封装格式。
	// 强收敛模式下不再使用 sub_flag 表示“有字幕”，只输出 hard/soft/ex。

	bilibili := false
	for _, attr := range *e.attrs {
		if attr.Type == model.AttrPlatform && attr.Normalized == "BILIBILI" {
			bilibili = true
			break
		}
	}
	for _, token := range tokens {
		switch strings.ToLower(token.Text) {
		case "bsite", "bstation", "bilibili", "bili":
			bilibili = true
		}
	}
	explicitJapaneseBilingual := strings.Contains(noSpace, "简日") || strings.Contains(noSpace, "簡日") || strings.Contains(noSpace, "繁日") || strings.Contains(noSpace, "简繁日") || strings.Contains(noSpace, "簡繁日") || strings.Contains(noSpace, "繁簡日")
	// CHS_JP / GB_JPN 下划线组合与 CHS&JP / CHS&CHT&ENG & 连接列表都只是语言
	// 枚举，与 GB&JP 等代码组合词条一致：拆成独立 sub_lang，不组队、不推 soft。
	trimmedLower := strings.Trim(noSpaceLower, "[]【】()（）")
	langEnumerationOnly := underscoreJoinedLangCombo(trimmedLower) || ampersandJoinedLangEnumeration(trimmedLower)
	platformJapaneseBilingual := bilibili && len(langs) == 2 && langs["JPN"] && (langs["SC"] || langs["TC"])
	independentLanguageList := len(langs) >= 2 && !explicitJapaneseBilingual && !platformJapaneseBilingual
	if independentLanguageList && flag == "" && !langEnumerationOnly {
		flag = "soft"
	}
	groups := map[string]bool{}
	if !independentLanguageList && langs["JPN"] && langs["SC"] {
		groups["JPSC"] = true
	}
	if !independentLanguageList && langs["JPN"] && langs["TC"] {
		groups["JPTC"] = true
	}
	if rawLike {
		langs = map[string]bool{"RAW": true}
		groups = map[string]bool{}
		flag = ""
	}

	emitted := false
	if flag != "" {
		e.addAttr(model.AttrSubFlag, flag, cutStart, end, conf)
		emitted = true
	}

	langOrder := []string{"RAW", "ZH", "SC", "TC", "JPSC", "JPTC", "JPN", "ENG", "KOR", "SPA", "CAT", "FRA", "DEU", "ITA", "POR", "RUS", "ARA", "IND", "THA", "POL", "MSA", "VIE"}
	for _, l := range langOrder {
		if langs[l] {
			e.addAttr(model.AttrSubLang, l, cutStart, end, conf)
			emitted = true
		}
	}
	fmtOrder := []string{"ASS", "SRT", "PGS", "SSA", "SUB", "VOBSUB", "VTT"}
	for _, f := range fmtOrder {
		if fmts[f] {
			e.addAttr(model.AttrSubFormat, f, cutStart, end, conf)
			emitted = true
		}
	}
	groupOrder := []string{"JPSC", "JPTC", "SC+ENG", "TC+ENG"}
	for _, g := range groupOrder {
		if groups[g] {
			e.addAttr(model.AttrSubLangGroup, g, cutStart, end, conf)
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

// run scans the token stream for bracket blocks, paren blocks, and plain
// segments, and extracts subtitle semantics from each via emitFromSegment.
func (e *techSubsSemanticsExtract) run() {
	tokens := e.tokens
	hasTechStart := e.hasTechStart
	techStart := e.techStart
	headGroupStart := e.headGroupStart
	headGroupEnd := e.headGroupEnd
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
				_ = e.emitFromSegment(i, end+1, 0.75)
				i = end
				continue
			}
		}
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			_, closeIdx, _, ok := parseParenBlock(tokens, i, len(tokens))
			if ok {
				_ = e.emitFromSegment(i, closeIdx+1, 0.75)
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
				_ = e.emitFromSegment(i, len(tokens), 0.75)
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
			_ = e.emitFromSegment(start, j, 0.75)
			i = j - 1
		}
	}
}
