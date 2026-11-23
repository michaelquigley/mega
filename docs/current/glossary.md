# mega: glossary

The words mega's documents, cards and interface use, and what each one means here. Where a word could be confused with a neighbour, the entry says which is which. `docs/current/mega.md` describes how each thing works; this file only fixes the names.

## The workspace

- **Workspace.** Everything one mega instance holds: its projects, their conversations, and the state about them. It lives in the data directory and is written only by the service.
- **Operator.** The person a mega instance works for: the one who registers projects, sends prompts and answers requests. Mega has one. An **external operator** is anyone other than Michael running their own mega, on their own machine with their own harness logins; the word for the people the install work is for.
- **Project.** The stable home for work: a name and a checkout. Conversations belong to a project and stay with it.
- **Checkout.** The directory a project's work happens in, usually a git repository. Mega never modifies or deletes it on its own account.
- **Conversation.** A durable identity wrapped around one native session of one harness. It keeps its history, name and place when its runtime stops.
- **Harness.** The coding agent program mega drives: Claude Code, Codex or pi. An **adapter** is mega's code for one harness; the **shim** is the small Node program the Claude adapter runs to reach Claude's SDK.
- **Native session.** The harness's own record of a conversation, in the harness's own store. **Discovery** lists the sessions a checkout has that no conversation holds; **adopting** one makes it a conversation.

## A conversation over time

- **Runtime.** The harness process currently serving a conversation. A conversation can exist with no runtime.
- **Start.** Bringing up a runtime without sending anything. **Release** stops a runtime so the session can be used in a terminal. **Resume** acknowledges lost supervision and clears Unsupervised. **View native transcript** separately reads the prompts and replies in the bound native session.
- **Unsupervised.** A conversation whose runtime mega did not see exit, after an unclean restart. The outcome of work mega did not witness stays unknown. Resume acknowledges that gap.
- **Turn.** One stretch of the agent's work, from the prompt that begins it to its end. A turn comes whole: its prompt, its speech, its actions and its requests.
- **Prompt.** What the operator sends. **Speech** is what the agent says back, the replies rendered as prose.
- **Action.** One thing the agent did during a turn, shown as a single expandable row: a shell command, a file edit, a search, an MCP tool. It is the word for what the operator sees. An **action group** is a run of actions between two pieces of speech, indented behind a hairline.
- **Invocation.** What an action invoked, in full: a shell command whole, or any other tool's name and input. Its row shows it when expanded, above the result; the adapter reads it from the recorded tool call, so it is never stored twice.
- **Tool call** and **tool result.** The recorded events underneath an action: what the harness asked for and what came back. Use these words for the event log and the adapters, and "action" for the row on screen.
- **Request.** Something the agent is waiting on the operator for: an **approval** to run something, or a **question** with choices or fields. A request is answerable only while it is pending. A pending request is **docked** above the input panel; its row in the history records that it happened.
- **Extension dialog.** A question a pi extension asks: a choice, a yes or no, a line of text, or a few lines to edit. pi has no permission model of its own, so even a dialog that gates a command is a question, never an approval, and never offered for the session.
- **Allow keys.** Ctrl+Enter, which answers the oldest docked approval with Allow, and Ctrl+Shift+Enter, which answers it with "Allow this tool for the session" where the harness offers that. They work only from an empty reply box or the page itself, the history's column counting as the page, only on an approval, never on a question, and only on a deliberate press. Each is the operator's own answer, given by pressing the card's button. The Allow they press names its key in its title.
- **Dismiss.** The operator's way to close a pi dialog without answering it: pi's own cancel, as Escape is in pi's terminal. mega never dismisses a dialog itself.
- **Request endings.** How a request's row in the history ends: **answered**; **aborted**, settled by an interrupt; **lost**, pending when the service restarted; **expired**, a pi dialog whose own time limit ran out, which pi settled with no answer; **never answered**, a pi dialog still waiting when its runtime was ended, as a release ends pi, whose gated command did not run.
- **Waiting on a dialog.** An interrupt pi cannot act on yet: pi stops a run only once its pending dialog is answered or dismissed, and the history says which dialog the interrupt waits on.
- **Native events.** What a harness emits that mega passes through without interpreting: status, bookkeeping, reasoning summaries. Hidden unless the native events box is ticked, and never counted as output.
- **Background job.** Work an agent left running after its turn ended. It is activity, not attention.
- **MCP server.** A program a harness connects to for more tools, configured in the harness's own settings, as an MCP tool approval's server is. A running conversation reports its servers, each **connected**, **connecting**, **failed**, **needs sign-in** or **disabled**, with its tools. mega shows them and changes nothing: signing in is done in the harness's own terminal. Like a background job, it is state to see, not attention.
- **The recent tail.** The last several turns of a conversation, which is what opening it loads. Earlier history is reached with "Show earlier" or "Show all".

## Attention

- **Attention.** What a conversation asks of the operator, computed by the service and never stored: a pending **request**, **unread** output from a finished turn, **working** while a turn runs, **idle**, an **error**, or **unsupervised**.
- **Needs you.** The list of conversations with a pending request or unread output, across every project.
- **Last instructed.** In each project, the conversation the operator most recently sent a prompt to through mega: where he left off. A small muted glyph after its name marks it, in the side panel's tree and on the project's page, with when as its title. It is a fact about the operator, not attention: nothing an agent does moves it.
- **Reading position.** How far the operator has read in a conversation. It moves only for what is on screen: opening a conversation moves it, and so does new output while the view follows the newest output. Scrolled up, behind a dialog, or in a tab that is not visible, it waits, and moves when the newest output is on screen again. It never answers a request.
- **Mark.** The small shape that shows a conversation's attention, in the side panel, the rail and the needs-you list. A project's mark shows the most urgent state among its conversations. The **harness mark**, a vendor's logo on the control bar saying which harness a conversation runs on, is not attention. Nor are the control bar's environment mark and the dot on More for an MCP server that needs a look.

