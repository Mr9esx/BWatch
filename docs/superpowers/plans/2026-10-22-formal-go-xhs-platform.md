# Formal Go Xiaohongshu Platform Implementation Plan

> **Implementation status:** The formal Go implementation is in place. Build verification is complete; real-account acceptance remains pending and is the gate for deleting the MVP.

**Goal:** Add Xiaohongshu as a first-class provider in the Go app, preserving Bilibili while sharing creator scanning, task scheduling, media storage, history, and library behavior.

**Architecture:** Define platform-neutral creator/content/auth contracts in `internal/app` and register Bilibili and Xiaohongshu adapters. Use a persistent, visible Chromium profile controlled by Go/CDP for user login and page-originated signed requests; capture the site's paginated note responses without embedding a Python or Node runtime. Normalize both platforms into `video` and `image_post`, then route video files through yt-dlp and image posts through the shared safe HTTP archiver.

**Tech Stack:** Go HTTP service, chromedp/CDP for the visible authenticated browser, existing React/TypeScript UI, yt-dlp for video media, Go HTTP clients for image downloads.

## Global Constraints

- Support only `bilibili` and `xiaohongshu`; remove YouTube as a product option and extraction branch.
- Both platforms require a valid user login before scan, history, or protected downloads.
- Store each platform's credentials separately with user-only file permissions; never return secrets in API state or logs.
- Use platform adapters for URL parsing, login state, scanning, note detail, and platform pacing; keep scheduling, dedupe, task lifecycle, storage, and UI models shared.
- Treat Xiaohongshu interfaces as unofficial and mutable; stop on CAPTCHA, verification, or rate-limit responses and do not automate challenge solving or proxy rotation.
- Use yt-dlp for video media only; retrieve creator note lists and image-post details through platform adapters.
- Migrate existing JSON state in place; preserve existing Bilibili subscriptions and completed downloads.
- Remove the standalone `scripts/xhs-mvp` prototype only after the Go app completes the same real-account acceptance flow.

---

## File Map

- `internal/app/store.go`: unified `ContentKind`, creator capabilities, task content fields, per-platform session/settings state, and state migration.
- `internal/app/platform.go` (new): provider interface, registry, normalized creator/content contracts, and platform capability declarations.
- `internal/app/app.go`: provider-based URL normalization and creator validation; remove YouTube and Bilibili-only content assumptions.
- `internal/app/runner.go`, `internal/app/history.go`: shared scan/history orchestration that consumes normalized provider pages.
- `internal/app/bilibili.go`, `internal/app/dynamic.go`: move existing Bilibili protocol into its provider without changing its runtime behavior.
- `internal/app/xiaohongshu.go` (new): persistent visible Chromium session, CDP capture of page-originated signed profile-list responses, pagination, media URL parsing, and explicit restriction errors.
- `internal/app/login.go`, `internal/app/http.go`, `internal/app/account.go`: per-provider login routes and account status, preserving Bilibili QR behavior and adding visible XHS web-session flow.
- `internal/app/runner.go`, `internal/app/archive.go`: media dispatch for `video` and `image_post`, secure image host allowlist, metadata/content files, and archives.
- `web/src/types.ts`, `web/src/CreatorDownloads.tsx`, `web/src/ContentPages.tsx`, `web/src/Library.tsx`, `web/src/SystemPages.tsx`, `web/src/api.ts`: platform and content-kind UI contracts, login/settings, filters, labels, and download actions.
- `scripts/xhs-mvp/`: delete after formal Go login, scan, image archive, and video archive are confirmed against the supplied creator.

## Task 1: Establish the Shared Platform and Content Model — Implemented

