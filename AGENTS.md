# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-24
**Commit:** 71efbc7
**Branch:** main

## OVERVIEW

Config-driven anime/film torrent title parser (v2.5). Extracts title, group, EP semantics, and tech specs from release titles. Runtime path is pure rule parser (no ML/ONNX dependency).

## STRUCTURE

```
title-parser/
├── pkg/                    # Core Go packages
│   ├── parser/             # Main parser engine (5351 lines)
│   ├── model/              # Data types (Attribute, ParseResult, Token)
│   ├── api/                # Gin HTTP handlers
│   ├── config/             # YAML config loader
│   ├── crf/                # CRF model (Viterbi)
│   └── tokenize/           # CJK+Latin tokenizer
├── cmd/                    # CLI tools (18 tools)
│   ├── server/             # HTTP server entry point
│   ├── parse/              # Single title parser
│   ├── parse_batch/        # Batch parser
│   ├── regression_check/   # Regression testing
│   └── v2_audit/           # Parser audit tool
├── configs/                # YAML configuration files
│   ├── lexicon.yaml     # Dictionary entries (116KB)
│   ├── template_catalog.yaml  # 33 parsing templates
│   ├── inference_rules.yaml   # Content type inference
│   └── attribute_config.yaml  # Field definitions
├── training/               # ML training (Python + Go submodule)
│   ├── scripts/            # 160+ Python training scripts
│   ├── go/titleparser/     # Secondary Go parser (separate module)
│   └── artifacts/          # Model checkpoints, datasets
├── docs/                   # Design documentation
└── tests/baselines/        # Golden file regression tests
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Parse a title | `pkg/parser/parser.go` | Entry: `Parser.Parse()` |
| Add lexicon entry | `configs/lexicon.yaml` | No code changes needed |
| Add parsing template | `configs/template_catalog.yaml` | 33 templates exist |
| HTTP API | `pkg/api/handler.go` | `/api/v1/parse`, `/api/v1/parse/batch` |
| Canonical output helpers | `pkg/model/canonical.go` | Access canonical attrs / `ep_semantics` |
| Training pipeline | `training/scripts/run_student_structured_ml_pipeline_v0.py` | Main ML loop |
| Regression tests | `tests/baselines/` | Golden file testing |
| Add new CLI tool | `cmd/<tool>/main.go` | Follow existing patterns |

## CODE MAP

| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `Parser` | struct | `pkg/parser/parser.go:19` | Main parser, orchestrates passes |
| `ParseResult` | struct | `pkg/model/types.go:39` | Output container |
| `Attribute` | struct | `pkg/model/types.go:5` | Single extracted attribute |
| `Token` | struct | `pkg/model/types.go:14` | Tokenized text unit |
| `Config` | struct | `pkg/config/types.go` | Config container |

## CONVENTIONS

- **Config-driven**: Rules in YAML, not hardcoded. Add lexicon/template entries via config files.
- **Pass-based parsing**: Parser runs multiple passes (extract_pack, detect_episode, etc.)
- **Two Go modules**: Root module + `training/go/titleparser/` submodule (cannot cross-import)
- **Golden file testing**: Regression tests use `.jsonl` baselines in `tests/baselines/`

## ANTI-PATTERNS (THIS PROJECT)

- **DO NOT** use `go build ./...` or `go build .` - fails due to dual module structure
- **DO NOT** import `training/go/titleparser` from root module - separate modules
- **DO NOT** hardcode parsing rules in Go code - use YAML configs
- **DO NOT** suppress type errors with `as any` or `@ts-ignore`

## UNIQUE STYLES

- **9-layer pipeline**: Tokenize → Lexicon → Signal → ContentType → Template → Extract → Inference → Clean → Output
- **Slot-based internals**: HEAD (group/title), EP candidates, TECH (resolution/codec), TAIL (tailgroup)
- **Public EP contract**: external consumers should read `ep_semantics`; `EPSlot` is internal parser machinery, not output contract
- **Template routing**: Templates classified by streaming vs standard release

## COMMANDS

```bash
# Build HTTP server
go build -o server ./cmd/server

# Run server (port 8080)
PORT=8080 ./server
# or
go run ./cmd/server

# Run tests
go test ./...

# Parse single title (CLI)
go run ./cmd/parse -title "[LoliHouse] Title - 01 [1080p]"

# Regression check
go run ./cmd/regression_check -- -config configs -base tests/baselines/baseline.jsonl

# Python training (requires config.yaml setup)
python training/scripts/run_student_structured_ml_pipeline_v0.py --do-export --do-teacher

```

## NOTES

- **Dual language**: Go for production, Python for ML training
- **Dual parser implementations**: Root `pkg/parser/` (v2 config-driven) + `training/go/titleparser/` (secondary)
- **No internal/ package**: All packages in `pkg/` are technically public
- **Large training data**: `training/` contains ~22GB of artifacts, not in gitignore
- **Content Kind before template**: v2 infers content type before template matching (unlike v1)
- **EP schema**: top-level episode semantics are emitted via `ParseResult.EPSemantics` / JSON `ep_semantics`; legacy EP attrs are filtered from serialized output
- **Known issue**: `parser.test`, `test_crf` binaries in root are legacy
