#!/usr/bin/env python3
"""
Title Parser Web Demo v2 - 轻量测试页
- 仅运行 v2 Go parser（configs/）
- 输出完整 JSON（含 tokens/attributes/epslot 等）
- 可选开启 ONNX token 输出
"""
import json
import os
import subprocess
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse


_REPO_ROOT = Path(__file__).resolve().parents[1]
_MONOREPO_ROOT = _REPO_ROOT.parent


_XLMR_CACHE = {"model_dir": None, "tokenizer": None, "model": None, "device": None}


def _default_xlmr_model_dir() -> str:
    p = _MONOREPO_ROOT / "training" / "artifacts" / "xlmr_title_boundary_v1_unfreeze6_len192"
    return str(p)


def _infer_xlmr_labels(tokens: list[str], model_dir: str, max_length: int) -> tuple[list[str], str, float]:
    start = time.time()
    try:
        import torch  # type: ignore
        from transformers import AutoModelForTokenClassification, AutoTokenizer  # type: ignore
    except Exception as exc:
        return [], f"xlmr_import_error: {exc}", time.time() - start

    if not tokens:
        return [], "", time.time() - start

    model_dir = str(model_dir or "").strip()
    if not model_dir:
        return [], "xlmr_missing_model_dir", time.time() - start

    if _XLMR_CACHE.get("model_dir") != model_dir or _XLMR_CACHE.get("model") is None:
        tokenizer = AutoTokenizer.from_pretrained(model_dir, use_fast=True)
        model = AutoModelForTokenClassification.from_pretrained(model_dir)
        device = "mps" if torch.backends.mps.is_available() else ("cuda" if torch.cuda.is_available() else "cpu")
        model.to(device)
        model.eval()
        _XLMR_CACHE["model_dir"] = model_dir
        _XLMR_CACHE["tokenizer"] = tokenizer
        _XLMR_CACHE["model"] = model
        _XLMR_CACHE["device"] = device

    tokenizer = _XLMR_CACHE["tokenizer"]
    model = _XLMR_CACHE["model"]
    device = _XLMR_CACHE["device"]

    enc = tokenizer(tokens, is_split_into_words=True, truncation=True, max_length=max_length, return_tensors="pt")
    word_ids = enc.word_ids()
    enc = {k: v.to(device) for k, v in enc.items()}

    with torch.no_grad():
        out = model(**enc)
    logits = out.logits[0]
    pred_ids = logits.argmax(-1).tolist()

    id2label_raw = getattr(model.config, "id2label", {}) or {}
    id2label: dict[int, str] = {}
    for k, v in id2label_raw.items():
        try:
            id2label[int(k)] = str(v)
        except Exception:
            continue

    first_subword_pos: dict[int, int] = {}
    prev_wid = None
    for pos, wid in enumerate(word_ids):
        if wid is None:
            continue
        if wid == prev_wid:
            continue
        first_subword_pos[int(wid)] = pos
        prev_wid = wid

    out_labels: list[str] = []
    for tok_i in range(len(tokens)):
        pos = first_subword_pos.get(tok_i)
        if pos is None:
            out_labels.append("O")
            continue
        lab_id = int(pred_ids[pos])
        out_labels.append(id2label.get(lab_id, "O"))
    return out_labels, "", time.time() - start


def _looks_like_month_new_season(t: str) -> bool:
    t = str(t or "").strip()
    if not t.endswith("月新番"):
        return False
    p = t[: -len("月新番")]
    if not p or len(p) > 2:
        return False
    try:
        n = int(p)
    except Exception:
        return False
    return 1 <= n <= 12


def _span_from_attrs(attrs: list[dict], typ: str) -> tuple[int, int] | None:
    if not isinstance(attrs, list):
        return None
    min_s = 10**9
    max_e = -1
    for a in attrs:
        if not isinstance(a, dict):
            continue
        if str(a.get("type") or "") != typ:
            continue
        s = a.get("token_start")
        e = a.get("token_end")
        if not isinstance(s, int) or not isinstance(e, int):
            continue
        if e <= s:
            continue
        min_s = min(min_s, s)
        max_e = max(max_e, e)
    if min_s == 10**9 or max_e < 0:
        return None
    return min_s, max_e


def _first_hard_tech_after(attrs: list[dict], start: int) -> int | None:
    hard = {
        "source_type",
        "resolution",
        "video_codec",
        "audio_codec",
        "bit_depth",
        "container",
        "crc",
    }
    best = None
    for a in attrs or []:
        if not isinstance(a, dict):
            continue
        if str(a.get("type") or "") not in hard:
            continue
        s = a.get("token_start")
        e = a.get("token_end")
        if not isinstance(s, int) or not isinstance(e, int) or e <= s:
            continue
        if s >= start:
            if best is None or s < best:
                best = s
    return best


