package parser

import (
	"sort"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func inferContentKind(cfg *config.Config, episode string, episodeRange string, season string, seasonRange string, packKind string, spKind string, movieIndex string, hasTech bool, hasMovieKeyword bool, hasYear bool, movieRouteHardExcluded bool, hasMovieContentSignal bool) (string, float64) {
	hasEpisode := episode != ""
	hasEpisodeRange := episodeRange != ""
	hasSeason := season != ""
	hasSeasonRange := seasonRange != ""
	hasPackKind := packKind != ""
	hasSPKind := spKind != ""
	hasMovieIndex := movieIndex != ""

	if movieRouteHardExcluded && hasMovieContentSignal {
		return "BATCH", 0.95
	}

	rules := append([]config.ContentTypeInferenceRule(nil), cfg.Inference.ContentTypeInference...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority > rules[j].Priority })

	for _, r := range rules {
		if matchContentTypeCondition(r, hasEpisode, hasEpisodeRange, hasSeason, hasSeasonRange, hasPackKind, hasSPKind, hasMovieIndex, hasTech, hasMovieKeyword, hasYear, spKind) {
			return r.Result, r.Confidence
		}
	}

	return "UNKNOWN", 0.5
}

func matchContentTypeCondition(r config.ContentTypeInferenceRule, hasEpisode bool, hasEpisodeRange bool, hasSeason bool, hasSeasonRange bool, hasPackKind bool, hasSPKind bool, hasMovieIndex bool, hasTech bool, hasMovieKeyword bool, hasYear bool, spKind string) bool {
	cond := r.Condition
	if cond == nil {
		return false
	}
	if v, ok := cond["default"]; ok {
		b, _ := v.(bool)
		if b {
			return true
		}
	}
	if v, ok := cond["has_episode"]; ok {
		b, _ := v.(bool)
		if b != hasEpisode {
			return false
		}
	}
	if v, ok := cond["no_episode"]; ok {
		b, _ := v.(bool)
		if b == hasEpisode {
			return false
		}
	}
	if v, ok := cond["has_episode_range"]; ok {
		b, _ := v.(bool)
		if b != hasEpisodeRange {
			return false
		}
	}
	if v, ok := cond["no_episode_range"]; ok {
		b, _ := v.(bool)
		if b == hasEpisodeRange {
			return false
		}
	}
	if v, ok := cond["has_season"]; ok {
		b, _ := v.(bool)
		if b != hasSeason {
			return false
		}
	}
	if v, ok := cond["has_season_range"]; ok {
		b, _ := v.(bool)
		if b != hasSeasonRange {
			return false
		}
	}
	if v, ok := cond["has_tech"]; ok {
		b, _ := v.(bool)
		if b != hasTech {
			return false
		}
	}
	if v, ok := cond["has_movie_keyword"]; ok {
		b, _ := v.(bool)
		if b != hasMovieKeyword {
			return false
		}
	}
	if v, ok := cond["no_movie_keyword"]; ok {
		b, _ := v.(bool)
		if b == hasMovieKeyword {
			return false
		}
	}
	if v, ok := cond["has_pack_kind"]; ok {
		b, _ := v.(bool)
		if b != hasPackKind {
			return false
		}
	}
	if v, ok := cond["has_sp_kind"]; ok {
		b, _ := v.(bool)
		if b != hasSPKind {
			return false
		}
	}
	if v, ok := cond["has_movie_index"]; ok {
		b, _ := v.(bool)
		if b != hasMovieIndex {
			return false
		}
	}
	if v, ok := cond["has_year"]; ok {
		b, _ := v.(bool)
		if b != hasYear {
			return false
		}
	}
	if v, ok := cond["no_year"]; ok {
		b, _ := v.(bool)
		if b == hasYear {
			return false
		}
	}
	if v, ok := cond["sp_kind"]; ok {
		want, _ := v.(string)
		if want != "" {
			if !strings.EqualFold(want, spKind) {
				return false
			}
		}
	}
	return true
}

