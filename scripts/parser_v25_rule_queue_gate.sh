#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
QUALIFIER="$REPO_ROOT/training/scripts/qualify_rule_learning_samples.py"
OUT_DIR="$REPO_ROOT/training/artifacts/parser_v25_eval"

usage() {
  cat <<USAGE
Usage: $(basename "$0") --input QUEUE.jsonl [options]

Options:
  --input PATH                  Input review queue jsonl (required)
  --out-dir PATH                Output directory (default: training/artifacts/parser_v25_eval)
  --run-prefix PREFIX           Prefix for output filenames (default: rule_queue_gate)
  --min-cluster-size N          Default: 10
  --min-group-diversity N       Default: 2
  --min-single-group-size N     Default: 20
  --allow-empty-actionable      Do not fail when actionable_rows == 0

Examples:
  $(basename "$0") \
    --input training/artifacts/parser_v25_eval/unknown_template_90_human_review_by_cluster.jsonl
USAGE
}

INPUT=""
RUN_PREFIX="rule_queue_gate"
MIN_CLUSTER_SIZE=10
MIN_GROUP_DIVERSITY=2
MIN_SINGLE_GROUP_SIZE=20
ALLOW_EMPTY_ACTIONABLE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input)
      INPUT="$2"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --run-prefix)
      RUN_PREFIX="$2"
      shift 2
      ;;
    --min-cluster-size)
      MIN_CLUSTER_SIZE="$2"
      shift 2
      ;;
    --min-group-diversity)
      MIN_GROUP_DIVERSITY="$2"
      shift 2
      ;;
    --min-single-group-size)
      MIN_SINGLE_GROUP_SIZE="$2"
      shift 2
      ;;
    --allow-empty-actionable)
      ALLOW_EMPTY_ACTIONABLE=1
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

if [[ -z "$INPUT" ]]; then
  echo "Missing --input" >&2
  usage
  exit 2
fi

if [[ ! -f "$INPUT" ]]; then
  echo "Input queue not found: $INPUT" >&2
  exit 2
fi

if [[ ! -f "$QUALIFIER" ]]; then
  echo "Qualifier script not found: $QUALIFIER" >&2
  exit 2
fi

ts="$(date +%Y%m%d_%H%M%S)"
run_id="${RUN_PREFIX}_${ts}"
mkdir -p "$OUT_DIR"

actionable="$OUT_DIR/${run_id}.actionable.jsonl"
holdout="$OUT_DIR/${run_id}.holdout.jsonl"
non_target="$OUT_DIR/${run_id}.non_target.jsonl"
report="$OUT_DIR/${run_id}.report.json"

python3 "$QUALIFIER" \
  --input "$INPUT" \
  --out-actionable "$actionable" \
  --out-holdout "$holdout" \
  --out-non-target "$non_target" \
  --out-report "$report" \
  --min-cluster-size "$MIN_CLUSTER_SIZE" \
  --min-group-diversity "$MIN_GROUP_DIVERSITY" \
  --min-single-group-size "$MIN_SINGLE_GROUP_SIZE" \
  --non-target-cluster 'C_APP_PACKAGE' \
  --non-target-pattern '(?i)(?:官方)?(?:x?apk|ipa)' \
  --non-target-pattern '(?i)(?:windows|android|ios|mod|破解|安装包|installer|setup)' \
  --force-holdout-cluster 'C_OTHER_UNKNOWN'

python3 - "$report" "$ALLOW_EMPTY_ACTIONABLE" <<'PY'
import json, sys
p = sys.argv[1]
allow_empty = bool(int(sys.argv[2]))
r = json.load(open(p, encoding='utf-8'))
failed = []
if r.get("total_rows", 0) <= 0:
    failed.append("empty_input")
if r.get("target_rows", 0) <= 0:
    failed.append("no_target_rows")
if (not allow_empty) and r.get("actionable_rows", 0) <= 0:
    failed.append("no_actionable_rows")

print(json.dumps({
    "report": p,
    "total_rows": r.get("total_rows", 0),
    "target_rows": r.get("target_rows", 0),
    "non_target_rows": r.get("non_target_rows", 0),
    "actionable_rows": r.get("actionable_rows", 0),
    "holdout_rows": r.get("holdout_rows", 0),
    "actionable_cluster_count": r.get("actionable_cluster_count", 0),
    "pass": not failed,
    "failed": failed,
}, ensure_ascii=False, indent=2))

if failed:
    sys.exit(1)
PY

echo "run_id=$run_id"
echo "actionable=$actionable"
echo "holdout=$holdout"
echo "non_target=$non_target"
echo "report=$report"
