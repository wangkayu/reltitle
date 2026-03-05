# Parser Package

Core parsing engine for anime/film torrent titles. 9-layer pipeline extracts group, title, episode, season, tech specs.

## OVERVIEW

Config-driven parser at `pkg/parser/parser.go` (5351 lines). Orchestrates tokenization, lexicon matching, content type inference, template matching, and field extraction.

## STRUCTURE

```
pkg/parser/
├── parser.go              # Main Parser struct, Parse() entry point
├── pass_types.go          # Pass function types, parseContext
├── pass_registry.go       # Registered passes
├── pass_pack.go           # extract_pack pass
├── pass_ep_detect.go      # detect_episode pass
├── pass_tech.go           # extract_tech_* passes
├── pass_bracket_title_alias.go  # Alias extraction
├── pass_group_subs_override.go  # Group overrides
├── pass_tag_code.go       # Tag/code extraction
├── ep_detect_helpers.go   # Episode detection helpers
├── epslot.go              # EPSlot, EPEntity types
├── audio_slot.go          # AudioSlot extraction
├── bracket_title_alias.go # Bracket alias logic
├── title_alias_infer.go   # Alias inference
├── template_and_content_infer.go  # Template/content inference
├── pack_extract.go        # Pack content extraction
└── hints.go               # ParseHints for ML re-rank
```

## WHERE TO LOOK

| Task | File | Function/Type |
|------|------|---------------|
| Entry point | `parser.go` | `Parser.Parse()` |
| Add new pass | `pass_registry.go` | Register in `allPasses` |
| Modify EP detection | `pass_ep_detect.go` | `detectEpisodeRangeAndEpisode()` |
| EPSlot structure | `epslot.go` | `EPSlot`, `EPEntity` |
| Audio extraction | `audio_slot.go` | `extractAudioSlot()` |
| Pack detection | `pack_extract.go` | `ExtractPackContents()` |
| ML hints | `hints.go` | `ParseHints` |

## KEY TYPES

```go
type Parser struct {
    cfg        *config.Config
    groupIndex map[string]*config.GroupDef
}

type parseContext struct {
    RawTitle, Group, PackKind string
    Season, SeasonIdx, SeasonRange string
    Episode, EpisodeIdx, EpisodeRange string
    EPSlot *model.EPSlot
    // ...
}

type parsePass func(tokens []model.Token, attrs *[]model.Attribute, ctx *parseContext)
```

## PASS ORDER

1. `extract_pack` - Pack content detection
2. `group_subs_x2_override` - Group subtitle overrides
3. `extract_tech_subs` - Tech subtitle attributes
4. `extract_tech_audio` - Audio attributes
5. `extract_bracket_title_aliases` - Bracket alias extraction
6. `extract_tag_code_audio_lang` - Tag codes
7. `detect_season_sp_movie` - Season/SP/Movie detection
8. `detect_episode_range_and_episode` - Episode detection

## CONVENTIONS

- **Pass-based**: Each pass modifies `parseContext` and `attrs`
- **Token removal**: Set `token.Removed = true` to exclude from title
- **Attribute emission**: Append to `*attrs` slice
- **Config access**: Use `p.cfg` for lexicon, templates, inference rules

## ANTI-PATTERNS

- **DO NOT** hardcode patterns - use configs
- **DO NOT** skip pass registration in `allPasses`
- **DO NOT** modify token text directly - only set Removed
- **DO NOT** introduce ML fallback logic in v2.5 parser path

## TESTING

```bash
# Unit tests
go test ./pkg/parser -v

# Single title parse
go run ./cmd/parse -title "[LoliHouse] Title - 01 [1080p]"
```
