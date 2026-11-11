# Icon renderings

PNG renderings of mega's mark, for the daemon's tray and desktop integration. The source is `ui/public/favicon.svg` (the hub) and `ui/public/favicon-needs-you.svg` (the same mark inverted on the notice gold, for when something needs the operator). Edit those, never these, then render again.

| File | Size | Use |
| --- | --- | --- |
| `desktop-icon-32.png`, `-48`, `-192`, `-512` | 32 to 512 px | desktop entry and launcher icons |
| `tray-icon.png` | 128 px | the tray; a panel scales it down smoothly |
| `desktop-icon-needs-you-*.png`, `tray-icon-needs-you.png` | the same | the needs-you state |

No SVG renderer is assumed. `render-icons.mjs` draws each SVG in headless Chromium through playwright-core, loaded from a directory you name, so the repository takes no dependency. The renderings checked in on 2026-11-11 came from playwright-core 1.65.0 and its Chromium 1243:

```bash
npm install --prefix /tmp/mega-render playwright-core@1.65.0
npx --prefix /tmp/mega-render playwright-core install chromium
PLAYWRIGHT_DIR=/tmp/mega-render node cmd/mega/render-icons.mjs \
  ~/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome
```

Run it from the repository root. It writes all ten files beside itself, with transparent corners.
