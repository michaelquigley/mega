.DEFAULT_GOAL := build
GOBIN ?= $(shell go env GOPATH)/bin

ifeq ($(filter-out /,$(abspath $(GOBIN))),)
$(error GOBIN is '$(GOBIN)'; it must name a real directory)
endif

# the claude shim runs from the checkout (it needs its node_modules), so the
# installed binary is stamped with where to find it; claude.shim overrides.
SHIM_DIR := $(CURDIR)/shim/claude
LDFLAGS := -X github.com/michaelquigley/mega/internal/config.defaultClaudeShim=$(SHIM_DIR)

.PHONY: build test e2e checks checks-deps checks-browser clean frontend frontend-test shim readme-image app

# the macOS application, built by cmd/mega/macos/app.sh.
APP ?= build/mega.app

# browser checks keep their binary, data, logs and screenshots here.
CHECKS_DIR ?= $(or $(TMPDIR),/tmp)/mega-checks

# build the embedded single-page UI into ui/dist.
frontend:
	npm --prefix ui install
	npm --prefix ui run build

frontend-test: frontend
	npm --prefix ui run lint

# install the shim's pinned agent sdk exactly as locked.
shim:
	npm --prefix shim/claude ci

# depends on frontend so go:embed all:dist always has content.
build: frontend shim
	go install -ldflags "$(LDFLAGS)" ./...

# the macOS application: a menu bar app that runs the daemon, with its own
# window and the claude shim inside it. macOS only; copy it to /Applications
# or ~/Applications to install it.
app: frontend
	sh cmd/mega/macos/app.sh "$(APP)"

# the full gate includes the frontend because the shipped Go build embeds it.
test: frontend-test shim
	npm --prefix shim/claude test
	go test ./... -count=1
	go vet ./...

# harness round trips against the disposable fixtures. these drive the real
# claude code with the operator's login and cost model turns; run by hand.
# the fixtures, and any other setting internal/e2e/doc.go names, come from
# the environment or from e2e.local.mk, an optional, git-ignored file of make
# assignments beside this one (MEGA_E2E_CLAUDE_FIXTURE ?= /path, so a value in
# the environment still wins). only those set are passed on; a round trip
# whose fixture is unset is skipped.
-include e2e.local.mk
E2E_VARS := MEGA_E2E_CLAUDE_FIXTURE MEGA_E2E_CODEX_FIXTURE MEGA_E2E_PI_FIXTURE MEGA_E2E_NODE MEGA_E2E_CLAUDE MEGA_E2E_CODEX MEGA_E2E_PI MEGA_E2E_CLAUDE_MODEL MEGA_E2E_CODEX_MODEL MEGA_E2E_PI_MODEL
E2E_ENV = $(foreach v,$(E2E_VARS),$(if $($(v)),$(v)='$($(v))'))

e2e: shim
	$(E2E_ENV) go test -tags e2e -count=1 -v -timeout 30m ./internal/e2e/

# the browser checks: every suite in checks/, or SUITE=name (or a list), each
# against its own scratch mega on a scratch port with the committed fakes.
# its own target, not part of test: it needs a browser and takes minutes.
checks: frontend shim checks-deps
	mkdir -p "$(CHECKS_DIR)"
	go build -o "$(CHECKS_DIR)/mega" ./cmd/mega
	MEGA_CHECKS_DIR="$(CHECKS_DIR)" MEGA_CHECK_BIN="$(CHECKS_DIR)/mega" node checks/run.mjs $(SUITE)

# the checks' pinned playwright, exactly as locked.
# the README's picture of a workspace: the readme suite builds a scratch one
# with the fakes and saves it, and this copies it into the repository.
readme-image:
	$(MAKE) checks SUITE=readme
	mkdir -p docs/images
	cp "$(CHECKS_DIR)/readme/shots/workspace.png" docs/images/workspace.png

checks-deps:
	npm --prefix checks ci

# the chromium build the pinned playwright drives, into playwright's own
# cache (~/.cache/ms-playwright); run once, and again when playwright moves.
checks-browser: checks-deps
	npm --prefix checks exec -- playwright install chromium

clean:
	go clean ./...
	rm -f "$(GOBIN)/mega"
	rm -rf build
	rm -rf ui/dist ui/node_modules shim/claude/node_modules checks/node_modules
