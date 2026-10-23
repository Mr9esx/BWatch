"""内存中的 Spider_XHS 登录会话，通过私有管道与 Go 通信。"""
import contextlib
import io
import json
import re
import sys


def protect_signer(runtime):
    """固定版本的签名器接收 stdin，手机号、验证码和 Cookie 不进入 argv。"""
    original = runtime.subprocess
    script = runtime._SIGN_JS

    class PrivateSigner:
        def __getattr__(self, name):
            return getattr(original, name)

        def run(self, args, **options):
            if len(args) == 3 and args[:2] == ["node", script]:
                options["input"] = args[2]
                args = ["node", "-e", "const fs=require('fs');const s=require(process.argv[1]);process.stdout.write(JSON.stringify(s.signFull(JSON.parse(fs.readFileSync(0,'utf8'))))+'\\n');", script]
            return original.run(args, **options)

    runtime.subprocess = PrivateSigner()


class LoginError(RuntimeError, ValueError):
    pass


class LoginSession:
    def __init__(self, api):
        self.api = api
        self.cookies = {}
        self.qr = None
        self.phone = ""

    def authenticated(self):
        success, user, self.cookies = self.api.get_user_info(self.cookies)
        if not success or user.get("guest") is not False or not user.get("user_id") or not self.cookies.get("a1") or not self.cookies.get("web_session"):
            raise LoginError("未获得正式登录会话，请重新登录")
        lines = ["# Netscape HTTP Cookie File"]
        for name, value in self.cookies.items():
            if not name or any(c in str(name) + str(value) for c in "\r\n\t;"):
                continue
            lines.append(f".xiaohongshu.com\tTRUE\t/\tTRUE\t0\t{name}\t{value}")
        return {"state": "success", "message": "登录成功", "cookies": "\n".join(lines) + "\n"}

    def handle(self, request):
        operation = request["operation"]
        if operation in ("qr_start", "sms_send"):
            self.cookies = self.api.generate_init_cookies()
            if operation == "qr_start":
                success, _, self.qr = self.api.generate_qrcode(self.cookies)
                if not success or not self.qr:
                    raise LoginError("申请二维码失败，请稍后重试")
                self.cookies = self.qr["cookies"]
                success, message, self.cookies = self.api.check_qrcode_status(self.qr["qr_id"], self.qr["code"], self.cookies)
                if success or message != "请扫描二维码":
                    raise LoginError("二维码预检查失败，请重新生成")
                self.api.ensure_webprofile(self.cookies)
                return {"state": "waiting_scan", "message": "请使用小红书 App 扫码", "url": self.qr["qr_url"]}
            self.phone = request.get("phone", "")
            if not re.fullmatch(r"1[3-9]\d{9}", self.phone, flags=re.ASCII):
                raise LoginError("请输入有效的中国大陆手机号")
            self.api.ensure_webprofile(self.cookies)
            success, _, _ = self.api.send_phone_code(self.phone, self.cookies)
            if not success:
                raise LoginError("验证码发送失败；如需额外验证，请在小红书 App 完成后重试")
            return {"state": "waiting_code", "message": "验证码已发送，请输入短信验证码"}
        if operation == "qr_poll" and self.qr:
            success, message, self.cookies = self.api.check_qrcode_status(self.qr["qr_id"], self.qr["code"], self.cookies)
            if success:
                return self.authenticated()
            states = {"请扫描二维码": "waiting_scan", "请确认登录": "waiting_confirm", "二维码已过期": "expired"}
            if message not in states:
                raise LoginError("查询二维码状态失败；如需额外验证，请在小红书 App 完成后重试")
            return {"state": states[message], "message": message}
        if operation == "sms_login" and self.phone:
            if request.get("phone") != self.phone:
                raise LoginError("手机号已变更，请重新获取验证码")
            code = request.get("code", "")
            if not re.fullmatch(r"\d{6}", code, flags=re.ASCII):
                raise LoginError("请输入 6 位短信验证码")
            success, _, result = self.api.login_by_phone(self.phone, code, self.cookies)
            if not success:
                raise LoginError("验证码错误或已失效，请核对后重试")
            self.cookies = result["cookies"]
            return self.authenticated()
        raise LoginError("登录会话不存在，请重新登录")


def main():
    sys.path.insert(0, sys.argv[1])
    session = None
    api = None
    try:
        for line in sys.stdin:
            # 上游日志可能包含请求参数和局部变量，禁止进入应用日志。
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                try:
                    request = json.loads(line)
                    if session is None:
                        from loguru import logger
                        logger.remove()
                        from xhs_utils.xhs_pc import runtime
                        protect_signer(runtime)
                        from apis.xhs_pc_login_apis import XHSLoginApi
                        proxy = request.get("proxy") or ""
                        api = XHSLoginApi(proxies={"http": proxy, "https": proxy} if proxy else None)
                        session = LoginSession(api)
                    result = {"ok": True, "data": session.handle(request)}
                except LoginError as error:
                    result = {"ok": False, "error": str(error)}
                except Exception:
                    result = {"ok": False, "error": "小红书登录请求失败，请检查 API 运行环境与网络；如需验证请先在小红书 App 完成"}
            print(json.dumps(result, ensure_ascii=False, separators=(",", ":")), flush=True)
    finally:
        if api:
            api.close()


if __name__ == "__main__":
    main()