def _is_year_token(w: str) -> bool:
    w = str(w or "").strip()
    if not w:
        return False
    if not (len(w) == 4 and w.isdigit()):
        return False
    if not (w.startswith("19") or w.startswith("20")):
        return False
    try:
        y = int(w)
    except Exception:
        return False
    return 1900 <= y <= 2099


def _is_file_ext_token(w: str) -> bool:
    up = str(w or "").strip().upper()
    return up in {"MKV", "MP4", "AVI", "M2TS", "TS"}


def _is_meta_or_boundary_token(w: str) -> bool:
    if _is_year_token(w):
        return True
    if _is_file_ext_token(w):
        return True
    up = str(w or "").strip().upper()
    if up in {
        "MULTI",
        "TRUEFRENCH",
        "TRUEFRENCHSUBS",
        "FRENCH",
        "VF",
        "VFF",
        "VO",
        "VOST",
        "VOSTFR",
        "SUB",
        "SUBS",
        "SUBBED",
        "DUB",
        "DUBBED",
    }:
        return True
    if up in {"BDMV", "BLURAY", "BLU-RAY", "HDTV", "WEBDL", "WEB-DL", "WEBRIP", "DVDRIP", "BRRIP"}:
        return True
    if len(up) <= 4 and up.startswith("S") and up[1:].isdigit():
        return True
    return False


def _extend_title_end_right(tokens: list[dict], start: int, end: int, end_bound: int) -> int:
    if not tokens:
        return end
    n = len(tokens)
    start = max(0, min(start, n))
    end = max(0, min(end, n))
    end_bound = max(0, min(end_bound, n))
    if end > end_bound:
        end = end_bound
    if start >= end or end >= end_bound:
        return end

    last_good = end
    for i in range(end, end_bound):
        t = tokens[i] if isinstance(tokens[i], dict) else {}
        tx = str(t.get("text") or "").strip()
        if not tx:
            continue
        if _is_meta_or_boundary_token(tx):
            break
        if tx == ".":
            continue
        if tx in {"[", "]", "【", "】", "(", ")", "（", "）", "|", "｜"}:
            break
        last_good = i + 1
    return last_good


def _infer_title_span_from_labels_and_attrs(tokens: list[dict], attrs: list[dict], labels: list[str]) -> tuple[int, int] | None:
    if not tokens or not labels or len(tokens) != len(labels):
        return None

    title_idx: list[int] = []
    for i, lab in enumerate(labels):
        s = str(lab or "")
        if s == "O":
            continue
        if "TITLE" in s:
            title_idx.append(i)
    if not title_idx:
        return None

    start = min(title_idx)
    end = max(title_idx) + 1

    ep = _span_from_attrs(attrs, "episode")
    if ep is not None and ep[0] >= start and ep[0] > start:
        end = min(end, ep[0])
    epr = _span_from_attrs(attrs, "episode_range")
    if epr is not None and epr[0] >= start and epr[0] > start:
        end = min(end, epr[0])

    ht = _first_hard_tech_after(attrs, start)
    if ht is not None and ht > start:
        end = min(end, ht)

    yr = _span_from_attrs(attrs, "year")
    if yr is not None and yr[0] >= start and yr[0] > start:
        end = min(end, yr[0])

    sep = {"★", "☆"}
    while start < end:
        t = tokens[start] if isinstance(tokens[start], dict) else {}
        tx = str(t.get("text") or "").strip()
        if not tx or tx in sep or _looks_like_month_new_season(tx) or tx in {"[", "]", "【", "】", "(", ")", "（", "）", "|", "｜"}:
            start += 1
            continue
        break
    while end > start:
        t = tokens[end - 1] if isinstance(tokens[end - 1], dict) else {}
        tx = str(t.get("text") or "").strip()
        if not tx or tx in sep or tx in {"[", "]", "【", "】", "(", ")", "（", "）", "|", "｜"}:
            end -= 1
            continue
        break
    if end <= start:
        return None

    end_bound = len(tokens)
    for cand in [
        _span_from_attrs(attrs, "episode"),
        _span_from_attrs(attrs, "episode_range"),
        _span_from_attrs(attrs, "year"),
    ]:
        if cand is None:
            continue
        s = cand[0]
        if isinstance(s, int) and s > start:
            end_bound = min(end_bound, s)
    ht2 = _first_hard_tech_after(attrs, start)
    if ht2 is not None and ht2 > start:
        end_bound = min(end_bound, ht2)

    ext_end = _extend_title_end_right(tokens, start, end, end_bound)
    if ext_end > end:
        end = ext_end
    return start, end


