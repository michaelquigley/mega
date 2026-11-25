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

- The scratch root is `$TMPDIR/mega-checks` (`/tmp/mega-checks` without `TMPDIR`). Set `CHECKS_DIR` to put it elsewhere. Set `MEGA_CHECK_DISABLE_SNAPSHOTS=1` to run scratch services with `snapshots.keep: 0`.
- Each suite gets `<root>/<suite>/`, emptied when the suite starts and left afterwards. It holds the suite's service log, `service.log`, its screenshots under `shots/`, its checkouts and data.
- Each suite's printed output is also kept in `<root>/<suite>.out`.
- The port is 18470, or `MEGA_CHECK_PORT`. A suite refuses 6342, the operator's default, and 8470, the default before 2026-11-18, which a mega not yet restarted may still serve. It refuses to start while something else answers on its port. The firstrun suite also starts megas on the four ports above its own, and refuses those the same way, before anything starts (`requireScratchPort`).

## What each suite covers

- **`conversation.mjs`** (106 steps): the conversation view, on long conversations and in its layout. Its service runs in `Asia/Kolkata` (`TZ`) and its page in UTC, as a hardened browser reports, so every time it checks is the service's. Two conversations are made about twenty thousand events long with `lib/gen.mjs` while the service is stopped. It checks the following:
  - opening reads only the tail: ten turns, at the newest reply, saying how much is earlier;
  - Show earlier and Show all, each holding the view;
  - the native events toggle refetching what is shown;
  - a log that ends in a native event read through it;
  - a streamed reply re-rendering only the tail with no long task;
  - a dropped stream naming what it waits for, and its reconnect reading only what it missed;
  - each upward scroll made with the mouse wheel (`toTop`), as a reader makes it;
  - a held, then failed, opening read naming the request, its time, the attempt and the error;
  - the connecting view naming a failing workspace read;
  - the 960px column centred in its pane, with the panel open and collapsed to the rail, light and dark; the header, history, input panel, show-earlier bar, newer-activity control and shutdown banner sharing it; the project page and the connecting view centred the same way;
  - a stop an earlier build flagged as a gap, shown as any stop, with no gap sentence and nothing of the work seeded on its session outside mega; More offering no transcript, and the transcript route gone;
  - acknowledging a loss with an unknown session, its Unsupervised row carrying no gap sentence, and paging and timing across an Unsupervised row;
  - no label on the operator's own prompts, "outside mega" on a prompt from outside, and "From outside mega" on its reply;
  - each turn's line saying what it ran with, when its prompt was sent and how long it took:
    - on turns recorded before mega kept what they ran with, their time alone, from another day with its date;
    - the time in the service's zone, not the page's UTC, and both times to the second, with the zone, in its title;
    - the operator's prompt carrying its time as its title, and a prompt from outside mega "Imported";
    - hidden at rest, shown while the pointer is over the turn and while focus is inside it, with nothing moving either way;
    - kept visible at rest, in the notice colour, naming both, on a turn the fake reports another model for (the fake shim's `fallback` verb);
    - on the turn in flight, shown on hover once it has run, with when it started, and nothing moving;
  - the "Runtime started" line, with the fake shim reporting the version its `claude-version` file names: unmarked and with no added words for a version inside the tested range and for one newer, the normal state; marked for one older, in the notice colour, naming the range after the version, its title saying what the Agent SDK expects; and none of them refused;
  - MCP servers, for the fake Claude and the fake Codex:
    - nothing of them in the header;
    - the More menu's item with the count, and "not running" with no runtime;
    - a dot on the More button, named, while a server failed or needs signing in, in view at the foot of the conversation;
    - the list the item opens: each status, a failed server's error, a server needing sign-in saying where that is done in the harness, and tools on expansion, with descriptions from Codex;
    - nothing to change in it;
    - a sign-in followed in the open list and the item;
    - never attention;
    - cleared on release;
  - a pi conversation, on the fake pi:
    - pi's own words under its header about how it asks;
    - its prompt, tool call and reply in the history as the others', the prompt shown once, its record of pi's identity hidden with the native events;
    - its gated tool call, while the dialog is pending, saying it waits on the dialog, with no spinner;
    - its turn line with no permission mode. Its extension's gate is blocked, since allowing it is not what the suite is about.
  - actions, for each harness's fake (its `actions` verb):
    - a failed action marked " failed" while collapsed, in the error colour, and one that ran not marked;
    - expanded, the whole command, which the collapsed line cut short, under "Command", then the result under "Result";
    - the command's copy control taking all of it;
    - another tool (Claude's Edit, a Codex file change, pi's read) shown as its name and its input, field by field;
    - an action from the generated history, with nothing its adapter reads, showing the one line it has;
    - a web search the fake Codex reports for the turn before while the next one runs (its `late-search` verb): no spinner, only "no result recorded", and not named on the activity line as the turn's work; and a message it reports for the turn before while the next streams (`late-message`), leaving that streaming reply and "Writing" be, with nothing it streamed for the turn before in it.

  It saves the centred conversation as `shots/centred-{light,dark}-{panel,rail}.png`, the MCP list as `shots/mcp-claude.png`, a turn at rest, hovered and mismatched as `shots/turn-{rest,hover,mismatch}.png`, the start lines as `shots/start-{tested,newer,older}.png`, and each harness's expanded command and input as `shots/action-{claude,codex,pi}{,-input}.png`.
- **`firstrun.mjs`** (59 steps): the first run on a machine that has never had mega, with no harness installed, then with Claude Code and Codex. It is a bare suite: no fakes, no Claude settings, a config naming no harness path, and no harness on any `PATH` it gives. Claude Code's default, the bare name `claude`, is on no `PATH`, and the scratch `HOME` has no build in the installer's versions folder; Codex's and pi's, bare names, are on no `PATH`. It checks the following:
  - the start-up log's line for each harness, Claude Code, Codex and pi, saying it cannot run and why, Claude Code's naming the versions folder it looked in;
  - the empty workspace's first page, light and dark: each harness missing with its reason and the config file mega read, the register control named in words, and the control carrying a word;
  - the register dialog opening with the focus in its checkout field, which starts at the home directory with no projects root configured, refusing a relative path and a missing one with what is wrong, then registering a checkout and opening its page;
  - the project page before any dialog: all three harnesses listed as missing, from the resolution registering started; Claude Code's reason saying `claude` was not found and naming the versions folder, saying to install it or set `claude.executable`, and naming the config file; a resolution shown, not "Not resolved yet"; a trust notice that does not tell anyone to run a Claude that is not installed; and New conversation carrying a word;
  - the new-conversation dialog opening with the focus in the opening prompt, all three harnesses marked "(cannot run)" with their reasons, the model list reading "Models unavailable", nothing stale said, and Launch disabled and in view at 1280×900 with the notices;
  - the API refusing Claude Code's model list and a launch of either harness as `executable_missing`, with the reason, and making no conversation;
  - never "internal error" on a page, and no answer of 500, on the whole walk;
  - a restart: the project kept, what is missing known again with nothing opened, and the start-up log's line for each harness again;
  - `--listen`: another mega, on its own scratch data, on the port the flag names, and one refused for `0.0.0.0`, as the config value is;
  - both harnesses installed, as stand-ins that only say their versions: a native Claude Code build, built from `internal/harness/claude/fakebuild` into the versions folder, and the fake Codex package at 0.166.0 on the `PATH`, inside its range. A mega on fresh data shows them:
    - **with Claude Code 2.1.330, newer than its range:** the start-up log, at info, says it runs from the newest build installed with no `claude` on the `PATH`. The first page says each harness in one short plain sentence: its build, where it is and the range, with no node or shim and no notice colour. `/api/version` names the ranges and what each found is, a newer one with no sentence, and `mega version` says the same plainly;
    - **with 2.1.300, older:** the start-up log warns, and the first page marks Claude Code in the notice colour with the stronger sentence;
    - **an update behind the same path, with mega running:** Claude Code found through `bin/claude`, a link the check moves to a new build, and Codex and pi as packages whose programs the check replaces in place with the modification time npm's tarballs carry. The open version panel follows all three within seconds, with no restart and no reload, and so does the first page.

  It prints the words each step showed, so its output is a record of what a newcomer reads. It saves `shots/{empty,empty-dark,register,register-refused,project,launch-missing,restarted,versions-newer,versions-older}.png`.
- **`dialogs.mjs`** (58 steps): the project page's two dialogs. Its config names `~/Repos`, in its scratch `HOME`, as the projects root. It checks the following:
  - the register dialog's folder picker:
    - the field starting at the root;
    - folders only, a git checkout marked, and no dot-folder, file or link leading out;
    - a registered folder saying so;
    - no way up at the root;
    - completion as it is typed, and Tab completing it, the name following;
    - a click going in and the Up control coming back;
    - a registered folder not gone into, and refused when named;
    - a folder outside the root listing nothing and saying why;
    - the keyboard: Down into the list, arrows, Enter in, Left up;
    - a name changed by hand kept;
    - a picked folder registered with its name, and a path outside the root typed in full registered too.
  - the adopt dialog, over sessions of each harness's fake, made by launching and removing conversations, with one still held. At 700, 1280 and 1920 pixels:
    - the dialog wide and most of the window's height;
    - each harness's control with its mark and count;
    - the held session counted;
    - every row laid out the same, newest first: the first prompt on up to two lines, how long ago on one line with the exact time as its title, the id on one line;
    - the filter;
    - the look at a picked session for each harness, beside the list, or below it when narrow;
    - the terminal warning, pi's saying it has no lock;
    - adopting.

  It saves `shots/register.png` and `shots/adopt-{narrow,1280,wide}.png`.
- **`environment.mjs`** (32 steps): a project's environment on its page. It checks the following:
  - the unapproved state, and its refusal on New conversation and Adopt;
  - approval, and the names it records;
  - the header's environment line;
  - a changed file shown beside the approved version;
  - the refusal on Send and Start, and approving again;
  - nothing for the environment on the control bar while nothing needs attention; a runtime in an older environment marked on the bar, in view at the foot of a long conversation with the header scrolled away, the header keeping the fingerprint; then the refusal on the mark once released;
  - a sourced file changing the names;
  - the override;
  - values only on request;
  - a project with no file.
- **`panel.mjs`** (137 steps): the input panel. Its config names `~/Sandbox` and `~/Screenshots`, in its scratch `HOME`, beyond the checkout. It checks the following:
  - the reply box resting at one line with the cursor in it when the page loads on a conversation, and its title naming Ctrl+Space;
  - focus alone leaving it at one line, the first character opening it to its height, and a draft opening it on focus;
  - the line above the panel staying put as the box opens and closes, and a view at the bottom staying there;
  - a control in the bar keeping the box open, and a press outside closing it;
  - a draft kept across a switch of conversation, the box open on return with the cursor at its end;
  - the focus following the switch: the cursor in the reply box, at one line, after a pick in the side panel or the needs-you list, Ctrl+/, launching and adopting; never with a dialog open, nor from a field being typed in; and Ctrl+/ doing nothing while a dialog is open;
  - nothing read behind a dialog: a conversation switched to by the address behind one, and output arriving in the open conversation while one is open, staying unread until the dialog closes and read then;
  - Ctrl+Space from the history and from the side panel, and nothing with a dialog open or with the cursor already there;
  - Escape leaving the box for the history, whose keys then scroll it, and closing the path list first;
  - Page Up and Page Down scrolling the history from an empty box and ending following as a wheel does, and the box's own with a draft;
  - the allow keys from a box the switch focused, and from the history after Escape;
  - a read waiting for the view: output arriving while scrolled up, "Newer activity below" showing, staying unread and read on return to the newest output; the same when a dialog closes over a view still scrolled up; and a read refused once, tried again once, after a wait;
  - Ctrl+Enter sending, closing the box and going to the end, following the reply;
  - a refused send keeping the box open, focused and with its draft;
  - Send and Start disabled with the reason while a shutdown is armed or the checkout is missing, the shutdown's first;
  - Start on a released conversation;
  - the header keeping identity only: the harness and version in words, and no model, effort or permission mode;
  - a new conversation's request concrete from creation, for Claude and for Codex, and none of the removed inherit and next-send fields on the wire;
  - the bar's closed controls showing the request in short values, Claude's `default` mode as "ask", with the full names and descriptions in their lists, and no control or note on the bar saying inherit, requested or next;
  - the harness's mark on the bar, by name, for Claude and for Codex, with the box collapsed and open, in the light and dark themes, and pi's in the dark theme;
  - a pi conversation:
    - pi's own model and thinking level copied, and pi's mark leftmost;
    - no permission mode control, no MCP item in More and no mark on it;
    - pi's words about how it asks;
    - a model with no thinking levels to choose from having no effort control, and applied with none;
    - in the launch dialog, the same words, no mode, and no effort control for such a model;
  - the permission mode switched live, and stored when nothing runs, recorded as one `mode_requested` event drawn in the history with no attention, a live switch recorded as a request before Claude's confirmation with only the confirmation drawn, the bar showing the request at once;
  - a model and effort applied for Claude and for Codex, the bar showing the request at once while the harness still runs the old ones;
  - the launch dialog preselecting the copy from the settings for each harness, with no inherit choice, and a launch from it holding a concrete request;
  - the bar on one row with the mark, mode, model and effort;
  - while the view follows the bottom, the newest content kept above the dock as it grows:
    - a prompt sent with the reply held keeps its block above the dock once the activity line shows, with the workspace read held so the line arrives after the view went to the end;
    - a docked request;
    - the reply box growing with its draft;
  - scrolled up, neither the activity line nor a docked request moving the view;
  - sent from above five times with the reply held, the prompt row in view above the dock before any output, each time;
  - while following, the reader's wheel during a streaming reply keeping the view where they put it, with "Newer activity below" offered;
  - while following, a move up with no input event at all during a streaming reply, the position set from the page, keeping the view where it was put. That stands in for the find bar and for a scrollbar the page cannot see;
  - `@~/` opening the list of the named directory, shown with `~`, its own files newest first and then a newer one deeper, with nothing from a dot-directory or outside it, and a directory not named never listed; `@~/Screenshots/`, the second named directory, looked in and listing its own files; the pick written per harness, `@"~/Sandbox/shots/shot one.png"` for Claude and the bare absolute path for Codex and pi.

  The dock's steps turn off chromium's scroll anchoring, which re-pins a view at the bottom by itself and so hides the defect; firefox does not. The suite moves the view up with the mouse wheel (`wheel`), as a reader does. Any move up that mega did not make ends following, so a scripted one would do too, and step 7 relies on it. It saves the control bar for each harness, collapsed and open, light and dark, as `shots/bar-{claude,codex}-{collapsed,open}[-dark].png`, and pi's as `shots/bar-pi-{collapsed,small}.png` and `shots/bar-pi-{collapsed,open}-dark.png`, and the `@~/` lists as `shots/path-search-{outside,second}.png`.
- **`readme.mjs`** (5 steps): the README's picture of a workspace. A scratch workspace of three projects, with conversations on all three harnesses in the states the side panel shows: a Claude Code approval waiting, a Codex reply unread, a pi turn running, one read and idle, and one Claude Code conversation holding hand-written turns from `lib/scene.mjs`, so its history reads as real work. It checks that needs-you holds the approval and the unread reply and that the pi turn is running. It then opens the hand-written conversation, unfolds every project, opens the last action, and saves the page at 1440×900 as `shots/workspace.png`. `make readme-image` runs it and copies the picture to `docs/images/workspace.png`.
- **`requests.mjs`** (74 steps): requests. It checks the following:
  - a permission card docked above the reply box, not in the history, with its one-line row there;
  - focus on the card neither opening the box nor moving the card;
  - Allow, and Deny with a reason, each clearing the card, with the answer reaching the turn;
  - answering keeping an open box's draft;
  - two requests stacked oldest first;
  - a question card answering only once a choice is made;
  - a request answered on another page refused on a page still showing it, and by the API;
  - an MCP approval from the fake Codex, allowed for the session or denied;
  - a typed form needing its required field;
  - Interrupt aborting a pending card, with a late answer refused;
  - a question the fake Codex asks in its message, while its turn waits:
    - its card steering the answer into that turn, once, and the question answered;
    - a steer written that Codex never replies to, the fake exiting on it (`askq lose`): the card closes, and the question's row says "Codex did not confirm whether it received your answer", after a reload too. One steer reaches the fake, by its log, one `unconfirmed` resolution is recorded, and a second answer is refused as `not_pending` and sends nothing. The deadline that ends a steer with no reply is a minute, which the adapter's tests shorten; this suite loses the connection instead;
  - the allow keys:
    - Ctrl+Enter answering the oldest approval from the empty reply box, then the next from the page itself, one per press, the Allow that answers naming its key in its title and Deny none;
    - nothing from another field, with a draft in the box, on a repeating key, on a request docked under the press, or with a dialog open;
    - neither key on a question or on a pi dialog;
    - Ctrl+Shift+Enter allowing a Codex MCP tool for the session, and doing nothing on a Claude approval, which offers no such allow;
  - a docked request held while the page is stale, after its stream is dropped and before the workspace is read again: its answers disabled, saying why, and Ctrl+Enter answering nothing; answerable, and answered, once the page has caught up. The drop closes only the page's end of the stream (`reconnectable` in `lib/mega.mjs`), so the service keeps the request pending. It saves the held card as `shots/request-held.png`;
  - pi, on the fake pi:
    - its extension's gate docked as a "Question from a pi extension", with Dismiss and no answer of one's own or allow for the session, and answered;
    - Dismiss reaching pi as its cancel;
    - an input dialog's text and an editor dialog's several prefilled lines;
    - a dialog that expires, its row saying so;
    - an interrupt waiting on a dialog, saying so, until the dialog is dismissed. Meanwhile a chosen answer, the gate's own Block after the request is checked to be the gate, is refused as `conflict` with the request still pending, and the card holds Answer, saying why, with Dismiss still live;
    - a release with a dialog pending, whose dialog warns that pi is ended and the gated command does not run, leaving the dialog's row "never answered". The release dialog is a confirmation: it says Release stops the runtime and the next prompt or Start brings the conversation back on the same session, with no terminal command and no copy control.

  It saves the pi dialog and the release warning as `shots/pi-{dialog,release-warning}.png`, the closed question's history as `shots/question-unconfirmed.png`, and the approval the keys answer, with its Allow's title written under it by the check, since the browser draws a title outside the page, as `shots/approval-key.png`.
- **`shutdown.mjs`** (35 steps): the shutdown gate. Its service runs in `Asia/Kolkata` and its pages in UTC. It checks the following:
  - arming from the footer, with the banner naming what it waits for by count and by name, and the request marked as the operator's;
  - a second page seeing it;
  - new work refused with the reason, on the page and by the API;
  - a blocker's name opening it;
  - Shut down neither offered nor accepted before idle;
  - cancelling lifting it on both pages;
  - at idle, Shut down offered, the title saying so and one notification, each carrying the mega's name;
  - Shut down ending the service with status 0;
  - the page that asked saying when, in the service's zone, while the other shows only what it last knew;
  - the other page, its workspace reads given the refusal a closed service answers in the moment before it stops, `unavailable`, showing the shutdown it last read and no read error;
  - with the footer's longest status, "Shut down from here, retrying", the mega's name beside it shown whole, saved as `shots/foot-shut-down.png`;
  - the asking page, back in touch before its first workspace read of the new service has landed, which the check holds, not reporting the old shutdown as under way; and, the new service armed from elsewhere before that read lands, showing the gate armed afresh, never the old shutdown;
  - both pages recovering when the service returns.

- **`side.mjs`** (76 steps): the side panel and the rail. Its mega is named `side-check` in its config. It checks the following:
  - projects sorted by name ignoring case, the id breaking a tie, and starred first in their own group;
  - project rows bolder than their conversations;
  - folds: a starred project unfolded and any other folded until the operator chooses; his choice kept across a reload and outlasting a star or an unstar; navigation by the needs-you list or Ctrl+/ unfolding without recording a choice; the old folded list carried over as folded choices; a removed project's choice forgotten; a folded project keeping its rollup mark;
  - stars seen by a second page from the stream alone;
  - the panel's width within 200 to 480, kept across a reload;
  - shortcuts: Ctrl+/ from the page and from the reply box, no bare n, Ctrl+' collapsing and expanding from either, typing nothing, and the hint and titles naming them;
  - the rail: the count, one mark per project in tree order, unread output in an unopened conversation updating its mark and the count, the working arc, its width, kept across a reload, and a mark opening the panel on its project;
  - a missing checkout's mark in the tree and on the rail, lifting when it is back;
  - project removal refused while a runtime lives and naming it, then deleting the project's logs;
  - the build: `GET /api/version` naming this page's interface, the footer's version and its panel, with each harness's tested range and the version found (the fake Codex's inside its range, the stand-in Claude Code's unknown and said so, in the notice colour), and when the newest configuration snapshot was taken and where the snapshots are, with nothing of what one holds, the rail's control, and a page older than the service saying so where the connection status is, without reloading;
  - every control in the panel and on the rail with a title and a label;
  - on a first load, before the first workspace read lands, nothing saying Live; after a reconnect, before the workspace read lands, the footer saying "Catching up" and the rail's dot an empty ring, not Live; and Live once it lands. It saves the footer as `shots/foot-catching-up.png`;
  - after a reconnect, a change announced while the read is held: once that read lands with another queued behind it, still "Catching up", not Live;
  - where the operator left off, in two new projects:
    - the glyph on the conversation last instructed in each, one per project, after its name;
    - moved by a prompt to another conversation in the same project, and not by that project's agent waking on its own, nor by a request answered;
    - its title saying when, in the service's zone;
    - the same marker on the project's page, and none on the rail;
    - passed to the next most recently instructed conversation when its own is removed;
    - the project's rows with the marker saved as `shots/last-instructed.png`;
  - the mega's name, `side-check`, as the config sets it: on `GET /api/version` with where it came from, in the tab title, in the footer beside the connection status, in the version panel, and in the rail's title;
  - then, restarted with no name in the config, the machine's hostname in the same places. These steps compare it without printing it, and nothing is saved after them.

Each suite also checks that the page raised no errors and fetched nothing off 127.0.0.1.

The other surfaces' suites were lost with `/tmp` on 2026-11-12. Each is rebuilt by the first batch that changes its surface. The roadmap card `ui-test-runner` listed which lost checks covered which surface; it was retired in commit 32aaefe, and `git show 32aaefe^:docs/future/roadmap/ui-test-runner.md` reads it.

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

`suite(name)` empties the suite's scratch directory and makes its directories, its config and its `HOME`, then starts the service and launches the browser. Its `HOME` has a `.profile` that puts the check's own `node` on `PATH`, since project environments resolve through a login shell from it. The operator's shell start-up files and Claude settings are never read. `suite(name, { bare: true })` sets up a newcomer's machine instead: no fakes and no Claude settings, a config naming no harness path so mega looks where its defaults say, and `node` alone, in a directory of its own, ahead of the system directories on mega's `PATH` and the project's. A bare suite can start no harness, real or fake. `suite(name, { tz })` runs the service in the zone `tz` names, with `TZ`, so a suite can give its pages another and check that times are the service's. `suite(name, { projectsRoot })` sets the register dialog's projects root, `projects_root`. `suite(name, { searchDirs })` names directories beyond the checkout for the `@` search, `path_search.directories`, `~` being the suite's scratch `HOME`. Every suite's mega is named `mega-check` in its config, or `{ megaName }`, so no machine's hostname lands in a screenshot or a step; `{ megaName: false }` leaves the name out, for a step about the default. It returns the following:

- `check(ok, what)` records a step. `must(ok, what)` records one the suite cannot go past: a suite checks with it, before any Allow, that the request is exactly the one the fake raised, so nothing unexpected is approved. `until(what, pred, ms)` waits on a condition, checking every 50 ms, and fails saying what it waited for.
- `api(method, path, body)`, `workspace()`, `projectOf(id)` and `convOf(id)` read and call the service.
- `checkout(name, { git })` makes a scratch checkout. `register(name, path)` adds it as a project, with an override activation that points Claude at the suite's scratch settings, since mega's own environment does not reach the processes it starts. Pass `{ override: false }` for a project whose environment is under test.
- `launch(projectId, harness, prompt, extra)` starts a conversation, and `idle(id)` waits until its turn ends.
- `page({ width, height, colorScheme, init, timezoneId })` opens a page in its own context, in the zone `timezoneId` names when given. `shot(page, name)` saves a screenshot under `shots/`.
- `stop()` asks the service to stop and waits for it; `start()` starts it again on the same data. `stopped()` waits for a service a page shut down, and reports how it exited.
- `done()` adds the page-wide steps, closes everything, prints the summary the runner reads, and exits.

`reconnectable(page)`, exported beside `suite`, lets a step take a page's stream and workspace reads in hand, without stopping the service, so pending requests stay pending. Install it before the page loads. It returns:
- `drop()` closes the page's end of the stream, and holds the workspace reads after it.
- `hold()` holds the reads with the stream left as it is, for a stream that drops by itself, as when the service stops.
- `held()` counts the reads waiting.
- `releaseOne()` lets the oldest go, still holding those after it.
- `catchUp()` lets them all go, and every read after them.

The rules:

- Wait on conditions, never on delays. The one exception is a step that shows a key did nothing: nothing can be waited for, so it gives the page longer than the press needed to act (`settle` in `requests.mjs`) and then checks nothing changed. A step that cannot be made stable is reported, not left flaky.
- Approve, or choose an answer, only where that answer is under test, after a `must` on what is pending, made before the first press that could answer it, including the presses that are meant to answer nothing; where the decision is incidental, deny or dismiss. The Go tests keep the same rule: the service's `rig.looked`, Codex's `askQuestion`, and a look at the sink's payload in each adapter's tests.
- Drive the fakes by the text of each prompt. The fake shim's verbs are in its header, and the fake Codex's in its own; a verb a suite needs is added there.
- Only data goes in scratch. A suite never writes outside its scratch directory, apart from what the service writes there for it.
- Say at the top of the file what the suite checks, and add it to the list above.

`lib/gen.mjs` appends generated turns to a scratch conversation's log, for a suite that needs a long conversation. It is seeded, so the same seed writes the same conversation.
