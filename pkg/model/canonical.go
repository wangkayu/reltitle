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

func cloneEPSemantics(in *EPSemantics) *EPSemantics {
	if in == nil {
		return nil
	}
	cp := *in
	if len(in.EpisodeList) > 0 {
		cp.EpisodeList = append([]string(nil), in.EpisodeList...)
	}
	return &cp
}

func (r *ParseResult) CanonicalEPSemantics() *EPSemantics {
	if r == nil {
		return nil
	}
	return cloneEPSemantics(r.EPSemantics)
}

func (r *ParseResult) CanonicalEpisode() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		return strings.TrimSpace(sem.Episode)
	}
	return ""
}

func (r *ParseResult) CanonicalEpisodeRange() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		start := strings.TrimSpace(sem.RangeStart)
		end := strings.TrimSpace(sem.RangeEnd)
		if start != "" && end != "" {
			return start + "-" + end
		}
	}
	return ""
}

func (r *ParseResult) CanonicalSeason() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		return strings.TrimSpace(sem.Season)
	}
	return ""
}

func (r *ParseResult) CanonicalSeasonRange() string {
	if sem := r.CanonicalEPSemantics(); sem != nil {
		return strings.TrimSpace(sem.SeasonRange)
	}
	return ""
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
		return strings.TrimSpace(sem.SPKind)
	}
	return ""
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
	source := strings.TrimSpace(sem.Source)
	switch k {
	case "RANGE":
		return "RANGE"
	case "EP_LIST":
		return "EP_LIST"
	case "SP":
		return "SP_ONLY"
	case "EP":
		if strings.EqualFold(source, "tagged_ep_final") {
			return "ABS_EP"
		}
		if strings.TrimSpace(sem.Season) != "" {
			return "SXXEXX"
		}
		return "PLAIN_EP"
	case "SEASON", "SEASON_RANGE":
		return "NONE"
	default:
		return ""
	}
}
