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
	if v := r.firstAttrValue(AttrTitle); v != "" {
		return v
	}
	return strings.TrimSpace(r.Title)
}

func (r *ParseResult) CanonicalGroup() string {
	if v := r.firstAttrValue(AttrGroup); v != "" {
		return v
	}
	return strings.TrimSpace(r.Group)
}

func (r *ParseResult) CanonicalTitleAliases() []string {
	if vs := r.attrValues(AttrTitleAlias); len(vs) > 0 {
		return vs
	}
	out := make([]string, 0, len(r.Aliases))
	for _, a := range r.Aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (r *ParseResult) CanonicalSearchHints() []string {
	return r.attrValues(AttrSearchHint)
}

func (r *ParseResult) CanonicalEpisode() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Episode); v != "" {
			return v
		}
	}
	if v := r.firstAttrValue(AttrEpisode); v != "" {
		return v
	}
	return strings.TrimSpace(r.Episode)
}

func (r *ParseResult) CanonicalEpisodeRange() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.EpisodeRange); v != "" {
			return v
		}
	}
	if v := r.firstAttrValue(AttrEpisodeRange); v != "" {
		return v
	}
	return strings.TrimSpace(r.EpisodeRange)
}

func (r *ParseResult) CanonicalSeason() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Season); v != "" {
			return v
		}
	}
	if v := r.firstAttrValue(AttrSeason); v != "" {
		return v
	}
	return strings.TrimSpace(r.Season)
}

func (r *ParseResult) CanonicalSeasonRange() string {
	if v := r.firstAttrValue(AttrSeasonRange); v != "" {
		return v
	}
	return strings.TrimSpace(r.SeasonRange)
}

func (r *ParseResult) CanonicalVersion() string {
	if v := r.firstAttrValue(AttrVersion); v != "" {
		return v
	}
	return strings.TrimSpace(r.Version)
}

func (r *ParseResult) CanonicalPackKind() string {
	if v := r.firstAttrValue(AttrPackKind); v != "" {
		return v
	}
	return strings.TrimSpace(r.PackKind)
}

func (r *ParseResult) CanonicalSPKind() string {
	if v := r.firstAttrValue(AttrSPKind); v != "" {
		return v
	}
	return strings.TrimSpace(r.SPKind)
}

func (r *ParseResult) CanonicalMovieIndex() string {
	if v := r.firstAttrValue(AttrMovieIndex); v != "" {
		return v
	}
	return strings.TrimSpace(r.MovieIndex)
}

func (r *ParseResult) CanonicalEpRoute() string {
	if v := strings.TrimSpace(r.EpRoute); v != "" {
		return v
	}
	if r == nil || r.EPSlot == nil || r.EPSlot.Primary == nil {
		return ""
	}
	k := strings.ToUpper(strings.TrimSpace(r.EPSlot.Primary.Kind))
	switch k {
	case "RANGE":
		return "RANGE"
	case "SP":
		return "SP_ONLY"
	case "EP":
		if strings.TrimSpace(r.EPSlot.Primary.Season) != "" {
			return "SXXEXX"
		}
		return "PLAIN_EP"
	default:
		return ""
	}
}
