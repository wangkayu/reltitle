package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"title-parser/pkg/model"
)

var dashSeasonEpisodePattern = regexp.MustCompile(`^\d{1,2}$`)
var dashEpisodeTokenPattern = regexp.MustCompile(`^\d{1,4}$`)
var seasonRangeCompactConnectorPattern = regexp.MustCompile(`^\s*(\d{1,2})\s*(?:&|＆|\+|＋|and|et|to)\s*(\d{1,2})\s*$`)

func detectSeasonRangeByKeyword(tokens []model.Token) (seasonRange string, season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		key := strings.TrimSpace(tokens[i].Text)
		if !seasonWordLoosePattern.MatchString(key) {
			continue
		}
		if i+1 >= len(tokens) || tokens[i+1].Removed {
			continue
		}
		// Compact: Season 1&2 / saison 1+2
		if m := seasonRangeCompactConnectorPattern.FindStringSubmatch(strings.TrimSpace(tokens[i+1].Text)); len(m) == 3 {
			a := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			b := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if a == "" {
				a = "0"
			}
			if b == "" {
				b = "0"
			}
			if a != "0" && b != "0" {
				tokens[i].Removed = true
				tokens[i+1].Removed = true
				return fmt.Sprintf("S%s-S%s", a, b), a, i
			}
		}
		// Split: Season 1 & 2 / saison 1 et 2
		if i+3 >= len(tokens) || tokens[i+2].Removed || tokens[i+3].Removed {
			continue
		}
		l := strings.TrimSpace(tokens[i+1].Text)
		sep := strings.ToLower(strings.TrimSpace(tokens[i+2].Text))
		r := strings.TrimSpace(tokens[i+3].Text)
		if !regexp.MustCompile(`^\d{1,2}$`).MatchString(l) || !regexp.MustCompile(`^\d{1,2}$`).MatchString(r) {
			continue
		}
		if !(sep == "&" || sep == "＆" || sep == "+" || sep == "＋" || sep == "and" || sep == "et" || sep == "to") {
			continue
		}
		a := strings.TrimLeft(l, "0")
		b := strings.TrimLeft(r, "0")
		if a == "" {
			a = "0"
		}
		if b == "" {
			b = "0"
		}
		if a == "0" || b == "0" {
			continue
		}
		tokens[i].Removed = true
		tokens[i+1].Removed = true
		tokens[i+2].Removed = true
		tokens[i+3].Removed = true
		return fmt.Sprintf("S%s-S%s", a, b), a, i
	}
	return "", "", -1
}

func detectSeasonOnlyLoose(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if s := seasonTokenValue(tokens[i].Text); s != "" {
			return s, i
		}
	}
	return "", -1
}

func detectSeasonBeforeDashEpisode(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if isInBracket(tokens, i+2) || isInParen(tokens, i+2) {
			continue
		}
		mid := strings.TrimSpace(tokens[i+1].Text)
		if mid != "-" {
			continue
		}
		sn := strings.TrimSpace(tokens[i].Text)
		ep := strings.TrimSpace(tokens[i+2].Text)
		if !dashSeasonEpisodePattern.MatchString(sn) {
			continue
		}
		if !dashEpisodeTokenPattern.MatchString(ep) {
			continue
		}
		v, err := strconv.Atoi(sn)
		if err != nil {
			continue
		}
		if v <= 1 || v > 30 {
			continue
		}
		if i-1 < 0 {
			continue
		}
		prev := strings.TrimSpace(tokens[i-1].Text)
		if prev == "" || prev == "-" || prev == "/" || prev == "|" || prev == "\\" {
			continue
		}
		// Guard: explicit season syntax like "Season 2 - 01" should be handled by
		// detectSeasonKeyword so both the season word and number are removed cleanly.
		if seasonWordLoosePattern.MatchString(prev) {
			continue
		}
		// Guard: "Part/Cour N - EP" uses N as title partition index, not season number.
		if partIndexLeadPattern.MatchString(prev) {
			continue
		}
		season = strings.TrimLeft(sn, "0")
		if season == "" {
			season = sn
		}
		// Remove the season-number token so it does not pollute title.
		tokens[i].Removed = true
		return season, i
	}
	return "", -1
}

