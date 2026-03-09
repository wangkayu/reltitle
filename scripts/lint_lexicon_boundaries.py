#!/usr/bin/env python3
"""
Lexicon boundary lint for parser v2.5.

Checks:
1) duplicate entry ids
2) conflicting emits on same token+scope
3) short alnum token (len<=2) cannot be global plain (except allowlist)
4) digit+curated-pack token cannot be remove:true in lexicon
"""

from __future__ import annotations

import argparse
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


SHORT_ALNUM = re.compile(r"^[A-Za-z0-9]{1,2}$")
DIGIT_CURATED_PACK = re.compile(r"^\d{1,4}(?:修正)?合集$", re.IGNORECASE)

# Version short forms are intentionally global.
SHORT_GLOBAL_ALLOWLIST = {"v2", "v3", "v4", "v5", "4k", "dv", "nf", "cr"}


@dataclass(frozen=True)
class RuleLeaf:
    entry_id: str
    scope: str
    tokens: tuple[str, ...]
    emit_sig: tuple[tuple[str, str], ...]
    remove: bool
    path: str


def _as_scope(v: Any) -> str:
    if v is None:
        return "plain"
    s = str(v).strip().lower()
    if not s:
        return "plain"
    return s


def _emit_sig(obj: dict[str, Any]) -> tuple[tuple[str, str], ...]:
    emits = obj.get("emit", [])
    out: list[tuple[str, str]] = []
    if not isinstance(emits, list):
        return tuple(out)
    for e in emits:
        if not isinstance(e, dict):
            continue
        typ = str(e.get("type", "")).strip()
        norm = str(e.get("normalized", "")).strip()
        out.append((typ, norm))
    return tuple(out)


def _walk(
    obj: Any,
    *,
    entry_id: str,
    inherited_scope: str,
    path: str,
    out: list[RuleLeaf],
) -> None:
    if isinstance(obj, dict):
        scope = _as_scope(obj.get("scope", inherited_scope))
        if "tokens" in obj and isinstance(obj["tokens"], list):
            tokens = tuple(str(x).strip() for x in obj["tokens"])
            remove = bool(obj.get("remove", False))
            out.append(
                RuleLeaf(
                    entry_id=entry_id,
                    scope=scope,
                    tokens=tokens,
                    emit_sig=_emit_sig(obj),
                    remove=remove,
                    path=path,
                )
            )
        for k, v in obj.items():
            _walk(
                v,
                entry_id=entry_id,
                inherited_scope=scope,
                path=f"{path}/{k}",
                out=out,
            )
        return
    if isinstance(obj, list):
        for i, v in enumerate(obj):
            _walk(
                v,
                entry_id=entry_id,
                inherited_scope=inherited_scope,
                path=f"{path}[{i}]",
                out=out,
            )


def collect_rules(lexicon_path: Path) -> tuple[list[RuleLeaf], list[str]]:
    with lexicon_path.open("r", encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}
    entries = data.get("entries", [])
    if not isinstance(entries, list):
        raise ValueError("lexicon.yaml entries must be a list")

    entry_ids: list[str] = []
    leaves: list[RuleLeaf] = []
    for idx, e in enumerate(entries):
        if not isinstance(e, dict):
            continue
        eid = str(e.get("id", "")).strip() or f"<missing-id-{idx}>"
        entry_ids.append(eid)
        base_scope = _as_scope(e.get("scope"))
        _walk(e, entry_id=eid, inherited_scope=base_scope, path=f"entries[{idx}]", out=leaves)
    return leaves, entry_ids


def lint(leaves: list[RuleLeaf], entry_ids: list[str]) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warnings: list[str] = []

    # 1) duplicate ids
    id_cnt = Counter(entry_ids)
    for eid, cnt in sorted(id_cnt.items()):
        if cnt > 1:
            errors.append(f"duplicate entry id: {eid} (count={cnt})")

    # 2) conflicting emits for same token+scope
    by_key: dict[tuple[tuple[str, ...], str], list[RuleLeaf]] = defaultdict(list)
    for r in leaves:
        key_tokens = tuple(t.lower() for t in r.tokens)
        by_key[(key_tokens, r.scope)].append(r)
    for (tokens, scope), rs in sorted(by_key.items()):
        sigs = {r.emit_sig for r in rs}
        if len(sigs) > 1:
            sig_str = "; ".join(sorted(str(s) for s in sigs))
            errors.append(
                f"conflicting emits for tokens={list(tokens)} scope={scope}: {sig_str}"
            )

    # 3) short token boundary guard
    for r in leaves:
        if len(r.tokens) != 1:
            continue
        t = r.tokens[0].strip()
        tl = t.lower()
        if not SHORT_ALNUM.fullmatch(t):
            continue
        if r.scope != "plain":
            continue
        if tl in SHORT_GLOBAL_ALLOWLIST:
            continue
        errors.append(
            f"short token without scope: entry={r.entry_id} token={t!r} path={r.path}"
        )

    # 4) digit+curated-pack must not be lexicon remove:true
    for r in leaves:
        if len(r.tokens) != 1:
            continue
        tok = r.tokens[0].strip()
        if DIGIT_CURATED_PACK.fullmatch(tok) and r.remove:
            errors.append(
                f"digit-curated-pack token must not remove in lexicon: "
                f"entry={r.entry_id} token={tok!r} path={r.path}"
            )

    return errors, warnings


def main() -> int:
    ap = argparse.ArgumentParser(description="Lint lexicon boundary constraints")
    ap.add_argument(
        "--lexicon",
        default="configs/lexicon.yaml",
        help="path to lexicon yaml (default: configs/lexicon.yaml)",
    )
    args = ap.parse_args()

    path = Path(args.lexicon)
    if not path.exists():
        print(f"[ERROR] file not found: {path}", file=sys.stderr)
        return 2

    leaves, ids = collect_rules(path)
    errors, warnings = lint(leaves, ids)
    for w in warnings:
        print(f"[WARN] {w}")
    for e in errors:
        print(f"[ERROR] {e}")
    if errors:
        print(f"\nlexicon lint failed: {len(errors)} error(s)")
        return 1
    print("lexicon lint passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
