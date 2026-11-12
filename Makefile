.DEFAULT_GOAL := build
GOBIN ?= $(shell go env GOPATH)/bin

ifeq ($(filter-out /,$(abspath $(GOBIN))),)
$(error GOBIN is '$(GOBIN)'; it must name a real directory)
endif

# the claude shim runs from the checkout (it needs its node_modules), so the
# installed binary is stamped with where to find it; claude.shim overrides.
SHIM_DIR := $(CURDIR)/shim/claude
LDFLAGS := -X github.com/michaelquigley/mega/internal/config.defaultClaudeShim=$(SHIM_DIR)

.PHONY: build test e2e checks checks-deps checks-browser clean frontend frontend-test shim

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

# the full gate includes the frontend because the shipped Go build embeds it.
test: frontend-test shim
	npm --prefix shim/claude test
	go test ./... -count=1
	go vet ./...

# harness round trips against the disposable fixtures. these drive the real
# claude code with the operator's login and cost model turns; run by hand.
e2e: shim
	go test -tags e2e -count=1 -v -timeout 30m ./internal/e2e/

# the browser checks: every suite in checks/, or SUITE=name (or a list), each
# against its own scratch mega on a scratch port with the committed fakes.
# its own target, not part of test: it needs a browser and takes minutes.
checks: frontend shim checks-deps
	mkdir -p "$(CHECKS_DIR)"
	go build -o "$(CHECKS_DIR)/mega" ./cmd/mega
	MEGA_CHECKS_DIR="$(CHECKS_DIR)" MEGA_CHECK_BIN="$(CHECKS_DIR)/mega" node checks/run.mjs $(SUITE)

# the checks' pinned playwright, exactly as locked.
checks-deps:
	npm --prefix checks ci

# the chromium build the pinned playwright drives, into playwright's own
# cache (~/.cache/ms-playwright); run once, and again when playwright moves.
checks-browser: checks-deps
	npm --prefix checks exec -- playwright install chromium

clean:
	go clean ./...
	rm -f "$(GOBIN)/mega"
	rm -rf ui/dist ui/node_modules shim/claude/node_modules checks/node_modules
