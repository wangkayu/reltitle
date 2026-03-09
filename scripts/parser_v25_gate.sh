#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCANNER="$REPO_ROOT/training/scripts/scan_retrieval_aliases_quality.py"
OUT_DIR="$REPO_ROOT/training/artifacts/parser_v25_eval"
LEXICON_LINT="$REPO_ROOT/scripts/lint_lexicon_boundaries.py"
SP_FOCUS_GATE="$REPO_ROOT/scripts/parser_v25_sp_focus_gate.sh"

usage() {
  cat <<USAGE
Usage: $(basename "$0") [--stage 10k|50k|120k|all] [--run-prefix PREFIX]
                           [--skip-lexicon-lint]
                           [--skip-sp-focus]

Examples:
  $(basename "$0") --stage 10k
  $(basename "$0") --stage all --run-prefix release_gate
USAGE
}

STAGE="10k"
RUN_PREFIX="gate"
SKIP_LEXICON_LINT=0
RUN_SP_FOCUS=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage)
      STAGE="$2"
      shift 2
      ;;
    --run-prefix)
      RUN_PREFIX="$2"
      shift 2
      ;;
    --skip-lexicon-lint)
      SKIP_LEXICON_LINT=1
      shift 1
      ;;
    --skip-sp-focus)
      RUN_SP_FOCUS=0
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown arg: $1" >&2
      usage
      exit 2
      ;;
  esac
done

case "$STAGE" in
  10k|50k|120k|all) ;;
  *)
    echo "Invalid --stage: $STAGE" >&2
    exit 2
    ;;
esac

if [[ "$SKIP_LEXICON_LINT" != "1" ]]; then
  if [[ ! -f "$LEXICON_LINT" ]]; then
    echo "Missing lexicon lint script: $LEXICON_LINT" >&2
    exit 2
  fi
  echo "[gate] run lexicon boundary lint"
  python3 "$LEXICON_LINT"
fi

if [[ "$RUN_SP_FOCUS" == "1" && ! -f "$SP_FOCUS_GATE" ]]; then
  echo "Missing sp focus gate script: $SP_FOCUS_GATE" >&2
  exit 2
fi

run_stage() {
  local stage="$1"
  local titles_file limit
  case "$stage" in
    10k)
      titles_file="$REPO_ROOT/training/artifacts/parser_v25_eval/canary_tk_10000_20260305/titles_10000.txt"
      limit=10000
      ;;
    50k)
      titles_file="$REPO_ROOT/training/artifacts/parser_v25_eval/raw120k_50000_titles_r10.txt"
      limit=50000
      ;;
    120k)
      titles_file="$REPO_ROOT/training/artifacts/parser_v25_eval/raw120k_120000_titles_r11.txt"
      limit=120000
      ;;
    *)
      echo "internal: unsupported stage=$stage" >&2
      return 2
      ;;
  esac

  if [[ ! -f "$titles_file" ]]; then
    echo "Missing titles file: $titles_file" >&2
    return 2
  fi

  local ts run_id summary_json
  ts="$(date +%Y%m%d_%H%M%S)"
  run_id="${RUN_PREFIX}_${stage}_${ts}"

  echo "[gate] stage=$stage run_id=$run_id"

  python3 "$SCANNER" \
    --repo-root "$REPO_ROOT" \
    --config-dir configs \
    --titles-file "$titles_file" \
    --limit "$limit" \
    --out-dir "$OUT_DIR" \
    --run-id "$run_id"

  summary_json="$OUT_DIR/${run_id}.summary.json"
  python3 - "$summary_json" <<'PY'
import json, sys
p = sys.argv[1]
s = json.load(open(p, encoding='utf-8'))
checks = {
    'rows_meta_polluted': s.get('rows_meta_polluted', 0) == 0,
    'title_meta_polluted_rows': s.get('title_meta_polluted_rows', 0) == 0,
    'rows_episode_like_polluted': s.get('rows_episode_like_polluted', 0) == 0,
}
failed = [k for k, ok in checks.items() if not ok]
print(json.dumps({
    'summary_json': p,
    'total': s.get('total'),
    'retrieval_nonempty_rate': s.get('retrieval_nonempty_rate'),
    'rows_meta_polluted': s.get('rows_meta_polluted'),
    'title_meta_polluted_rows': s.get('title_meta_polluted_rows'),
    'rows_episode_like_polluted': s.get('rows_episode_like_polluted'),
    'pass': not failed,
    'failed': failed,
}, ensure_ascii=False, indent=2))
if failed:
    sys.exit(1)
PY
}

if [[ "$STAGE" == "all" ]]; then
  run_stage 10k
  run_stage 50k
  run_stage 120k
else
  run_stage "$STAGE"
fi

if [[ "$RUN_SP_FOCUS" == "1" ]]; then
  echo "[gate] run sp focus gate"
  bash "$SP_FOCUS_GATE" --run-prefix "${RUN_PREFIX}_sp_focus"
fi

echo "[gate] done"
