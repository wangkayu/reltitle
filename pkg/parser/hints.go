package parser

// ParseHints carries optional hints for debugging/testing purposes only.
// It must not change default Parse behavior when nil.
//
// IMPORTANT: Hints are NOT for overriding parse results.
// - Overrides should be handled by the caller (e.g., downstream's torrent_override table)
// - Hints should only be used for debugging or testing specific parse paths
//
// The current scope is intentionally minimal: only override Episode for testing.
// Future extensions may include debug flags or test-specific behaviors.
type ParseHints struct {
	// For testing/debugging only - forces episode detection
	OverrideEpisode    string
	OverrideEpisodeIdx int
	
	// For testing/debugging only - forces title span
	OverrideTitleStart int
	OverrideTitleEnd   int
	
	// DebugMode enables additional logging (future use)
	DebugMode bool
}
