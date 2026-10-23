"""Private stdin/stdout bridge to the pinned Spider_XHS HTTP adapter.

Credentials stay in memory: they are never placed in argv, files, or logs.
No browser or browser profile is used by this adapter.
"""
import contextlib
import io
import json
import sys
import time


def cookies_from_netscape(text):
    cookies = {}
    for line in text.splitlines():
        if line.startswith("#HttpOnly_"):
            line = line[len("#HttpOnly_"):]
        if not line or line.startswith("#"):
            continue
        fields = line.split("\t")
        if len(fields) != 7:
            continue
        domain, _, _, _, expires, name, value = fields
        if domain.lstrip(".") not in ("xiaohongshu.com", "www.xiaohongshu.com", "edith.xiaohongshu.com"):
            continue
        if float(expires) > 0 and float(expires) <= time.time():
            continue
        cookies[name] = value
    if not cookies.get("a1") or not cookies.get("web_session"):
        raise ValueError("小红书登录凭据已失效，请重新扫码登录")
    return cookies


def accepted(result):
    success, message, payload = result
    if not success or not isinstance(payload, dict):
        code = payload.get("code", "unknown") if isinstance(payload, dict) else "unknown"
        raise RuntimeError(f"小红书接口拒绝请求（code {code}）：{message or '未返回有效数据'}")
    return payload


def resolve_creator(api, red_id):
    payload = accepted(api.search_user(red_id, page=1))
    matches = {}
    users = (payload.get("data") or {}).get("users") or []
    for index, item in enumerate(users):
        user = item.get("user_info") or item
        uid = user.get("user_id") or user.get("id") or item.get("user_id") or item.get("id")
        if not uid:
            continue
        actual_id = user.get("red_id") or user.get("redId") or ""
        basic = user
        # Some search versions omit red_id. Confirm the public profile rather
        # than silently subscribing to the first similarly named search result.
        if not actual_id and index < 3:
            profile = accepted(api.get_user_info(str(uid)))
            basic = (profile.get("data") or {}).get("basic_info") or {}
            actual_id = basic.get("red_id") or basic.get("redId") or ""
        if str(actual_id).lower() != red_id.lower():
            continue
        matches[str(uid)] = {
            "id": str(uid),
            "redID": str(actual_id),
            "name": basic.get("nickname") or user.get("nickname") or user.get("nick_name") or user.get("name") or "",
            "avatar": basic.get("imageb") or basic.get("images") or user.get("image") or "",
            "token": item.get("xsec_token") or user.get("xsec_token") or "",
        }
    if len(matches) != 1:
        raise RuntimeError("未找到小红书号完全匹配的创作者，请核对小红书号或使用主页链接")
    return next(iter(matches.values()))


def run(request):
    from apis.xhs_pc_apis import XHS_Apis
    from xhs_utils.xhs_pc import XHSPcAuth

    cookies = cookies_from_netscape(request["cookies"])
    proxy = request.get("proxy") or ""
    proxies = {"http": proxy, "https": proxy} if proxy else None
    auth = XHSPcAuth.from_cookie(cookies, proxies=proxies)
    try:
        api = XHS_Apis(auth)
        operation = request["operation"]
        if operation == "account":
            user = accepted(api.get_user_me()).get("data") or {}
            uid = user.get("user_id") or user.get("id") or ""
            if not uid or user.get("guest") is True:
                raise RuntimeError("小红书未返回当前登录用户的资料，请重新登录后刷新")
            name = user.get("nickname") or user.get("nick_name") or ""
            avatar = user.get("imageb") or user.get("images") or user.get("image") or ""
            red_id = user.get("red_id") or ""
            if not name or not isinstance(avatar, str) or not avatar:
                basic = (accepted(api.get_user_info(str(uid))).get("data") or {}).get("basic_info") or {}
                name = basic.get("nickname") or name
                avatar = basic.get("imageb") or basic.get("images") or avatar
                red_id = basic.get("red_id") or red_id
            if not name or not isinstance(avatar, str):
                raise RuntimeError("小红书账号资料格式变化，请稍后刷新")
            return {"id": str(uid), "redID": str(red_id), "name": str(name), "avatar": avatar}
        if operation == "resolve_creator":
            return resolve_creator(api, request["redID"])
        if operation in ("note_detail", "note_publication"):
            note_id = request["noteID"]
            payload = accepted(api.get_note_info(request["url"]))
            items = (payload.get("data") or {}).get("items") or []
            matches = [item for item in items if str(item.get("id") or (item.get("note_card") or {}).get("note_id") or "") == note_id]
            if len(matches) != 1:
                raise RuntimeError("笔记详情未返回对应笔记，已停止归档")
            item = matches[0]
            card = item.get("note_card") or {}
            if operation == "note_publication":
                return {"time": card.get("time") or 0}
            if card.get("type") != "normal" or not isinstance(card.get("image_list"), list) or not isinstance(card.get("desc"), str):
                raise RuntimeError("笔记详情缺少完整的图片列表或正文，已停止归档")
            return item
        if operation == "scan":
            cursor = ""
            seen = set()
            notes = []
            for _ in range(100):
                payload = accepted(api.get_user_note_info(request["userID"], cursor, request.get("token", ""), request.get("source") or "pc_user"))
                data = payload.get("data") or {}
                page = data.get("notes") or []
                notes.extend(page)
                if data.get("has_more") is False:
                    return {"notes": notes, "complete": True}
                following = str(data.get("cursor") or "")
                if not page or not following or following == cursor or following in seen:
                    raise RuntimeError("小红书分页没有继续前进，当前结果不完整")
                seen.add(following)
                cursor = following
                time.sleep(1.5)
            raise RuntimeError("小红书笔记超过 100 页，本次读取未完成")
        raise ValueError("不支持的小红书 API 操作")
    finally:
        auth.close()


def main():
    request = json.load(sys.stdin)
    sys.path.insert(0, sys.argv[1])
    # Upstream exception logging includes stack locals. Suppress it completely.
    from loguru import logger
    logger.remove()
    output = None
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
        try:
            output = {"ok": True, "data": run(request)}
        except Exception as error:
            message = str(error)
            try:
                for value in cookies_from_netscape(request.get("cookies", "")).values():
                    if len(value) > 4:
                        message = message.replace(value, "[redacted]")
            except Exception:
                pass
            output = {"ok": False, "error": message[:500]}
    print(json.dumps(output, ensure_ascii=False, separators=(",", ":")))


if __name__ == "__main__":
    main()
