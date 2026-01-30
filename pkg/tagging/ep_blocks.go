package tagging

import (
	"sort"
	"strings"

	"title-parser/pkg/model"
)

type TokenSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func FindBracketPairs(tokens []model.Token) [][2]int {
	return findBracketPairs(tokens)
}

func IsBracketTokenText(t string) bool {
	return isBracketToken(strings.TrimSpace(t))
}

func InferEPBlocksFromTags(tokens []model.Token, tags []string) []TokenSpan {
	if len(tokens) == 0 || len(tags) == 0 {
		return nil
	}
	n := len(tokens)
	if len(tags) < n {
		n = len(tags)
	}

	pairs := findBracketPairs(tokens[:n])
	insidePaired := make([]bool, n)
	var spans []TokenSpan

	for _, pr := range pairs {
		l, r := pr[0], pr[1]
		if l < 0 || r < 0 || l >= n || r >= n || l >= r {
			continue
		}
		hasCore := false
		for i := l + 1; i < r; i++ {
			if isEPTag(tags[i]) || isRangeTag(tags[i]) {
				hasCore = true
				break
			}
		}
		if !hasCore {
			continue
		}
		for i := l; i <= r; i++ {
			insidePaired[i] = true
		}
		spans = append(spans, TokenSpan{Start: l, End: r + 1})
	}

	for i := 0; i < n; {
		if insidePaired[i] {
			i++
			continue
		}
		if !isEPTag(tags[i]) && !isRangeTag(tags[i]) {
			i++
			continue
		}
		j := i + 1
		for j < n && !insidePaired[j] && (isEPTag(tags[j]) || isRangeTag(tags[j])) {
			j++
		}
		l, r := expandCoreSpan(tokens[:n], i, j)
		spans = append(spans, TokenSpan{Start: l, End: r})
		i = j
	}

	return mergeNearSpans(tokens[:n], spans)
}

func mergeNearSpans(tokens []model.Token, spans []TokenSpan) []TokenSpan {
	if len(spans) == 0 {
		return nil
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			return spans[i].End < spans[j].End
		}
		return spans[i].Start < spans[j].Start
	})
	out := make([]TokenSpan, 0, len(spans))
	cur := spans[0]
	if cur.Start < 0 {
		cur.Start = 0
	}
	if cur.End < cur.Start {
		cur.End = cur.Start
	}

	for _, sp := range spans[1:] {
		if sp.Start < 0 {
			sp.Start = 0
		}
		if sp.End < sp.Start {
			sp.End = sp.Start
		}
		if sp.Start <= cur.End {
			if sp.End > cur.End {
				cur.End = sp.End
			}
			continue
		}
		gap := sp.Start - cur.End
		if gap <= 3 && isAllGlueOrBrackets(tokens, cur.End, sp.Start) {
			cur.End = sp.End
			continue
		}
		out = append(out, cur)
		cur = sp
	}
	out = append(out, cur)
	return out
}

func isAllGlueOrBrackets(tokens []model.Token, start int, end int) bool {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if isGlueToken(t) || isBracketToken(t) {
			continue
		}
		return false
	}
	return true
}

func expandCoreSpan(tokens []model.Token, start int, end int) (int, int) {
	l := start
	for k := 0; k < 2 && l-1 >= 0; k++ {
		t := strings.TrimSpace(tokens[l-1].Text)
		if isGlueToken(t) {
			l--
			continue
		}
		break
	}
	r := end
	for k := 0; k < 2 && r < len(tokens); k++ {
		t := strings.TrimSpace(tokens[r].Text)
		if isGlueToken(t) {
			r++
			continue
		}
		break
	}
	if l < 0 {
		l = 0
	}
	if r < l {
		r = l
	}
	if r > len(tokens) {
		r = len(tokens)
	}
	return l, r
}

func isEPTag(t string) bool {
	return strings.EqualFold(strings.TrimSpace(t), "EP")
}

func isRangeTag(t string) bool {
	return strings.EqualFold(strings.TrimSpace(t), "RANGE")
}

func isGlueToken(t string) bool {
	switch t {
	case "-", "–", "—", "_", ":", "：", "~":
		return true
	default:
		return false
	}
}

func isBracketToken(t string) bool {
	switch t {
	case "[", "]", "【", "】", "(", ")", "（", "）", "{", "}":
		return true
	default:
		return false
	}
}

func findBracketPairs(tokens []model.Token) [][2]int {
	openToClose := map[string]string{"[": "]", "【": "】", "(": ")", "（": "）", "{": "}"}
	closeToOpen := map[string]string{"]": "[", "】": "【", ")": "(", "）": "（", "}": "{"}
	type stItem struct {
		open string
		idx  int
	}
	stack := make([]stItem, 0, 8)
	pairs := make([][2]int, 0, 8)
	for i, tk := range tokens {
		t := strings.TrimSpace(tk.Text)
		if t == "" {
			continue
		}
		if _, ok := openToClose[t]; ok {
			stack = append(stack, stItem{open: t, idx: i})
			continue
		}
		open, ok := closeToOpen[t]
		if !ok {
			continue
		}
		for j := len(stack) - 1; j >= 0; j-- {
			if stack[j].open != open {
				continue
			}
			pairs = append(pairs, [2]int{stack[j].idx, i})
			stack = stack[:j]
			break
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] == pairs[j][0] {
			return pairs[i][1] < pairs[j][1]
		}
		return pairs[i][0] < pairs[j][0]
	})
	return pairs
}
