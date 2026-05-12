# Title Parser

**Config-driven anime/film torrent title parser (v2.5)**

Extracts title, group, episode semantics, and technical specs from release titles. Pure rule-based architecture with zero ML/LLM dependency.

## Features

- **Config-driven**: All parsing rules in YAML configs - no code changes needed for new formats
- **Multi-format support**: Anime releases, TV series, movies, OVA/SP content
- **Episode semantics**: Unified `ep_semantics` output for episode/season/range handling
- **Tech spec extraction**: Resolution, codec, audio, platform detection
- **Multi-language**: CJK + Latin tokenization with Unicode support
- **HTTP API + stdio batch**: Both server and batch processing modes

## Quick Start

### Build & Run

```bash
# HTTP server (port 8080)
go build -o server ./cmd/server
./server

# Or run directly
go run ./cmd/server
```

### Parse a Title

```bash
# CLI
go run ./cmd/parse -title "[LoliHouse] 药屋少女 - 01 [WebRip 1080p HEVC AAC]"
```

### HTTP API

```bash
# Single parse
curl -X POST http://localhost:8080/api/v1/parse \
  -H "Content-Type: application/json" \
  -d '{"title": "[VCB-Studio] Kyoukai no Kanata 01-12+MOVIE [Ma10p_1080p]"}'

# Batch parse
curl -X POST http://localhost:8080/api/v1/parse/batch \
  -H "Content-Type: application/json" \
  -d '{"titles": ["[LoliHouse] Title - 01", "[VCB-Studio] Movie [1080p]"]}'
```

### Batch Service (stdio)

```bash
go run ./cmd/serve -- -config configs
```

Input/output via JSON lines on stdin/stdout:

```json
{"id":"1","method":"parse_batch","params":{"items":[{"title":"[LoliHouse] Title - 01"}]}}
{"id":"1","ok":true,"result":{"items":[{"parsed":{"title":"Title","ep_semantics":{"kind":"EP","episode":"01"}}}]}}
```

## Output Structure

```json
{
  "raw_title": "[VCB-Studio] Kyoukai no Kanata 01-12+MOVIE [Ma10p_1080p]",
  "title": "Kyoukai no Kanata",
  "group": "VCB-Studio",
  "content_kind": "TV_EP",
  "ep_semantics": {
    "kind": "RANGE",
    "range_start": "01",
    "range_end": "12"
  },
  "contents": ["MOVIE"],
  "attributes": [
    {"key": "resolution", "value": "1080p"},
    {"key": "video_codec", "value": "HEVC"},
    {"key": "audio_codec", "value": "E-AC3"}
  ]
}
```

### EP Semantics

The `ep_semantics` field is the **single source of truth** for episode/season semantics:

| Field | Description |
|-------|-------------|
| `kind` | `EP`, `EP_LIST`, `RANGE`, `SEASON_RANGE`, `SP`, `NONE` |
| `axis` | `SE` (season-relative), `ABS` (absolute), `TV`, `OVA`, `SP` |
| `season` | Season number (e.g., `4`) |
| `episode` | Single episode (e.g., `25`) |
| `range_start/end` | Episode range bounds |
| `episode_list` | Multiple episodes (e.g., `["804", "805"]`) |

## Configuration

All rules defined in YAML configs under `configs/`:

| Config | Purpose |
|--------|---------|
| `lexicon.yaml` | Dictionary: release groups, platforms, codecs, etc. |
| `template_catalog.yaml` | 33 parsing templates for different formats |
| `inference_rules.yaml` | Content type inference, EP semantics rules |
| `attribute_config.yaml` | Field definitions, normalization rules |
| `boundary_rules.yaml` | Title/group/EP boundary detection |
| `tailgroup_whitelist.yaml` | Valid tail groups (tech specs) |
| `dict_*.yaml` | Alias, canonical values, synonyms |

### Adding New Rules

| What to add | File | Time |
|-------------|------|------|
| New codec/platform/group | `lexicon.yaml` | 5 min |
| New release format template | `template_catalog.yaml` | 15 min |
| New attribute type | `attribute_config.yaml` | 30 min |

## Project Structure

```
title-parser/
├── pkg/
│   ├── parser/       # Main parser engine (9-layer pipeline)
│   ├── config/       # YAML config loader
│   ├── model/        # Data types (ParseResult, Attribute, Token)
│   ├── api/          # Gin HTTP handlers
│   ├── serve/        # stdio batch service
│   └── tokenize/     # CJK+Latin tokenizer
├── cmd/
│   ├── server/       # HTTP server
│   ├── serve/        # Batch stdio service
│   ├── parse/        # CLI single title parser
│   └── parse_batch/  # CLI batch parser
├── configs/          # 18 YAML config files
├── tests/baselines/  # Golden file regression tests
└── docs/             # Design documentation
```

## Architecture

9-layer parsing pipeline:

```
Tokenize → Lexicon → Signal → ContentType → Template → Extract → Inference → Clean → Output
```

**Key design**: Content type inference **before** template matching, preventing Season Pack misclassification.

## Testing

```bash
# Run tests
go test ./...

# Regression check against baselines
go run ./cmd/regression_check -- -config configs -base tests/baselines/baseline.jsonl
```

## Documentation

- [ARCHIVE.md](./ARCHIVE.md) - Access to historical ML components (git tag)
- `docs/` - Design documentation for rule-based architecture

## License

MIT