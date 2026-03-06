#!/usr/bin/env python3
"""
Title Parser Web Demo v2.5 (pure parser)
- Runs Go parser only (no ML/ONNX/XLM-R path)
- Displays canonical ParseResult JSON
"""

import html
import json
import subprocess
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse


REPO_ROOT = Path(__file__).resolve().parents[1]


def run_parse(title: str) -> tuple[dict, str]:
    cmd = [
        "go",
        "run",
        "./cmd/parse",
        "--",
        "-config",
        "configs",
        "-title",
        title,
    ]
    try:
        p = subprocess.run(
            cmd,
            cwd=str(REPO_ROOT),
            capture_output=True,
            text=True,
            timeout=20,
            check=False,
        )
    except Exception as exc:
        return {}, f"run_error: {exc}"

    if p.returncode != 0:
        err = (p.stderr or p.stdout or "parse_failed").strip()
        return {}, err

    out = (p.stdout or "").strip()
    if not out:
        return {}, "empty_output"

    try:
        return json.loads(out), ""
    except Exception as exc:
        return {}, f"json_decode_error: {exc}"


def render_page(title: str, result: dict, err: str) -> bytes:
    title_esc = html.escape(title or "")
    err_html = ""
    if err:
        err_html = (
            "<div class='card error'><div class='card-title'>Error</div>"
            f"<pre>{html.escape(err)}</pre></div>"
        )

    res_html = ""
    if result:
        res_html = (
            "<div class='card'><div class='card-title'>ParseResult (canonical)</div>"
            f"<pre>{html.escape(json.dumps(result, ensure_ascii=False, indent=2))}</pre></div>"
        )

    body = f"""<!doctype html>
<html>
<head>
  <meta charset=\"utf-8\" />
  <title>Title Parser v2.5 Demo</title>
  <style>
    body {{ font-family: -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif; margin: 24px; }}
    h1 {{ margin: 0 0 16px; }}
    form {{ display: grid; gap: 12px; max-width: 1100px; }}
    textarea {{ width: 100%; min-height: 120px; padding: 10px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }}
    button {{ width: 140px; height: 36px; border: 0; border-radius: 8px; background: #111; color: #fff; cursor: pointer; }}
    .muted {{ color: #666; margin-bottom: 16px; }}
    .card {{ margin-top: 16px; border: 1px solid #ddd; border-radius: 10px; padding: 12px; }}
    .card-title {{ font-weight: 700; margin-bottom: 8px; }}
    pre {{ white-space: pre-wrap; word-break: break-word; margin: 0; }}
    .error {{ border-color: #f1a3a3; background: #fff6f6; }}
  </style>
</head>
<body>
  <h1>Title Parser v2.5 Demo</h1>
  <div class=\"muted\">Pure parser path, canonical output only. No ML/ONNX/XLM-R.</div>
  <form method=\"post\">
    <textarea name=\"title\" placeholder=\"Paste one release title...\">{title_esc}</textarea>
    <button type=\"submit\">Parse</button>
  </form>
  {err_html}
  {res_html}
</body>
</html>"""
    return body.encode("utf-8")


class Handler(BaseHTTPRequestHandler):
    def _send_html(self, payload: bytes) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self) -> None:
        _ = parse_qs(urlparse(self.path).query)
        self._send_html(render_page("", {}, ""))

    def do_POST(self) -> None:
        n = int(self.headers.get("Content-Length", "0") or "0")
        raw = self.rfile.read(n).decode("utf-8", errors="ignore")
        data = parse_qs(raw)
        title = (data.get("title") or [""])[0].strip()

        result, err = ({}, "")
        if title:
            result, err = run_parse(title)

        self._send_html(render_page(title, result, err))


def main() -> None:
    host = "127.0.0.1"
    port = 18080
    srv = HTTPServer((host, port), Handler)
    print(f"serving: http://{host}:{port}")
    srv.serve_forever()


if __name__ == "__main__":
    main()
