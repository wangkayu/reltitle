#!/usr/bin/env python3
import argparse
import json
import random
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple


def _norm_num(s: str) -> str:
    s = (s or "").strip()
    if s == "":
        return ""
    # keep decimals as-is (e.g. 1.5)
    if "." in s:
        left, right = s.split(".", 1)
        left2 = left.lstrip("0") or left
        return left2 + "." + right
    s2 = s.lstrip("0")
    return s2 if s2 != "" else s


def _norm_range(s: str) -> str:
    s = (s or "").strip()
    if s == "":
        return ""
    m = re.match(r"^0*(\d{1,4})\s*[-~]\s*0*(\d{1,4})", s)
    if m:
        return _norm_num(m.group(1)) + "-" + _norm_num(m.group(2))
    for sep in ("-", "~"):
        if sep in s:
            a, b = s.split(sep, 1)
            return _norm_num(a) + "-" + _norm_num(re.match(r"^0*(\d{1,4})", b).group(1) if re.match(r"^0*(\d{1,4})", b) else b)
    return s


def _safe_slice(raw: str, start: int, end: int) -> str:
    if start < 0:
        start = 0
    if end < start:
        end = start
    if start > len(raw):
        start = len(raw)
    if end > len(raw):
        end = len(raw)
    return raw[start:end]


@dataclass
class Span:
    start: int
    end: int


_SXXEXX = re.compile(r"(?i)^s(\d{1,2})e(\d{1,4})(?:v\d+)?$")
_SXXEXX_PREFIX = re.compile(r"(?i)^s(\d{1,2})e(\d{1,4})")


def _token_norm(t: Dict[str, Any]) -> str:
    v = t.get("norm")
    if v is None:
        v = t.get("text")
    return str(v or "").strip()


def _extract_first_attr_value(tokens: List[Dict[str, Any]], attr_type: str) -> str:
    """Extract a single-field value from token annotations.

    Uses token-level attributes instead of slicing raw string spans, to avoid
    accidentally unioning unrelated spans.
    """
    for idx, t in enumerate(tokens):
        types = t.get("attr_types")
        if not isinstance(types, list) or attr_type not in types:
            continue
        norm = _token_norm(t)
        if not norm:
            continue

        # Special handling: S01E02 token can yield season and episode.
        m = _SXXEXX.match(norm) or _SXXEXX_PREFIX.match(norm)
        if m:
            season = _norm_num(m.group(1))
            episode = _norm_num(m.group(2))
            if attr_type == "season":
                return season
            if attr_type == "episode":
                return episode
            return norm

        if attr_type == "season":
            m = re.match(r"(?i)^s(\d{1,2})$", norm)
            if m:
                return _norm_num(m.group(1))
            m = re.match(r"(?i)^s(\d{1,2})[^0-9a-z].+$", norm)
            if m:
                return _norm_num(m.group(1))
            m = re.match(r"^第?([一二三四五六七八九十0-9]{1,3})季$", norm)
            if m:
                cn = {"一": "1", "二": "2", "三": "3", "四": "4", "五": "5", "六": "6", "七": "7", "八": "8", "九": "9", "十": "10"}
                v = m.group(1)
                if v.isdigit():
                    return _norm_num(v)
                if v in cn:
                    return cn[v]
            m = re.match(r"^第?([一二三四五六七八九十0-9]{1,3})期$", norm)
            if m:
                cn = {"一": "1", "二": "2", "三": "3", "四": "4", "五": "5", "六": "6", "七": "7", "八": "8", "九": "9", "十": "10"}
                v = m.group(1)
                if v.isdigit():
                    return _norm_num(v)
                if v in cn:
                    return cn[v]
            m = re.match(r"^(\d{1,2})期$", norm)
            if m:
                return _norm_num(m.group(1))
            if norm.lower() == "season":
                # Only emit if next season token provides the number
                if idx + 1 < len(tokens):
                    n2 = _token_norm(tokens[idx + 1])
                    if re.match(r"^\d{1,2}$", n2):
                        return _norm_num(n2)
                return ""
            return _norm_num(norm)

        # numeric normalization
        if attr_type == "episode":
            return _norm_num(norm)
        if attr_type == "episode_range":
            return _norm_range(norm)

        return norm
    return ""


