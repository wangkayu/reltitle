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

| Component | Path | Description |
|-----------|------|-------------|
| ONNX Engine | `pkg/onnx/` | ONNX inference for token boundary |
| XLM-RoBERTa | `pkg/xlmr/` | XLM-R tokenizer + ONNX inference |
| CRF Model | `pkg/crf/` | Conditional Random Field with Viterbi |
| Token Boundary | `pkg/token_boundary/` | LLM-based boundary detection |
| Quality Gate | `pkg/quality/gate.go` | ML-triggered quality evaluation |
| Training Workspace | `training/` | Complete ML training pipeline |

### Key Design Documents (Still in main branch)

| Document | Path | Purpose |
|----------|------|---------|
| ML Integration Plan | `training/docs/GO_CRF_INTEGRATION_PLAN.md` | CRF integration design |
| Phase 3 Complete | `training/docs/GO_CRF_PHASE3_COMPLETE.md` | Implementation summary |
| Feature Fix Guide | `training/docs/GO_CRF_FEATURE_FIX_GUIDE.md` | Feature engineering details |
| XLM-R Integration | `docs/XLMR_TOKEN_TAGGING_INTEGRATION.md` | (archived in tag) |
| Pipeline V3 | `docs/PIPELINE_V3_QUALITY_GATED_RERANK.md` | (archived in tag) |

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
Current     v2.5+ config-driven, no ML dependency
```

## Why These Were Archived

1. **Runtime Complexity**: ML models added deployment overhead (ONNX runtime, model files)
2. **Maintenance Cost**: Dual parser paths (rules + ML) increased debugging difficulty
3. **Performance**: Pure-rule parser achieved sufficient accuracy without ML overhead
4. **Config-driven Success**: YAML configs proved flexible enough for edge cases

## Design Patterns Worth Revisiting

### 1. CRF Feature Engineering
```bash
git show archive/ml-last-version-20260305:pkg/crf/features.go
```
- BIO tagging scheme for token classification
- Context window features (-1, +1 tokens)
- Character type detection (CJK, Latin, digit)

### 2. Viterbi Decoding
```bash
git show archive/ml-last-version-20260305:pkg/crf/model.go
```
- Dynamic programming for optimal label sequence
- Transition and emission score combination

### 3. Quality Gate Mechanism
```bash
git show archive/ml-last-version-20260305:pkg/quality/gate.go
```
- Sample-level quality thresholds
- ML trigger conditions
- Batch evaluation pipeline

### 4. Training Data Generation
```bash
git checkout archive/ml-last-version-20260305
ls training/scripts/run_*.py
```
- Teacher-student pipeline
- Contrastive learning data generation
- LLM-assisted annotation

## Restoring Specific Files

If you need to restore a specific archived file:

```bash
# Example: Restore CRF model implementation
git show archive/ml-last-version-20260305:pkg/crf/model.go > /tmp/crf_model.go

# View in browser (if GitHub)
# https://github.com/YOUR_REPO/tree/archive/ml-last-version-20260305
```

## Related Tags

- `v2.5-gate-green` - v2.5 gate passing
- `v2.5-pure-canonical-20260306` - Pure canonical output migration
- `archive/ml-last-version-20260305` - Last ML version (this archive)
