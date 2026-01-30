#!/usr/bin/env python3

import json
import os
import subprocess
import errno
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse


_REPO_ROOT = Path(__file__).resolve().parents[1]


def _norm_space(s: object) -> str:
    return " ".join(str(s or "").strip().split())


def _run_parse(raw_title: str) -> dict:
    raw_title = str(raw_title or "").replace("\r", " ").replace("\n", " ").strip()
    if not raw_title:
        return {"error": "empty_raw_title"}

    cli_bin = str(os.environ.get("TITLEPARSER_PARSE_BIN") or "").strip()
    if cli_bin:
        cmd = [cli_bin, "-config", "configs", "-title", raw_title]
        cwd = None
    else:
        cmd = ["go", "run", "./cmd/parse", "--", "-config", "configs", "-title", raw_title]
        cwd = str(_REPO_ROOT)

    proc = subprocess.run(cmd, cwd=cwd, text=True, capture_output=True)
    if proc.returncode != 0:
        return {"error": "parse_failed", "stderr": (proc.stderr or "")[:2000]}

    raw_stdout = (proc.stdout or "").strip()
    if not raw_stdout:
        return {"error": "empty_output"}

    try:
        return json.loads(raw_stdout)
    except Exception:
        start = raw_stdout.find("{")
        end = raw_stdout.rfind("}")
        if start >= 0 and end > start:
            try:
                return json.loads(raw_stdout[start : end + 1])
            except Exception as exc:
                return {"error": "bad_json", "detail": str(exc), "stdout": raw_stdout[:2000]}
        return {"error": "bad_json", "detail": "no_json_object", "stdout": raw_stdout[:2000]}


def _build_query_text(parse_result: dict, raw_title: str) -> str:
    title = _norm_space(parse_result.get("title"))
    aliases = parse_result.get("aliases")
    if not isinstance(aliases, list):
        aliases = []
    aliases = [_norm_space(x) for x in aliases if _norm_space(x)]

    alias_text = " | ".join(aliases[:3])
    if title and alias_text:
        return _norm_space(f"{title} | {alias_text}")
    if title:
        return title
    if alias_text:
        return alias_text
    return _norm_space(raw_title)


def _html_page(last_title: str, last_query_text: str) -> str:
    t = last_title or ""
    qt = last_query_text or ""
    return (
        "<!doctype html>"
        "<html><head><meta charset='utf-8'/>"
        "<title>Title QueryText v2</title>"
        "<style>body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial;max-width:880px;margin:24px auto;padding:0 16px}"
        "textarea{width:100%;min-height:72px;font-size:14px;padding:10px}"
        "button{padding:10px 14px;font-size:14px}"
        "pre{white-space:pre-wrap;word-break:break-word;background:#111827;color:#e5e7eb;padding:12px;border-radius:8px}"
        "</style></head><body>"
        "<form method='POST' action='/'>"
        "<textarea name='title' placeholder='Paste raw title...'>"
        + html_escape(t)
        + "</textarea>"
        "<div style='margin-top:12px'><button type='submit'>Build query_text</button></div>"
        "</form>"
        "<div style='margin-top:16px'>"
        "<pre>"
        + html_escape(qt)
        + "</pre>"
        "</div>"
        "</body></html>"
    )


def html_escape(s: str) -> str:
    return (
        str(s)
        .replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
        .replace("'", "&#39;")
    )


class Handler(BaseHTTPRequestHandler):
    def _send(self, code: int, body: str, content_type: str) -> None:
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.end_headers()
        self.wfile.write(body.encode("utf-8"))

    def _read_body(self) -> str:
        length = int(self.headers.get("Content-Length", "0"))
        if length <= 0:
            return ""
        return self.rfile.read(length).decode("utf-8", errors="replace")

    def do_GET(self) -> None:
        u = urlparse(self.path)
        if u.path == "/api/querytext":
            q = parse_qs(u.query)
            title = (q.get("title") or [""])[0]
            res = _run_parse(title)
            if isinstance(res, dict) and res.get("error"):
                self._send(400, "", "text/plain; charset=utf-8")
                return
            qt = _build_query_text(res, title)
            self._send(200, qt + "\n", "text/plain; charset=utf-8")
            return

        q = parse_qs(u.query)
        title = (q.get("title") or [""])[0]
        qt = ""
        if str(title).strip():
            res = _run_parse(title)
            if isinstance(res, dict) and not res.get("error"):
                qt = _build_query_text(res, title)
        self._send(200, _html_page(title, qt), "text/html; charset=utf-8")

    def do_POST(self) -> None:
        u = urlparse(self.path)
        if u.path == "/api/querytext":
            raw = self._read_body()
            title = ""
            ct = str(self.headers.get("Content-Type") or "").split(";")[0].strip().lower()
            if ct == "application/json":
                try:
                    obj = json.loads(raw or "{}")
                    title = str(obj.get("title") or "")
                except Exception:
                    title = ""
            else:
                data = parse_qs(raw)
                title = (data.get("title") or [""])[0]

            res = _run_parse(title)
            if isinstance(res, dict) and res.get("error"):
                self._send(400, "", "text/plain; charset=utf-8")
                return
            qt = _build_query_text(res, title)
            self._send(200, qt + "\n", "text/plain; charset=utf-8")
            return

        raw = self._read_body()
        data = parse_qs(raw)
        title = (data.get("title") or [""])[0]
        qt = ""
        res = _run_parse(title)
        if isinstance(res, dict) and not res.get("error"):
            qt = _build_query_text(res, title)
        self._send(200, _html_page(title, qt), "text/html; charset=utf-8")


def main() -> None:
    host = str(os.environ.get("HOST") or "127.0.0.1").strip() or "127.0.0.1"
    port = int(str(os.environ.get("PORT") or "8768").strip() or "8768")

    httpd = None
    last_err: OSError | None = None
    for p in range(port, port + 20):
        try:
            httpd = HTTPServer((host, p), Handler)
            port = p
            last_err = None
            break
        except OSError as e:
            last_err = e
            if getattr(e, "errno", None) in {errno.EADDRINUSE, 48}:
                continue
            raise

    if httpd is None:
        raise last_err or OSError("failed_to_bind")

    print(f"Title QueryText v2 running at http://{host}:{port}")
    httpd.serve_forever()


if __name__ == "__main__":
    main()
