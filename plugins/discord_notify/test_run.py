"""Tests for discord_notify plugin."""
from __future__ import annotations

import json
import subprocess
import sys
import unittest
from typing import Any, Dict
from unittest.mock import MagicMock, patch

from run import (
    error_response,
    handle_command,
    handle_health,
    ok_response,
    pick,
    post_to_discord,
)

WEBHOOK_URL = "https://discord.com/api/webhooks/123/abc"
CONFIG = {"webhook_url": WEBHOOK_URL, "default_username": "Ductile"}


class TestPick(unittest.TestCase):
    def test_payload_wins_over_context(self):
        self.assertEqual(pick({"k": "a"}, {"k": "b"}, "k"), "a")

    def test_falls_back_to_context(self):
        self.assertEqual(pick({}, {"k": "ctx"}, "k"), "ctx")

    def test_skips_none(self):
        self.assertEqual(pick({"k": None}, {"k": "ctx"}, "k"), "ctx")

    def test_skips_empty_string(self):
        self.assertEqual(pick({"k": ""}, {"k": "ctx"}, "k"), "ctx")

    def test_multiple_keys(self):
        self.assertEqual(pick({"b": "val"}, {}, "a", "b"), "val")

    def test_default_returned(self):
        self.assertIsNone(pick({}, {}, "missing"))

    def test_custom_default(self):
        self.assertEqual(pick({}, {}, "missing", default="fallback"), "fallback")


class TestHandleHealth(unittest.TestCase):
    def test_ok_with_valid_url(self):
        r = handle_health(CONFIG)
        self.assertEqual(r["status"], "ok")

    def test_error_no_webhook(self):
        r = handle_health({})
        self.assertEqual(r["status"], "error")
        self.assertIn("webhook_url", r["error"])

    def test_error_bad_url(self):
        r = handle_health({"webhook_url": "http://example.com/not-discord"})
        self.assertEqual(r["status"], "error")
        self.assertIn("does not look like", r["error"])


class TestHandleCommand(unittest.TestCase):
    def _call(self, payload=None, context=None, config=None):
        return handle_command(
            config or CONFIG,
            payload or {},
            context or {},
        )

    def test_no_webhook_url(self):
        r = handle_command({}, {"message": "hi"}, {})
        self.assertEqual(r["status"], "error")
        self.assertIn("webhook_url", r["error"])
        self.assertFalse(r["retry"])

    def test_no_content(self):
        r = self._call(payload={})
        self.assertEqual(r["status"], "error")
        self.assertIn("No message content", r["error"])
        self.assertFalse(r["retry"])

    def test_message_field(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"message": "hello"})
        self.assertEqual(r["status"], "ok")
        mock_post.assert_called_once()
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["content"], "hello")

    def test_content_field_fallback(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"content": "from content"})
        self.assertEqual(r["status"], "ok")
        mock_post.assert_called_once()
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["content"], "from content")

    def test_title_and_message_combined(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"title": "Title", "message": "Body"})
        self.assertEqual(r["status"], "ok")
        _, args, _ = mock_post.mock_calls[0]
        self.assertIn("**Title**", args[1]["content"])
        self.assertIn("Body", args[1]["content"])

    def test_title_only(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"title": "Just a title"})
        self.assertEqual(r["status"], "ok")
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["content"], "Just a title")

    def test_context_fallback(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={}, context={"message": "from context"})
        self.assertEqual(r["status"], "ok")
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["content"], "from context")

    def test_truncation(self):
        long_msg = "x" * 3000
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"message": long_msg})
        self.assertEqual(r["status"], "ok")
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(len(args[1]["content"]), 2000)
        self.assertTrue(args[1]["content"].endswith("..."))

    def test_custom_username_from_payload(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"message": "hi", "username": "Bot"})
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["username"], "Bot")

    def test_default_username_from_config(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"message": "hi"})
        _, args, _ = mock_post.mock_calls[0]
        self.assertEqual(args[1]["username"], "Ductile")

    def test_avatar_url_included_when_set(self):
        cfg = {**CONFIG, "default_avatar_url": "https://example.com/avatar.png"}
        with patch("run.post_to_discord") as mock_post:
            r = handle_command(cfg, {"message": "hi"}, {})
        _, args, _ = mock_post.mock_calls[0]
        self.assertIn("avatar_url", args[1])

    def test_avatar_url_omitted_when_empty(self):
        with patch("run.post_to_discord") as mock_post:
            r = self._call(payload={"message": "hi"})
        _, args, _ = mock_post.mock_calls[0]
        self.assertNotIn("avatar_url", args[1])

    def test_http_error_no_retry_on_4xx(self):
        import urllib.error
        err = urllib.error.HTTPError(WEBHOOK_URL, 403, "Forbidden", {}, None)
        with patch("run.post_to_discord", side_effect=err):
            r = self._call(payload={"message": "hi"})
        self.assertEqual(r["status"], "error")
        self.assertFalse(r["retry"])

    def test_http_error_retry_on_5xx(self):
        import urllib.error
        err = urllib.error.HTTPError(WEBHOOK_URL, 503, "Unavailable", {}, None)
        with patch("run.post_to_discord", side_effect=err):
            r = self._call(payload={"message": "hi"})
        self.assertEqual(r["status"], "error")
        self.assertTrue(r["retry"])

    def test_url_error_retries(self):
        import urllib.error
        err = urllib.error.URLError("connection refused")
        with patch("run.post_to_discord", side_effect=err):
            r = self._call(payload={"message": "hi"})
        self.assertEqual(r["status"], "error")
        self.assertTrue(r["retry"])

    def test_generic_exception_retries(self):
        with patch("run.post_to_discord", side_effect=RuntimeError("boom")):
            r = self._call(payload={"message": "hi"})
        self.assertEqual(r["status"], "error")
        self.assertTrue(r["retry"])


class TestMainProtocol(unittest.TestCase):
    """Integration tests via subprocess — validates stdin/stdout protocol."""

    PLUGIN = __file__.replace("test_run.py", "run.py")

    def _run(self, request: Dict[str, Any]) -> Dict[str, Any]:
        result = subprocess.run(
            [sys.executable, self.PLUGIN],
            input=json.dumps(request),
            capture_output=True,
            text=True,
        )
        return json.loads(result.stdout)

    def test_invalid_json_exits_with_error(self):
        result = subprocess.run(
            [sys.executable, self.PLUGIN],
            input="not json",
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        out = json.loads(result.stdout)
        self.assertEqual(out["status"], "error")
        self.assertIn("Invalid JSON", out["error"])

    def test_unknown_command(self):
        r = self._run({"command": "bogus", "config": {}, "event": {}, "context": {}})
        self.assertEqual(r["status"], "error")
        self.assertIn("Unknown command", r["error"])

    def test_health_no_config(self):
        r = self._run({"command": "health", "config": {}, "event": {}, "context": {}})
        self.assertEqual(r["status"], "error")

    def test_health_ok(self):
        r = self._run({
            "command": "health",
            "config": {"webhook_url": WEBHOOK_URL},
            "event": {},
            "context": {},
        })
        self.assertEqual(r["status"], "ok")

    def test_handle_no_message(self):
        r = self._run({
            "command": "handle",
            "config": {"webhook_url": WEBHOOK_URL},
            "event": {"type": "test", "payload": {}},
            "context": {},
        })
        self.assertEqual(r["status"], "error")
        self.assertIn("No message content", r["error"])


if __name__ == "__main__":
    unittest.main()
