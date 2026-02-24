# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-24
**Commit:** 71efbc7
**Branch:** main

## OVERVIEW

Config-driven anime/film torrent title parser (v2). Extracts title, episode, season, group, tech specs from release titles. Go 1.22 + Gin HTTP server + Python ML training pipeline.

## STRUCTURE

```
title-parser/
├── pkg/                    # Core Go packages
│   ├── parser/             # Main parser engine (5351 lines)
│   ├── model/              # Data types (Attribute, ParseResult, Token)
│   ├── api/                # Gin HTTP handlers
│   ├── config/             # YAML config loader
│   ├── xlmr/               # XLM-RoBERTa ONNX inference
│   ├── crf/                # CRF model (Viterbi)
│   ├── quality/            # Quality gate / ML trigger
│   └── tokenize/           # CJK+Latin tokenizer
├── cmd/                    # CLI tools (18 tools)
│   ├── server/             # HTTP server entry point
│   ├── parse/              # Single title parser
│   ├── parse_batch/        # Batch parser
│   ├── regression_check/   # Regression testing
│   └── v2_audit/           # Parser audit tool
├── configs/                # YAML configuration files
│   ├── lexicon_v1.yaml     # Dictionary entries (116KB)
│   ├── template_catalog.yaml  # 33 parsing templates
│   ├── inference_rules.yaml   # Content type inference
│   └── attribute_config.yaml  # Field definitions
├── training/               # ML training (Python + Go submodule)
│   ├── scripts/            # 160+ Python training scripts
│   ├── go/titleparser/     # Secondary Go parser (separate module)
│   └── artifacts/          # Model checkpoints, datasets
├── xlmr-service/           # FastAPI XLM-R service (port 8081)
├── docs/                   # Design documentation
└── tests/baselines/        # Golden file regression tests
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Parse a title | `pkg/parser/parser.go` | Entry: `Parser.Parse()` |
| Add lexicon entry | `configs/lexicon_v1.yaml` | No code changes needed |
| Add parsing template | `configs/template_catalog.yaml` | 33 templates exist |
| HTTP API | `pkg/api/handler.go` | `/api/v1/parse`, `/api/v1/parse/batch` |
| Quality check logic | `pkg/quality/gate.go` | Triggers XLM-R on low confidence |
| XLM-R integration | `pkg/xlmr/xlmr_onnx.go` | ONNX Runtime inference |
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
| `XLMREngine` | struct | `pkg/xlmr/xlmr_onnx.go` | XLM-R ONNX inference |
| `QualityCheck` | struct | `pkg/quality/gate.go` | Quality assessment |

## CONVENTIONS

- **Config-driven**: Rules in YAML, not hardcoded. Add lexicon/template entries via config files.
- **Pass-based parsing**: Parser runs multiple passes (extract_pack, detect_episode, etc.)
- **Quality-gated ML**: Low confidence results trigger XLM-R model
- **Two Go modules**: Root module + `training/go/titleparser/` submodule (cannot cross-import)
- **Golden file testing**: Regression tests use `.jsonl` baselines in `tests/baselines/`

## ANTI-PATTERNS (THIS PROJECT)

- **DO NOT** use `go build ./...` or `go build .` - fails due to dual module structure
- **DO NOT** import `training/go/titleparser` from root module - separate modules
- **DO NOT** hardcode parsing rules in Go code - use YAML configs
- **DO NOT** suppress type errors with `as any` or `@ts-ignore`
- **DO NOT** skip quality gate when modifying parser logic

## UNIQUE STYLES

- **9-layer pipeline**: Tokenize → Lexicon → Signal → ContentType → Template → Extract → Inference → Clean → Output
- **Slot-based**: HEAD (group/title), EP (episode/season), TECH (resolution/codec), TAIL (tailgroup)
- **EPSlot pattern**: Episode candidates with primary + alternatives
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

# XLM-R service (requires XLMR_MODEL_DIR)
cd xlmr-service && python main.py
```

## NOTES

- **Dual language**: Go for production, Python for ML training
- **Dual parser implementations**: Root `pkg/parser/` (v2 config-driven) + `training/go/titleparser/` (secondary)
- **No internal/ package**: All packages in `pkg/` are technically public
- **Large training data**: `training/` contains ~22GB of artifacts, not in gitignore
- **Content Kind before template**: v2 infers content type before template matching (unlike v1)
- **Known issue**: `parser.test`, `test_crf` binaries in root are legacy