func removeSeasonTokens(tokens []model.Token, seasonIdx int) {
	if seasonIdx < 0 || seasonIdx >= len(tokens) {
		return
	}
	text := strings.TrimSpace(tokens[seasonIdx].Text)
	if text == "" {
		return
	}
	// Always remove the matched token itself.
	tokens[seasonIdx].Removed = true

	// Season 3 => remove "Season" and the next small number token.
	if seasonWordLoosePattern.MatchString(text) {
		if seasonIdx+1 < len(tokens) {
			next := strings.TrimSpace(tokens[seasonIdx+1].Text)
			if parseSmallSeasonNumberToken(next) != "" {
				tokens[seasonIdx+1].Removed = true
			}
		}
		return
	}

	// 2nd Season => remove "2nd" and the next "Season".
	if seasonOrdinalPattern.MatchString(text) {
		if seasonIdx+1 < len(tokens) {
			next := strings.TrimSpace(tokens[seasonIdx+1].Text)
			if seasonWordPattern.MatchString(next) {
				tokens[seasonIdx+1].Removed = true
			}
		}
		return
	}
}

func isSharedPrefixSeasonEpisodeRangeAt(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	left := strings.TrimSpace(tokens[idx].Text)
	if left == "" {
		return false
	}
	if sxxExxEpisodeRangePattern.MatchString(left) {
		return true
	}
	if idx+2 >= len(tokens) {
		return false
	}
	if strings.TrimSpace(tokens[idx+1].Text) != "-" && strings.TrimSpace(tokens[idx+1].Text) != "~" {
		return false
	}
	if sxxExxLeftPattern.FindStringSubmatch(left) == nil {
		return false
	}
	right := strings.TrimSpace(tokens[idx+2].Text)
	if rm := epPrefixedDigitsPattern.FindStringSubmatch(strings.ToLower(right)); len(rm) == 2 {
		right = strings.TrimSpace(rm[1])
	}
	return episodeOfTotalPattern.MatchString(right)
}

func isRightSideOfSharedPrefixSeasonEpisodeRange(tokens []model.Token, idx int) bool {
	if idx < 2 || idx >= len(tokens) {
		return false
	}
	right := strings.TrimSpace(tokens[idx].Text)
	if right == "" {
		return false
	}
	if rm := epPrefixedDigitsPattern.FindStringSubmatch(strings.ToLower(right)); len(rm) == 2 {
		right = strings.TrimSpace(rm[1])
	}
	if !episodeOfTotalPattern.MatchString(right) {
		return false
	}
	return isSharedPrefixSeasonEpisodeRangeAt(tokens, idx-2)
}

func removeMovieIndexTokens(tokens []model.Token, idx int) {
	if idx < 0 || idx >= len(tokens) {
		return
	}
	if strings.TrimSpace(tokens[idx].Text) == "" {
		return
	}
	tokens[idx].Removed = true

	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if prev == "." || prev == "_" || prev == "-" {
			tokens[idx-1].Removed = true
		}
	}
	if idx+1 < len(tokens) {
		next := strings.TrimSpace(tokens[idx+1].Text)
		if next == "." || next == "_" || next == "-" {
			tokens[idx+1].Removed = true
		}
	}

	kwIdx := idx - 1
	for kwIdx >= 0 {
		if tokens[kwIdx].Removed {
			kwIdx--
			continue
		}
		p := strings.TrimSpace(tokens[kwIdx].Text)
		if p == "." || p == "_" || p == "-" {
			kwIdx--
			continue
		}
		break
	}
	if kwIdx >= 0 {
		p := strings.TrimSpace(tokens[kwIdx].Text)
		lp := strings.ToLower(p)
		if lp == "film" || lp == "movie" || lp == "themovie" || lp == "eiga" || lp == "gekijouban" || strings.Contains(p, "劇場版") || strings.Contains(p, "剧场版") || strings.Contains(p, "映画") || strings.Contains(p, "大电影") || strings.Contains(p, "大電影") {
			tokens[kwIdx].Removed = true
			if kwIdx-1 >= 0 {
				prev := strings.TrimSpace(tokens[kwIdx-1].Text)
				if prev == "." || prev == "_" || prev == "-" {
					tokens[kwIdx-1].Removed = true
				}
			}
			if kwIdx+1 < len(tokens) {
				next := strings.TrimSpace(tokens[kwIdx+1].Text)
				if next == "." || next == "_" || next == "-" {
					tokens[kwIdx+1].Removed = true
				}
			}
		}
	}
}

