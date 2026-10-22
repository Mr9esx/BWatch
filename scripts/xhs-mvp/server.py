#!/usr/bin/env python3
"""Local-only XHS archive MVP. Uses an unofficial third-party client adapter."""

import http.server
import json
import re
import shutil
import subprocess
import sys
import threading
import atexit
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.parse import parse_qs, quote, urlencode, urlparse


ROOT = Path(__file__).resolve().parents[2]
DATA_DIR = ROOT / ".tools" / "xhs-mvp"
OUTPUT_DIR = ROOT / "downloads" / "xhs-mvp"
COOKIE_FILE = DATA_DIR / "cookies.json"
COOKIE_JAR = DATA_DIR / "cookies.txt"
BROWSER_PROFILE_DIR = DATA_DIR / "browser-profile"
SPIDER_XHS_DIR = DATA_DIR / "spider-xhs"
PORT = 8769
MAX_SCAN_PAGES = 100

DATA_DIR.mkdir(parents=True, exist_ok=True)
OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

# xhs-reader uses its own per-user config directory. Redirect it into this ignored MVP area.
import xhs_cli.config as xhs_config

xhs_config.CONFIG_DIR = DATA_DIR
xhs_config.COOKIES_FILE = COOKIE_FILE
xhs_config.TOKEN_CACHE_FILE = DATA_DIR / "token_cache.json"

from xhs_cli.client import XhsClient, XhsError
from xhs_cli.api.user import get_self_info

if SPIDER_XHS_DIR.is_dir():
    sys.path.insert(0, str(SPIDER_XHS_DIR))
try:
    from apis.xhs_pc_apis import XHS_Apis
    from xhs_utils.xhs_pc.auth import XHSPcAuth
except ImportError:
    XHS_Apis = XHSPcAuth = None


LOCK = threading.RLock()
CLIENT = None
SPIDER_AUTH = None
SPIDER_API = None
LOGIN_STATE = "logged_out"
LOGIN_MESSAGE = "打开小红书浏览器窗口以登录"
LOGIN_WORKER = ThreadPoolExecutor(max_workers=1, thread_name_prefix="xhs-browser")
PLAYWRIGHT = None
BROWSER_CONTEXT = None
BROWSER_PAGE = None
NOTES = []
SCAN_STATE = {"status": "idle", "count": 0, "complete": False, "message": ""}
ARCHIVE_STATE = {"status": "idle", "done": 0, "total": 0, "current": "", "results": []}


def parse_profile_user_id(value):
    return parse_profile_reference(value)[0]


def parse_profile_reference(value):
    value = (value or "").strip()
    if re.fullmatch(r"[a-fA-F0-9]{8,64}", value):
        return value, "", ""
    parsed = urlparse(value)
    host = (parsed.hostname or "").lower()
    if parsed.scheme == "https" and host in ("xhslink.cn", "www.xhslink.cn", "xhslink.com", "www.xhslink.com"):
        import httpx
        try:
            with httpx.Client(follow_redirects=True, max_redirects=5, timeout=15) as resolver:
                response = resolver.get(value, headers={"User-Agent": "Mozilla/5.0"})
            for hop in list(response.history) + [response]:
                hop_url = str(hop.url)
                hop_parsed = urlparse(hop_url)
                hop_host = (hop_parsed.hostname or "").lower()
                if hop_parsed.scheme == "https" and (hop_host == "xiaohongshu.com" or hop_host.endswith(".xiaohongshu.com")):
                    match = re.fullmatch(r"/user/profile/([a-fA-F0-9]{8,64})/?", hop_parsed.path or "")
                    if match:
                        query = parse_qs(hop_parsed.query)
                        return (match.group(1), query.get("xsec_token", [""])[0],
                                query.get("xsec_source", [""])[0])
        except Exception:
            pass
        raise ValueError("短链没有跳转到可识别的小红书博主主页，请粘贴主页完整链接")
    if parsed.scheme != "https" or not (host == "xiaohongshu.com" or host.endswith(".xiaohongshu.com")):
        raise ValueError("请输入小红书博主主页地址")
    match = re.fullmatch(r"/user/profile/([a-fA-F0-9]{8,64})/?", parsed.path or "")
    if not match:
        raise ValueError("主页链接格式不正确，请使用 /user/profile/<博主ID> 地址")
    query = parse_qs(parsed.query)
    return (match.group(1), query.get("xsec_token", [""])[0],
            query.get("xsec_source", [""])[0])


