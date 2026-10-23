"""Shared SQLite-backed token bucket for app-originated XHS API requests."""

from __future__ import annotations

import os
import sqlite3
import time
from urllib.parse import urlparse


def is_xhs_api_url(raw_url: str) -> bool:
    try:
        parsed = urlparse(raw_url)
    except ValueError:
        return False
    host = (parsed.hostname or "").lower()
    if host != "xiaohongshu.com" and not host.endswith(".xiaohongshu.com"):
        return False
    path = parsed.path.lower()
    return host.startswith(("api.", "edith.", "as.", "passport.")) or path.startswith(("/api/", "/web_api/", "/app-api/"))


def acquire_xhs_token(db_path: str | None = None) -> None:
    path = db_path or os.environ.get("BILIFETCH_XHS_RATE_DB", "")
    if not path:
        raise RuntimeError("小红书 API 限流数据库路径未配置")
    while True:
        connection = sqlite3.connect(path, timeout=30, isolation_level=None)
        try:
            connection.execute("PRAGMA busy_timeout=30000")
            connection.execute("BEGIN IMMEDIATE")
            row = connection.execute("SELECT tokens, updated_at, qps FROM xhs_api_bucket WHERE id=1").fetchone()
            if row is None:
                raise RuntimeError("小红书 API 令牌桶未初始化")
            tokens, updated_at, qps = map(float, row)
            if qps <= 0:
                raise RuntimeError("小红书 API QPS 配置无效")
            now = time.time()
            tokens = min(1.0, tokens + max(0.0, now - updated_at) * qps)
            wait = 0.0
            if tokens >= 1.0:
                tokens -= 1.0
            else:
                wait = (1.0 - tokens) / qps
            connection.execute("UPDATE xhs_api_bucket SET tokens=?, updated_at=? WHERE id=1", (tokens, now))
            connection.execute("COMMIT")
        except Exception:
            if connection.in_transaction:
                connection.execute("ROLLBACK")
            raise
        finally:
            connection.close()
        if wait <= 0:
            return
        time.sleep(wait)


def install_browser_http_limiter() -> None:
    from xhs_utils.xhs_core.http import BrowserHttpClient

    if getattr(BrowserHttpClient, "_bilifetch_limited", False):
        return
    original = BrowserHttpClient.request

    def limited_request(self, method, url, *args, **kwargs):
        if is_xhs_api_url(str(url)):
            acquire_xhs_token()
        return original(self, method, url, *args, **kwargs)

    BrowserHttpClient.request = limited_request
    BrowserHttpClient._bilifetch_limited = True
