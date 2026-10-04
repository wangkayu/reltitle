package parser

import (
	"regexp"
	"strings"

	"reltitle/pkg/model"
)

var parenSummaryAltSxxExxPattern = regexp.MustCompile(`(?i)(\d{1,4}(?:\.\d{1,2})?)\s*(?:总集篇|總集篇|总集|總集|recap|special)\s*[\(（]\s*S0*(\d{1,2})E0*(\d{1,4})\s*[\)）]`)

func buildEPReferenceFromPrimaryCandidate(p *model.EPEntity, season string, seasonRange string, episode string, episodeList string, episodeRange string, spKind string) *model.EPReference {
	season = strings.TrimSpace(season)
	seasonRange = strings.TrimSpace(seasonRange)
	episode = strings.TrimSpace(episode)
	episodeList = strings.TrimSpace(episodeList)
	episodeRange = strings.TrimSpace(episodeRange)
	spKind = strings.TrimSpace(spKind)

	if p != nil {
		kind := strings.ToUpper(strings.TrimSpace(p.Kind))
		if kind != "" && kind != "NONE" {
			out := &model.EPReference{
				Kind:       kind,
				Axis:       strings.ToUpper(strings.TrimSpace(p.Axis)),
				Season:     firstNonEmpty(strings.TrimSpace(p.Season), season),
				SPKind:     firstNonEmpty(strings.TrimSpace(p.SPKind), spKind),
				Source:     strings.TrimSpace(p.Source),
				TokenStart: p.TokenStart,
				TokenEnd:   p.TokenEnd,
			}
			switch out.Kind {
			case "EP":
				out.Episode = strings.TrimSpace(p.Episode)
			case "EP_LIST":
				out.Episode = strings.TrimSpace(p.Episode)
				out.EpisodeList = splitEpisodeListCSV(strings.TrimSpace(p.EpisodeList))
			case "RANGE":
				start, end := splitNormalizedRangeBounds(firstNonEmpty(strings.TrimSpace(p.EpisodeRange), episodeRange))
				out.RangeStart = start
				out.RangeEnd = end
			case "SEASON_RANGE":
				out.SeasonRange = firstNonEmpty(strings.TrimSpace(p.EpisodeRange), seasonRange)
			}
			return out
		}
	}

	if seasonRange != "" {
		return &model.EPReference{
			Kind:        "SEASON_RANGE",
			Axis:        "SE",
			Season:      season,
			SeasonRange: seasonRange,
			Source:      "canonical",
			TokenStart:  -1,
			TokenEnd:    -1,
		}
	}
	if season != "" && episode == "" && episodeList == "" && episodeRange == "" && spKind == "" {
		return &model.EPReference{
			Kind:       "SEASON",
			Axis:       "SE",
			Season:     season,
			Source:     "canonical",
			TokenStart: -1,
			TokenEnd:   -1,
		}
	}
	switch {
	case episodeRange != "":
		start, end := splitNormalizedRangeBounds(episodeRange)
		axis := "ABS"
		if season != "" {
			axis = "SE"
		}
		return &model.EPReference{
			Kind:       "RANGE",
			Axis:       axis,
			Season:     season,
			RangeStart: start,
			RangeEnd:   end,
			Source:     "canonical",
			TokenStart: -1,
			TokenEnd:   -1,
		}
	case episodeList != "":
		axis := "ABS"
		if season != "" {
			axis = "SE"
		}
		return &model.EPReference{
			Kind:        "EP_LIST",
			Axis:        axis,
			Season:      season,
			Episode:     episode,
			EpisodeList: splitEpisodeListCSV(episodeList),
			Source:      "canonical",
			TokenStart:  -1,
			TokenEnd:    -1,
		}
	case episode != "":
		axis := "ABS"
		if season != "" {
			axis = "SE"
		}
		return &model.EPReference{
			Kind:       "EP",
			Axis:       axis,
			Season:     season,
			Episode:    episode,
			SPKind:     strings.ToUpper(spKind),
			Source:     "canonical",
			TokenStart: -1,
			TokenEnd:   -1,
		}
	case spKind != "":
		return &model.EPReference{
			Kind:       "SP",
			Axis:       strings.ToUpper(spKind),
			Season:     season,
			SPKind:     strings.ToUpper(spKind),
			Source:     "canonical",
			TokenStart: -1,
			TokenEnd:   -1,
		}
	default:
		return nil
	}
}