def normalize_note(item):
    card = item.get("note_card") if isinstance(item.get("note_card"), dict) else item
    note_id = item.get("note_id") or item.get("id") or card.get("note_id") or card.get("id") or ""
    if not re.fullmatch(r"[a-zA-Z0-9_-]{4,80}", str(note_id)):
        return None
    title = card.get("display_title") or card.get("title") or "（无标题）"
    note_type = str(card.get("type") or item.get("type") or "normal").lower()
    token = item.get("xsec_token") or card.get("xsec_token") or ""
    source = item.get("xsec_source") or card.get("xsec_source") or "pc_user"
    return {
        "id": str(note_id),
        "title": str(title)[:200],
        "kind": "video" if note_type == "video" else "image_post",
        "created_at": card.get("time") or card.get("last_update_time") or "",
        "xsec_token": str(token),
        "xsec_source": str(source),
        "url": "https://www.xiaohongshu.com/explore/" + quote(str(note_id)),
    }


def restriction_category(code, message):
    text = (message or "").lower()
    if code in (461, 471) or "captcha" in text or "验证码" in text:
        return "captcha_required"
    if code == 429 or "rate limit" in text or "too many requests" in text or "http error 429" in text or "限流" in text:
        return "rate_limited"
    if code == 406 or "http error 406" in text:
        return "signature_rejected"
    if code == 403 or "http error 403" in text or "verification" in text or "验证" in text or "ip block" in text:
        return "verification_required"
    if code in (401, 23001) or "login" in text or "登录" in text:
        return "login_expired"
    if code == -2 or "network error" in text:
        return "network_error"
    if code == -3:
        return "browser_api_missing"
    return "platform_error"


def _write_cookie_jar(cookies):
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    lines = ["# Netscape HTTP Cookie File"]
    for name, value in cookies.items():
        if not name or not value or any(c in str(value) for c in "\r\n\t;"):
            continue
        lines.append(".xiaohongshu.com\tTRUE\t/\tTRUE\t0\t%s\t%s" % (name, value))
    COOKIE_JAR.write_text("\n".join(lines) + "\n", encoding="utf-8")
    COOKIE_JAR.chmod(0o600)


