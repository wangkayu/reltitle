#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORTER="$REPO_ROOT/training/scripts/build_actionable_cluster_report.py"
OUT_DIR="$REPO_ROOT/training/artifacts/parser_v25_eval"

usage() {
  cat <<USAGE
Usage: $(basename "$0") --input ACTIONABLE.jsonl [options]

Options:
  --input PATH        actionable queue jsonl (required)
  --run-prefix NAME   output prefix (default: actionable_cluster)
  --out-dir PATH      output dir (default: training/artifacts/parser_v25_eval)
  --top-k N           default: 20
  --sample-per N      default: 5
USAGE
}

INPUT=""
RUN_PREFIX="actionable_cluster"
TOP_K=20
SAMPLE_PER=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input)
      INPUT="$2"
      shift 2
      ;;
    --run-prefix)
      RUN_PREFIX="$2"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --top-k)
      TOP_K="$2"
      shift 2
      ;;
    --sample-per)
      SAMPLE_PER="$2"
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

if [[ -z "$INPUT" ]]; then
  echo "Missing --input" >&2
  usage
  exit 2
fi
if [[ ! -f "$INPUT" ]]; then
  echo "Input file not found: $INPUT" >&2
  exit 2
fi
if [[ ! -f "$REPORTER" ]]; then
  echo "Reporter script not found: $REPORTER" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
ts="$(date +%Y%m%d_%H%M%S)"
run_id="${RUN_PREFIX}_${ts}"
out_json="$OUT_DIR/${run_id}.json"
out_md="$OUT_DIR/${run_id}.md"

python3 "$REPORTER" \
  --input "$INPUT" \
  --out-json "$out_json" \
  --out-md "$out_md" \
  --top-k "$TOP_K" \
  --sample-per-cluster "$SAMPLE_PER"

echo "run_id=$run_id"
echo "report_json=$out_json"
echo "report_md=$out_md"
