"""Regression fixtures for the browser-free Spider_XHS adapter."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("xhs_api", Path(__file__).resolve().parents[1] / "internal/app/xhs_api.py")
api = importlib.util.module_from_spec(spec)
spec.loader.exec_module(api)


class SearchFixture:
    def search_user(self, query, page=1):
        return True, "成功", {"success": True, "code": 1000, "data": {"users": [
            {"id": "5da907190000000001005c5d", "name": "咬太阳Bite", "red_id": "875230756", "sub_title": "小红书号：875230756", "xsec_token": "fixture-token"},
            {"id": "5da907190000000001005c5e", "name": "相近账号", "red_id": "8752307560"},
        ], "has_more": False}}


class XHSAPIRegression(unittest.TestCase):
    def test_numeric_account_matches_exact_red_id(self):
        result = api.resolve_creator(SearchFixture(), "875230756")
        self.assertEqual(result["id"], "5da907190000000001005c5d")
        self.assertEqual(result["name"], "咬太阳Bite")
        self.assertEqual(result["token"], "fixture-token")

    def test_similar_account_is_not_silently_subscribed(self):
        with self.assertRaisesRegex(RuntimeError, "完全匹配"):
            api.resolve_creator(SearchFixture(), "87523075")

    def test_expired_or_foreign_cookie_does_not_authorize(self):
        with self.assertRaisesRegex(ValueError, "凭据已失效"):
            api.cookies_from_netscape(".evil.test\tTRUE\t/\tTRUE\t0\ta1\tforeign\n.xiaohongshu.com\tTRUE\t/\tTRUE\t1\tweb_session\texpired\n")

    def test_saved_http_only_cookie_is_read_in_memory(self):
        result = api.cookies_from_netscape(".xiaohongshu.com\ttrue\t/\ttrue\t0\ta1\tfixture\n#HttpOnly_.xiaohongshu.com\tTRUE\t/\tTRUE\t0\tweb_session\tsession\n")
        self.assertEqual(result, {"a1": "fixture", "web_session": "session"})


if __name__ == "__main__":
    unittest.main()
