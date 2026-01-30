package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/model"
)

func extractTechAudioSlotAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) *model.AudioSlot {
	if len(tokens) == 0 || attrs == nil {
		return nil
	}
	techStart, hasTechStart := firstTechTokenIndexFromAttrs(*attrs)
	if !hasTechStart {
		techStart = 0
	}

	canonCodec := func(s string) string {
		low := strings.ToLower(strings.TrimSpace(s))
		low = strings.ReplaceAll(low, "_", "-")
		switch low {
		case "dtshdma", "dts-hdma", "dts-hd", "dts-hd-ma", "dts-hdmalossless":
			return "DTS-HDMA"
		case "truehd":
			return "TRUEHD"
		case "eac3", "e-ac3", "ddp":
			return "DDP"
		case "ac3", "dd":
			return "AC3"
		case "aac":
			return "AAC"
		case "flac":
			return "FLAC"
		case "opus":
			return "OPUS"
		case "pcm":
			return "PCM"
		case "dts":
			return "DTS"
		default:
			return strings.ToUpper(strings.TrimSpace(s))
		}
	}

	canonLang := func(s string) string {
		low := strings.ToLower(strings.TrimSpace(s))
		low = strings.ReplaceAll(low, "_", "-")
		switch low {
		case "jp", "jpn", "jap", "japanese", "日语", "日語", "日文":
			return "JPN"
		case "en", "eng", "english", "英语", "英語", "英文":
			return "ENG"
		case "zh", "zho", "chi", "chn", "chinese", "中文", "国语", "國語", "华语", "華語":
			return "CHN"
		case "yue", "cantonese", "粤语", "粵語":
			return "YUE"
		case "kor", "kr", "korean", "韩语", "韓語", "韩文", "韓文":
			return "KOR"
		default:
			return ""
		}
	}

	parseChannelsFromTokens := func(i int, end int) (string, int) {
		if i < 0 {
			return "", i
		}
		if i >= end || i >= len(tokens) {
			return "", i
		}
		t0 := strings.TrimSpace(tokens[i].Text)
		if regexp.MustCompile(`^\d\.\d(?:\.\d)?$`).MatchString(t0) {
			return t0, i + 1
		}
		if i+2 < end && i+2 < len(tokens) {
			a := strings.TrimSpace(tokens[i].Text)
			b := strings.TrimSpace(tokens[i+1].Text)
			c := strings.TrimSpace(tokens[i+2].Text)
			if regexp.MustCompile(`^\d$`).MatchString(a) && b == "." && regexp.MustCompile(`^\d$`).MatchString(c) {
				return a + "." + c, i + 3
			}
		}
		return "", i
	}

	parseCodecWithCount := func(s string) (string, int) {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", 0
		}
		low := strings.ToLower(s)
		low = strings.ReplaceAll(low, "_", "-")
		m := regexp.MustCompile(`(?i)^(aac|flac|opus|ac3|eac3|ddp|dd|truehd|dts(?:-hdma)?|dtshdma|pcm)x(\d{1,2})$`).FindStringSubmatch(low)
		if len(m) != 3 {
			return "", 0
		}
		codec := canonCodec(m[1])
		cnt := 0
		// count is intentionally limited; invalid values fall back to 0.
		for _, r := range m[2] {
			if r < '0' || r > '9' {
				return "", 0
			}
			cnt = cnt*10 + int(r-'0')
		}
		if cnt <= 0 {
			return "", 0
		}
		if cnt > 8 {
			cnt = 8
		}
		return codec, cnt
	}

	isAudioCodecToken := func(s string) (string, string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", ""
		}
		low := strings.ToLower(s)
		low = strings.ReplaceAll(low, "_", "-")
		m := regexp.MustCompile(`(?i)^(aac|flac|opus|ac3|eac3|ddp|dd|truehd|dts(?:-hdma)?|dtshdma|pcm)(\d\.\d)?$`).FindStringSubmatch(low)
		if len(m) == 3 {
			codec := canonCodec(m[1])
			ch := strings.TrimSpace(m[2])
			return codec, ch
		}
		codecSet := map[string]bool{"aac": true, "flac": true, "opus": true, "ac3": true, "eac3": true, "ddp": true, "dd": true, "truehd": true, "dts": true, "dts-hdma": true, "dtshdma": true, "pcm": true}
		if codecSet[low] {
			return canonCodec(low), ""
		}
		return "", ""
	}

	isAudioMarkerToken := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		low := strings.ToLower(s)
		if c, cnt := parseCodecWithCount(s); c != "" && cnt > 0 {
			return true
		}
		if _, _ = isAudioCodecToken(s); true {
			c, _ := isAudioCodecToken(s)
			if c != "" {
				return true
			}
		}
		if regexp.MustCompile(`^\d\.\d(?:\.\d)?$`).MatchString(strings.TrimSpace(s)) {
			return true
		}
		if strings.Contains(low, "atmos") || strings.Contains(low, "dtsx") || strings.Contains(low, "dts:x") {
			return true
		}
		if canonLang(s) != "" {
			return true
		}
		if strings.Contains(low, "audio") || strings.Contains(low, "dual") || strings.Contains(low, "multi") || strings.Contains(low, "dub") {
			return true
		}
		return false
	}

	addAttr := func(t model.AttrType, val string, start int, end int, conf float64) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		*attrs = append(*attrs, model.Attribute{Type: t, Normalized: val, TokenStart: start, TokenEnd: end, Confidence: conf, Source: "tech_audio_slot"})
	}

	emitFromSegment := func(start int, end int, conf float64) ([]model.AudioTrack, bool) {
		if start < 0 {
			start = 0
		}
		if end > len(tokens) {
			end = len(tokens)
		}
		if end <= start {
			return nil, false
		}

		isBracketOrParenBlock := false
		if tokens[start].Text == "[" || tokens[start].Text == "【" || tokens[start].Text == "(" || tokens[start].Text == "（" {
			isBracketOrParenBlock = true
		}

		cutStart := start
		if !isBracketOrParenBlock {
			foundMarker := false
			for k := start; k < end; k++ {
				if tokens[k].Removed {
					// Allow removed audio markers (often removed by lexicon) to act as anchors.
					if !isAudioMarkerToken(tokens[k].Text) {
						continue
					}
				}
				if isAudioMarkerToken(tokens[k].Text) {
					cutStart = k
					foundMarker = true
					break
				}
			}
			if !foundMarker {
				return nil, false
			}
		}

		// If we already have a techStart, avoid scanning arbitrary plain segments before it.
		// But allow bracket/paren blocks (common for tech/audio segments).
		if hasTechStart && start < techStart && !isBracketOrParenBlock {
			inBr := false
			for k := start; k < end; k++ {
				t := strings.TrimSpace(tokens[k].Text)
				if t == "[" || t == "【" {
					inBr = true
					break
				}
			}
			if !inBr {
				return nil, false
			}
		}

		tracks := make([]model.AudioTrack, 0, 4)
		curLang := ""
		curLangStart := -1
		pendingObj := ""
		pendingObjStart := -1
		for i := cutStart; i < end; i++ {
			if i < 0 || i >= len(tokens) {
				break
			}
			t := strings.TrimSpace(tokens[i].Text)
			if tokens[i].Removed {
				// Many tech/audio tokens are removed early by lexicon or other passes.
				// We still want to extract structured audio tracks, but only from tokens that
				// clearly look like audio markers.
				if !isAudioMarkerToken(t) {
					if c, cnt := parseCodecWithCount(t); c == "" || cnt <= 0 {
						continue
					}
				}
			}
			if t == "" {
				continue
			}
			if t == "[" || t == "]" || t == "【" || t == "】" || t == "(" || t == ")" || t == "（" || t == "）" {
				continue
			}
			if t == "+" || t == "＋" || t == "/" || t == "／" || t == "|" || t == "｜" || t == ";" || t == "；" || t == "," {
				continue
			}

			if l := canonLang(t); l != "" {
				curLang = l
				curLangStart = i
				addAttr(model.AttrAudioLang, l, i, i+1, conf)
				continue
			}
			low := strings.ToLower(t)
			if strings.Contains(low, "atmos") {
				pendingObj = "ATMOS"
				pendingObjStart = i
				addAttr(model.AttrAudioObject, "ATMOS", i, i+1, conf)
				continue
			}
			if low == "dtsx" || low == "dts:x" {
				pendingObj = "DTSX"
				pendingObjStart = i
				addAttr(model.AttrAudioObject, "DTSX", i, i+1, conf)
				continue
			}

			if codec, cnt := parseCodecWithCount(t); codec != "" && cnt > 0 {
				// Common shorthand: AACx2/DDPx2 => multiple audio tracks.
				ch := ""
				j := i + 1
				if cc, nj := parseChannelsFromTokens(j, end); cc != "" {
					ch = cc
					j = nj
				}
				obj := ""
				for k := j; k < end && k < j+3; k++ {
					kk := strings.ToLower(strings.TrimSpace(tokens[k].Text))
					if strings.Contains(kk, "atmos") {
						obj = "ATMOS"
						j = k + 1
						break
					}
					if kk == "dtsx" || kk == "dts:x" {
						obj = "DTSX"
						j = k + 1
						break
					}
				}
				if obj == "" && pendingObj != "" && pendingObjStart >= 0 {
					if pendingObjStart >= cutStart && pendingObjStart >= i-3 {
						obj = pendingObj
					}
				}
				trStart := i
				if curLangStart >= cutStart && curLangStart < trStart {
					trStart = curLangStart
				}
				if pendingObjStart >= cutStart && pendingObjStart < trStart {
					trStart = pendingObjStart
				}
				for c := 0; c < cnt; c++ {
					tracks = append(tracks, model.AudioTrack{Language: curLang, Codec: codec, Channels: ch, Object: obj, TokenStart: trStart, TokenEnd: j, Source: "tech_audio_slot"})
				}
				addAttr(model.AttrAudioCodec, codec, i, j, conf)
				if ch != "" {
					addAttr(model.AttrAudioChannels, ch, i, j, conf)
				}
				if obj != "" {
					addAttr(model.AttrAudioObject, obj, i, j, conf)
				}
				if pendingObj != "" && pendingObjStart >= cutStart && pendingObjStart <= i {
					pendingObj = ""
					pendingObjStart = -1
				}
				i = j - 1
				continue
			}

			codec, inlineCh := isAudioCodecToken(t)
			if codec == "" {
				continue
			}
			ch := strings.TrimSpace(inlineCh)
			j := i + 1
			if ch == "" {
				if cc, nj := parseChannelsFromTokens(j, end); cc != "" {
					ch = cc
					j = nj
				}
			}
			obj := ""
			for k := j; k < end && k < j+3; k++ {
				kk := strings.ToLower(strings.TrimSpace(tokens[k].Text))
				if strings.Contains(kk, "atmos") {
					obj = "ATMOS"
					j = k + 1
					break
				}
				if kk == "dtsx" || kk == "dts:x" {
					obj = "DTSX"
					j = k + 1
					break
				}
			}
			if obj == "" && pendingObj != "" && pendingObjStart >= 0 {
				if pendingObjStart >= cutStart && pendingObjStart >= i-3 {
					obj = pendingObj
				}
			}

			trStart := i
			if curLangStart >= cutStart && curLangStart < trStart {
				trStart = curLangStart
			}
			if pendingObjStart >= cutStart && pendingObjStart < trStart {
				trStart = pendingObjStart
			}

			tr := model.AudioTrack{Language: curLang, Codec: codec, Channels: ch, Object: obj, TokenStart: trStart, TokenEnd: j, Source: "tech_audio_slot"}
			tracks = append(tracks, tr)
			addAttr(model.AttrAudioCodec, codec, i, j, conf)
			if ch != "" {
				addAttr(model.AttrAudioChannels, ch, i, j, conf)
			}
			if obj != "" {
				addAttr(model.AttrAudioObject, obj, i, j, conf)
			}
			if pendingObj != "" && pendingObjStart >= cutStart && pendingObjStart <= i {
				pendingObj = ""
				pendingObjStart = -1
			}
			i = j - 1
		}

		if len(tracks) == 0 {
			return nil, false
		}
		for k := cutStart; k < end; k++ {
			tokens[k].Removed = true
		}
		return tracks, true
	}

	allTracks := make([]model.AudioTrack, 0, 8)
	for i := 0; i < len(tokens); i++ {
		if hasTechStart && i < techStart {
			// Allow blocks that start before techStart but may contain tech/audio markers.
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
				if tr, ok := emitFromSegment(i, end+1, 0.75); ok {
					allTracks = append(allTracks, tr...)
				}
				i = end
				continue
			}
		}
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			_, closeIdx, _, ok := parseParenBlock(tokens, i, len(tokens))
			if ok {
				if tr, ok2 := emitFromSegment(i, closeIdx+1, 0.75); ok2 {
					allTracks = append(allTracks, tr...)
				}
				i = closeIdx
				continue
			}
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
			if tr, ok := emitFromSegment(start, j, 0.75); ok {
				allTracks = append(allTracks, tr...)
			}
			i = j - 1
		}
	}

	if len(allTracks) == 0 {
		return nil
	}
	// Conservative backfill: if audio tracks lack language but we have exactly one
	// audio_lang attribute (often derived from group/tag rules), apply it.
	allEmptyLang := true
	for i := range allTracks {
		if strings.TrimSpace(allTracks[i].Language) != "" {
			allEmptyLang = false
			break
		}
	}
	if allEmptyLang {
		langSet := map[string]bool{}
		langOrder := make([]string, 0, 4)
		for _, a := range *attrs {
			if a.Type != model.AttrAudioLang {
				continue
			}
			v := strings.TrimSpace(strings.ToUpper(a.Normalized))
			if v == "" {
				continue
			}
			if !langSet[v] {
				langSet[v] = true
				langOrder = append(langOrder, v)
			}
		}
		if len(langOrder) == 1 {
			for i := range allTracks {
				allTracks[i].Language = langOrder[0]
			}
		}
	}
	return &model.AudioSlot{Tracks: allTracks}
}
