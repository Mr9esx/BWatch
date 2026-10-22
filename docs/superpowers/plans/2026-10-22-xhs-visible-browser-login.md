# 小红书可见浏览器登录 MVP 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户通过真实可见的小红书网页完成登录/验证码，并让本机 MVP 安全复用会话读取和归档博主笔记。

**Architecture:** `server.py` 管理独立 Playwright 持久化上下文、登录状态、会话 Cookie 导出及生命周期；`index.html` 提供启动浏览器、检查登录、退出登录的操作；现有笔记列表、媒体归档流程保持不变。用户链接仍先归一化为主页 user ID。

**Tech Stack:** Python 3.10+, Playwright Python sync API, 现有 xhs-reader/yt-dlp，本地 HTTP 页面。

## Global Constraints

- 浏览器资料目录固定在 `.tools/xhs-mvp/browser-profile/`，不触碰用户日常浏览器资料。
- Cookie 文件只保存在 `.tools/xhs-mvp/` 并设当前用户读写权限。
- 不自动处理验证码、不代理轮换、不重试被平台拒绝的请求。
- 支持小红书 HTTPS PC 主页及 xhslink.cn/xhslink.com App 短链；限制短链跳转目标。
- 不输出 Cookie、xsec token、或完整分享 URL 到页面和日志。

---

## Files and responsibilities

- `scripts/xhs-mvp/server.py`: 浏览器生命周期、Cookie 导入/校验/清除、profile URL 解析、API 路由和安全错误。
- `scripts/xhs-mvp/index.html`: 登录方式交互、用户等待提示、页面状态。
- `scripts/xhs-mvp/requirements.txt`: 固定 Playwright 版本。
- `scripts/xhs-mvp/run.sh`: 依赖安装后检查系统 Chrome/Chromium，不自动下载额外浏览器。
- `scripts/xhs-mvp/README.md`: 首次登录、依赖和验证码说明。

## Task 1: 本机浏览器登录会话

**Interfaces:** `start_browser_login() -> dict[state,message]` 启动专用持久化可见浏览器并打开小红书；`check_browser_login() -> dict[state,message]` 从浏览器上下文提取小红书 Cookie，使用现有 `get_self_info` 校验后保存；`stop_browser_login(clear_profile: bool) -> None` 关闭浏览器，退出时清 Cookie 和用户资料目录。

- [x] 用当前流程封装浏览器管理器；启动失败时返回“请安装 Chrome/Chromium”这类可操作错误。
- [x] 从浏览器上下文只提取 `.xiaohongshu.com` 域 Cookie，至少检查 `a1` 和 `web_session`，赋给 `XhsClient` 后通过 `get_self_info` 校验。
- [x] 校验成功后写 `cookies.json` 与 yt-dlp Netscape jar，设置 0600；校验失败时不保存 Cookie，并分类验证码/登录状态。
- [x] 修改退出接口，关闭 Playwright context 并清理 Cookie/profile；服务关闭时也关闭上下文。
- [x] 静态检查改动、检查生命周期分支和目录权限代码。

## Task 2: 页面流程、链接和文档

**Interfaces:** `POST /api/login/start` 打开登录浏览器，`POST /api/login/check` 检查会话，`POST /api/login/logout` 退出并清除；`GET /api/state` 返回脱敏状态。

- [x] 更新页面按钮为“打开小红书登录页”“我已完成登录，检查状态”，显示用户手动处理验证码的提示。
- [x] 将本机 PC 主页链接作为默认示例输入，保留 App 短链输入；不在状态或访问日志中泄漏链接查询 token。
- [x] 确认 profile URL parser 接受含分享查询参数的 PC URL，并验证短链只接受有限次 HTTPS 重定向到小红书 profile 路径。
- [x] 更新 README：Playwright 与已有 Chrome/Chromium 需求、持久化专用 profile、Cookie 本地保存和验证码处理边界。
- [x] 运行 Python 编译检查、`git diff --check`、检查 `/` 与 `/api/state`；登录接口未触发，登录/笔记读取仍需用户在本机操作。
