import unittest

from server import normalize_note, parse_profile_user_id, restriction_category, _public_note


class XhsMvpParsingTests(unittest.TestCase):
    def test_profile_url_is_normalized_to_user_id(self):
        self.assertEqual(
            parse_profile_user_id("https://www.xiaohongshu.com/user/profile/661234abcd?xsec_token=secret"),
            "661234abcd",
        )
        self.assertEqual(parse_profile_user_id("661234abcd"), "661234abcd")

    def test_rejects_non_profile_urls(self):
        with self.assertRaises(ValueError):
            parse_profile_user_id("https://www.xiaohongshu.com/explore/note-id")
        with self.assertRaises(ValueError):
            parse_profile_user_id("https://www.xiaohongshu.com.evil.test/user/profile/id")

    def test_public_note_serialization_keeps_auth_token_private(self):
        note = normalize_note({
            "id": "note1",
            "xsec_token": "private-token",
            "note_card": {"display_title": "测试视频", "type": "video", "user": {"nickname": "作者"}},
        })
        self.assertEqual(note["id"], "note1")
        self.assertEqual(note["kind"], "video")
        self.assertNotIn("private-token", repr(_public_note(note)))

    def test_restriction_errors_stop_without_exposing_raw_details(self):
        self.assertEqual(restriction_category(461, "Captcha required"), "captcha_required")
        self.assertEqual(restriction_category(429, "Rate limited"), "rate_limited")
        self.assertEqual(restriction_category(406, "verification required"), "verification_required")


if __name__ == "__main__":
    unittest.main()
