# Browser checks

Browser checks are tests, and they live here, not in a scratch directory. Each suite starts its own mega on a scratch port, with a data directory, XDG directories, a `HOME` and checkouts under its own scratch directory, and drives the page with Playwright. Only that data goes in scratch. Every suite runs against the committed fakes, the fake Claude shim (`shim/claude/fake/shim.mjs`) and the fake Codex (`internal/harness/codex/fake/codex`), so none costs a model turn, touches a real harness or reaches the operator's mega.

## Running them

```sh
make checks-browser          # once: installs the Chromium the pinned Playwright drives
make checks                  # every suite
make checks SUITE=requests   # one suite; a space- or comma-separated list runs several
```

`make checks` builds the UI, the shim's dependencies and mega, then runs each suite in turn as its own process. It prints every step and ends with a table of each suite's steps, failures and time. It exits 0 when every suite passed, 1 when a step failed or a suite crashed, and 3 when the checks could not run at all. A missing browser stops it before any service starts, with the command that installs it; nothing is skipped silently. It is not part of `make test`, and `make test` never starts a browser.

Playwright is a pinned development dependency in `checks/package.json`; `make checks` installs it with `npm ci`. `make checks-browser` downloads Playwright's Chromium into Playwright's own cache (`~/.cache/ms-playwright`), which Playwright reads by default.

Where things go:

- The scratch root is `$TMPDIR/mega-checks` (`/tmp/mega-checks` without `TMPDIR`). Set `CHECKS_DIR` to put it elsewhere.
- Each suite gets `<root>/<suite>/`, emptied when the suite starts and left afterwards. It holds the suite's service log, `service.log`, its screenshots under `shots/`, its checkouts and data.
- Each suite's printed output is also kept in `<root>/<suite>.out`.
- The port is 18470, or `MEGA_CHECK_PORT`. A suite refuses 8470, the operator's, and refuses to start while something else answers on its port.

## What each suite covers

- **`conversation.mjs`** (52 steps): the conversation view, on long conversations and in its layout. Two conversations are made about twenty thousand events long with `lib/gen.mjs` while the service is stopped. It checks the following:
  - opening reads only the tail: ten turns, at the newest reply, saying how much is earlier;
  - Show earlier and Show all, each holding the view;
  - the native events toggle refetching what is shown;
  - a log that ends in a native event read through it;
  - a streamed reply re-rendering only the tail with no long task;
  - a dropped stream naming what it waits for, and its reconnect reading only what it missed;
  - a held, then failed, opening read naming the request, its time, the attempt and the error;
  - the connecting view naming a failing workspace read;
  - the 960px column centred in its pane, with the panel open and collapsed to the rail, light and dark; the header, history, input panel, show-earlier bar, newer-activity control and shutdown banner sharing it; the project page and the connecting view centred the same way;
  - no label on the operator's own prompts, "outside mega" on a prompt from outside, and "From outside mega" on its reply;
  - MCP servers, for the fake Claude and the fake Codex:
    - the header's summary, on the notice tone while a server is not connected, and "not running" with no runtime;
    - the list it opens: each status, a failed server's error, a server needing sign-in saying where that is done in the harness, and tools on expansion, with descriptions from Codex;
    - nothing to change in it;
    - a sign-in followed in the summary and the open list;
    - never attention;
    - cleared on release.

  It saves the centred conversation as `shots/centred-{light,dark}-{panel,rail}.png`, and the MCP list as `shots/mcp-claude.png`.
- **`environment.mjs`** (29 steps): a project's environment on its page. It checks the following:
  - the unapproved state, and its refusal on New conversation and Adopt;
  - approval, and the names it records;
  - the header's environment line;
  - a changed file shown beside the approved version;
  - the refusal on Send and Start, and approving again;
  - a sourced file changing the names;
  - the override;
  - values only on request;
  - a project with no file.
- **`panel.mjs`** (40 steps): the input panel. It checks the following:
  - the reply box resting at one line and opening to its height;
  - the line above the panel staying put as the box opens and closes, and a view at the bottom staying there;
  - a control in the bar keeping the box open, and a press outside closing it;
  - a draft kept across a switch of conversation;
  - Ctrl+Enter sending, closing the box and going to the end, following the reply;
  - a refused send keeping the box open, focused and with its draft;
  - Send and Start disabled with the reason while a shutdown is armed or the checkout is missing, the shutdown's first;
  - Start on a released conversation;
  - the permission mode switched live, and stored for the next launch;
  - a model and effort applied for Claude and for Codex, shown as the next prompt's until a turn reports them;
  - while the view follows the bottom, the newest content kept above the dock as it grows:
    - a prompt sent with the reply held keeps its block above the dock once the activity line shows, with the workspace read held so the line arrives after the view went to the end;
    - a docked request;
    - the reply box growing with its draft;
  - scrolled up, neither the activity line nor a docked request moving the view.

  These steps turn off chromium's scroll anchoring, which re-pins a view at the bottom by itself and so hides the defect; firefox does not.
