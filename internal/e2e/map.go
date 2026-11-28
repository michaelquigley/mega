package e2e

import "github.com/michaelquigley/mega/internal/model"

// the map: what admitting a harness build needs, entry by entry, and the
// round trips that make each entry against the real harness, or why none
// does. a build is admitted to its harness's tested range when every round
// trip this map names for that harness passed on it and none was skipped;
// `make e2e` ends by saying where each range stands (summary_test.go).
//
// the lists are finite, and the ordinary gate keeps them (map_test.go): every
// method of the adapter interfaces in internal/harness has a row for each
// harness, and every round trip a row names exists. the version drivers'
// steps and the pi probes' scenarios are rows too, so that what admits a
// build is no less than what admitted one before `make e2e` did, except where
// a row says it checks less, and why.
//
// a change to what mega asks of a harness adds or changes its round trip, and
// its row here.

// Row is one entry of what admission needs, for one harness.
type Row struct {
	Harness string
	// List is where the entry comes from: an interface of internal/harness,
	// or a version driver, or the pi probes.
	List string
	// Entry is the method or callback, the driver's step, or the scenario.
	Entry string
	// By names the round trips that make it against the real harness.
	By []string
	// None says why no round trip makes it, where By is empty.
	None string
}

// the lists. the interfaces' are checked against internal/harness by the
// ordinary gate; the others are read off the drivers and the pi probes.
const (
	ListHarness      = "harness.Harness"
	ListRuntime      = "harness.Runtime"
	ListSink         = "harness.Sink"
	ListLooker       = "harness.Looker"
	ListLocator      = "harness.Locator"
	ListVersioner    = "harness.Versioner"
	ListAnswers      = "harness.Answer decisions"
	ListClaudeDriver = "probes/versions/driver.mjs"
	ListCodexDriver  = "probes/versions/codex-driver.mjs"
	ListPiProbes     = "probes/pi/rerun.sh, close-with-dialog.mjs"
)

const (
	hClaude = model.HarnessClaude
	hCodex  = model.HarnessCodex
	hPi     = model.HarnessPi
)

func by(harnessName, list, entry string, tests ...string) Row {
	return Row{Harness: harnessName, List: list, Entry: entry, By: tests}
}

func none(harnessName, list, entry, why string) Row {
	return Row{Harness: harnessName, List: list, Entry: entry, None: why}
}

// the reasons given more than once.
const (
	noModes       = "the service refuses a permission mode for any harness but Claude (checkPermissionMode) before the adapter is asked"
	piNoMCP       = "the pi adapter never calls it: pi gives its MCP servers' state only as the text answering /mcp"
	piNoJobs      = "the pi adapter never calls it: pi reports no work kept running outside a turn"
	probeOnly     = "the version probe's own: it exists so that a build that is not installed runs safely, and a round trip runs the installed build"
	bySummary     = "made by mega's own start-up check in every round trip's service, as the summary `make e2e` ends with reads it"
	piQuestions   = "a pi dialog is a question, answered with one of its options, and never an approval; the gate's Block option is not sent by any round trip"
	onlyPiDismiss = "only a pi dialog offers it"
	locatorNames  = "reads names and modes only, and is made by mega's own start-up check in every round trip's service"
)

