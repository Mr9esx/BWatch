# Frontend consistency implementation plan

> Execute inline in the current session with the executing-plans workflow. The user approved reusing the existing component library and current visual direction.

**Goal:** Make repeated UI elements share one implementation and one visual specification across routes and breakpoints.

**Architecture:** Reuse installed HeroUI Button and Chip through the existing shared components module. Correct the current CSS rules in place, replacing overlapping count/source rules and the universal font-size override with explicit component rules.

**Tech stack:** React 19, TypeScript, HeroUI 2, existing CSS and Vite.

## Constraints

- Preserve the current working tree, including uncommitted platform functionality.
- Preserve the current neobrutalist visual language and existing dependencies.
- Keep counts distinct from source labels and task statuses by semantic purpose.
- Use the same count geometry, source label component, and selected state semantics everywhere they repeat.
- Scope runtime interaction checks to reversible navigation, filters, views and dialogs.

## Tasks

- [x] Reuse HeroUI Chip and Button in shared CountBadge, TaskOrigin, FilterButton and ViewSwitch components; use the shared count in navigation, dashboard, filters and library authors, and TaskOrigin in tasks and creator downloads.
- [x] Remove obsolete library toolbar and per-page count rules; define explicit typography and tag specifications in the existing theme; align platform notices, status surfaces, focus and compact controls.
- [x] Correct scheduler action wording, add pending feedback, and provide selected states and consistent empty-state descriptions for filters.
- [x] Run the existing frontend production build and creator-download checks; inspect all routes at desktop and mobile widths, plus tablet tasks and creators; repair any regressions and review the patch against the saved working-tree baseline.

## Acceptance

Counts have the same height, border, radius, padding and tabular numerals in every location, including 0 and multiple digits. Task sources use the same neutral label in table, responsive card and dialog layouts. Heading/metric sizes are explicit and body/form text stays readable. Filter and view buttons expose aria-pressed; busy actions prevent repeated requests. Platform surfaces use theme tokens. Pages and dialogs fit narrow viewports with reachable controls and visible focus.

## Verification results

- `npm run build`: TypeScript and production build passed. Vite retains the existing large-chunk warning.
- `git diff --check`: passed.
- `npm run test:downloads`: fails because the pre-existing test context omits `libraryScope`. The same error was reproduced against a temporary copy of the exact pre-edit source; the script was not modified.
- Runtime checks: all seven routes at 1280×720 and 390×844; creators/tasks at 768×1024 and 320×740; adding a subscription and the creator download dialog on mobile; search, empty results/reset, content type, view switching, navigation opening/Escape/focus return.
- Measured repeated count tags: 22px high, 12px type, 1px border, 3px radius, 0/5px padding. Task source labels match in table, responsive card and download dialog: 24px high, 12px type, identical neutral background.
- Standard actions and form controls measure 40px; icon actions use 32px and the shared view switch uses 30px buttons within a 40px group.
- Task actions remain visible and the table has no internal horizontal overflow at the reviewed desktop/tablet/mobile widths. No page horizontal overflow was observed.

- Preview follow-up: a 273-character image-post description no longer collapses the main image (desktop measured 250px+, mobile 247px+). The description remains reachable by scrolling; next-image navigation updates the main image and current thumbnail.
- Fresh application reload: no new browser warnings/errors observed. Closing mobile navigation keeps the active search and returns focus to its trigger.