func collectAlternateEPReferences(rawTitle string, tokens []model.Token, primary *model.EPReference) []model.EPReference {
	if primary == nil {
		return nil
	}
	out := make([]model.EPReference, 0, 2)
	seen := map[string]struct{}{}
	add := func(ref model.EPReference) {
		ref.Kind = strings.ToUpper(strings.TrimSpace(ref.Kind))
		ref.Axis = strings.ToUpper(strings.TrimSpace(ref.Axis))
		ref.Season = strings.TrimSpace(ref.Season)
		ref.SeasonRange = strings.TrimSpace(ref.SeasonRange)
		ref.Episode = strings.TrimSpace(ref.Episode)
		ref.RangeStart = strings.TrimSpace(ref.RangeStart)
		ref.RangeEnd = strings.TrimSpace(ref.RangeEnd)
		ref.SPKind = strings.ToUpper(strings.TrimSpace(ref.SPKind))
		ref.Relation = strings.TrimSpace(ref.Relation)
		ref.Source = strings.TrimSpace(ref.Source)
		if ref.Kind == "" || ref.Kind == "NONE" {
			return
		}
		key := strings.Join([]string{
			ref.Kind,
			ref.Axis,
			ref.Season,
			ref.SeasonRange,
			ref.Episode,
			ref.RangeStart,
			ref.RangeEnd,
			ref.SPKind,
			ref.Relation,
			ref.Source,
		}, "|")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}

	if alt, ok := detectParenAbsoluteEpisodeAlternate(tokens, primary); ok {
		add(alt)
	}
	if alt, ok := detectSummaryCatalogEpisodeAlternate(rawTitle, primary); ok {
		add(alt)
	}
	if alt, ok := detectParenRangeAlternateFromRaw(rawTitle, primary); ok {
		add(alt)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func overridePrimaryWithSummaryCatalogEpisode(rawTitle string, primary *model.EPReference) *model.EPReference {
	if primary == nil || strings.ToUpper(strings.TrimSpace(primary.Kind)) != "EP" {
		return primary
	}
	m := parenSummaryAltSxxExxPattern.FindStringSubmatch(rawTitle)
	if len(m) != 4 {
		return primary
	}
	altSeason := normalizeEpisodeIdentity(m[2])
	altEpisode := normalizeEpisodeIdentity(m[3])
	if strings.TrimSpace(primary.Season) != altSeason || strings.TrimSpace(primary.Episode) != altEpisode {
		return primary
	}
	cp := *primary
	cp.Axis = "ABS"
	cp.Season = ""
	cp.Episode = normalizeEpisodeIdentity(m[1])
	cp.Source = "summary_episode_primary"
	return &cp
}

func detectParenAbsoluteEpisodeAlternate(tokens []model.Token, primary *model.EPReference) (model.EPReference, bool) {
	if primary == nil || strings.ToUpper(strings.TrimSpace(primary.Kind)) != "EP" {
		return model.EPReference{}, false
	}
	if primary.TokenEnd < 0 || primary.TokenEnd+2 >= len(tokens) {
		return model.EPReference{}, false
	}
	open := strings.TrimSpace(tokens[primary.TokenEnd].Text)
	inner := strings.TrimSpace(tokens[primary.TokenEnd+1].Text)
	close := strings.TrimSpace(tokens[primary.TokenEnd+2].Text)
	if (open != "(" && open != "（") || (close != ")" && close != "）") {
		return model.EPReference{}, false
	}
	if !episodeOfTotalPattern.MatchString(inner) && !regexp.MustCompile(`^\d{1,4}(?:\.\d{1,2})?$`).MatchString(inner) {
		return model.EPReference{}, false
	}
	norm := strings.TrimLeft(inner, "0")
	if strings.Contains(inner, ".") {
		parts := strings.SplitN(inner, ".", 2)
		left := strings.TrimLeft(parts[0], "0")
		if left == "" {
			left = "0"
		}
		norm = left
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			norm += "." + strings.TrimSpace(parts[1])
		}
	}
	if norm == "" {
		norm = "0"
	}
	if norm == strings.TrimSpace(primary.Episode) {
		return model.EPReference{}, false
	}
	return model.EPReference{
		Kind:       "EP",
		Axis:       "ABS",
		Episode:    norm,
		Relation:   "overall",
		Source:     "paren_alt_episode",
		TokenStart: primary.TokenEnd + 1,
		TokenEnd:   primary.TokenEnd + 2,
	}, true
}

func detectSummaryCatalogEpisodeAlternate(rawTitle string, primary *model.EPReference) (model.EPReference, bool) {
	if primary == nil || strings.ToUpper(strings.TrimSpace(primary.Kind)) != "EP" {
		return model.EPReference{}, false
	}
	if strings.TrimSpace(primary.Episode) == "" {
		return model.EPReference{}, false
	}
	m := parenSummaryAltSxxExxPattern.FindStringSubmatch(rawTitle)
	if len(m) != 4 {
		return model.EPReference{}, false
	}
	if normalizeEpisodeIdentity(m[1]) != normalizeEpisodeIdentity(primary.Episode) {
		return model.EPReference{}, false
	}
	return model.EPReference{
		Kind:       "EP",
		Axis:       "SE",
		Season:     normalizeEpisodeIdentity(m[2]),
		Episode:    normalizeEpisodeIdentity(m[3]),
		Relation:   "catalog",
		Source:     "summary_catalog_paren",
		TokenStart: -1,
		TokenEnd:   -1,
	}, true
}

func detectParenRangeAlternateFromRaw(rawTitle string, primary *model.EPReference) (model.EPReference, bool) {
	if primary == nil || strings.ToUpper(strings.TrimSpace(primary.Kind)) != "RANGE" {
		return model.EPReference{}, false
	}
	start := strings.TrimSpace(primary.RangeStart)
	end := strings.TrimSpace(primary.RangeEnd)
	if start == "" || end == "" {
		return model.EPReference{}, false
	}
	pat := regexp.MustCompile(`(?i)` +
		`0*` + regexp.QuoteMeta(start) + `\s*[-－–—~～〜]\s*0*` + regexp.QuoteMeta(end) +
		`\s*[\(（]\s*(\d{1,4}(?:\.\d{1,2})?)\s*[-－–—~～〜]\s*(\d{1,4}(?:\.\d{1,2})?)\s*[\)）]`)
	m := pat.FindStringSubmatch(rawTitle)
	if len(m) != 3 {
		return model.EPReference{}, false
	}
	altStart := normalizeEpisodeIdentity(m[1])
	altEnd := normalizeEpisodeIdentity(m[2])
	if altStart == "" || altEnd == "" {
		return model.EPReference{}, false
	}
	if altStart == start && altEnd == end {
		return model.EPReference{}, false
	}
	return model.EPReference{
		Kind:       "RANGE",
		Axis:       "ABS",
		RangeStart: altStart,
		RangeEnd:   altEnd,
		Relation:   "overall",
		Source:     "paren_alt_range",
		TokenStart: -1,
		TokenEnd:   -1,
	}, true
}

func normalizeEpisodeIdentity(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, ".") {
		parts := strings.SplitN(s, ".", 2)
		left := strings.TrimLeft(strings.TrimSpace(parts[0]), "0")
		if left == "" {
			left = "0"
		}
		if len(parts) == 1 || strings.TrimSpace(parts[1]) == "" {
			return left
		}
		return left + "." + strings.TrimSpace(parts[1])
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}
