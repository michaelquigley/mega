# Which keys reach the page

A browser or the system can take a key before the page sees it, and no check can tell: Playwright sends its keys to the page directly. So a person confirms each of mega's chords by hand, in each browser on each machine.

`index.html` is a page with no server and no network. Open it from the file system in the browser to be checked. Press each key twice: once with the focus on the page, and once in the text field. A row turns green when the page sees the key. The page takes every key it lists, as mega does, so a browser's own action for that key does not run. The log below the table shows every key press with its modifiers, `key` and `code`.

The keys it lists are those in `ui/src/keys.ts`, matched the same way. When that table changes, change this one.

`demo.mjs [PORT]` starts a mega to try the keys in, on port 18490 by default. It builds mega from this checkout and runs it with its own data and XDG directories and the fake Claude, so it reaches no real harness and costs no model turn. It seeds four projects, one starred and one empty, with two conversations waiting on an approval, one working and one with unread output. Ctrl+C stops it and removes its scratch. It needs the UI built and the shim's dependencies installed, as `make checks` leaves them. On a Mac, project environments resolve through zsh, because the system bash closes the descriptor the resolver reads.

`focus-walk.mjs [URL] [chromium|webkit]` drives a running `demo.mjs` as a page on a Mac and prints where the focus is after each step: a click on a project row, on a conversation, on each heading, then ⌃⌘P from the reply box and from the rail, then typing from outside the reply box and a send with ⌘↩. It loads the checks' Playwright. WebKit, the engine behind Safari, needs `npm --prefix checks exec -- playwright install webkit` once.

## Results

None recorded yet for the keys. Record each as a dated line: the machine, the browser and its version, and the keys that did not reach the page.

- **2026-11-20, `focus-walk.mjs`, WebKit 27.2 (Playwright 1.65) on macOS:** before the fix, a click on a project row left the focus on the page's body, so the arrow keys did nothing; Chromium focused the row. After it, both engines give the same walk: the clicked project row focused, a clicked conversation's reply box, a heading's list's selected row, and ⌃⌘P's selected tree row, from the reply box and from the rail.
- **2026-11-20, later, the same WebKit:** a click in a list, on a project row or a heading, now leaves the list itself (`section.projects`, `section.needs-you`) holding the focus with no row highlighted, and the first ArrowDown highlights the selected row. ⌃⌘P does the same from the reply box and from the rail. A clicked conversation still puts the cursor in its reply box. Chromium gives the same walk.
