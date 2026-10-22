# Creator folders in the content library

**Goal:** Open the library at creator folders; open a folder to browse only that creator's archived content.

**Approved direction:** The user requested the folder → file model and reuse of the existing component library and visual style.

**Implementation:** Use HeroUI Card for creator folders and Breadcrumbs for the parent path. Reuse existing grouping, platform routes, file cards, search, filtering and window virtualization. The root and folder views have separate component lifecycles so the file virtualizer observes real mounted elements. Preserve archived content after subscriptions are removed and support the existing creator deep links.

- [x] Add and run a failing rendered integration check covering root folders, creator-specific content, deleted creators, platform scope and existing deep links.
- [x] Replace the permanent author sidebar with full-width creator folders at the root; add a folder-only content view with parent navigation and scoped filter reset.
- [x] Replace obsolete sidebar CSS with folder/root/detail layout rules that reuse current tokens and controls.
- [x] Run the new checks and production build; inspect root/detail at desktop and phone widths, keyboard opening, parent/browser navigation, filtering and existing subscription shortcuts.

**Boundaries:** Preserve current uncommitted work. No backend/storage changes, new dependencies or extra folder levels. File preview, download and copy-path functions remain available.

## Verification

- The new rendered integration check failed on the old implementation (missing creator-folder href), then passed after the change.
- `npm run test:library`, `npm run test:downloads`, `npm run build` and `git diff --check` passed. Build retains the existing large-chunk advisory.
- Real-data browser checks covered 1280px desktop, 768px tablet, 390px and 320px phones with no horizontal overflow. At 390px the complete creator folder is visible in the first viewport; the first file card begins around y400 after opening the folder. Desktop content uses three columns and list rows measure about 154px.
- Verified click/Enter folder opening, parent path, browser back/forward, platform scope, root and content searches, empty/reset states, content-kind filters, list/grid switching, preview via file title and the subscription page's existing content shortcut.
- Fresh reload produced no browser warning/error logs. Screenshots saved as `library-creator-folders.jpg` and `library-folder-content.jpg` in the task visualization directory.