func matchTemplate(cfg *config.Config, epRoute string, contentKind string, group string, episode string, episodeRange string, season string, packKind string, spKind string, movieIndex string, hasTech bool, hasMovieKeyword bool, attrs []model.Attribute, tokens []model.Token) (string, float64, string) {
	epRoute = strings.TrimSpace(epRoute)
	if epRoute == "RANGE" {
		if strings.TrimSpace(episodeRange) == "" {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}
	if epRoute == "SXXEXX" {
		if strings.TrimSpace(season) == "" || strings.TrimSpace(episode) == "" || !hasSxxExxInTokens(tokens) {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}
	if epRoute == "ABS_EP" {
		if strings.TrimSpace(episode) == "" {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}
	if epRoute == "PLAIN_EP" {
		if strings.TrimSpace(episode) == "" {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}
	if epRoute == "MOVIE_ONLY" {
		if !hasMovieKeyword && strings.TrimSpace(movieIndex) == "" {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}
	if epRoute == "SP_ONLY" {
		if strings.TrimSpace(spKind) == "" {
			return "TF_UNKNOWN_DEFAULT", 0, ""
		}
	}

	tpls := templatesByEpRoute(cfg, epRoute)
	if len(tpls) == 0 {
		tpls = templatesByContentKind(cfg, contentKind)
	}
	if len(tpls) == 0 {
		return "TF_UNKNOWN_DEFAULT", 0, ""
	}

	bestID := ""
	bestConf := 0.0
	bestRoute := ""
	bestSpec := -1
	admitStreaming := isStreamingRelease(tokens, attrs)
	for _, t := range tpls {
		if strings.TrimSpace(t.RouteID) == "streaming_release_v1" {
			if !admitStreaming {
				continue
			}
		}
		if epRoute == "SXXEXX" {
			sigUp := strings.ToUpper(strings.TrimSpace(t.Signature))
			if !strings.Contains(sigUp, " SE") {
				continue
			}
		}
		if !signatureSatisfiable(t.Signature, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword) {
			continue
		}
		if epRoute == "SP_ONLY" {
			sigUp := strings.ToUpper(strings.TrimSpace(t.Signature))
			if !(strings.Contains(sigUp, " SP") || strings.Contains(sigUp, " OVA") || strings.Contains(sigUp, " OAD") || strings.Contains(sigUp, " ONA") || strings.Contains(sigUp, " EX")) {
				continue
			}
		}
		if !templateConditionsSatisfiable(t.Conditions, attrs, tokens) {
			continue
		}
		spec := templateConditionSpecificity(t.Conditions)
		if spec > bestSpec || (spec == bestSpec && t.Confidence > bestConf) {
			bestSpec = spec
			bestConf = t.Confidence
			bestID = t.ID
			bestRoute = strings.TrimSpace(t.RouteID)
		}
	}
	if strings.TrimSpace(bestID) == "" {
		return "TF_UNKNOWN_DEFAULT", 0, ""
	}
	return bestID, bestConf, bestRoute
}

func templatesByEpRoute(cfg *config.Config, epRoute string) []config.TemplateDef {
	switch strings.TrimSpace(epRoute) {
	case "RANGE":
		return cfg.TemplateCatalog.BatchTemplates
	case "MOVIE_ONLY":
		return cfg.TemplateCatalog.MovieTemplates
	case "SXXEXX", "ABS_EP", "PLAIN_EP", "SP_ONLY":
		return cfg.TemplateCatalog.TVEpisodeTemplates
	case "NONE":
		return nil
	default:
		return nil
	}
}

func templateByID(cfg *config.Config, id string) *config.TemplateDef {
	id = strings.TrimSpace(id)
	if cfg == nil || id == "" {
		return nil
	}
	for i := range cfg.TemplateCatalog.TVEpisodeTemplates {
		t := &cfg.TemplateCatalog.TVEpisodeTemplates[i]
		if strings.TrimSpace(t.ID) == id {
			return t
		}
	}
	for i := range cfg.TemplateCatalog.BatchTemplates {
		t := &cfg.TemplateCatalog.BatchTemplates[i]
		if strings.TrimSpace(t.ID) == id {
			return t
		}
	}
	for i := range cfg.TemplateCatalog.MovieTemplates {
		t := &cfg.TemplateCatalog.MovieTemplates[i]
		if strings.TrimSpace(t.ID) == id {
			return t
		}
	}
	for i := range cfg.TemplateCatalog.SpecialTemplates {
		t := &cfg.TemplateCatalog.SpecialTemplates[i]
		if strings.TrimSpace(t.ID) == id {
			return t
		}
	}
	return nil
}

func templateConditionSpecificity(cond *config.TemplateConditions) int {
	if cond == nil || cond.Require == nil {
		return 0
	}
	s := 0
	if len(cond.Require.PlatformIn) > 0 {
		s++
	}
	if len(cond.Require.SourceTypeIn) > 0 {
		s++
	}
	if cond.Require.HasSxxExx {
		s++
	}
	if cond.Require.HasTailGroup {
		s++
	}
	return s
}

func templateConditionsSatisfiable(cond *config.TemplateConditions, attrs []model.Attribute, tokens []model.Token) bool {
	if cond == nil || cond.Require == nil {
		return true
	}
	// has_sxxexx
	if cond.Require.HasSxxExx {
		if !hasSxxExxInTokens(tokens) {
			return false
		}
	}
	// has_tail_group
	if cond.Require.HasTailGroup {
		ok := false
		for _, a := range attrs {
			if a.Type == model.AttrTailGroup {
				if strings.TrimSpace(a.Normalized) != "" {
					ok = true
					break
				}
			}
		}
		if !ok {
			return false
		}
	}
	// platform_in
	if len(cond.Require.PlatformIn) > 0 {
		plat := ""
		for _, a := range attrs {
			if a.Type != model.AttrPlatform {
				continue
			}
			plat = strings.TrimSpace(a.Normalized)
			break
		}
		if plat == "" {
			return false
		}
		ok := false
		for _, w := range cond.Require.PlatformIn {
			if strings.EqualFold(strings.TrimSpace(w), plat) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	// source_type_in
	if len(cond.Require.SourceTypeIn) > 0 {
		src := ""
		for _, a := range attrs {
			if a.Type != model.AttrSourceType {
				continue
			}
			src = strings.TrimSpace(a.Normalized)
			break
		}
		if src == "" {
			return false
		}
		ok := false
		for _, w := range cond.Require.SourceTypeIn {
			if strings.EqualFold(strings.TrimSpace(w), src) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func hasSourceTypeInTokens(tokens []model.Token, want []string) bool {
	if len(tokens) == 0 || len(want) == 0 {
		return false
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		ww := strings.ToUpper(strings.TrimSpace(w))
		if ww != "" {
			wantSet[ww] = true
		}
	}
	for i := 0; i < len(tokens); i++ {
		t := strings.ToUpper(strings.TrimSpace(tokens[i].Text))
		if t == "" {
			continue
		}
		if wantSet[t] {
			return true
		}
		if wantSet["WEBRIP"] || wantSet["WEB-RIP"] {
			if t == "WEBRIP" || t == "WEB-RIP" {
				return true
			}
		}
		if wantSet["WEB-DL"] {
			if t == "WEB-DL" {
				return true
			}
			if t == "WEB" {
				if i+2 < len(tokens) {
					t1 := strings.ToUpper(strings.TrimSpace(tokens[i+1].Text))
					t2 := strings.ToUpper(strings.TrimSpace(tokens[i+2].Text))
					if (t1 == "-" && t2 == "DL") || t1 == "DL" {
						return true
					}
				}
			}
		}
	}
	return false
}

func templatesByContentKind(cfg *config.Config, contentKind string) []config.TemplateDef {
	switch contentKind {
	case "TV_EP":
		return cfg.TemplateCatalog.TVEpisodeTemplates
	case "BATCH":
		return cfg.TemplateCatalog.BatchTemplates
	case "MOVIE":
		return cfg.TemplateCatalog.MovieTemplates
	case "SP", "SPECIAL":
		return cfg.TemplateCatalog.SpecialTemplates
	default:
		return nil
	}
}

func signatureSatisfiable(sig string, group string, episode string, episodeRange string, season string, packKind string, spKind string, movieIndex string, hasTech bool, hasMovieKeyword bool) bool {
	fields := strings.Fields(sig)
	for _, f := range fields {
		switch f {
		case "G":
			if group == "" {
				return false
			}
		case "E":
			if episode == "" {
				return false
			}
		case "R":
			if episodeRange == "" {
				return false
			}
		case "S":
			if season == "" {
				return false
			}
		case "SE":
			if season == "" || episode == "" {
				return false
			}
		case "M":
			if !hasMovieKeyword && movieIndex == "" {
				return false
			}
		case "I":
			if movieIndex == "" {
				return false
			}
		case "X":
			if !hasTech {
				return false
			}
		case "C":
			if packKind == "" {
				return false
			}
		case "SP":
			if spKind == "" {
				return false
			}
		case "OVA", "OAD", "ONA", "EX":
			if strings.ToUpper(spKind) != f {
				return false
			}
		case "T", "CONTENTS":
		default:
		}
	}

	return true
}