func detectSeasonInParen(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if !isInParen(tokens, i) {
			continue
		}
		text := strings.TrimSpace(tokens[i].Text)
		if text == "" {
			continue
		}
		// (S01) / (S01+OVAs)
		m := seasonOnlyPattern.FindStringSubmatch(text)
		if len(m) == 2 {
			return strings.TrimLeft(m[1], "0"), i
		}
		m2 := seasonEmbeddedPattern.FindStringSubmatch(text)
		if len(m2) == 2 {
			return strings.TrimLeft(m2[1], "0"), i
		}
		// (Season 1)
		if seasonWordPattern.MatchString(text) {
			if i+1 < len(tokens) {
				next := strings.TrimSpace(tokens[i+1].Text)
				if isInParen(tokens, i+1) {
					if v := parseSmallSeasonNumberToken(next); v != "" {
						return v, i
					}
				}
			}
		}
		// (2nd Season)
		if m3 := seasonOrdinalPattern.FindStringSubmatch(text); len(m3) == 2 {
			if i+1 < len(tokens) && isInParen(tokens, i+1) && seasonWordPattern.MatchString(strings.TrimSpace(tokens[i+1].Text)) {
				return strings.TrimLeft(m3[1], "0"), i
			}
		}
	}
	return "", -1
}

func detectEpisodeOfTotalInBracket(tokens []model.Token) (string, int) {
	// Pattern: [ 01 of 12 ] -> episode_range=1-1
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if tokens[i].Text != "[" {
			continue
		}
		// Find "N" "of" "M" inside this bracket block.
		for j := i + 1; j+2 < len(tokens); j++ {
			if tokens[j].Text == "]" {
				break
			}
			if tokens[j].Removed {
				continue
			}
			n := strings.TrimSpace(tokens[j].Text)
			if !episodeOfTotalPattern.MatchString(n) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(tokens[j+1].Text), "of") {
				m := strings.TrimSpace(tokens[j+2].Text)
				if episodeOfTotalPattern.MatchString(m) {
					nn := strings.TrimLeft(n, "0")
					if nn == "" {
						nn = "0"
					}
					return nn + "-" + nn, j
				}
			}
		}
	}
	return "", -1
}

func parseSmallSeasonNumberToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// digits
	if regexp.MustCompile(`^\d{1,2}$`).MatchString(s) {
		return strings.TrimLeft(s, "0")
	}
	cn1 := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	r := []rune(s)
	if len(r) == 1 {
		if r[0] == '十' {
			return "10"
		}
		if v, ok := cn1[r[0]]; ok {
			return strconv.Itoa(v)
		}
	}
	if len(r) == 2 {
		if r[0] == '十' {
			if v, ok := cn1[r[1]]; ok {
				return strconv.Itoa(10 + v)
			}
		}
		if r[1] == '十' {
			if v, ok := cn1[r[0]]; ok {
				return strconv.Itoa(v * 10)
			}
		}
	}
	if len(r) == 3 {
		if v, ok := cn1[r[0]]; ok {
			if r[1] == '十' {
				if v2, ok2 := cn1[r[2]]; ok2 {
					return strconv.Itoa(v*10 + v2)
				}
				return strconv.Itoa(v * 10)
			}
		}
	}
	return ""
}

func detectSeasonOnly(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if s := seasonTokenValue(tokens[i].Text); s != "" {
			return s, i
		}
	}
	return "", -1
}

func seasonTokenValue(text string) string {
	if m := seasonOnlyPattern.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimLeft(m[1], "0")
	}
	if m := seasonOnlyVersionPattern.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimLeft(m[1], "0")
	}
	if m := seasonEmbeddedPattern.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimLeft(m[1], "0")
	}
	return ""
}
