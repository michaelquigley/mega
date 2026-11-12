# Browser checks

Browser checks are tests, and they live here, not in a scratch directory. Each one starts its own mega on `127.0.0.1:18470` with a data directory, XDG directories and checkouts under the scratch directory it is given, and drives the page with Playwright. Only that data goes in scratch. Each one runs against the fake Claude shim and the fake Codex, so it costs no model turns. There is no runner yet; the `ui-test-runner` roadmap card is where one arrives.

Each check takes the `playwright-core` package directory, a Chromium executable, the mega binary to run, and a scratch directory, which it empties first:

```sh
go build -o /tmp/mega-check/mega ./cmd/mega
node checks/environment.mjs ~/path/to/node_modules/playwright-core \
  ~/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome \
  /tmp/mega-check/mega /tmp/mega-check/environment
```

- `environment.mjs`: a project's environment on its page. It covers the unapproved state and its refusal on New conversation and Adopt; approval; the header's environment line; a changed file beside the approved version; the refusal on Send and Start; approving again; the override; values only on request; a project with no file.
- `rescued/`: checks saved after a reboot erased the earlier ones, kept as written and not yet runnable from here.