def _extract_gold_fields(obj: Dict[str, Any]) -> Dict[str, Any]:
    raw = (obj.get("raw_title") or "").replace("\n", " ").strip()
    cv = (obj.get("contract_v2") or {}).get("corrected_view") or {}
    tokens = (obj.get("contract_v2") or {}).get("tokens") or []

    title = ""
    aliases: List[str] = []
    template_family = ""
    qualified: Optional[bool] = None
    title_span: Optional[Dict[str, Any]] = None
    if isinstance(cv, dict):
        title = (cv.get("title") or "").strip()
        als = cv.get("aliases")
        if isinstance(als, list):
            aliases = [str(x).strip() for x in als if str(x).strip()]
        template_family = str(cv.get("template_family") or "").strip()
        qv = cv.get("qualified")
        if isinstance(qv, bool):
            qualified = qv
        tsp = cv.get("title_span")
        if isinstance(tsp, dict):
            title_span = tsp

    group = _extract_first_attr_value(tokens, "group")
    episode = _extract_first_attr_value(tokens, "episode")
    episode_range = _extract_first_attr_value(tokens, "episode_range")
    season = _extract_first_attr_value(tokens, "season")

    return {
        "raw_title": raw,
        "group": group or "",
        "title": title or "",
        "aliases": aliases or None,
        "episode": episode or "",
        "episode_range": episode_range or "",
        "season": season or "",
        # v2-only fields left empty for now
        "content_kind": "",
        "template_id": "",
        "template_family": template_family or "",
        "qualified": qualified,
        "title_span": title_span,
        # Keep output compatible with title-parser/pkg/model.ParseResult
    }


def _iter_jsonl(path: Path) -> Iterable[Dict[str, Any]]:
    with path.open("r", encoding="utf-8") as f:
        for line_no, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except Exception as e:
                raise RuntimeError(f"bad json at line {line_no}: {e}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="in_path", required=True, help="contract_v2 jsonl path")
    ap.add_argument("--out-titles", dest="out_titles", required=True, help="output titles txt (one raw_title per line)")
    ap.add_argument("--out-gold", dest="out_gold", required=True, help="output gold jsonl (ParseResult-like)")
    ap.add_argument("--only-passed", action="store_true", default=False, help="only keep rows with quality.passed=true")
    ap.add_argument("--limit", type=int, default=0, help="max rows (0=no limit)")
    ap.add_argument("--shuffle", action="store_true", default=False)
    ap.add_argument("--seed", type=int, default=0)
    args = ap.parse_args()

    in_path = Path(args.in_path)
    out_titles = Path(args.out_titles)
    out_gold = Path(args.out_gold)

    rows: List[Dict[str, Any]] = []
    for obj in _iter_jsonl(in_path):
        if args.only_passed:
            q = obj.get("quality")
            if isinstance(q, dict) and q.get("passed") is not True:
                continue
        rows.append(obj)

    if args.shuffle:
        rnd = random.Random(args.seed)
        rnd.shuffle(rows)

    if args.limit and args.limit > 0:
        rows = rows[: args.limit]

    out_titles.parent.mkdir(parents=True, exist_ok=True)
    out_gold.parent.mkdir(parents=True, exist_ok=True)

    wrote = 0
    with out_titles.open("w", encoding="utf-8") as t_out, out_gold.open("w", encoding="utf-8") as g_out:
        for obj in rows:
            raw = (obj.get("raw_title") or "").replace("\n", " ").strip()
            if not raw:
                continue
            gold = _extract_gold_fields(obj)
            if gold.get("raw_title") != raw:
                gold["raw_title"] = raw

            t_out.write(raw + "\n")
            g_out.write(json.dumps(gold, ensure_ascii=False) + "\n")
            wrote += 1

    print(f"wrote_titles={wrote}")
    print(f"out_titles={out_titles}")
    print(f"out_gold={out_gold}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
