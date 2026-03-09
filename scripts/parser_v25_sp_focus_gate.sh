#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKER="$REPO_ROOT/training/scripts/check_sp_focus_samples.py"
OUT_DIR="$REPO_ROOT/training/artifacts/parser_v25_eval"
SAMPLES="$REPO_ROOT/tests/samples/sp_focus_nced_pv_v1.jsonl"

usage() {
  cat <<USAGE
Usage: $(basename "$0") [--samples PATH] [--run-prefix PREFIX]

Options:
  --samples PATH      sample jsonl path (default: tests/samples/sp_focus_nced_pv_v1.jsonl)
  --run-prefix NAME   output filename prefix (default: sp_focus_gate)
USAGE
}

RUN_PREFIX="sp_focus_gate"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --samples)
      SAMPLES="$2"
      shift 2
      ;;
    --run-prefix)
      RUN_PREFIX="$2"
      shift 2
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

if [[ ! -f "$CHECKER" ]]; then
  echo "Missing checker script: $CHECKER" >&2
  exit 2
fi
if [[ ! -f "$SAMPLES" ]]; then
  echo "Missing samples file: $SAMPLES" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
ts="$(date +%Y%m%d_%H%M%S)"
report="$OUT_DIR/${RUN_PREFIX}_${ts}.report.json"

python3 "$CHECKER" \
  --repo-root "$REPO_ROOT" \
  --config-dir "configs" \
  --samples "$SAMPLES" \
  --out-report "$report"

echo "report=$report"