def _find_chrome_executable():
    candidates = [
        Path("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
        Path("/Applications/Chromium.app/Contents/MacOS/Chromium"),
    ]
    for candidate in candidates:
        if candidate.is_file():
            return str(candidate)
    for name in ("google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "msedge"):
        executable = shutil.which(name)
        if executable:
            return executable
    return None


def _start_browser_context():
    global PLAYWRIGHT, BROWSER_CONTEXT, BROWSER_PAGE
    if BROWSER_CONTEXT:
        if BROWSER_PAGE:
            BROWSER_PAGE.bring_to_front()
        return
    executable = _find_chrome_executable()
    if not executable:
        raise RuntimeError("没有找到 Chrome 或 Chromium。请安装后再打开小红书登录页。")
    from playwright.sync_api import sync_playwright

    BROWSER_PROFILE_DIR.mkdir(parents=True, exist_ok=True)
    PLAYWRIGHT = sync_playwright().start()
    try:
        BROWSER_CONTEXT = PLAYWRIGHT.chromium.launch_persistent_context(
            str(BROWSER_PROFILE_DIR),
            executable_path=executable,
            chromium_sandbox=True,
            headless=False,
            args=["--no-first-run"],
        )
    except Exception as error:
        PLAYWRIGHT.stop()
        PLAYWRIGHT = None
        raise RuntimeError("Chrome 启动失败。请关闭已占用 MVP 浏览器资料的窗口后重试。") from error
    pages = BROWSER_CONTEXT.pages
    BROWSER_PAGE = pages[0] if pages else BROWSER_CONTEXT.new_page()
    try:
        BROWSER_PAGE.goto("https://www.xiaohongshu.com/login", wait_until="domcontentloaded", timeout=30000)
    except Exception:
        # Keep the visible window open so the user can retry navigation or complete login manually.
        pass


def _browser_cookie_values():
    if not BROWSER_CONTEXT:
        raise ValueError("请先点击“打开小红书登录页”")
    allowed = []
    for cookie in BROWSER_CONTEXT.cookies([
        "https://www.xiaohongshu.com",
        "https://xiaohongshu.com",
    ]):
        domain = str(cookie.get("domain") or "").lstrip(".").lower()
        name, value = str(cookie.get("name") or ""), str(cookie.get("value") or "")
        if (domain == "xiaohongshu.com" or domain.endswith(".xiaohongshu.com")) and name and value:
            allowed.append((name, value))
    return dict(allowed)


def _close_browser_context(clear_profile=False):
    global PLAYWRIGHT, BROWSER_CONTEXT, BROWSER_PAGE
    try:
        if BROWSER_CONTEXT:
            BROWSER_CONTEXT.close()
    finally:
        try:
            if PLAYWRIGHT:
                PLAYWRIGHT.stop()
        finally:
            BROWSER_CONTEXT = BROWSER_PAGE = PLAYWRIGHT = None
            if clear_profile:
                shutil.rmtree(BROWSER_PROFILE_DIR, ignore_errors=True)


def start_browser_login():
    global LOGIN_STATE, LOGIN_MESSAGE
    with LOCK:
        LOGIN_WORKER.submit(_start_browser_context).result(timeout=45)
        LOGIN_STATE, LOGIN_MESSAGE = "browser_open", "请在打开的小红书窗口扫码登录并完成平台验证。完成后回到此页检查登录状态。"
        return {"state": LOGIN_STATE, "message": LOGIN_MESSAGE}


def check_browser_login():
    global CLIENT, LOGIN_STATE, LOGIN_MESSAGE
    with LOCK:
        cookies = LOGIN_WORKER.submit(_browser_cookie_values).result(timeout=20)
        if not cookies.get("a1") or not cookies.get("web_session"):
            LOGIN_STATE, LOGIN_MESSAGE = "browser_open", "浏览器中还没有完整登录态。请在小红书窗口完成登录/验证码后再检查。"
            return {"state": LOGIN_STATE, "message": LOGIN_MESSAGE}
        client = XhsClient()
        client._max_retries = 1
        client._request_delay = 1.5
        client._cookies = cookies
        try:
            user = get_self_info(client)
        except XhsError as error:
            category = restriction_category(error.code, error.message)
            LOGIN_STATE, LOGIN_MESSAGE = "failed", _friendly_error(category)
            return {"state": LOGIN_STATE, "message": LOGIN_MESSAGE}
        if user.get("guest"):
            LOGIN_STATE, LOGIN_MESSAGE = "browser_open", "网页会话尚未登录。请完成小红书登录后再检查。"
            return {"state": LOGIN_STATE, "message": LOGIN_MESSAGE}
        client.save_cookies(cookies)
        _write_cookie_jar(cookies)
        CLIENT = client
        _reset_spider_api()
        LOGIN_STATE = "success"
        LOGIN_MESSAGE = "已登录" + ("：" + str(user.get("nickname")) if user.get("nickname") else "")
        return {"state": LOGIN_STATE, "message": LOGIN_MESSAGE}


def stop_browser_login(clear_profile=False):
    global CLIENT, LOGIN_STATE, LOGIN_MESSAGE
    with LOCK:
        LOGIN_WORKER.submit(_close_browser_context, clear_profile).result(timeout=20)
        if CLIENT:
            CLIENT.clear_cookies()
            CLIENT.close()
        _reset_spider_api()
        COOKIE_JAR.unlink(missing_ok=True)
        CLIENT = None
        LOGIN_STATE, LOGIN_MESSAGE = "logged_out", "已退出登录"


def _reset_spider_api():
    global SPIDER_AUTH, SPIDER_API
    if SPIDER_AUTH is not None:
        try:
            SPIDER_AUTH.close()
        except Exception:
            pass
    SPIDER_AUTH = SPIDER_API = None


def _get_spider_api():
    global SPIDER_AUTH, SPIDER_API
    if XHSPcAuth is None or XHS_Apis is None:
        raise RuntimeError("Spider_XHS 适配器尚未安装，请通过 scripts/xhs-mvp/run.sh 启动")
    if LOGIN_STATE != "success" or CLIENT is None:
        raise XhsError(401, "登录已失效，请重新登录")
    if SPIDER_API is None:
        SPIDER_AUTH = XHSPcAuth.from_cookie(dict(CLIENT._cookies))
        SPIDER_API = XHS_Apis(SPIDER_AUTH)
    return SPIDER_API


def _shutdown_browser():
    try:
        LOGIN_WORKER.submit(_close_browser_context, False).result(timeout=5)
    except Exception:
        pass
    LOGIN_WORKER.shutdown(wait=False, cancel_futures=True)


atexit.register(_shutdown_browser)


def _public_note(note):
    return {key: note.get(key, "") for key in ("id", "title", "kind", "created_at", "url")}


def _read_user_notes(user_id, xsec_token="", xsec_source=""):
    """Read creator notes through Spider_XHS's signed PC API adapter."""
    api = _get_spider_api()
    collected, seen_ids = [], set()
    cursor = ""
    source = xsec_source or "pc_search"
    for page_index in range(MAX_SCAN_PAGES):
        success, message, result = api.get_user_note_info(user_id, cursor, xsec_token, source)
        if not success:
            code = result.get("code", -1) if isinstance(result, dict) else -1
            raise XhsError(code, str(message or "笔记列表请求失败"))
        data = result.get("data") if isinstance(result, dict) else None
        if not isinstance(data, dict):
            raise XhsError(-1, "笔记列表返回格式无效")
        for raw in data.get("notes") or []:
            normalized = normalize_note(raw)
            if normalized and normalized["id"] not in seen_ids:
                normalized["xsec_source"] = source
                if xsec_token and not normalized["xsec_token"]:
                    normalized["xsec_token"] = xsec_token
                seen_ids.add(normalized["id"])
                collected.append(normalized)
        next_cursor = str(data.get("cursor") or "")
        has_more = bool(data.get("has_more"))
        with LOCK:
            global NOTES, SCAN_STATE
            NOTES = list(collected)
            SCAN_STATE = {"status": "running", "count": len(collected), "complete": False,
                          "message": "已读取 %d 页" % (page_index + 1)}
        if not has_more or not next_cursor or next_cursor == cursor:
            return collected, True
        cursor = next_cursor
    return collected, False


def _scan_worker(user_id, xsec_token="", xsec_source=""):
    global NOTES, SCAN_STATE
    with LOCK:
        SCAN_STATE = {"status": "running", "count": 0, "complete": False, "message": "正在读取笔记列表"}
        NOTES = []
    collected = []
    try:
        collected, complete = _read_user_notes(user_id, xsec_token, xsec_source)
        with LOCK:
            NOTES = list(collected)
            SCAN_STATE = {"status": "completed" if complete else "partial",
                          "count": len(collected), "complete": complete,
                          "message": "列表读取完成" if complete else "已到 MVP 安全页数上限，当前数量为已读取数量"}
    except XhsError as error:
        category = restriction_category(error.code, error.message)
        print("[xhs-mvp] scan stopped: code=%s category=%s detail=%s" %
              (error.code, category, error.message[:240]), flush=True)
        with LOCK:
            SCAN_STATE = {"status": "stopped", "count": len(NOTES), "complete": False,
                          "error": category, "error_code": error.code,
                          "message": _friendly_error(category)}
    except Exception:
        with LOCK:
            SCAN_STATE = {"status": "stopped", "count": len(collected), "complete": False,
                          "error": "internal_error", "message": "读取失败，请查看本机服务终端"}


def _friendly_error(category):
    return {
        "captcha_required": "平台要求验证码，已停止读取。请稍后重试或通过小红书正常完成验证。",
        "rate_limited": "请求被限流，已停止读取。",
        "signature_rejected": "小红书拒绝了笔记列表请求签名（HTTP 406）；登录状态有效，但当前请求协议未被接受。",
        "browser_api_missing": "已登录浏览器没有收到笔记列表接口响应。",
        "verification_required": "小红书要求额外验证或拒绝了当前网络请求。",
        "login_expired": "登录已失效，请重新在小红书窗口登录。",
        "network_error": "本机到小红书的网络请求失败，请检查网络后重试。",
        "platform_error": "平台返回错误，已停止读取。",
    }.get(category, "平台请求失败，已停止读取。")


def scan_creator(value):
    global SCAN_STATE
    if LOGIN_STATE != "success":
        raise ValueError("请先扫码登录")
    user_id, xsec_token, xsec_source = parse_profile_reference(value)
    with LOCK:
        if SCAN_STATE.get("status") == "running":
            raise ValueError("已有列表读取任务在运行")
        SCAN_STATE = {"status": "running", "count": 0, "complete": False, "message": "正在启动"}
    threading.Thread(target=_scan_worker, args=(user_id, xsec_token, xsec_source), daemon=True).start()


def _safe_ext(url, content_type):
    lower = (url or "").lower().split("?", 1)[0]
    ext = Path(lower).suffix.lower()
    if ext in (".jpg", ".jpeg", ".png", ".webp", ".avif"):
        return ext
    if "png" in content_type:
        return ".png"
    if "webp" in content_type:
        return ".webp"
    if "avif" in content_type:
        return ".avif"
    return ".jpg"


def _get_note_detail(note):
    api = _get_spider_api()
    query = {"xsec_source": note.get("xsec_source") or "pc_user"}
    if note.get("xsec_token"):
        query["xsec_token"] = note["xsec_token"]
    url = note["url"] + "?" + urlencode(query)
    success, message, result = api.get_note_info(url)
    if not success:
        code = result.get("code", -1) if isinstance(result, dict) else -1
        raise XhsError(code, str(message or "笔记详情请求失败"))
    data = result.get("data") if isinstance(result, dict) else None
    items = data.get("items") if isinstance(data, dict) else None
    if not items or not isinstance(items[0], dict):
        raise XhsError(-1, "笔记详情未返回内容")
    return items[0].get("note_card") or items[0]


def _archive_note(note):
    import httpx

    detail = _get_note_detail(note)
    staging = OUTPUT_DIR / ("." + note["id"] + ".partial")
    destination = OUTPUT_DIR / note["id"]
    if staging.exists():
        shutil.rmtree(staging)
    staging.mkdir(parents=True, exist_ok=True)
    (staging / "meta.json").write_text(json.dumps(detail, ensure_ascii=False, indent=2), encoding="utf-8")
    title = str(detail.get("title") or note.get("title") or "")
    description = str(detail.get("desc") or "")
    (staging / "content.md").write_text(
        "# " + title.replace("\n", " ") + "\n\n" + description + "\n\n" + note["url"] + "\n",
        encoding="utf-8",
    )
    note_type = str(detail.get("type") or note.get("kind") or "normal").lower()
    if note_type == "video":
        if not COOKIE_JAR.exists():
            raise RuntimeError("缺少登录 Cookie 文件")
        media_url = note["url"]
        if note.get("xsec_token"):
            media_url += "?" + urlencode({"xsec_token": note["xsec_token"], "xsec_source": "pc_feed"})
        output_template = str(staging / "video.%(ext)s")
        command = [sys.executable, "-m", "yt_dlp", "--cookies", str(COOKIE_JAR),
                   "--retries", "0", "--fragment-retries", "0", "--socket-timeout", "20",
                   "--no-playlist", "--output", output_template, media_url]
        completed = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   universal_newlines=True, timeout=180, check=False)
        if completed.returncode != 0:
            category = restriction_category(0, completed.stderr)
            raise XhsError(461 if category == "captcha_required" else 429 if category == "rate_limited" else 0,
                           category)
        if not list(staging.glob("video.*")):
            raise RuntimeError("yt-dlp 未生成视频文件")
    else:
        image_list = detail.get("image_list") or []
        if not image_list:
            raise RuntimeError("图文笔记没有返回图片列表")
        if len(image_list) > 50:
            raise RuntimeError("图片数超过 MVP 单篇上限（50）")
        image_dir = staging / "images"
        image_dir.mkdir()
        for index, image in enumerate(image_list, 1):
            info_list = image.get("info_list") or []
            image_url = (info_list[-1].get("url") if info_list else "") or image.get("url_default") or ""
            image_host = (urlparse(image_url).hostname or "").lower()
            if not (image_host == "xhscdn.com" or image_host.endswith(".xhscdn.com")):
                raise RuntimeError("图片链接无效")
            if image_url.startswith("http://"):
                image_url = "https://" + image_url[len("http://"):]
            if not image_url.startswith("https://"):
                raise RuntimeError("图片链接协议无效")
            response = httpx.get(image_url, timeout=45, follow_redirects=True,
                                 headers={"Referer": "https://www.xiaohongshu.com/"})
            response.raise_for_status()
            ext = _safe_ext(image_url, response.headers.get("content-type", ""))
            (image_dir / ("%02d%s" % (index, ext))).write_bytes(response.content)
    if destination.exists():
        shutil.rmtree(destination)
    staging.replace(destination)
    return str(destination.relative_to(ROOT))


def _archive_worker(selected):
    global ARCHIVE_STATE
    results = []
    with LOCK:
        ARCHIVE_STATE = {"status": "running", "done": 0, "total": len(selected), "current": "", "results": []}
    for index, note in enumerate(selected):
        with LOCK:
            ARCHIVE_STATE["current"] = note["title"]
        try:
            path = _archive_note(note)
            result = {"id": note["id"], "title": note["title"], "status": "completed", "path": path}
        except XhsError as error:
            category = restriction_category(error.code, error.message)
            print("[xhs-mvp] archive stopped: code=%s category=%s detail=%s" %
                  (error.code, category, error.message[:240]), flush=True)
            result = {"id": note["id"], "title": note["title"], "status": "stopped", "error": category}
            shutil.rmtree(OUTPUT_DIR / ("." + note["id"] + ".partial"), ignore_errors=True)
            results.append(result)
            with LOCK:
                ARCHIVE_STATE = {"status": "stopped", "done": index, "total": len(selected),
                                 "current": note["title"], "results": list(results),
                                 "message": _friendly_error(category)}
            return
        except Exception as error:
            print("[xhs-mvp] archive failed: id=%s exception=%s" %
                  (note["id"], type(error).__name__), flush=True)
            result = {"id": note["id"], "title": note["title"], "status": "failed", "error": "archive_failed"}
            shutil.rmtree(OUTPUT_DIR / ("." + note["id"] + ".partial"), ignore_errors=True)
        results.append(result)
        with LOCK:
            ARCHIVE_STATE = {"status": "running", "done": index + 1, "total": len(selected),
                             "current": note["title"], "results": list(results)}
    with LOCK:
        ARCHIVE_STATE = {"status": "completed", "done": len(selected), "total": len(selected),
                         "current": "", "results": list(results)}


def start_archive(note_ids=None):
    with LOCK:
        if ARCHIVE_STATE.get("status") == "running":
            raise ValueError("已有归档任务在运行")
        available = {note["id"]: note for note in NOTES}
        selected = [available[nid] for nid in (note_ids or available.keys()) if nid in available]
        if not selected:
            raise ValueError("没有可归档的笔记")
    threading.Thread(target=_archive_worker, args=(selected,), daemon=True).start()


def snapshot():
    with LOCK:
        archive = dict(ARCHIVE_STATE)
        results = {result["id"]: result for result in archive.get("results", [])}
        for note in NOTES:
            if note["id"] not in results and (OUTPUT_DIR / note["id"] / "content.md").is_file():
                results[note["id"]] = {"id": note["id"], "title": note["title"],
                                       "status": "completed", "path": str((OUTPUT_DIR / note["id"]).relative_to(ROOT))}
        archive["results"] = list(results.values())
        if archive.get("status") != "running" and NOTES:
            archive["done"] = sum(1 for note in NOTES if results.get(note["id"], {}).get("status") == "completed")
            archive["total"] = len(NOTES)
            if archive["done"] == archive["total"]:
                archive["status"] = "completed"
        return {
            "login": {"state": LOGIN_STATE, "message": LOGIN_MESSAGE},
            "scan": dict(SCAN_STATE),
            "archive": archive,
            "notes": [_public_note(note) for note in NOTES],
        }


class Handler(http.server.BaseHTTPRequestHandler):
    server_version = "XhsMVP/0.1"

    def log_message(self, fmt, *args):
        # Avoid logging URLs, cookies, or platform error bodies.
        print("[xhs-mvp] " + (fmt % args).split("?")[0])

    def _send(self, status, body, content_type="application/json; charset=utf-8"):
        data = body if isinstance(body, bytes) else json.dumps(body, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(data)

    def _body(self):
        length = min(int(self.headers.get("Content-Length", "0")), 100000)
        return json.loads(self.rfile.read(length) or b"{}")

    def do_GET(self):
        if self.path == "/":
            page = Path(__file__).with_name("index.html").read_bytes()
            return self._send(200, page, "text/html; charset=utf-8")
        if self.path.startswith("/api/state"):
            return self._send(200, snapshot())
        return self._send(404, {"error": "not found"})

    def do_POST(self):
        global LOGIN_STATE, LOGIN_MESSAGE
        try:
            body = self._body()
            if self.path == "/api/login/start":
                result = start_browser_login()
                return self._send(200, result)
            if self.path == "/api/login/check":
                result = check_browser_login()
                return self._send(200, result)
            if self.path == "/api/login/logout":
                stop_browser_login(clear_profile=True)
                return self._send(200, {"state": LOGIN_STATE, "message": LOGIN_MESSAGE})
            if self.path == "/api/creator/scan":
                scan_creator(body.get("url", ""))
                return self._send(202, {"status": "running"})
            if self.path == "/api/archive":
                start_archive(body.get("ids"))
                return self._send(202, {"status": "running"})
            return self._send(404, {"error": "not found"})
        except ValueError as error:
            return self._send(400, {"error": str(error)})
        except XhsError as error:
            category = restriction_category(error.code, error.message)
            with LOCK:
                LOGIN_STATE, LOGIN_MESSAGE = ("failed", _friendly_error(category))
            return self._send(503, {"error": _friendly_error(category), "category": category})
        except RuntimeError as error:
            return self._send(503, {"error": str(error)})
        except Exception:
            return self._send(500, {"error": "请求失败，请查看本机服务终端"})


class LocalServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True


if __name__ == "__main__":
    print("XHS MVP 页面：http://127.0.0.1:%d" % PORT)
    print("会话文件位于 .tools/xhs-mvp/；归档文件位于 downloads/xhs-mvp/。")
    try:
        LocalServer(("127.0.0.1", PORT), Handler).serve_forever()
    finally:
        _shutdown_browser()
