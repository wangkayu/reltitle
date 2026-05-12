# Baselines

This folder stores pinned regression inputs and baseline outputs for v2.5, so future config/code changes can be evaluated via diffs.

## Files

### Current Baseline (v2.5)

- `baseline.v25.20260307.regression_set_v1.jsonl`
  - Authoritative v2.5 canonical baseline (pure rule-based parser)
  - Includes current normalization contracts:
    - `EP_RANGE + TV全集(+extras)` -> `episode_range + pack_kind`
    - `12END/12FIN` -> `episode + endflag(note)` (not batch trigger)
    - multi-episode token (`804&805`) -> `ep_semantics.kind=EP_LIST`, `episode_list`

- `regression_set_v1.titles.txt`
  - Input titles (one title per line)
  - Source: manually curated test titles

- `rss_titles_step2_v2_3000.titles.txt`
  - RSS feed titles for broader coverage testing

### Archive

- `archive/`
  - Historical baselines from intermediate iterations (kept for reference)

## Re-generate

From `title-parser/`:

```bash
# Run v2.5 batch parse
go run ./cmd/parse_batch -- -config configs \
  -in tests/baselines/regression_set_v1.titles.txt \
  -out tests/baselines/baseline.v25.20260307.regression_set_v1.jsonl

# Print quick stats
go run ./cmd/regression_stats -- -in tests/baselines/baseline.v25.20260307.regression_set_v1.jsonl

# Regression gate (golden diff)
go run ./cmd/regression_check -- -config configs \
  -base tests/baselines/baseline.v25.20260307.regression_set_v1.jsonl
```

## Regression Testing

Before committing parser/config changes:

1. Generate new baseline
2. Run regression check to compare against canonical baseline
3. Review diffs - expected changes should be intentional improvements
4. Update canonical baseline if changes are verified correct