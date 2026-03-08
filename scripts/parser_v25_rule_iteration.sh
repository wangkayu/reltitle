#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
QUEUE_GATE="$REPO_ROOT/scripts/parser_v25_rule_queue_gate.sh"
CLUSTER_REPORT="$REPO_ROOT/scripts/parser_v25_actionable_cluster_report.sh"

usage() {
  cat <<USAGE
Usage: $(basename "$0") --input QUEUE.jsonl [options]

Options:
  --input PATH                  Input queue jsonl (required)
  --run-prefix PREFIX           Prefix for all generated artifacts (default: rule_iter)
  --allow-empty-actionable      Pass through to queue gate (for stats-only runs)
  --min-cluster-size N          Pass through to queue gate
  --min-group-diversity N       Pass through to queue gate
  --min-single-group-size N     Pass through to queue gate
USAGE
}

INPUT=""
RUN_PREFIX="rule_iter"
ALLOW_EMPTY=0
MIN_CLUSTER_SIZE=""
MIN_GROUP_DIVERSITY=""
MIN_SINGLE_GROUP_SIZE=""

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
    --allow-empty-actionable)
      ALLOW_EMPTY=1
      shift 1
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

if [[ ! -x "$QUEUE_GATE" ]]; then
  echo "Queue gate script not executable: $QUEUE_GATE" >&2
  exit 2
fi
if [[ ! -x "$CLUSTER_REPORT" ]]; then
  echo "Cluster report script not executable: $CLUSTER_REPORT" >&2
  exit 2
fi

extra_args=()
if [[ "$ALLOW_EMPTY" == "1" ]]; then
  extra_args+=(--allow-empty-actionable)
fi
if [[ -n "$MIN_CLUSTER_SIZE" ]]; then
  extra_args+=(--min-cluster-size "$MIN_CLUSTER_SIZE")
fi
if [[ -n "$MIN_GROUP_DIVERSITY" ]]; then
  extra_args+=(--min-group-diversity "$MIN_GROUP_DIVERSITY")
fi
if [[ -n "$MIN_SINGLE_GROUP_SIZE" ]]; then
  extra_args+=(--min-single-group-size "$MIN_SINGLE_GROUP_SIZE")
fi

cmd=("$QUEUE_GATE" --input "$INPUT" --run-prefix "$RUN_PREFIX")
if [[ "${#extra_args[@]}" -gt 0 ]]; then
  cmd+=("${extra_args[@]}")
fi
set +e
gate_output="$("${cmd[@]}" 2>&1)"
gate_code=$?
set -e
echo "$gate_output"
if [[ "$gate_code" -ne 0 ]]; then
  exit "$gate_code"
fi

actionable_path="$(echo "$gate_output" | awk -F= '/^actionable=/{print $2}' | tail -n1)"
if [[ -z "$actionable_path" ]]; then
  echo "Failed to parse actionable path from queue gate output." >&2
  exit 2
fi

if [[ ! -f "$actionable_path" ]]; then
  echo "Actionable file not found: $actionable_path" >&2
  exit 2
fi

actionable_count="$(wc -l < "$actionable_path" | tr -d ' ')"
if [[ "$actionable_count" -le 0 ]]; then
  echo "No actionable rows. Skip cluster report."
  exit 0
fi

"$CLUSTER_REPORT" --input "$actionable_path" --run-prefix "${RUN_PREFIX}_clusters"
