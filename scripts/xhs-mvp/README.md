# 小红书归档 MVP

这是一个独立运行在 `127.0.0.1` 的试验页面，验证登录、读取某博主的笔记列表和数量、逐条保存笔记媒体。它不改 BWatch 正式后端和订阅数据。

## 启动

需要 Python 3.10+ 和本机 Chrome/Chromium，运行：

```sh
scripts/xhs-mvp/run.sh
```

首次启动会在忽略目录 `.tools/xhs-mvp/venv` 建虚拟环境，并安装固定版本的第三方只读客户端、Playwright 及 yt-dlp。打开终端显示的 `http://127.0.0.1:8769`，点击“打开小红书登录页”，在新开的专用浏览器窗口扫码并自行完成小红书验证码，然后回到页面点“我已完成登录，检查状态”。

支持 App 分享短链（`xhslink.cn` / `xhslink.com`）及 PC 博主主页链接（`xiaohongshu.com/user/profile/<id>`）；主页链接上的分享查询参数会被忽略。

## 当前技术边界

- 官方扫码 OAuth 目前只开放 `basic_info`，`read_notes` 仍在规划中，无法用官方 Token 读取博主笔记。[官方 scope](https://openaccount.xiaohongshu.com/docs/scope)、[官方设备授权 API](https://openaccount.xiaohongshu.com/docs/api-reference)
- 本原型通过固定 Git commit 的第三方只读客户端访问网页登录会话中的未公开接口 `/api/sns/web/v1/user_posted`；这不是官方 API，也不代表平台授权。[第三方客户端](https://github.com/hammershock/xhs-cli)
- App/PC 分享主页链接中的 `xsec_token`、`xsec_source` 会随列表请求传递；仅有干净主页 URL 时使用 `pc_feed` 作为来源。它们是链接上下文参数，不会解除平台验证或 IP 限制。
- 对照 [XHS-Downloader](https://github.com/JoeanAmier/XHS-Downloader)：仓库里有一个签名 GET 的 `UserPosted` 实现，参数含 `xsec_token`、`xsec_source`，使用 `xhshow` 生成签名；当前主分支的 HTTP 客户端还配置了 `curl_cffi` Chrome impersonation。不过 `UserPosted` 在当前代码中没有调用点，不能当作其作者主页归档端到端可用的证据。该项目 README 说明自动滚动默认关闭，并提示可能触发风控。
- 登录资料保存在专用目录 `.tools/xhs-mvp/browser-profile/`，不读取用户日常 Chrome 配置。Cookie 与 yt-dlp Cookie jar 只保存在 `.tools/xhs-mvp/`，权限为当前用户可读写。退出登录会关闭专用窗口并清除该资料目录。
- 用户可在真实小红书浏览器窗口手动完成登录验证；若读取接口仍要求验证或限流，MVP 会停止，不代理轮换、不自动解验证码、不循环重试。列表分页最多 100 页，达到上限会明确标为部分结果。
- 视频逐条调用 yt-dlp；图文逐张通过 HTTP 保存图片。每篇笔记先写入临时目录，成功后再移动为正式归档目录。归档输出到 `downloads/xhs-mvp/`。

## 本次实测

- 用户提供的 App 分享短链可解析到博主主页；PC 主页链接也可直接解析，忽略分享查询参数。
- 专用浏览器登录校验成功，Cookie 会话已被 MVP 识别。
- 读取该博主列表时，平台返回额外验证或 IP 限制状态；MVP 在读取到任何笔记前停止（数量 0），没有归档内容。
- 对照其他公开实现后补齐了列表请求的 `xsec_token` / `xsec_source` 参数，并用用户的 App 短链验证解析结果为目标主页 ID、含 token、来源 `app_share`；带这些参数重试后仍返回验证/限制分类。因此，缺参数确实是实现缺口，但目前证据表明它不是唯一原因，接口仍受平台侧限制。
- 小红书官方开放平台当前只开放 `basic_info`，`read_notes` 仍标为规划中；公开官方 API 文档没有任意博主笔记列表接口。[官方 Scope 文档](https://openaccount.xiaohongshu.com/docs/scope)
- 本地离线测试曾覆盖链接解析、笔记字段脱敏和风控状态分类，4 项通过；本次修复后运行了 Python 编译检查和 `git diff --check`。

## 退出

页面点“退出登录”删除 MVP 会话并关闭专用浏览器。服务进程可在启动终端按 `Ctrl-C` 结束。
