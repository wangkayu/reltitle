package model

import "strings"

func (r *ParseResult) attrValues(t AttrType) []string {
	if r == nil || len(r.Attributes) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	for _, a := range r.Attributes {
		if a.Type != t {
			continue
		}
		v := strings.TrimSpace(a.Normalized)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (r *ParseResult) firstAttrValue(t AttrType) string {
	for _, v := range r.attrValues(t) {
		return v
	}
	return ""
}

func (r *ParseResult) CanonicalTitle() string {
	return r.firstAttrValue(AttrTitle)
}

func (r *ParseResult) CanonicalGroup() string {
	return r.firstAttrValue(AttrGroup)
}

func (r *ParseResult) CanonicalTitleAliases() []string {
	return r.attrValues(AttrTitleAlias)
}

func (r *ParseResult) CanonicalSearchHints() []string {
	return r.attrValues(AttrSearchHint)
}

func splitEpisodeCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitEpisodeRangeBounds(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '~'
	})
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func (r *ParseResult) CanonicalEPSemantics() *EPSemantics {
	if r == nil {
		return nil
	}
	if r.EPSemantics != nil {
		cp := *r.EPSemantics
		return &cp
	}
	if r.EPSlot != nil && r.EPSlot.Primary != nil {
		p := r.EPSlot.Primary
		out := &EPSemantics{
			Kind:   strings.ToUpper(strings.TrimSpace(p.Kind)),
			Axis:   strings.ToUpper(strings.TrimSpace(p.Axis)),
			Season: strings.TrimSpace(p.Season),
			SPKind: strings.TrimSpace(p.SPKind),
			Source: strings.TrimSpace(p.Source),
		}
		switch out.Kind {
		case "EP":
			out.Episode = strings.TrimSpace(p.Episode)
		case "EP_LIST":
			out.Episode = strings.TrimSpace(p.Episode)
			out.EpisodeList = splitEpisodeCSV(p.EpisodeList)
		case "RANGE":
			start, end := splitEpisodeRangeBounds(p.EpisodeRange)
			out.RangeStart = start
			out.RangeEnd = end
		case "SP":
			// no-op
		}
		return out
	}
	seasonRange := r.firstAttrValue(AttrSeasonRange)
	if seasonRange != "" {
		return &EPSemantics{
			Kind:        "SEASON_RANGE",
			Axis:        "SE",
			SeasonRange: seasonRange,
			Season:      r.firstAttrValue(AttrSeason),
			Source:      "canonical",
		}
	}
	episode := r.firstAttrValue(AttrEpisode)
	episodeList := r.firstAttrValue(AttrEpisodeList)
	episodeRange := r.firstAttrValue(AttrEpisodeRange)
	spKind := r.firstAttrValue(AttrSPKind)
	season := r.firstAttrValue(AttrSeason)
	axis := "ABS"
	if strings.TrimSpace(season) != "" {
		axis = "SE"
	}
	switch {
	case episodeRange != "":
		start, end := splitEpisodeRangeBounds(episodeRange)
		return &EPSemantics{
			Kind:       "RANGE",
			Axis:       axis,
			Season:     season,
			RangeStart: start,
			RangeEnd:   end,
			Source:     "canonical",
		}
	case episodeList != "":
		return &EPSemantics{
			Kind:        "EP_LIST",
			Axis:        axis,
			Season:      season,
			Episode:     episode,
			EpisodeList: splitEpisodeCSV(episodeList),
			Source:      "canonical",
		}
	case episode != "":
		return &EPSemantics{
			Kind:    "EP",
			Axis:    axis,
			Season:  season,
			Episode: episode,
			Source:  "canonical",
		}
	case spKind != "":
		return &EPSemantics{
			Kind:   "SP",
			Axis:   strings.ToUpper(strings.TrimSpace(spKind)),
			Season: season,
			SPKind: spKind,
			Source: "canonical",
		}
	default:
		return &EPSemantics{Kind: "NONE", Source: "canonical"}
	}
}

func (r *ParseResult) CanonicalEpisode() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		if v := strings.TrimSpace(sem.Episode); v != "" {
			return v
		}
	}
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Episode); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrEpisode)
}

func (r *ParseResult) CanonicalEpisodeRange() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		if strings.TrimSpace(sem.RangeStart) != "" && strings.TrimSpace(sem.RangeEnd) != "" {
			return strings.TrimSpace(sem.RangeStart) + "-" + strings.TrimSpace(sem.RangeEnd)
		}
	}
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.EpisodeRange); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrEpisodeRange)
}

func (r *ParseResult) CanonicalSeason() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		if v := strings.TrimSpace(sem.Season); v != "" {
			return v
		}
	}
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Season); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrSeason)
}

func (r *ParseResult) CanonicalSeasonRange() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		if v := strings.TrimSpace(sem.SeasonRange); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrSeasonRange)
}

func (r *ParseResult) CanonicalVersion() string {
	return r.firstAttrValue(AttrVersion)
}

func (r *ParseResult) CanonicalPackKind() string {
	return r.firstAttrValue(AttrPackKind)
}

func (r *ParseResult) CanonicalPackProfile() string {
	return r.firstAttrValue(AttrPackProfile)
}

func (r *ParseResult) CanonicalSPKind() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		if v := strings.TrimSpace(sem.SPKind); v != "" {
			return v
		}
	}
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.SPKind); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrSPKind)
}

func (r *ParseResult) CanonicalMovieIndex() string {
	return r.firstAttrValue(AttrMovieIndex)
}

func (r *ParseResult) CanonicalMovieIndexRange() string {
	return r.firstAttrValue(AttrMovieIndexRange)
}

func (r *ParseResult) CanonicalEpRoute() string {
	if r == nil {
		return ""
	}
	sem := r.CanonicalEPSemantics()
	if sem == nil {
		return ""
	}
	k := strings.ToUpper(strings.TrimSpace(sem.Kind))
	switch k {
	case "RANGE":
		return "RANGE"
	case "EP_LIST":
		return "EP_LIST"
	case "SP":
		return "SP_ONLY"
	case "EP":
		if strings.EqualFold(strings.TrimSpace(sem.Source), "tagged_ep_final") {
			return "ABS_EP"
		}
		if strings.TrimSpace(sem.Season) != "" {
			return "SXXEXX"
		}
		return "PLAIN_EP"
	case "SEASON_RANGE":
		return "NONE"
	default:
		return ""
	}
}