_ALIAS_META_RE = (
    r"(?i)\b(?:"
    r"multi[-_ ]?subs?|multisub|multi[-_ ]?audio|dual[-_ ]?audio|english\s*dub|dub|"
    r"web[-_ ]?dl|webrip|web[-_ ]?rip|bdrip|dvdrip|hdr|10bit|8bit|"
    r"aac\d?(?:\.\d+)?|flac|opus|ac3|dts|truehd|"
    r"hevc|avc|x264|x265|h\.?264|h\.?265|"
    r"\d{3,4}p|\d{3,4}x\d{3,4}|"
    r"mp4|mkv|bdmv|"
    r"cr|amzn|nf|bilibili|iqiyi|abema|hidive"
    r")\b"
)


def _build_alias_prompt(raw_title: str, current_v2: dict) -> str:
    cur = json.dumps(current_v2, ensure_ascii=False, indent=2)
    return (
        "你是“动漫作品标题别名标准化”助手。你的任务：从 raw_title 中抽取并规范化 aliases（作品名别名）。\n\n"
        "重要规则：\n"
        "- 只输出“作品名/别名”（中文/日文/罗马字/英文均可）。\n"
        "- 必须排除任何发布信息与技术参数，即使它们在括号或中括号里：\n"
        "  分辨率/编码/音频/字幕/容器/来源/站点/平台/制作组尾缀/版本说明。\n"
        "  例如：1080p, 2160p, HEVC, AVC, x264, x265, H.264, AAC, FLAC, OPUS, MP4, MKV, BDMV,\n"
        "       WEB-DL, WEBRip, CR, AMZN, NF, Bilibili, IQIYI, HDR, 10bit,\n"
        "       简繁内封, 官译, Multi-Subs, Multi-Audio, Dual-Audio, Dub, v2, END 等。\n\n"
        "标准化要求：\n"
        "- 输出尽可能覆盖多语言别名（中文/日文/英文/罗马字）。\n"
        "- 如果存在 “/ | ｜” 分隔的多语言标题，把每段作为别名候选。\n"
        "- 对 “第二季 / Season 2 / S2”：\n"
        "  - 允许输出“含季号”的别名，也允许输出“去季号的基底别名”（两者都可输出）。\n"
        "- 去重：大小写不敏感去重；去掉首尾空格；不要空串；不要输出明显是 meta 的短词。\n\n"
        "输出必须是严格 JSON（不要 Markdown，不要解释）：\n"
        "{\n  \"aliases\": string[],\n  \"confidence\": number\n}\n\n"
        "raw_title:\n<<<" + raw_title + ">>>\n\n"
        "current_v2_output (for reference, may be incomplete):\n<<<" + cur + ">>>\n"
    )


def _extract_json_object(text: str) -> dict | None:
    s = (text or "").strip()
    if not s:
        return None
    if s.startswith("```"):
        s = s.replace("```json", "```")
        parts = s.split("```")
        if len(parts) >= 3:
            s = parts[1].strip()
    start = s.find("{")
    end = s.rfind("}")
    if start >= 0 and end > start:
        try:
            return json.loads(s[start : end + 1])
        except Exception:
            return None
    return None


def _ollama_generate(prompt: str) -> tuple[str, str]:
    host = _env("OLLAMA_HOST", "http://127.0.0.1:11434")
    model = _env("OLLAMA_MODEL", "qwen3:0.6b")
    url = host.rstrip("/") + "/api/generate"
    payload = {"model": model, "prompt": prompt, "stream": False}
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode("utf-8")
        obj = json.loads(raw)
        return str(obj.get("response") or ""), ""
    except Exception as exc:
        return "", f"ollama_error: {exc}"


def _filter_and_merge_aliases(existing: list[str], candidates: list[str]) -> list[str]:
    import re

    meta_re = re.compile(_ALIAS_META_RE)
    seen = {a.strip().lower(): a.strip() for a in existing if str(a).strip()}
    out = [seen[k] for k in list(seen.keys())]
    for a in candidates or []:
        a = str(a or "").strip()
        if not a:
            continue
        if len(a) < 2:
            continue
        if meta_re.search(a):
            continue
        key = a.lower()
        if key in seen:
            continue
        seen[key] = a
        out.append(a)
    return out


def _env(name: str, default: str) -> str:
    v = os.environ.get(name)
    return str(v).strip() if v is not None and str(v).strip() else default


