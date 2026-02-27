import json
import os
import subprocess
import sys
import unittest


PLUGIN_PATH = os.path.join(os.path.dirname(__file__), "run.py")


def run_plugin(request):
    result = subprocess.run(
        [sys.executable, PLUGIN_PATH],
        input=json.dumps(request),
        text=True,
        capture_output=True,
        check=True,
    )
    return json.loads(result.stdout)


def handle_request(config, payload):
    return run_plugin(
        {
            "protocol": 2,
            "command": "handle",
            "config": config,
            "event": {"payload": payload},
        }
    )


class IfClassifierTests(unittest.TestCase):
    def test_contains(self):
        resp = handle_request(
            {"field": "text", "checks": [{"contains": "youtu", "emit": "hit"}]},
            {"text": "YouTu.Be link"},
        )
        self.assertEqual(resp["status"], "ok")
        self.assertEqual(resp["events"][0]["type"], "hit")

    def test_startswith(self):
        resp = handle_request(
            {"field": "text", "checks": [{"startswith": "http", "emit": "url"}]},
            {"text": "HTTP://example.com"},
        )
        self.assertEqual(resp["events"][0]["type"], "url")

    def test_endswith(self):
        resp = handle_request(
            {"field": "text", "checks": [{"endswith": ".pdf", "emit": "doc"}]},
            {"text": "Report.PDF"},
        )
        self.assertEqual(resp["events"][0]["type"], "doc")

    def test_equals(self):
        resp = handle_request(
            {"field": "status", "checks": [{"equals": "error", "emit": "fail"}]},
            {"status": "ERROR"},
        )
        self.assertEqual(resp["events"][0]["type"], "fail")

    def test_regex(self):
        resp = handle_request(
            {"field": "code", "checks": [{"regex": "^[0-9]{3}$", "emit": "ok"}]},
            {"code": "123"},
        )
        self.assertEqual(resp["events"][0]["type"], "ok")

    def test_default_fallback(self):
        resp = handle_request(
            {
                "field": "text",
                "checks": [
                    {"contains": "youtube", "emit": "yt"},
                    {"default": "fallback"},
                ],
            },
            {"text": "something else"},
        )
        self.assertEqual(resp["events"][0]["type"], "fallback")

    def test_missing_field_treated_as_empty(self):
        resp = handle_request(
            {
                "field": "missing",
                "checks": [
                    {"contains": "x", "emit": "hit"},
                    {"default": "empty"},
                ],
            },
            {"text": "value"},
        )
        self.assertEqual(resp["events"][0]["type"], "empty")

    def test_no_match_error(self):
        resp = handle_request(
            {"field": "text", "checks": [{"contains": "x", "emit": "hit"}]},
            {"text": "nope"},
        )
        self.assertEqual(resp["status"], "error")

    def test_dot_notation(self):
        resp = handle_request(
            {"field": "source.text", "checks": [{"equals": "hello", "emit": "ok"}]},
            {"source": {"text": "Hello"}},
        )
        self.assertEqual(resp["events"][0]["type"], "ok")

    def test_health_invalid(self):
        resp = run_plugin({"protocol": 2, "command": "health", "config": {}})
        self.assertEqual(resp["status"], "error")


if __name__ == "__main__":
    unittest.main()
