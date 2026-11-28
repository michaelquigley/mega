// Package e2e holds the harness round trips run by `make e2e`. they drive the
// real harnesses against disposable fixtures with the operator's logins, so
// they sit behind the e2e build tag and never run in the ordinary gate.
//
// a harness build is admitted to its tested range (harness.Tested) when
// every round trip map.go names for its harness passed on it and none was
// skipped. map.go is the map of what admission needs: every method and
// callback of the adapter interfaces, every decision an answer carries, every
// step of the version drivers and every scenario of the pi probes, each with
// the round trips that make it or why none does. map_test.go, which has no
// build tag, is the ordinary gate's hold on it. `make e2e` ends by saying
// where each range stands (summary_test.go).
//
// each test takes its fixture checkout from the environment and is skipped
// when it is unset:
//
//	MEGA_E2E_CLAUDE_FIXTURE  a git checkout whose CLAUDE.md names a marker
//	                         token, with fixture.txt and a project
//	                         .claude/settings.json that asks before Bash(sleep *)
//	MEGA_E2E_CODEX_FIXTURE   a git checkout whose AGENTS.md names the same
//	                         marker token, with fixture.txt and the fixture MCP
//	                         server in .codex/config.toml
//	MEGA_E2E_PI_FIXTURE      a git checkout whose AGENTS.md names the same
//	                         marker token, with fixture.txt and an extension in
//	                         .pi/extensions that asks before any bash command
//	                         containing "sleep" and gives the model an ask_user
//	                         question. it must be trusted in pi's own terminal
//	                         once (run pi in it and accept); mega never trusts
//	                         a checkout for pi. the pi round trips run on pi's
//	                         own default model, and skip when it does not
//	                         answer; the idle model switch then moves to
//	                         MEGA_E2E_PI_MODEL, by default the hosted
//	                         openai-codex/gpt-6-sol, Michael's choice, and skips where pi does
//	                         not list it
//
// `make e2e` also reads them from e2e.local.mk, an optional, git-ignored
// file of make assignments beside the Makefile, and passes on only those set.
//
// MEGA_E2E_NODE, MEGA_E2E_CLAUDE, MEGA_E2E_CODEX, MEGA_E2E_PI and the *_MODEL variables
// override the executables and models used. Claude Code defaults to what mega
// runs by default, config.DefaultClaudeExecutable, found as mega finds it.
//
// TestClaudeBackgroundJob, for its life, writes bgprobe.sh into claude-rt1,
// a script that sleeps ten seconds, and a .claude/settings.local.json
// allowing exactly Bash(./bgprobe.sh), and puts back whatever those paths
// held when it ends. it approves no harness request. the pi and Codex round
// trips write files of their own beside or in their fixtures (the commands'
// marker files, the held files R1 and R4 follow) and remove them.
//
// TestEachProjectRunsInItsOwnEnvironment needs both fixtures and devprj on
// the interactive shell's PATH. for its life it writes a scratch .devprj, a
// .mega/environment.yaml naming devprj and envprobe.sh into each fixture,
// and into claude-rt1 a .claude/settings.local.json allowing exactly
// Bash(./envprobe.sh). it approves the environment file in its own scratch
// workspace, approves no harness request, and puts back whatever those paths
// held when it ends.
package e2e
