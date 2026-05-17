import json
import os
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer


PLUGIN_PATH = os.path.join(os.path.dirname(__file__), "run.py")


class _HostileHeaderHandler(BaseHTTPRequestHandler):
    """Returns a normal body but a non-numeric x-markdown-tokens header."""

    def do_GET(self):
        body = b"<html><body>hello</body></html>"
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.send_header("x-markdown-tokens", "not-a-number")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


def run_plugin(request):
    # check=False on purpose: pre-fix the plugin crashes with no protocol
    # response; we assert it instead produces a bounded protocol message.
    result = subprocess.run(
        [sys.executable, PLUGIN_PATH],
        input=json.dumps(request),
        text=True,
        capture_output=True,
        check=False,
    )
    if not result.stdout.strip():
        raise AssertionError(
            f"plugin produced no protocol response (exit={result.returncode}, "
            f"stderr={result.stderr!r})"
        )
    return json.loads(result.stdout)


class FetchHostileMarkdownTokensTests(unittest.TestCase):
    """Reproduces C-FRO-10: a non-numeric remote x-markdown-tokens header
    hit int() outside the try that handles ValueError, crashing the plugin
    with no protocol response. A hostile upstream header must yield a
    bounded protocol response."""

    def setUp(self):
        self.server = HTTPServer(("127.0.0.1", 0), _HostileHeaderHandler)
        self.port = self.server.server_address[1]
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def test_non_numeric_markdown_tokens_is_bounded_error(self):
        resp = run_plugin(
            {
                "protocol": 2,
                "command": "handle",
                "config": {"output_format": "html"},
                "event": {"payload": {"url": f"http://127.0.0.1:{self.port}/"}},
            }
        )
        self.assertEqual(resp["status"], "error")
        self.assertIn("error", resp)


if __name__ == "__main__":
    unittest.main()