1. Define `PlatformID` (`bilibili`, `xiaohongshu`) and `ContentKind` (`video`, `image_post`) plus normalized `CreatorRef`, `ContentEntry`, and paginated scan result types.
2. Add a `PlatformAdapter` interface for `NormalizeCreatorURL`, `LoginStatus`, `ScanPage`, `GetContentDetail`, and platform pacing/error classification. Add a registry populated with the two providers.
3. Convert `Task.Kind` and creator watch flags from Bilibili-specific `dynamic` naming to `image_post`; migrate legacy JSON fields and task kinds without discarding state.
4. Replace YouTube URL parsing and extractor branches with Bilibili/XHS provider selection. Reject unsupported hostnames before any network request.
5. Keep Bilibili behavior behind its adapter and shared core; do not change Bilibili pacing, scan semantics, or download output paths beyond what the normalized model requires.

## Task 2: Implement Xiaohongshu Authentication and Signed API Capture

1. [x] Add a persistent visible Chromium profile owned by the Go app; let the user complete QR login and any CAPTCHA directly in that window.
2. [x] Use CDP to capture the XHS page's own signed `user_posted` responses while scrolling the profile page; do not synthesize, replay, or log signatures in Go.
3. [x] Read XHS cookies from that browser profile only for yt-dlp video downloads, exporting a private temporary Netscape cookie jar; keep the persistent browser profile and Bilibili credentials separate.
4. [x] Add provider login status/start/logout endpoints; ensure XHS logout does not alter Bilibili credentials and vice versa.
5. [x] Map session, CAPTCHA/verification, rate limit, network failure, and unexpected response shape to safe user-visible errors.

## Task 3: Implement XHS Creator Scanning — Implemented

1. [x] Accept XHS app short links and PC creator profile URLs; resolve short links in the visible browser and validate the final destination host.
2. [x] Trigger cursor pagination by scrolling, capture `user_posted` responses over CDP, honor `has_more`, reject repeated cursors, and stop with an incomplete-result error at the safety bound.
3. [x] Parse note ID, title, type, per-note token, source URL, cover and image URLs from profile-list responses.
4. [ ] Fetch note details individually when list data does not contain the archive fields. Current profile-list response supplies text/image fields; videos are delegated to yt-dlp using the note URL and browser cookies.
5. [x] Feed XHS entries through shared baseline, dedupe, periodic scan, and history-job orchestration.

## Task 4: Archive Image Posts and Videos Through Shared Jobs — Implemented

1. Route normalized `video` tasks to yt-dlp with the matching provider's cookie jar and note URL.
2. Route `image_post` tasks to the Go HTTP image downloader; accept only HTTPS URLs on validated platform media domains and safe HTTPS redirects.
3. Save `content.md`, `metadata.json`, and every image under the normal task directory; fail the task if any required image fails so partial archives are not marked complete.
4. Preserve progress, pause/cancel, retry, bounded concurrency, task logs, and history-job attribution across both content types.
5. Ensure content previews, thumbnails, zip archive downloads, and library filters use the unified `image_post` kind.

## Task 5: Integrate Platform-Aware UI and Remove YouTube — Implemented

1. Update TypeScript provider/content types to `bilibili | xiaohongshu` and `video | image_post`.
2. Update creator add flow to auto-detect Bilibili UID/profile URLs and XHS short/profile URLs, and show only provider-supported content choices.
3. Add XHS login controls and account state while preserving the Bilibili QR screen; keep both credential states independent.
4. Update subscription cards, history actions, task lists, library filters, labels, error messages, and download actions to use platform-neutral kinds.
5. Remove YouTube brand/options/copy and its production extractor branch.

## Task 6: Build Verification and Formal Acceptance

1. [x] Build the Go backend and frontend without adding Python/Node runtime requirements.
2. [ ] Using the supplied creator profile and user-authorized XHS session, verify login, complete pagination, image-post archive, video archive, error stops, and session reuse after restart.
3. [ ] Verify existing Bilibili login, creator scan, image-post archive, video download, and saved state remain intact.
4. [x] Keep session cookies out of API state; save XHS browser data in its isolated private profile and cookie exports with owner-only permissions.
5. [ ] Remove `scripts/xhs-mvp/` after real-account acceptance confirms the Go app replaces its flow.

## Known Limitation

Xiaohongshu's web API and signing inputs are unofficial and may change. The app must show platform restrictions and stop; no implementation can guarantee access when XHS rejects the signed request or requires verification.