def _run_parse(
    raw_title: str,
    with_onnx: bool = False,
    override_title_span: tuple[int, int] | None = None,
    override_episode: tuple[str, int] | None = None,
) -> dict:
    raw_title = str(raw_title or "").replace("\r", " ").replace("\n", " ").strip()
    if not raw_title:
        return {"error": "empty_raw_title"}

    cmd = ["go", "run", "./cmd/parse", "--", "-config", "configs", "-title", raw_title]
    if override_title_span is not None:
        cmd.extend(["-override_title_start", str(int(override_title_span[0]))])
        cmd.extend(["-override_title_end", str(int(override_title_span[1]))])
    if override_episode is not None:
        cmd.extend(["-override_episode", str(override_episode[0])])
        cmd.extend(["-override_episode_idx", str(int(override_episode[1]))])
    env = os.environ.copy()
    if with_onnx:
        env["TITLEPARSER_TOKEN_ONNX"] = "1"

    proc = subprocess.run(
        cmd,
        cwd=str(_REPO_ROOT),
        text=True,
        capture_output=True,
        env=env,
    )
    if proc.returncode != 0:
        return {"error": "parse_failed", "stderr": (proc.stderr or "")[:2000]}

    raw_stdout = proc.stdout.strip()
    if not raw_stdout:
        return {"error": "empty_output"}
    try:
        return json.loads(raw_stdout)
    except Exception:
        start = raw_stdout.find("{")
        end = raw_stdout.rfind("}")
        if start >= 0 and end > start:
            try:
                return json.loads(raw_stdout[start:end+1])
            except Exception as exc:
                return {"error": "bad_json", "detail": str(exc), "stdout": raw_stdout[:2000]}
        return {"error": "bad_json", "detail": "no_json_object", "stdout": raw_stdout[:2000]}


def _run_alias_llm(raw_title: str, current_v2: dict) -> tuple[list[str], float, str]:
    prompt = _build_alias_prompt(raw_title, current_v2)
    resp, err = _ollama_generate(prompt)
    if err:
        return [], 0.0, err
    obj = _extract_json_object(resp)
    if not obj:
        return [], 0.0, "bad_llm_json"
    aliases = obj.get("aliases")
    if not isinstance(aliases, list):
        return [], 0.0, "bad_llm_aliases"
    conf = obj.get("confidence")
    try:
        conf = float(conf)
    except Exception:
        conf = 0.0
    cleaned = []
    for a in aliases:
        if isinstance(a, str) and a.strip():
            cleaned.append(a.strip())
    return cleaned, conf, ""


class Handler(BaseHTTPRequestHandler):
    def _send(self, code: int, body: str, content_type: str = "text/html; charset=utf-8"):
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.end_headers()
        self.wfile.write(body.encode("utf-8"))

    def do_GET(self):
        query = parse_qs(urlparse(self.path).query)
        title = (query.get("title") or [""])[0]
        with_onnx = (query.get("onnx") or [""])[0] in ("1", "true", "yes")
        action = (query.get("action") or ["parse"])[0]
        result = None
        llm = None
        llm_conf = None
        llm_err = None
        merged_aliases = None
        xlmr_pack = None
        if title.strip():
            result = _run_parse(title, with_onnx=with_onnx)
            if action == "xlmr_parse" and isinstance(result, dict) and not result.get("error"):
                model_dir = _env("XLMR_MODEL_DIR", _default_xlmr_model_dir())
                max_length = int(_env("XLMR_MAX_LENGTH", "192"))
                toks = result.get("tokens") or []
                token_texts = [str(t.get("text") or "") for t in toks if isinstance(t, dict)]
                labels, xlmr_err, infer_s = _infer_xlmr_labels(token_texts, model_dir=model_dir, max_length=max_length)
                span = _infer_title_span_from_labels_and_attrs(toks, result.get("attributes") or [], labels)
                xlmr_res = None
                rerank_s = 0.0
                if span is not None:
                    t0 = time.time()
                    xlmr_res = _run_parse(title, with_onnx=with_onnx, override_title_span=span)
                    rerank_s = time.time() - t0
                xlmr_pack = {
                    "model_dir": model_dir,
                    "max_length": max_length,
                    "infer_s": infer_s,
                    "rerank_s": rerank_s,
                    "error": xlmr_err,
                    "labels": labels,
                    "override_title_span": span,
                    "result": xlmr_res,
                }
            if action == "ollama_alias" and isinstance(result, dict) and not result.get("error"):
                llm, llm_conf, llm_err = _run_alias_llm(title, result)
                merged_aliases = _filter_and_merge_aliases(result.get("aliases") or [], llm)
        body = render_page(title, with_onnx, result, action, llm, llm_conf, llm_err, merged_aliases, xlmr_pack)
        self._send(200, body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length).decode("utf-8")
        data = parse_qs(raw)
        title = (data.get("title") or [""])[0]
        with_onnx = (data.get("onnx") or [""])[0] in ("1", "true", "yes", "on")
        action = (data.get("action") or ["parse"])[0]
        result = _run_parse(title, with_onnx=with_onnx)
        llm = None
        llm_conf = None
        llm_err = None
        merged_aliases = None
        xlmr_pack = None
        if action == "xlmr_parse" and isinstance(result, dict) and not result.get("error"):
            model_dir = _env("XLMR_MODEL_DIR", _default_xlmr_model_dir())
            max_length = int(_env("XLMR_MAX_LENGTH", "192"))
            toks = result.get("tokens") or []
            token_texts = [str(t.get("text") or "") for t in toks if isinstance(t, dict)]
            labels, xlmr_err, infer_s = _infer_xlmr_labels(token_texts, model_dir=model_dir, max_length=max_length)
            span = _infer_title_span_from_labels_and_attrs(toks, result.get("attributes") or [], labels)
            xlmr_res = None
            rerank_s = 0.0
            if span is not None:
                t0 = time.time()
                xlmr_res = _run_parse(title, with_onnx=with_onnx, override_title_span=span)
                rerank_s = time.time() - t0
            xlmr_pack = {
                "model_dir": model_dir,
                "max_length": max_length,
                "infer_s": infer_s,
                "rerank_s": rerank_s,
                "error": xlmr_err,
                "labels": labels,
                "override_title_span": span,
                "result": xlmr_res,
            }
        if action == "ollama_alias" and isinstance(result, dict) and not result.get("error"):
            llm, llm_conf, llm_err = _run_alias_llm(title, result)
            merged_aliases = _filter_and_merge_aliases(result.get("aliases") or [], llm)
        body = render_page(title, with_onnx, result, action, llm, llm_conf, llm_err, merged_aliases, xlmr_pack)
        self._send(200, body)