- **`requests.mjs`** (27 steps): requests. It checks the following:
  - a permission card docked above the reply box, not in the history, with its one-line row there;
  - focus on the card neither opening the box nor moving the card;
  - Allow, and Deny with a reason, each clearing the card, with the answer reaching the turn;
  - answering keeping an open box's draft;
  - two requests stacked oldest first;
  - a question card answering only once a choice is made;
  - a request answered on another page refused on a page still showing it, and by the API;
  - an MCP approval from the fake Codex, allowed for the session or denied;
  - a typed form needing its required field;
  - Interrupt aborting a pending card, with a late answer refused.
- **`shutdown.mjs`** (23 steps): the shutdown gate. It checks the following:
  - arming from the footer, with the banner naming what it waits for by count and by name, and the request marked as the operator's;
  - a second page seeing it;
  - new work refused with the reason, on the page and by the API;
  - a blocker's name opening it;
  - Shut down neither offered nor accepted before idle;
  - cancelling lifting it on both pages;
  - at idle, Shut down offered, the title saying so and one notification;
  - Shut down ending the service with status 0;
  - the page that asked saying when, while the other shows only what it last knew;
  - both pages recovering when the service returns.

- **`side.mjs`** (44 steps): the side panel and the rail. It checks the following:
  - projects sorted by name ignoring case, the id breaking a tie, and starred first in their own group;
  - project rows bolder than their conversations;
  - folds: a starred project unfolded and any other folded until the operator chooses; his choice kept across a reload and outlasting a star or an unstar; navigation by the needs-you list or Ctrl+/ unfolding without recording a choice; the old folded list carried over as folded choices; a removed project's choice forgotten; a folded project keeping its rollup mark;
  - stars seen by a second page from the stream alone;
  - the panel's width within 200 to 480, kept across a reload;
  - shortcuts: Ctrl+/ from the page and from the reply box, no bare n, Ctrl+' collapsing and expanding from either, typing nothing, and the hint and titles naming them;
  - the rail: the count, one mark per project in tree order, the working arc, its width, kept across a reload, and a mark opening the panel on its project;
  - a missing checkout's mark in the tree and on the rail, lifting when it is back;
  - project removal refused while a runtime lives and naming it, then deleting the project's logs;
  - the build: `GET /api/version` naming this page's interface, the footer's version and its panel, the rail's control, and a page older than the service saying so where the connection status is, without reloading;
  - every control in the panel and on the rail with a title and a label.

Each suite also checks that the page raised no errors and fetched nothing off 127.0.0.1.

The other surfaces' suites were lost with `/tmp` on 2026-11-12. Each is rebuilt by the first batch that changes its surface; the roadmap card `ui-test-runner` lists which lost checks covered which surface.

## Adding a suite

A suite is a file `checks/<name>.mjs`; the runner finds it by its name. It is its steps and nothing else. `lib/mega.mjs` does the rest:

```js
import { suite } from './lib/mega.mjs'

const t = await suite('example')
const P = await t.register('example', t.checkout('example'))
const C = (await t.launch(P.id, 'claude', 'echo\nhello')).id
await t.idle(C)

const page = await t.page()
await page.goto(`${t.url}/#/c/${C}`)
await t.until('the reply', async () => (await page.getByText('hello').count()) > 0)
t.check(true, 'the reply is shown')

await t.done()
```

`suite(name)` empties the suite's scratch directory and makes its directories, its config and its `HOME`, then starts the service and launches the browser. Its `HOME` has a `.profile` that puts the check's own `node` on `PATH`, since project environments resolve through a login shell from it. The operator's shell start-up files and Claude settings are never read. It returns the following:

- `check(ok, what)` records a step. `must(ok, what)` records one the suite cannot go past: a suite checks with it, before any Allow, that the request is exactly the one the fake raised, so nothing unexpected is approved. `until(what, pred, ms)` waits on a condition, checking every 50 ms, and fails saying what it waited for.
- `api(method, path, body)`, `workspace()`, `projectOf(id)` and `convOf(id)` read and call the service.
- `checkout(name, { git })` makes a scratch checkout. `register(name, path)` adds it as a project, with an override activation that points Claude at the suite's scratch settings, since mega's own environment does not reach the processes it starts. Pass `{ override: false }` for a project whose environment is under test.
- `launch(projectId, harness, prompt, extra)` starts a conversation, and `idle(id)` waits until its turn ends.
- `page({ width, height, colorScheme, init })` opens a page in its own context. `shot(page, name)` saves a screenshot under `shots/`.
- `stop()` asks the service to stop and waits for it; `start()` starts it again on the same data. `stopped()` waits for a service a page shut down, and reports how it exited.
- `done()` adds the page-wide steps, closes everything, prints the summary the runner reads, and exits.

The rules:

- Wait on conditions, never on delays. A step that cannot be made stable is reported, not left flaky.
- Approve only where the approval is under test, after a `must` on what is pending; where the decision is incidental, deny.
- Drive the fakes by the text of each prompt. The fake shim's verbs are in its header, and the fake Codex's in its own; a verb a suite needs is added there.
- Only data goes in scratch. A suite never writes outside its scratch directory, apart from what the service writes there for it.
- Say at the top of the file what the suite checks, and add it to the list above.

`lib/gen.mjs` appends generated turns to a scratch conversation's log, for a suite that needs a long conversation. It is seeded, so the same seed writes the same conversation.
