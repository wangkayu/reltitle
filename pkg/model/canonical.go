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

func (r *ParseResult) CanonicalEpisode() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Episode); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrEpisode)
}

func (r *ParseResult) CanonicalEpisodeRange() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.EpisodeRange); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrEpisodeRange)
}

func (r *ParseResult) CanonicalSeason() string {
	if r != nil && r.EPSlot != nil && r.EPSlot.Primary != nil {
		if v := strings.TrimSpace(r.EPSlot.Primary.Season); v != "" {
			return v
		}
	}
	return r.firstAttrValue(AttrSeason)
}

func (r *ParseResult) CanonicalSeasonRange() string {
	return r.firstAttrValue(AttrSeasonRange)
}

func (r *ParseResult) CanonicalVersion() string {
	return r.firstAttrValue(AttrVersion)
}

func (r *ParseResult) CanonicalPackKind() string {
	return r.firstAttrValue(AttrPackKind)
}

func (r *ParseResult) CanonicalSPKind() string {
	return r.firstAttrValue(AttrSPKind)
}

func (r *ParseResult) CanonicalMovieIndex() string {
	return r.firstAttrValue(AttrMovieIndex)
}

func (r *ParseResult) CanonicalEpRoute() string {
	if r == nil || r.EPSlot == nil || r.EPSlot.Primary == nil {
		// Fallback for list-like packs where epslot may not have primary.
		if r == nil {
			return ""
		}
		if strings.TrimSpace(r.CanonicalEpisodeRange()) != "" {
			return "RANGE"
		}
		if strings.TrimSpace(r.CanonicalSPKind()) != "" {
			return "SP_ONLY"
		}
		if strings.TrimSpace(r.CanonicalEpisode()) != "" {
			if strings.TrimSpace(r.CanonicalSeason()) != "" {
				return "SXXEXX"
			}
			return "PLAIN_EP"
		}
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
