package parserv26

import "title-parser/pkg/model"

// CandidateSlot is a local slot taxonomy for the v2.6 candidate graph.
type CandidateSlot string

const (
	SlotGroup    CandidateSlot = "GROUP"
	SlotTitle    CandidateSlot = "TITLE"
	SlotAlias    CandidateSlot = "ALIAS"
	SlotEpisode  CandidateSlot = "EP"
	SlotSeason   CandidateSlot = "SEASON"
	SlotTech     CandidateSlot = "TECH"
	SlotLanguage CandidateSlot = "LANG"
	SlotNoise    CandidateSlot = "NOISE"
)

// SpanCandidate is one candidate label for a token span.
type SpanCandidate struct {
	Slot       CandidateSlot `json:"slot"`
	TokenStart int           `json:"token_start"`
	TokenEnd   int           `json:"token_end"`
	Score      float64       `json:"score"`
	Source     string        `json:"source,omitempty"`
}

type SlotSelection struct {
	Slot         CandidateSlot `json:"slot"`
	TokenStart   int           `json:"token_start"`
	TokenEnd     int           `json:"token_end"`
	Score        float64       `json:"score"`
	Source       string        `json:"source,omitempty"`
	Value        string        `json:"value,omitempty"`
	EpisodeRange string        `json:"episode_range,omitempty"`
	EpisodeList  []string      `json:"episode_list,omitempty"`
}

type Solution struct {
	Group   *SlotSelection `json:"group,omitempty"`
	Title   *SlotSelection `json:"title,omitempty"`
	Episode *SlotSelection `json:"episode,omitempty"`
	Season  *SlotSelection `json:"season,omitempty"`
}

// Trace keeps intermediate artifacts for offline comparison and debugging.
type Trace struct {
	Tokens     []model.Token   `json:"tokens,omitempty"`
	Candidates []SpanCandidate `json:"candidates,omitempty"`
	Signals    []string        `json:"signals,omitempty"`
	Solution   *Solution       `json:"solution,omitempty"`
}
