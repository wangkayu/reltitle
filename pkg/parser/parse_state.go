package parser

import "reltitle/pkg/model"

// parseState holds the values that Parser.Parse hands from one stage to the next.
//
// The first block is fixed once the pre-pass is done (rawTitle, tokens, ctx, ...);
// the rest is progressively filled in and refined by the stage methods in parser.go.
type parseState struct {
	originalRawTitle string
	rawTitle         string
	tokens           []model.Token
	ctx              *parseContext
	groupSpan        [2]int
	tailParenBlock   string
	version          string

	attrs   []model.Attribute
	group   string
	title   string
	aliases []string

	titleStart int
	titleEnd   int

	packKind        string
	season          string
	seasonIdx       int
	seasonRange     string
	spKind          string
	movieIndex      string
	movieIndexIdx   int
	movieIndexRange string
	hasMovieKeyword bool

	episode         string
	episodeIdx      int
	episodeList     string
	episodeRange    string
	episodeRangeIdx int
	epslot          *model.EPSlot

	hasTech bool
	hasYear bool

	movieRouteHardExcluded bool
	movieContentSignal     bool
	epRoute                string
	contentKind            string
	contentConf            float64
	templateID             string
	templateConf           float64
	routeID                string
}
