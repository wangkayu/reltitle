# Archive Guide - Historical ML Components

This document explains how to access archived ML/ML-related components for reference.

## Quick Access

```bash
# View the complete ML implementation before v2.5 migration
git checkout archive/ml-last-version-20260305

# Return to current main branch
git checkout main
```

## Archived Components

### Git Tag: `archive/ml-last-version-20260305`

All ML-related code and training infrastructure has been archived:

| Component | Path | Description |
|-----------|------|-------------|
| **Training Workspace** | `training/` | Complete ML training pipeline (~2.4GB) |
| Training Scripts | `training/scripts/` | 210+ Python scripts for ML training |
| v1 Parser | `training/go/titleparser/` | Independent Go module (ML-first) |
| Training Docs | `training/docs/` | ML design documents |
| Training Resources | `training/resources/` | YAML configs for training |
| **ONNX Engine** | `pkg/onnx/` | ONNX inference for token boundary |
| **XLM-RoBERTa** | `pkg/xlmr/` | XLM-R tokenizer + ONNX inference |
| **CRF Model** | `pkg/crf/` | Conditional Random Field with Viterbi |
| **Token Boundary** | `pkg/token_boundary/` | LLM-based boundary detection |
| **Quality Gate** | `pkg/quality/gate.go` | ML-triggered quality evaluation |
| **v2.6 Experimental** | `pkg/parserv26/` | Candidate graph inference prototype |
| **XLM-R Service** | `xlmr-service/` | Flask HTTP service for XLM-R |
| **Temporal Workflow** | `orchestration/` | Workflow orchestration for ML |
| **Docker Configs** | `infra/` | Docker compose for ML services |

### Key Design Documents (In Tag)

| Document | Purpose |
|----------|---------|
| `training/docs/GO_CRF_INTEGRATION_PLAN.md` | CRF integration design |
| `training/docs/GO_CRF_PHASE3_COMPLETE.md` | Implementation summary |
| `training/docs/GO_CRF_FEATURE_FIX_GUIDE.md` | Feature engineering details |
| `docs/XLMR_TOKEN_TAGGING_INTEGRATION.md` | XLM-R integration |
| `docs/PIPELINE_V3_QUALITY_GATED_RERANK.md` | Quality gate pipeline |

## Migration Timeline

```
2025-01     v1 ML-first parser (training/go/titleparser/)
    ↓
2026-01-17  v2 config-driven design starts
    ↓
2026-03-05  archive/ml-last-version-20260305 (last ML version)
    ↓
2026-03-06  530e5ef: remove ml runtime path
    ↓
2026-03-10  v2.5 pure-rule parser released
    ↓
Current     v2.5+ config-driven, zero ML dependency
```

## Why These Were Archived

1. **Runtime Complexity**: ML models added deployment overhead
2. **Maintenance Cost**: Dual parser paths increased debugging difficulty
3. **Performance**: Pure-rule parser achieved sufficient accuracy
4. **Config-driven Success**: YAML configs proved flexible enough

## Current Project Structure

```
title-parser/
├── pkg/parser/        # v2.5 pure-rule parser (main)
├── pkg/config/        # YAML config loader
├── pkg/model/         # Data types
├── pkg/api/           # HTTP handlers
├── pkg/serve/         # stdio batch service
├── pkg/tokenize/      # CJK+Latin tokenizer
├── configs/           # 18 YAML config files
├── cmd/               # CLI tools (server, parse, serve)
├── docs/              # Design docs (rule-based)
└── tests/             # Regression tests
```

**No ML dependency. No training infrastructure. Pure rule-based configuration.**

## Viewing Archived Code

```bash
# Example: View CRF Viterbi implementation
git show archive/ml-last-version-20260305:pkg/crf/model.go

# Example: View XLM-R service
git show archive/ml-last-version-20260305:xlmr-service/main.py

# Example: View training pipeline
git show archive/ml-last-version-20260305:training/scripts/run_student_structured_ml_pipeline_v0.py

# Browse full archived structure
git checkout archive/ml-last-version-20260305
ls -la
```