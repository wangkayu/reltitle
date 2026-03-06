#!/usr/bin/env python3
"""
Audit parser outputs for "note" field drift.

Goal:
- Find note.normalized values that likely belong to concrete fields
  (source_type/platform/sub_flag/sub_lang/audio_flag/pack_kind).
- Provide counts + examples to guide schema hardening.
"""

from __future__ import annotations

import argparse
import json
import re
from collections import Counter, defaultdict
from pathlib import Path
from typing import Dict, List, Tuple


SOURCE_RE = re.compile(
    r"(?i)^(?:web|webrip|web-?dl|bd|bdrip|bluray|blu-?ray|hdtv|dvd|remux|uhd)$"
)
PLATFORM_RE = re.compile(
    r"(?i)^(?:nf|netflix|cr|baha|abema|unext|amzn|amazon|at-?x|bs11)$"
)
SUB_FLAG_RE = re.compile(r"(?i)^(?:vostfr|vosten|vf|multi|multisub|subs?)$")
SUB_LANG_RE = re.compile(
    r"(?i)^(?:chs|cht|chi|zho|eng|jpn|jp|fre|fra|rus|spa|esp|ita|ger|deu)$"
)
AUDIO_FLAG_RE = re.compile(r"(?i)^(?:dual\\s*audio|multi\\s*audio|english\\s*dub|dub)$")
TECH_RE = re.compile(
    r"(?i)^(?:1080p|2160p|720p|x26[45]|h26[45]|hevc|avc|aac|ac3|flac|ddp|dts|atmos|mkv|mp4)$"
)


def classify_note(value: str) -> str:
    v = value.strip()
    if not v:
        return "empty"
    if SOURCE_RE.match(v):
        return "source_type"
    if PLATFORM_RE.match(v):
        return "platform"
    if SUB_FLAG_RE.match(v):
        return "sub_flag"
    if SUB_LANG_RE.match(v):
        return "sub_lang"
    if AUDIO_FLAG_RE.match(v):
        return "audio_flag"
    if TECH_RE.match(v):
        return "tech"
    return "other"


def audit(path: Path, max_examples: int = 5) -> Dict:
    total_rows = 0
    total_attrs = 0
    note_total = 0

    attr_counter: Counter[str] = Counter()
    note_value_counter: Counter[str] = Counter()
    note_kind_counter: Counter[str] = Counter()
    note_kind_examples: Dict[str, List[Tuple[str, str]]] = defaultdict(list)

    with path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            total_rows += 1
            raw = obj.get("raw_title", "")
            for a in obj.get("attributes") or []:
                t = (a.get("type") or "").strip()
                n = (a.get("normalized") or "").strip()
                total_attrs += 1
                if t:
                    attr_counter[t] += 1
                if t != "note":
                    continue
                note_total += 1
                if n:
                    note_value_counter[n] += 1
                kind = classify_note(n)
                note_kind_counter[kind] += 1
                if len(note_kind_examples[kind]) < max_examples:
                    note_kind_examples[kind].append((n, raw))

    top_notes = note_value_counter.most_common(50)
    suggestible = {
        k: note_kind_counter[k]
        for k in ("source_type", "platform", "sub_flag", "sub_lang", "audio_flag", "tech")
    }
    suggestible_total = sum(suggestible.values())

    return {
        "input_file": str(path),
        "total_rows": total_rows,
        "total_attributes": total_attrs,
        "note_attributes": note_total,
        "note_ratio": (note_total / total_attrs) if total_attrs else 0.0,
        "top_attribute_types": attr_counter.most_common(20),
        "top_note_values": top_notes,
        "note_kind_distribution": dict(note_kind_counter),
        "suggestible_note_total": suggestible_total,
        "suggestible_note_breakdown": suggestible,
        "suggestible_note_ratio_in_note": (suggestible_total / note_total) if note_total else 0.0,
        "examples": {k: [{"value": n, "raw_title": r} for n, r in v] for k, v in note_kind_examples.items()},
    }


def to_markdown(report: Dict) -> str:
    lines: List[str] = []
    lines.append("# Note Field Drift Audit")
    lines.append("")
    lines.append(f"- input_file: `{report['input_file']}`")
    lines.append(f"- total_rows: {report['total_rows']}")
    lines.append(f"- total_attributes: {report['total_attributes']}")
    lines.append(f"- note_attributes: {report['note_attributes']}")
    lines.append(f"- note_ratio: {report['note_ratio']:.6f}")
    lines.append(
        f"- suggestible_note_total: {report['suggestible_note_total']} "
        f"(ratio_in_note={report['suggestible_note_ratio_in_note']:.6f})"
    )
    lines.append("")
    lines.append("## Suggestible Note Breakdown")
    for k, v in report["suggestible_note_breakdown"].items():
        lines.append(f"- {k}: {v}")
    lines.append("")
    lines.append("## Top Note Values (Top 30)")
    for n, c in report["top_note_values"][:30]:
        lines.append(f"- {n}: {c}")
    lines.append("")
    lines.append("## Examples")
    for kind in ("source_type", "platform", "sub_flag", "sub_lang", "audio_flag", "tech"):
        ex = report["examples"].get(kind, [])
        lines.append(f"### {kind}")
        if not ex:
            lines.append("- (none)")
            continue
        for item in ex:
            raw = item["raw_title"].replace("\n", " ")
            lines.append(f"- `{item['value']}` | {raw[:200]}")
        lines.append("")
    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description="Audit note field drift in parser jsonl output.")
    ap.add_argument("--in", dest="in_path", required=True, help="Input parsed jsonl")
    ap.add_argument("--out-json", dest="out_json", required=True, help="Output report json")
    ap.add_argument("--out-md", dest="out_md", required=True, help="Output report markdown")
    args = ap.parse_args()

    in_path = Path(args.in_path)
    out_json = Path(args.out_json)
    out_md = Path(args.out_md)

    report = audit(in_path)
    out_json.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    out_md.write_text(to_markdown(report), encoding="utf-8")
    print(json.dumps({"out_json": str(out_json), "out_md": str(out_md)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

