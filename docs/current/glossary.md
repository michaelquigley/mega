# mega: glossary

The words mega's documents, cards and interface use, and what each one means here. Where a word could be confused with a neighbour, the entry says which is which. `docs/current/mega.md` describes how each thing works; this file only fixes the names.

## The workspace

- **Workspace.** Everything one mega instance holds: its projects, their conversations, and the state about them. It lives in the data directory and is written only by the service.
- **Project.** The stable home for work: a name and a checkout. Conversations belong to a project and stay with it.
- **Checkout.** The directory a project's work happens in, usually a git repository. Mega never modifies or deletes it on its own account.
- **Conversation.** A durable identity wrapped around one native session of one harness. It keeps its history, name and place when its runtime stops.
- **Harness.** The coding agent program mega drives: Claude Code or Codex. An **adapter** is mega's code for one harness; the **shim** is the small Node program the Claude adapter runs to reach Claude's SDK.
- **Native session.** The harness's own record of a conversation, in the harness's own store. **Discovery** lists the sessions a checkout has that no conversation holds; **adopting** one makes it a conversation.

## A conversation over time

- **Runtime.** The harness process currently serving a conversation. A conversation can exist with no runtime.
- **Start.** Bringing up a runtime without sending anything. **Release** stops a runtime so the session can be used in a terminal. **Resume** re-reads native history after a terminal visit or lost supervision.
- **Unsupervised.** A conversation whose runtime mega did not see exit, after an unclean restart. What it did meanwhile is unknown until it is resumed.
- **Turn.** One stretch of the agent's work, from the prompt that begins it to its end. A turn comes whole: its prompt, its speech, its actions and its requests.
- **Prompt.** What the operator sends. **Speech** is what the agent says back, the replies rendered as prose.
- **Action.** One thing the agent did during a turn, shown as a single expandable row: a shell command, a file edit, a search, an MCP tool. It is the word for what the operator sees. An **action group** is a run of actions between two pieces of speech, indented behind a hairline.
- **Tool call** and **tool result.** The recorded events underneath an action: what the harness asked for and what came back. Use these words for the event log and the adapters, and "action" for the row on screen.
- **Request.** Something the agent is waiting on the operator for: an **approval** to run something, or a **question** with choices or fields. A request is answerable only while it is pending. A pending request is **docked** above the input panel; its row in the history records that it happened.
- **Native events.** What a harness emits that mega passes through without interpreting: status, bookkeeping, reasoning summaries. Hidden unless the native events box is ticked, and never counted as output.
- **Background job.** Work an agent left running after its turn ended. It is activity, not attention.
- **The recent tail.** The last several turns of a conversation, which is what opening it loads. Earlier history is reached with "Show earlier" or "Show all".

## Attention

- **Attention.** What a conversation asks of the operator, computed by the service and never stored: a pending **request**, **unread** output from a finished turn, **working** while a turn runs, **idle**, an **error**, or **unsupervised**.
- **Needs you.** The list of conversations with a pending request or unread output, across every project.
- **Reading position.** How far the operator has read in a conversation. Opening a conversation moves it; it never answers a request.
- **Mark.** The small shape that shows a conversation's attention, in the side panel, the rail and the needs-you list. A project's mark shows the most urgent state among its conversations.

## The interface

- **Side panel.** The left column: the needs-you list and the project tree. Collapsed, it is the **rail**.
- **Conversation view.** The main column: the header, the history, and the input panel.
- **Input panel.** The reply box and the **control bar** beneath it, where the conversation's controls cluster.
- **Activity line.** The line above the input panel that says what the conversation is doing and for how long.
- **Permission mode.** How a Claude conversation treats approvals: ask each time, let the classifier decide, and so on. **Inherit** means the harness uses the operator's own settings, for the mode and for model and effort alike.

## A project's environment

- **Project environment.** The shell environment a project's work runs in: the variables every process mega starts for that project is given. It is resolved fresh at each runtime start from the operator's shell, never from the environment mega itself was started in.
- **Activation.** The short shell snippet that turns a plain shell environment into a project's, such as `devprj`. It is defined in the project's own repository, in `.mega/environment.yaml`.
- **Approval.** The operator's agreement that mega may run a repository's activation. It is tied to the file's exact content, so a changed file is not run until it has been approved again.
- **Override.** A local instruction on a project to ignore the repository's file or to run a different activation. Workspace state, not repository content.
- **Fingerprint.** A short digest that identifies a file's content or a resolved environment without revealing it. Mega records fingerprints and variable names, never values.

## Running mega

- **Service.** The mega process. **Daemon** is the same process run from the tray; the **launcher** is the desktop entry that starts it.
- **The gate.** "Shut down when idle": once armed, no new work can start, and the shutdown is offered when every agent is at rest.
- **Operator.** The person using mega. There is one.
- **Environment.** In the spec, the place a service runs: a workstation today, possibly a container later. Not to be confused with a project environment, above.
