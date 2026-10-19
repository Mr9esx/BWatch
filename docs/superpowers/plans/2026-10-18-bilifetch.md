# BiliFetch 实施计划

**目标：** 交付可运行的 UP 主更新监测与自动下载应用。

**架构：** React + HeroUI 管理界面，通过同源 JSON API 访问 Go 服务。Go 调用 yt-dlp，原子 JSON 持久化，定时扫描与有限并发下载。

**设计：** `docs/superpowers/specs/2026-10-18-bilifetch-design.md`

## 任务 1：订阅、存储与队列

- [x] 在 `internal/app/app_test.go` 编写测试，手工构造 first/second 视频扫描结果，验证首次不下载、第二次仅新 ID 入队、重复检查幂等、10 条历史策略和重启恢复。
- [x] 执行 `go test ./...` 观察功能缺失失败。
- [x] 实现 `internal/app/store.go`（类型与原子事务）、`internal/app/app.go`（订阅与队列状态转换）。输入 `[]Entry`，输出持久化 Task；状态转换非法时返回错误。
- [x] 执行测试，修复失败。

## 任务 2：运行器与 API

- [x] 测试 UID 和频道地址白名单、任务暂停取消、非法设置、API 的 400/404 行为。
- [x] 实现 `internal/app/runner.go`：`--ignore-config --flat-playlist --dump-single-json --playlist-end 50` 扫描；`--newline --progress-template` 解析下载进度；`--print after_move:filepath` 记录最终文件。
- [x] 实现 `internal/app/http.go` 与 `cmd/bilifetch/main.go`，含静态前端、健康检查、同源校验、受限文件访问、优雅退出。
- [x] 使用临时目录和可控执行文件验证扫描至文件产出完整路径，执行 `go test -race ./...`。

## 任务 3：可交互界面

- [x] 创建 `web/` 的 Vite + React + TypeScript + HeroUI 配置，运行依赖安装。
- [x] 在 `web/src/` 实现类型、API、明确标注的独立演示数据、六个管理页面、订阅表单和下载控制。
- [x] 使用 `npm run build` 检查类型和生产构建；通过服务端渲染检查 12 个页面/模式组合。浏览器自动化通道启动失败，点击与移动布局仍需人工检查。

## 任务 4：运行交付

- [x] 添加多阶段 Dockerfile、compose、Makefile、中文 README，说明依赖、历史策略、50 条扫描窗口、单进程存储和平台限流。
- [x] 执行 Go 测试与生产构建，启动本机服务并打开预览，检查 API 与静态资源。

## 验证记录

- 18 项 Go 测试（含竞态检测）、go vet、Go 二进制构建通过。
- TypeScript + Vite 生产构建通过；主包有体积提示（约 874 KB，gzip 248 KB）。
- React 服务端渲染 12 个页面/模式组合通过，不替代浏览器验证。
- 真实 B 站视频请求返回 HTTP 412，动态返回风控网页，未宣称真实抓取成功。
- 当前没有 Docker CLI，容器构建未验证。
- 后续增补：动态图片 HTTPS 规范化、图文 ZIP 打包、状态写入故障重试、Unix 子进程组取消均有回归测试。