## The interface

- **Side panel.** The left column: the needs-you list and the project tree. Collapsed, it is the **rail**.
- **Conversation view.** The main column: the header, the history, and the input panel. The **header** is identity, rarely consulted, and scrolls away with the history: name, label, state, harness and version, the environment's fingerprint and names, checkout and session id.
- **Input panel.** The reply box and the **control bar** beneath it, where the conversation's controls cluster. The control bar is always in view, so it carries what the operator acts on or needs to notice:
  - the harness mark;
  - the **request**: the mode, model and effort, each shown closed as a short value;
  - an environment that needs attention;
  - the More menu, with the MCP servers.
- **Projects root.** Where the register dialog's folder picker starts, `projects_root` in the config, else the home directory. The picker lists and completes only folders inside it, and never goes above it; a path typed in full is registered wherever it is.
- **Look.** How a session not yet adopted began and how it left off: its first prompt, its last, and the end of its last reply, read by its harness's adapter when the operator picks it in the adopt dialog. It is a read of a private transcript, bounded, shown only there, and it changes nothing.
- **Named directories.** The directories the operator names in the config, `path_search.directories`, such as `~/Sandbox`, that the reply box's `@` search reaches beyond the checkout, for an `@` word starting with `~/` or `/`. Nothing outside them is ever listed.
- **Activity line.** The line above the input panel that says what the conversation is doing and for how long.
- **Request.** What a conversation asks its harness to run with: a model, an effort, and for Claude a permission mode. It is always concrete, copied once from the harness's settings when the conversation is created, and passed on every launch; only the operator changes it after that. The control bar shows the request; there is no inherit.
- **Turn line.** The compact line on a turn saying what it ran with, as the harness reported it, then when its prompt was sent and how long the turn took, or for the turn in flight when it started. A turn with no record of what it ran with has its time alone. It shows while the pointer is over the turn or focus is inside it, and stays, in the notice colour, when the turn ran with something other than its request.
- **Fill.** The one-time, recorded completion of a conversation created before every request was concrete: from what its log last reported, or at its next launch from the settings.
- **The service's zone.** The time zone the service runs in, which the page says every time in, whatever zone the browser reports. A hardened browser may report UTC.
- **Permission mode.** How a Claude conversation treats approvals: ask each time (Claude's `default`, shown as **ask**), let the classifier decide, and so on.

## A project's environment

- **Project environment.** The shell environment a project's work runs in: the variables every process mega starts for that project is given. It is resolved fresh at each runtime start from the operator's shell, never from the environment mega itself was started in.
- **Activation.** The short shell snippet that turns a plain shell environment into a project's, such as `devprj`. It is defined in the project's own repository, in `.mega/environment.yaml`.
- **Approval.** The operator's agreement that mega may run a repository's activation. It is tied to the file's exact content by its full SHA-256 digest, so a changed file is not run until it has been approved again. It covers that text only: an activation that reads other files in the repository is trusting them. So the approval also remembers the names of the variables the activation changed, and an activation that later changes others is refused until approved again.
- **Override.** A local instruction on a project to ignore the repository's file or to run a different activation. Workspace state, not repository content.
- **Fingerprint.** A short digest, the first 16 hex digits of a SHA-256, that identifies a file's content or a resolved environment for display, without revealing it. Mega records fingerprints and variable names, never values.
- **Digest.** The full SHA-256 of a file's content, what an approval is bound to. The fingerprint is its short form.

## Running mega

- **Service.** The mega process. **Daemon** is the same process run from the tray; the **launcher** is the desktop entry that starts it.
- **The gate.** "Shut down when idle": once armed, no new work can start, and the shutdown is offered when every agent is at rest.
- **Operator.** The person using mega. There is one.
- **Environment.** In the spec, the place a service runs: a workstation today, possibly a container later. Not to be confused with a project environment, above.
- **Tested range.** The lowest and highest build of a harness between which every build the probe in `probes/versions/` ran passed its steps: what that probe ran, not a promise that every call mega makes works on every build in it. The capability report lists the builds and steps. The ranges: Claude Code 2.1.318 to 2.1.326, Codex 0.163.0 to 0.169.0. A build outside it still runs. A **newer** one is the normal state, since Claude Code updates itself every day or two, and is said plainly. An **older** one is marked, on the "Runtime started" line too. A version that could not be read is **unknown**. Not to be confused with a pinned version, which mega no longer has for a harness; the shim's Agent SDK is still pinned.
- **Installed whole.** A Codex with the helper programs its package puts beside it, as npm or the release's `codex-package` archive installs it. A **lone binary**, the release's bare `codex` archive, cannot run a shell command, and mega refuses it as missing.
- **Snapshot.** A dated copy of each harness's configuration that mega keeps under its data directory, in `snapshots/`, taken when the configuration changed or a harness version did. It is configuration only, by each harness's **include list**: never a credential, transcript, session, history, database or cache. Restoring from one is the operator's, with the commands `mega snapshots diff` prints. Not to be confused with a shell snapshot, a harness's own record of the shell it runs commands in.
