import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from prometheus_client import generate_latest

_MAX_BODY = 4096


def make_admin(servicer, registry, port, host="0.0.0.0"):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):
            pass

        def _send(self, code, body=b"", content_type="text/plain"):
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.path == "/healthz":
                self._send(200, b"ok")
            elif self.path == "/metrics":
                self._send(200, generate_latest(registry))
            elif self.path == "/history":
                body = json.dumps(servicer.history()).encode()
                self._send(200, body, "application/json")
            elif self.path == "/behavior":
                body = json.dumps({"behavior": servicer.default_behavior()}).encode()
                self._send(200, body, "application/json")
            else:
                self._send(404, b"not found")

        def do_PUT(self):
            if self.path != "/behavior":
                self._send(404, b"not found")
                return
            length = min(int(self.headers.get("Content-Length") or 0), _MAX_BODY)
            raw = self.rfile.read(length).decode().strip().strip('"').strip()
            try:
                servicer.set_default_behavior(raw)
            except ValueError as exc:
                self._send(400, str(exc).encode())
                return
            self._send(200, b"")

        def do_POST(self):
            if self.path == "/reset":
                servicer.reset()
                self._send(200, b"")
            else:
                self._send(404, b"not found")

    return ThreadingHTTPServer((host, port), Handler)