def render_page(
    title: str,
    with_onnx: bool,
    result: dict | None,
    action: str,
    llm_aliases: list[str] | None,
    llm_conf: float | None,
    llm_err: str | None,
    merged_aliases: list[str] | None,
    xlmr_pack: dict | None,
) -> str:
    checked = "checked" if with_onnx else ""

    summary = ""
    tokens_html = ""
    attrs_html = ""
    aliases_html = ""
    llm_html = ""
    raw_html = ""
    onnx_html = ""


    xlmr_html = ""
    if isinstance(xlmr_pack, dict) and xlmr_pack is not None:
        xlmr_res = xlmr_pack.get("result")
        corrected_cards = ""
        if isinstance(xlmr_res, dict) and not xlmr_res.get("error"):
            group2 = escape_html(str(xlmr_res.get("group") or ""))
            main_title2 = escape_html(str(xlmr_res.get("title") or ""))
            episode2 = escape_html(str(xlmr_res.get("episode") or ""))
            episode_range2 = escape_html(str(xlmr_res.get("episode_range") or ""))
            season2 = escape_html(str(xlmr_res.get("season") or ""))
            ep_route2 = escape_html(str(xlmr_res.get("ep_route") or ""))
            content_kind2 = escape_html(str(xlmr_res.get("content_kind") or ""))
            template_id2 = escape_html(str(xlmr_res.get("template_id") or ""))
            tail_paren_block2 = escape_html(str(xlmr_res.get("tail_paren_block") or ""))

            epslot_primary2 = None
            epslot2 = xlmr_res.get("epslot")
            if isinstance(epslot2, dict):
                p2 = epslot2.get("primary")
                if isinstance(p2, dict):
                    epslot_primary2 = p2

            audio_tracks2 = None
            audio_slot2 = xlmr_res.get("audio_slot")
            if isinstance(audio_slot2, dict):
                t2 = audio_slot2.get("tracks")
                if isinstance(t2, list):
                    audio_tracks2 = [x for x in t2 if isinstance(x, dict)]

            slots_html2 = ""
            if epslot_primary2 is not None or audio_tracks2 is not None:
                slots_html2 = (
                    "<div class='card'><div class='card-title'>Slots</div>"
                    + "<div class='kv'>"
                    + "<div>epslot.primary</div><div>" + escape_html(json.dumps(epslot_primary2 or {}, ensure_ascii=False)) + "</div>"
                    + "<div>audio_slot.tracks</div><div>" + escape_html(json.dumps(audio_tracks2 or [], ensure_ascii=False)) + "</div>"
                    + "</div></div>"
                )

            summary2 = "".join(
                [
                    "<div class='grid'>",
                    f"<div class='card'><div class='card-title'>Corrected Core</div><div class='kv'><div>group</div><div>{group2}</div></div><div class='kv'><div>title</div><div>{main_title2}</div></div><div class='kv'><div>season</div><div>{season2}</div></div><div class='kv'><div>episode</div><div>{episode2}</div></div><div class='kv'><div>episode_range</div><div>{episode_range2}</div></div></div>",
                    f"<div class='card'><div class='card-title'>Corrected Routing</div><div class='kv'><div>ep_route</div><div>{ep_route2}</div></div><div class='kv'><div>content_kind</div><div>{content_kind2}</div></div><div class='kv'><div>template_id</div><div>{template_id2}</div></div><div class='kv'><div>tail_paren_block</div><div>{tail_paren_block2}</div></div></div>",
                    slots_html2,
                    "</div>",
                ]
            )

            tokens2 = xlmr_res.get("tokens") or []
            tokens_html2 = ""
            if isinstance(tokens2, list):
                token_texts2 = []
                for t in tokens2:
                    if isinstance(t, dict) and "text" in t:
                        token_texts2.append(str(t.get("text") or ""))
                tokens_html2 = (
                    "<div class='card'><div class='card-title'>Corrected Tokens</div>"
                    + "<div class='tokens'>"
                    + "".join([f"<span class='tok'>{escape_html(x)}</span>" for x in token_texts2 if x.strip()])
                    + "</div></div>"
                )

            attrs2 = xlmr_res.get("attributes") or []
            attrs_html2 = ""
            if isinstance(attrs2, list):
                rendered2 = []
                for a in attrs2:
                    if not isinstance(a, dict):
                        continue
                    t = str(a.get("type") or "").strip()
                    n = str(a.get("normalized") or "").strip()
                    if not t:
                        continue
                    label = t if not n else f"{t}:{n}"
                    rendered2.append(label)
                attrs_html2 = (
                    "<div class='card'><div class='card-title'>Corrected Attributes</div>"
                    + "<div class='chips'>"
                    + "".join([f"<div class='chip'>{escape_html(x)}</div>" for x in rendered2])
                    + "</div></div>"
                )

            raw_html2 = (
                "<div class='card'><div class='card-title'>Corrected Raw JSON</div>"
                + "<details><summary class='muted'>show/hide</summary>"
                + "<pre>"
                + escape_html(json.dumps(xlmr_res, ensure_ascii=False, indent=2))
                + "</pre></details></div>"
            )

            corrected_cards = "".join(
                [
                    summary2,
                    "<div class='grid'>",
                    tokens_html2,
                    attrs_html2,
                    "</div>",
                    "<div class='grid'>",
                    raw_html2,
                    "</div>",
                ]
            )

        xlmr_html = (
            "<div class='card'><div class='card-title'>XLM-R Rerank</div>"
            + "<div class='kv'>"
            + "<div>model_dir</div><div>" + escape_html(str(xlmr_pack.get("model_dir") or "")) + "</div>"
            + "<div>max_length</div><div>" + escape_html(str(xlmr_pack.get("max_length") or "")) + "</div>"
            + "<div>infer_s</div><div>" + escape_html(f"{float(xlmr_pack.get('infer_s') or 0.0):.3f}") + "</div>"
            + "<div>rerank_s</div><div>" + escape_html(f"{float(xlmr_pack.get('rerank_s') or 0.0):.3f}") + "</div>"
            + "<div>error</div><div>" + escape_html(str(xlmr_pack.get("error") or "")) + "</div>"
            + "<div>override_title_span</div><div>" + escape_html(json.dumps(xlmr_pack.get("override_title_span"), ensure_ascii=False)) + "</div>"
            + "</div>"
            + "<details><summary class='muted'>labels show/hide</summary><pre>"
            + escape_html(json.dumps(xlmr_pack.get("labels") or [], ensure_ascii=False, indent=2))
            + "</pre></details>"
            + "</div>"
            + corrected_cards
        )

    if isinstance(result, dict) and result is not None:
        if result.get("error"):
            summary = f"<div class='card error'><div class='card-title'>Error</div><pre>{escape_html(json.dumps(result, ensure_ascii=False, indent=2))}</pre></div>"
        else:
            group = escape_html(str(result.get("group") or ""))
            main_title = escape_html(str(result.get("title") or ""))
            episode = escape_html(str(result.get("episode") or ""))
            episode_range = escape_html(str(result.get("episode_range") or ""))
            season = escape_html(str(result.get("season") or ""))
            ep_route = escape_html(str(result.get("ep_route") or ""))
            content_kind = escape_html(str(result.get("content_kind") or ""))
            template_id = escape_html(str(result.get("template_id") or ""))
            tail_paren_block = escape_html(str(result.get("tail_paren_block") or ""))

            epslot_primary = None
            epslot = result.get("epslot")
            if isinstance(epslot, dict):
                p = epslot.get("primary")
                if isinstance(p, dict):
                    epslot_primary = p

            audio_tracks = None
            audio_slot = result.get("audio_slot")
            if isinstance(audio_slot, dict):
                t = audio_slot.get("tracks")
                if isinstance(t, list):
                    audio_tracks = [x for x in t if isinstance(x, dict)]

            slots_html = ""
            if epslot_primary is not None or audio_tracks is not None:
                slots_html = (
                    "<div class='card'><div class='card-title'>Slots</div>"
                    + "<div class='kv'>"
                    + "<div>epslot.primary</div><div>" + escape_html(json.dumps(epslot_primary or {}, ensure_ascii=False)) + "</div>"
                    + "<div>audio_slot.tracks</div><div>" + escape_html(json.dumps(audio_tracks or [], ensure_ascii=False)) + "</div>"
                    + "</div></div>"
                )

            summary = "".join(
                [
                    "<div class='grid'>",
                    f"<div class='card'><div class='card-title'>Core</div><div class='kv'><div>group</div><div>{group}</div></div><div class='kv'><div>title</div><div>{main_title}</div></div><div class='kv'><div>season</div><div>{season}</div></div><div class='kv'><div>episode</div><div>{episode}</div></div><div class='kv'><div>episode_range</div><div>{episode_range}</div></div></div>",
                    f"<div class='card'><div class='card-title'>Routing</div><div class='kv'><div>ep_route</div><div>{ep_route}</div></div><div class='kv'><div>content_kind</div><div>{content_kind}</div></div><div class='kv'><div>template_id</div><div>{template_id}</div></div><div class='kv'><div>tail_paren_block</div><div>{tail_paren_block}</div></div></div>",
                    slots_html,
                    "</div>",
                ]
            )

            aliases = result.get("aliases") or []
            if not isinstance(aliases, list):
                aliases = []
            merged = merged_aliases if isinstance(merged_aliases, list) else aliases

            def render_list(items: list[str]) -> str:
                if not items:
                    return "<div class='muted'>(empty)</div>"
                lis = "".join([f"<div class='chip'>{escape_html(str(x))}</div>" for x in items])
                return f"<div class='chips'>{lis}</div>"

            aliases_html = (
                "<div class='card'><div class='card-title'>Aliases</div>"
                + "<div class='sub-title'>v2 aliases</div>"
                + render_list([str(x) for x in aliases if str(x).strip()])
                + "<div class='sub-title'>merged aliases</div>"
                + render_list([str(x) for x in merged if str(x).strip()])
                + "</div>"
            )

            if llm_err:
                llm_html = (
                    "<div class='card error'><div class='card-title'>LLM Aliases</div>"
                    + f"<div class='muted'>{escape_html(str(llm_err))}</div>"
                    + "</div>"
                )
            elif llm_aliases is not None:
                c = "" if llm_conf is None else f"{llm_conf:.2f}"
                llm_html = (
                    "<div class='card'><div class='card-title'>LLM Aliases</div>"
                    + f"<div class='muted'>confidence={escape_html(c)}</div>"
                    + render_list([str(x) for x in llm_aliases if str(x).strip()])
                    + "</div>"
                )

            tokens = result.get("tokens") or []
            if isinstance(tokens, list):
                token_texts = []
                for t in tokens:
                    if isinstance(t, dict) and "text" in t:
                        token_texts.append(str(t.get("text") or ""))
                tokens_html = (
                    "<div class='card'><div class='card-title'>Tokens</div>"
                    + "<div class='tokens'>"
                    + "".join([f"<span class='tok'>{escape_html(x)}</span>" for x in token_texts if x.strip()])
                    + "</div></div>"
                )

            attrs = result.get("attributes") or []
            if isinstance(attrs, list):
                rendered = []
                for a in attrs:
                    if not isinstance(a, dict):
                        continue
                    t = str(a.get("type") or "").strip()
                    n = str(a.get("normalized") or "").strip()
                    if not t:
                        continue
                    label = t if not n else f"{t}:{n}"
                    rendered.append(label)
                attrs_html = (
                    "<div class='card'><div class='card-title'>Attributes</div>"
                    + "<div class='chips'>"
                    + "".join([f"<div class='chip'>{escape_html(x)}</div>" for x in rendered])
                    + "</div></div>"
                )

            token_onnx = result.get("token_onnx")
            if isinstance(token_onnx, dict):
                onnx_html = (
                    "<div class='card'><div class='card-title'>token_onnx</div>"
                    + "<details><summary class='muted'>show/hide</summary>"
                    + "<pre>"
                    + escape_html(json.dumps(token_onnx, ensure_ascii=False, indent=2))
                    + "</pre></details></div>"
                )

            raw_html = (
                "<div class='card'><div class='card-title'>Raw JSON</div>"
                + "<details><summary class='muted'>show/hide</summary>"
                + "<pre>"
                + escape_html(json.dumps(result, ensure_ascii=False, indent=2))
                + "</pre></details></div>"
            )

    result_html = "".join(
        [
            summary,
            "<div class='grid'>",
            aliases_html,
            llm_html,
            "</div>",
            "<div class='grid'>",
            tokens_html,
            attrs_html,
            "</div>",
            "<div class='grid'>",
            onnx_html,
            raw_html,
            "</div>",
        ]
    )

    return f"""
<!doctype html>
<html lang=\"zh\">
<head>
  <meta charset=\"utf-8\" />
  <title>Title Parser v2 Demo</title>
  <style>
    body {{ font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial; padding: 24px; background: #0b0f1a; color: #e6e8ef; }}
    h1 {{ font-size: 18px; margin: 0 0 16px 0; }}
    .wrap {{ max-width: 1100px; margin: 0 auto; }}
    .card {{ background: rgba(255,255,255,0.06); border: 1px solid rgba(255,255,255,0.10); border-radius: 12px; padding: 14px; margin: 12px 0; }}
    .card-title {{ font-weight: 600; margin-bottom: 10px; }}
    .sub-title {{ margin-top: 10px; margin-bottom: 6px; font-size: 12px; opacity: 0.9; }}
    .grid {{ display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }}
    @media (max-width: 900px) {{ .grid {{ grid-template-columns: 1fr; }} }}
    textarea {{ width: 100%; height: 110px; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; padding: 10px; border-radius: 10px; border: 1px solid rgba(255,255,255,0.12); background: rgba(0,0,0,0.35); color: #e6e8ef; }}
    button {{ padding: 8px 12px; font-size: 13px; border-radius: 10px; border: 1px solid rgba(255,255,255,0.14); background: rgba(255,255,255,0.08); color: #e6e8ef; cursor: pointer; }}
    button.primary {{ background: rgba(59,130,246,0.35); border-color: rgba(59,130,246,0.6); }}
    button.warn {{ background: rgba(245,158,11,0.20); border-color: rgba(245,158,11,0.45); }}
    label {{ display: inline-flex; align-items: center; gap: 8px; margin-top: 8px; font-size: 13px; }}
    pre {{ background: rgba(0,0,0,0.45); color: #e6e8ef; padding: 12px; overflow: auto; border-radius: 10px; border: 1px solid rgba(255,255,255,0.10); }}
    .kv {{ display: grid; grid-template-columns: 130px 1fr; gap: 6px 10px; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12.5px; }}
    .kv > div:nth-child(odd) {{ color: rgba(230,232,239,0.75); }}
    .muted {{ color: rgba(230,232,239,0.72); font-size: 12px; }}
    .chips {{ display: flex; flex-wrap: wrap; gap: 8px; }}
    .chip {{ font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; padding: 6px 8px; border-radius: 999px; background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.10); }}
    .tokens {{ display: flex; flex-wrap: wrap; gap: 6px; }}
    .tok {{ font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; padding: 4px 6px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.10); }}
    .error {{ border-color: rgba(239,68,68,0.55); background: rgba(239,68,68,0.10); }}
    details > summary {{ cursor: pointer; }}
    .btn-row {{ display: flex; gap: 10px; flex-wrap: wrap; margin-top: 10px; }}
  </style>
</head>
<body>
  <div class='wrap'>
    <h1>Title Parser v2 Demo</h1>
    <div class='card'>
      <form method=\"post\">
        <textarea name=\"title\" placeholder=\"paste a raw title here\">{escape_html(title)}</textarea>
        <div>
          <label><input type=\"checkbox\" name=\"onnx\" value=\"1\" {checked}/> 输出 token_onnx</label>
        </div>
        <div class='btn-row'>
          <button class='primary' type=\"submit\" name=\"action\" value=\"parse\">Parse</button>
          <button class='primary' type=\"submit\" name=\"action\" value=\"xlmr_parse\">Parse + XLM-R</button>
          <button class='warn' type=\"submit\" name=\"action\" value=\"ollama_alias\">LLM 生成别名 (Ollama)</button>
        </div>
        <div class='muted' style='margin-top:10px;'>action={escape_html(action)} | XLMR_MODEL_DIR={escape_html(_env("XLMR_MODEL_DIR", _default_xlmr_model_dir()))} | OLLAMA_HOST={escape_html(_env("OLLAMA_HOST","http://127.0.0.1:11434"))} | OLLAMA_MODEL={escape_html(_env("OLLAMA_MODEL","qwen3:0.6b"))}</div>
      </form>
    </div>
    {result_html}
    {xlmr_html}
  </div>
</body>
</html>
"""


def escape_html(s: str) -> str:
    return (
        s.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


if __name__ == "__main__":
    host = _env("HOST", "127.0.0.1")
    port = int(_env("PORT", "8767"))
    print(f"🎬 Title Parser v2 running at http://{host}:{port}")
    HTTPServer((host, port), Handler).serve_forever()
