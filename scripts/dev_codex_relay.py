#!/usr/bin/env python3

import argparse
import http.server
import socketserver
import sys
import urllib.error
import urllib.request


HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "host",
    "content-length",
}


class ThreadingHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True


def build_handler(target_base: str):
    class RelayHandler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_GET(self):
            self._proxy()

        def do_POST(self):
            self._proxy()

        def do_HEAD(self):
            self._proxy()

        def log_message(self, fmt, *args):
            sys.stderr.write("%s - - [%s] %s\n" % (
                self.client_address[0],
                self.log_date_time_string(),
                fmt % args,
            ))

        def _proxy(self):
            target = target_base.rstrip("/") + self.path
            length = int(self.headers.get("Content-Length", "0") or "0")
            body = self.rfile.read(length) if length > 0 else None

            req = urllib.request.Request(target, data=body, method=self.command)
            for key, value in self.headers.items():
                if key.lower() in HOP_BY_HOP_HEADERS:
                    continue
                req.add_header(key, value)

            try:
                with urllib.request.urlopen(req, timeout=600) as resp:
                    self.send_response(resp.status)
                    for key, value in resp.headers.items():
                        if key.lower() in HOP_BY_HOP_HEADERS:
                            continue
                        self.send_header(key, value)
                    self.end_headers()

                    if self.command == "HEAD":
                        return

                    while True:
                        chunk = resp.read(8192)
                        if not chunk:
                            break
                        self.wfile.write(chunk)
                        self.wfile.flush()
            except urllib.error.HTTPError as err:
                self.send_response(err.code)
                for key, value in err.headers.items():
                    if key.lower() in HOP_BY_HOP_HEADERS:
                        continue
                    self.send_header(key, value)
                self.end_headers()
                if self.command != "HEAD":
                    data = err.read()
                    if data:
                        self.wfile.write(data)
                        self.wfile.flush()
            except Exception as err:  # noqa: BLE001
                data = str(err).encode("utf-8", "replace")
                self.send_response(502)
                self.send_header("Content-Type", "text/plain; charset=utf-8")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                if self.command != "HEAD":
                    self.wfile.write(data)
                    self.wfile.flush()

    return RelayHandler


def main():
    parser = argparse.ArgumentParser(description="Dev relay for Codex-compatible Responses backends.")
    parser.add_argument("--listen", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=19091)
    parser.add_argument("--target-base", default="https://www.autodl.art/api/v1")
    args = parser.parse_args()

    handler = build_handler(args.target_base)
    server = ThreadingHTTPServer((args.listen, args.port), handler)
    print(f"Relay listening on http://{args.listen}:{args.port} -> {args.target_base}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
