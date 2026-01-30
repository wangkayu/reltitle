package parser

// ParseHints carries optional overrides for offline evaluation / reranking.
// It must not change default Parse behavior when nil.
//
// The current scope is intentionally minimal: only override Episode (token index + normalized).
// Future extensions may include EpisodeRange / Season / forced title spans.
type ParseHints struct {
	OverrideEpisode    string
	OverrideEpisodeIdx int
	OverrideTitleStart int
	OverrideTitleEnd   int
}
