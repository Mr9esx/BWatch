# Platform Download Pacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Execute this plan inline in the current BWatch checkout. Use checkbox steps for progress tracking.

**Goal:** Configure download pacing separately for Bilibili and Xiaohongshu, reduce recurring XHS scans, and gate every app-originated XHS API request with one shared token bucket.

**Architecture:** Store per-platform queue policies in app settings while migrating the previous global policy to Bilibili. Track queue slots and next-start times per platform. Persist XHS token-bucket state in the shared SQLite database and make Go, Python, browser API/XHR, and the XHS video extractor acquire from that same bucket before each API attempt.

**Tech Stack:** Go, SQLite (`database/sql` and Python `sqlite3`), Python `curl_cffi`, chromedp/CDP, React/TypeScript.

## Global Constraints

- Bilibili inherits the existing concurrency and 1–3 minute default spacing.
- Xiaohongshu defaults to one active download, 10–20 minute batch spacing, 0.2 API QPS, and token-bucket capacity 1.
- Routine XHS scans inspect at most the latest 3 pages and stop on the first locally known note; explicit history scans remain complete.
- Limit-platform responses stop new work through the existing per-platform cooldown; do not add signature/device evasion or proxy rotation.
- Do not add or run tests unless the user asks; use builds and static checks for validation.

---

## File Map

- `internal/app/store.go`, `store_sqlite.go`, `app.go`: settings model, defaults, migration, validation, and persisted limiter state.
- `internal/app/runner.go`, `history.go`, `xhs_api.go`, `xhs_api.py`: platform scheduling and bounded/complete scan modes.
- `internal/app/xhs_rate_limit.go` (new), `internal/app/xhs_login.go`, `xhs_login.py`, `xiaohongshu.go`: shared token acquisition for Go, API/login workers, and browser API/XHR.
- `internal/app/xhs_sitecustomize.py` (new), `runner.go`: token acquisition for XHS API requests made by yt-dlp subprocesses.
- `web/src/types.ts`, `SystemPages.tsx`, `ContentPages.tsx`, `CreatorDownloads.tsx`: per-platform settings and schedule/status display.
- `README.md`: explain separate queue settings, API QPS scope, and that pacing is not a safety guarantee.

## Task 1: Platform Settings and Migration

- [x] Add `DownloadPolicy{Concurrency, MinInterval, MaxInterval}` and `XHSAPIRateLimit` settings; retain old JSON fields only to migrate existing values to Bilibili.
- [x] Default Bilibili to concurrency 1 and 1–3 minutes; default XHS to concurrency 1 and 10–20 minutes; default XHS QPS to 0.2.
- [x] Validate both policies and QPS, reject NaN/Inf/zero/negative values, and persist the QPS in the shared settings state.
- [x] Update the settings UI with separate Bilibili and XHS sections and the XHS API QPS field; explain that the XHS bucket capacity is fixed at one.
- [x] Replace global concurrency/interval labels in the content and creator-download views with platform-aware values.

## Task 2: Per-Platform Queue Scheduling

- [x] Add persisted next-start timestamps keyed by platform, migrate the former global timestamp to Bilibili, and retain the legacy field for older clients.
- [x] Count currently running tasks by platform and select queued tasks subject to that platform's configured concurrency and cooldown.
- [x] Schedule each platform's next batch independently using its configured interval range; keep the global pause behavior.
- [x] Make task-list notices show the selected platform's next-start time and concurrency without confusing it with creator polling cadence.

## Task 3: Shared Persistent XHS Token Bucket

- [x] Add a singleton SQLite bucket row containing fractional tokens and last-refill time; capacity is one and refill rate follows the current XHS QPS setting.
- [x] Implement a cancellable Go `WaitXHSAPIToken(ctx)` using short SQLite transactions; when empty, release the transaction and wait until the next token is available.
- [x] Add the equivalent Python `sqlite3` acquisition helper using the same `bilifetch.db` file and transaction rules so concurrent child processes share one bucket.
- [x] Intercept XHS API HTTP attempts in `BrowserHttpClient` for data and login workers; acquire immediately before every send, including retries and redirects.
- [x] Intercept XHS API/XHR in the visible browser with CDP request pausing and the Go token waiter. Do not throttle page assets or media CDN requests.
- [x] Add a Python `sitecustomize.py` hook to gate XHS API requests made by yt-dlp, and pass its path and the shared DB path to the subprocess.
- [x] Ensure startup initializes an empty bucket so restarts cannot cause a full-token burst, and that cancellation exits token waits promptly.

## Task 4: Bounded Routine XHS Scans

- [x] Pass locally known video and image-post IDs to the XHS scan worker.
- [x] Stop routine scans at the first known note, with an absolute maximum of three fetched pages; return a partial feed without marking the catalog complete.
- [x] Keep explicit history scans in full mode and preserve their cursor/cache behavior; extend worker deadlines to account for configured low QPS.
- [x] Keep applying newly found items before a known-note boundary and merge partial catalogs without erasing older catalog entries.

## Task 5: Documentation and Validation

- [x] Document the XHS QPS limit, default settings, CDN exclusion, and lack of guaranteed risk-free thresholds.
- [x] Review migrations and `git diff --check`.
- [x] Run `go build ./...`, `npm --prefix web run build`, and `python3 -m py_compile internal/app/xhs_api.py internal/app/xhs_login.py internal/app/xhs_sitecustomize.py`.
- [x] Inspect final `git status` and summarize changed behavior and validation results.