// Map is every entry, harness by harness.
var Map = []Row{
	// harness.Harness
	by(hClaude, ListHarness, "Name", "TestClaudeRoundTrip"),
	by(hCodex, ListHarness, "Name", "TestCodexRoundTrip"),
	by(hPi, ListHarness, "Name", "TestPiRoundTrip"),
	by(hClaude, ListHarness, "Launch", "TestClaudeRoundTrip", "TestClaudeStartWithoutPrompt", "TestClaudeModelAndEffort", "TestClaudeAutoMode", "TestEachProjectRunsInItsOwnEnvironment"),
	by(hCodex, ListHarness, "Launch", "TestCodexRoundTrip", "TestCodexStartWithoutPrompt", "TestCodexModelAndEffort", "TestEachProjectRunsInItsOwnEnvironment"),
	by(hPi, ListHarness, "Launch", "TestPiRoundTrip", "TestPiModelSwitchWhileIdle", "TestPiResumesAfterPiIsKilled"),
	by(hClaude, ListHarness, "History", "TestClaudeRoundTrip", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption"),
	by(hCodex, ListHarness, "History", "TestCodexRoundTrip"),
	by(hPi, ListHarness, "History", "TestPiRoundTrip", "TestPiDiscoveryAndAdoption"),
	by(hClaude, ListHarness, "Discover", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption", "TestClaudeModelSwitchWhileIdle"),
	by(hCodex, ListHarness, "Discover", "TestCodexRoundTrip"),
	by(hPi, ListHarness, "Discover", "TestPiDiscoveryAndAdoption", "TestPiRoundTrip"),
	by(hClaude, ListHarness, "Models", "TestClaudeModelAndEffort", "TestClaudeModelSwitchWhileIdle"),
	by(hCodex, ListHarness, "Models", "TestCodexModelAndEffort", "TestCodexModelSwitchWhileIdle"),
	by(hPi, ListHarness, "Models", "TestPiRoundTrip", "TestPiModelSwitchWhileIdle"),
	by(hClaude, ListHarness, "Settings", "TestClaudeModelAndEffort"),
	by(hCodex, ListHarness, "Settings", "TestCodexModelAndEffort"),
	by(hPi, ListHarness, "Settings", "TestPiRoundTrip"),
	by(hClaude, ListHarness, "Check", "TestClaudeRoundTrip"),
	by(hCodex, ListHarness, "Check", "TestCodexRoundTrip"),
	by(hPi, ListHarness, "Check", "TestPiRoundTrip"),

	// harness.Runtime
	by(hClaude, ListRuntime, "NativeID", "TestClaudeRoundTrip"),
	by(hCodex, ListRuntime, "NativeID", "TestCodexRoundTrip"),
	by(hPi, ListRuntime, "NativeID", "TestPiRoundTrip"),
	by(hClaude, ListRuntime, "Prompt", "TestClaudeRoundTrip"),
	by(hCodex, ListRuntime, "Prompt", "TestCodexRoundTrip"),
	by(hPi, ListRuntime, "Prompt", "TestPiRoundTrip"),
	by(hClaude, ListRuntime, "Answer", "TestClaudeRoundTrip", "TestClaudeDeny"),
	by(hCodex, ListRuntime, "Answer", "TestCodexRoundTrip", "TestCodexMCPApproval", "TestCodexDeny"),
	by(hPi, ListRuntime, "Answer", "TestPiRoundTrip", "TestPiInterruptWaitsOnADialog"),
	by(hClaude, ListRuntime, "Interrupt", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption", "TestClaudeModeSwitch"),
	by(hCodex, ListRuntime, "Interrupt", "TestCodexRoundTrip", "TestCodexReleaseDuringCommand"),
	by(hPi, ListRuntime, "Interrupt", "TestPiInterruptsARunningCommand", "TestPiInterruptWaitsOnADialog"),
	by(hClaude, ListRuntime, "SetPermissionMode", "TestClaudeModeSwitch", "TestClaudeStartWithoutPrompt"),
	none(hCodex, ListRuntime, "SetPermissionMode", noModes),
	none(hPi, ListRuntime, "SetPermissionMode", noModes),
	by(hClaude, ListRuntime, "SetModel", "TestClaudeModelSwitchWhileIdle"),
	by(hCodex, ListRuntime, "SetModel", "TestCodexModelSwitchWhileIdle"),
	by(hPi, ListRuntime, "SetModel", "TestPiModelSwitchWhileIdle"),
	by(hClaude, ListRuntime, "Stopping", "TestClaudeRoundTrip"),
	by(hCodex, ListRuntime, "Stopping", "TestCodexRoundTrip", "TestCodexReleaseDuringCommand"),
	by(hPi, ListRuntime, "Stopping", "TestPiRoundTrip", "TestPiReleaseWithADialogPending"),
	by(hClaude, ListRuntime, "Close", "TestClaudeRoundTrip"),
	by(hCodex, ListRuntime, "Close", "TestCodexRoundTrip", "TestCodexReleaseDuringCommand"),
	by(hPi, ListRuntime, "Close", "TestPiRoundTrip", "TestPiReleaseWithADialogPending"),

	// harness.Sink, the callbacks an adapter makes.
	by(hClaude, ListSink, "Event", "TestClaudeRoundTrip"),
	by(hCodex, ListSink, "Event", "TestCodexRoundTrip"),
	by(hPi, ListSink, "Event", "TestPiRoundTrip"),
	by(hClaude, ListSink, "TurnStarted", "TestClaudeRoundTrip"),
	by(hCodex, ListSink, "TurnStarted", "TestCodexRoundTrip"),
	by(hPi, ListSink, "TurnStarted", "TestPiRoundTrip"),
	by(hClaude, ListSink, "Delta", "TestClaudeRoundTrip"),
	by(hCodex, ListSink, "Delta", "TestCodexRoundTrip"),
	by(hPi, ListSink, "Delta", "TestPiRoundTrip"),
	by(hClaude, ListSink, "Request", "TestClaudeRoundTrip", "TestClaudeAutoMode"),
	by(hCodex, ListSink, "Request", "TestCodexRoundTrip", "TestCodexMCPApproval"),
	by(hPi, ListSink, "Request", "TestPiRoundTrip"),
	by(hClaude, ListSink, "Resolved", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption"),
	by(hCodex, ListSink, "Resolved", "TestCodexRoundTrip"),
	by(hPi, ListSink, "Resolved", "TestPiReleaseWithADialogPending"),
	// claude-rt1 asks before Bash(sleep *), so the job is a script of the
	// test's own that a narrow allow rule lets run: no request is answered.
	by(hClaude, ListSink, "Background", "TestClaudeBackgroundJob"),
	by(hCodex, ListSink, "Background", "TestCodexBackgroundJob"),
	none(hPi, ListSink, "Background", piNoJobs),
	by(hClaude, ListSink, "MCPServers", "TestClaudeRoundTrip"),
	by(hCodex, ListSink, "MCPServers", "TestCodexRoundTrip"),
	none(hPi, ListSink, "MCPServers", piNoMCP),
	by(hClaude, ListSink, "Exited", "TestClaudeRoundTrip"),
	by(hCodex, ListSink, "Exited", "TestCodexRoundTrip", "TestCodexReleaseDuringCommand"),
	by(hPi, ListSink, "Exited", "TestPiRoundTrip", "TestPiResumesAfterPiIsKilled"),

	// the decisions an answer carries to the harness (harness.Answer).
	by(hClaude, ListAnswers, "allow", "TestClaudeRoundTrip", "TestClaudeAutoMode"),
	by(hCodex, ListAnswers, "allow", "TestCodexRoundTrip"),
	none(hPi, ListAnswers, "allow", piQuestions),
	by(hClaude, ListAnswers, "deny", "TestClaudeDeny"),
	by(hCodex, ListAnswers, "deny", "TestCodexDeny"),
	none(hPi, ListAnswers, "deny", piQuestions),
	none(hClaude, ListAnswers, "allow_session", "no Claude request offers it; only Codex's MCP approvals do"),
	by(hCodex, ListAnswers, "allow_session", "TestCodexMCPApproval"),
	none(hPi, ListAnswers, "allow_session", "no pi dialog offers it, which the round trips assert of every dialog"),
	none(hClaude, ListAnswers, "dismiss", onlyPiDismiss),
	none(hCodex, ListAnswers, "dismiss", onlyPiDismiss),
	by(hPi, ListAnswers, "dismiss", "TestPiInterruptWaitsOnADialog"),

	// harness.Looker: History plus LookOf in every adapter, made before each
	// adoption as the adopt dialog makes it.
	by(hClaude, ListLooker, "Look", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption", "TestClaudeModelSwitchWhileIdle"),
	by(hCodex, ListLooker, "Look", "TestCodexRoundTrip"),
	by(hPi, ListLooker, "Look", "TestPiRoundTrip", "TestPiDiscoveryAndAdoption"),

	// harness.Locator and harness.Versioner: no harness process runs a
	// conversation for them, so no round trip of their own.
	none(hClaude, ListLocator, "Locate", locatorNames),
	none(hCodex, ListLocator, "Locate", locatorNames),
	none(hPi, ListLocator, "Locate", locatorNames),
	none(hClaude, ListLocator, "Path", locatorNames),
	none(hCodex, ListLocator, "Path", locatorNames),
	none(hPi, ListLocator, "Path", locatorNames),
	none(hClaude, ListVersioner, "Version", bySummary),
	none(hCodex, ListVersioner, "Version", bySummary),
	none(hPi, ListVersioner, "Version", bySummary),

	// the Claude Code version driver's steps.
	none(hClaude, ListClaudeDriver, "mega's start-up check: the version and where it stands against the range", bySummary),
	none(hClaude, ListClaudeDriver, "the override that switches the updater off, and the project resolved from it", probeOnly),
	by(hClaude, ListClaudeDriver, "the model list, with its count and version", "TestClaudeModelAndEffort"),
	by(hClaude, ListClaudeDriver, "a launch, the marker turn, and the started facts", "TestClaudeRoundTrip", "TestClaudeStartWithoutPrompt"),
	by(hClaude, ListClaudeDriver, "an approval for Bash `sleep 3`, raised in default mode, asserted before Allow, the turn waiting on it", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "a late answer refused as not pending", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "an interrupt with an approval pending: the turn ends interrupted, the request aborted, nothing pending", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption"),
	by(hClaude, ListClaudeDriver, "a live switch to plan and back, each confirmed", "TestClaudeStartWithoutPrompt"),
	by(hClaude, ListClaudeDriver, "MCP status reported", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "a start without a prompt: the same session, no turn, nothing imported, no message", "TestClaudeStartWithoutPrompt"),
	by(hClaude, ListClaudeDriver, "a prompt answered in the resumed session", "TestClaudeStartWithoutPrompt"),
	by(hClaude, ListClaudeDriver, "a model switch while idle, confirmed, then a turn on the new model", "TestClaudeModelSwitchWhileIdle"),
	by(hClaude, ListClaudeDriver, "an exact-id resume that remembers, importing no event", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "no turn's line reports a difference", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "every start reports one version", "TestClaudeRoundTrip"),
	by(hClaude, ListClaudeDriver, "the session adopted again: the known reply as speech, Claude's synthetic interruption not", "TestClaudeInterruptedTurnStaysOutOfSpeechOnAdoption"),
	by(hClaude, ListClaudeDriver, "the session adopted again: its prompts exactly the two sent, so Claude's local /model command is not shown as a prompt, and the reply after the switch is speech; less than the driver, which also looked for the command in the assistant's speech by Claude's own markup, which a round trip does not read (protocol-confined-to-adapters)", "TestClaudeModelSwitchWhileIdle"),

	// the Codex version driver's steps.
	none(hCodex, ListCodexDriver, "mega's start-up check: the version and where it stands against the range", bySummary),
	none(hCodex, ListCodexDriver, "the guard: a scratch CODEX_HOME from the override before anything runs Codex, and the runtime started in it", probeOnly),
	by(hCodex, ListCodexDriver, "the model list, and what a new conversation copies", "TestCodexModelAndEffort"),
	by(hCodex, ListCodexDriver, "a launch, the marker turn, and the started facts", "TestCodexRoundTrip", "TestCodexStartWithoutPrompt"),
	by(hCodex, ListCodexDriver, "an approval for a write outside the checkout: nothing written before Allow, the turn waiting, the file written after", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "an interrupt with an approval pending: interrupted, aborted, nothing written", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "the MCP approval for fixture/echo: not run before it is allowed, run after; allowed for the session, which takes Allow's place, so a one-time Allow on an MCP approval is not sent", "TestCodexMCPApproval"),
	by(hCodex, ListCodexDriver, "MCP status reported", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "a start without a prompt: the same thread, no turn, nothing imported, the conversation's model", "TestCodexStartWithoutPrompt"),
	by(hCodex, ListCodexDriver, "a prompt answered in the resumed thread", "TestCodexStartWithoutPrompt"),
	by(hCodex, ListCodexDriver, "a model switch while idle, stored for the next prompt, whose turn runs on it", "TestCodexModelSwitchWhileIdle"),
	by(hCodex, ListCodexDriver, "an exact-id resume that remembers, importing no event", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "no turn's line reports a difference", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "every start reports one version", "TestCodexRoundTrip"),
	by(hCodex, ListCodexDriver, "--discover: the run's thread discovered and adopted, its history imported", "TestCodexRoundTrip"),

	// the pi probes' scenarios.
	by(hPi, ListPiProbes, "P1-P3: a new session, the marker, a tool call, the ask_user question, the gate for sleep 3 answered after 4 s", "TestPiRoundTrip"),
	// the markers are recalled in TestPiRoundTrip's resume; the gate is
	// raised again in a reopened process in TestPiReleaseWithADialogPending,
	// which asserts it and leaves it unanswered.
	by(hPi, ListPiProbes, "P4: the session reopened by exact id, the markers recalled, and the gate raised again in the reopened process, not answered", "TestPiRoundTrip", "TestPiReleaseWithADialogPending"),
	by(hPi, ListPiProbes, "R1: an abort while an ungated command runs, then the next turn", "TestPiInterruptsARunningCommand"),
	by(hPi, ListPiProbes, "R2: an abort while the gate is pending: it waits on the dialog, the command never runs", "TestPiInterruptWaitsOnADialog"),
	by(hPi, ListPiProbes, "R3: a dialog never answered: the turn waits, and a release ends pi", "TestPiRoundTrip", "TestPiReleaseWithADialogPending"),
	// R4 kills pi's owner, the probe, which a round trip cannot do: the owner
	// is mega, in the round trip's own process. so the owner's death stays
	// the probe's, and the round trip kills pi itself, the one process whose
	// parent is the test's own and whose working directory is the fixture. it
	// is not found by its session id, since pi 1.6.1's process title replaces
	// its arguments in /proc (capability report, 2026-11-27), nor by its name.
	none(hPi, ListPiProbes, "R4: pi's owner killed mid-command, and what pi and its command do then", "a round trip cannot kill its own process, which is pi's owner; the probe's R4 observes it"),
	by(hPi, ListPiProbes, "R4, as a round trip makes it: pi killed mid-command, the runtime seen to exit, and the next prompt resuming that session by its exact id and remembering the command it was asked for", "TestPiResumesAfterPiIsKilled"),
	none(hPi, ListPiProbes, "P7: two processes on one session id", "a fault in pi that mega makes no call about: mega holds one runtime per conversation, which the ordinary gate tests, and the observation stays with the pi probes"),
	by(hPi, ListPiProbes, "close-with-dialog: input closed, after an abort, with the gate pending", "TestPiReleaseWithADialogPending"),
}
