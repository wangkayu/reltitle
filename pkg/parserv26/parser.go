package parserv26

import (
	"errors"
	"sort"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/tokenize"
)

// Parser is an isolated experimental parser implementation for v2.6.
type Parser struct {
	cfg        *config.Config
	groupIndex map[string]struct{}
	groupPats  [][]string
}

func New(cfg *config.Config) *Parser {
	p := &Parser{
		cfg:        cfg,
		groupIndex: make(map[string]struct{}),
		groupPats:  make([][]string, 0, 256),
	}
	seenPat := make(map[string]struct{})
	for _, g := range cfg.GroupList.Groups {
		for _, pat := range g.Patterns {
			key := normalizeToken(pat)
			if key == "" {
				continue
			}
			p.groupIndex[key] = struct{}{}

			seq := normalizePattern(tokenize.Tokenize(pat))
			if len(seq) == 0 {
				continue
			}
			k := strings.Join(seq, "\x1f")
			if _, ok := seenPat[k]; ok {
				continue
			}
			seenPat[k] = struct{}{}
			p.groupPats = append(p.groupPats, seq)
		}
	}
	return p
}

// Parse returns canonical ParseResult plus v2.6 trace for offline comparison.
func (p *Parser) Parse(rawTitle string) (model.ParseResult, Trace, error) {
	if strings.TrimSpace(rawTitle) == "" {
		return model.ParseResult{}, Trace{}, errors.New("empty title")
	}

	rawTokens := tokenize.Tokenize(rawTitle)
	tokens := make([]model.Token, 0, len(rawTokens))
	for _, t := range rawTokens {
		tokens = append(tokens, model.Token{Text: t})
	}

	cands := p.BuildCandidates(tokens)
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].TokenStart != cands[j].TokenStart {
			return cands[i].TokenStart < cands[j].TokenStart
		}
		if cands[i].TokenEnd != cands[j].TokenEnd {
			return cands[i].TokenEnd < cands[j].TokenEnd
		}
		return cands[i].Score > cands[j].Score
	})

	solved, solution := solveStructure(tokens, cands)
	result := solved
	result.RawTitle = rawTitle
	result.Tokens = tokens

	trace := Trace{
		Tokens:     tokens,
		Candidates: cands,
		Signals:    buildSignals(cands),
		Solution:   solution,
	}
	return result, trace, nil
}
