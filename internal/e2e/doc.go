// Package e2e holds the harness round trips run by `make e2e`. they drive the
// real harnesses against disposable fixtures with the operator's logins, so
// they sit behind the e2e build tag and never run in the ordinary gate.
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
//
// MEGA_E2E_NODE, MEGA_E2E_CLAUDE, MEGA_E2E_CODEX and the *_MODEL variables
// override the executables and models used.
package e2e
