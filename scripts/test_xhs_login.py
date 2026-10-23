"""登录状态机回归；平台请求用固定响应代替，不发送短信。"""
import importlib.util
from pathlib import Path
import unittest
from types import SimpleNamespace

source = Path(__file__).resolve().parents[1] / "internal/app/xhs_login.py"
spec = importlib.util.spec_from_file_location("xhs_login", source)
login = importlib.util.module_from_spec(spec)


class Platform:
    def __init__(self):
        self.calls = []
        self.status = "请扫描二维码"
        self.guest = False

    def generate_init_cookies(self):
        self.calls.append("init")
        return {"a1": "private-a1", "web_session": "anonymous-session"}

    def generate_qrcode(self, cookies):
        self.calls.append("qr")
        return True, "成功", {"cookies": cookies, "qr_id": "id", "code": "secret-code", "qr_url": "https://www.xiaohongshu.com/qr/fixture"}

    def check_qrcode_status(self, qr_id, code, cookies):
        self.calls.append("poll")
        if self.status == "验证成功":
            cookies["web_session"] = "formal-session"
        return self.status == "验证成功", self.status, cookies

    def ensure_webprofile(self, cookies):
        self.calls.append("profile")

    def get_user_info(self, cookies):
        return True, {"guest": self.guest, "user_id": "5da907190000000001005c5d", "nickname": "测试用户"}, cookies

    def send_phone_code(self, phone, cookies, zone="86"):
        self.calls.append("sms")
        return True, "成功", {}

    def login_by_phone(self, phone, code, cookies, zone="86"):
        if code != "123456":
            return False, "验证码错误", {}
        cookies["web_session"] = "formal-session"
        return True, "成功", {"cookies": cookies}


class LoginRegression(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec.loader.exec_module(login)

    def setUp(self):
        self.api = Platform()
        self.session = login.LoginSession(self.api)

    def test_qr_requires_phone_confirmation_and_formal_account(self):
        status = self.session.handle({"operation": "qr_start"})
        self.assertEqual(self.api.calls, ["init", "qr", "poll", "profile"])
        self.assertEqual(status["state"], "waiting_scan")
        self.assertNotIn("cookies", status)
        self.api.status = "请确认登录"
        self.assertEqual(self.session.handle({"operation": "qr_poll"})["state"], "waiting_confirm")
        self.api.status = "验证成功"
        status = self.session.handle({"operation": "qr_poll"})
        self.assertEqual(status["state"], "success")
        self.assertIn("web_session\tformal-session", status["cookies"])

    def test_anonymous_cookie_is_never_accepted(self):
        self.session.handle({"operation": "qr_start"})
        self.api.status, self.api.guest = "验证成功", True
        with self.assertRaisesRegex(RuntimeError, "正式登录"):
            self.session.handle({"operation": "qr_poll"})

    def test_qr_expiry_and_unknown_status(self):
        self.session.handle({"operation": "qr_start"})
        self.api.status = "二维码已过期"
        self.assertEqual(self.session.handle({"operation": "qr_poll"})["state"], "expired")
        self.api.status = "二维码状态响应缺少 codeStatus"
        with self.assertRaises(RuntimeError):
            self.session.handle({"operation": "qr_poll"})

    def test_sms_is_bound_to_original_phone_and_retry_keeps_session(self):
        status = self.session.handle({"operation": "sms_send", "phone": "13800138000"})
        self.assertEqual(self.api.calls, ["init", "profile", "sms"])
        self.assertEqual(status["state"], "waiting_code")
        with self.assertRaisesRegex(ValueError, "手机号"):
            self.session.handle({"operation": "sms_login", "phone": "13900139000", "code": "123456"})
        with self.assertRaises(RuntimeError):
            self.session.handle({"operation": "sms_login", "phone": "13800138000", "code": "000000"})
        status = self.session.handle({"operation": "sms_login", "phone": "13800138000", "code": "123456"})
        self.assertEqual(status["state"], "success")

    def test_signer_receives_secrets_through_stdin(self):
        calls = []
        process = SimpleNamespace(run=lambda args, **kwargs: calls.append((args, kwargs)))
        runtime = SimpleNamespace(subprocess=process, _SIGN_JS="/fixture/sign.js")
        login.protect_signer(runtime)
        secret = '{"api":"/check?phone=13800138000&code=123456","cookie":"private-session"}'
        runtime.subprocess.run(["node", runtime._SIGN_JS, secret], text=True)
        args, options = calls[0]
        self.assertNotIn(secret, args)
        self.assertNotIn("123456", " ".join(args))
        self.assertEqual(options["input"], secret)


if __name__ == "__main__":
    unittest.main()
