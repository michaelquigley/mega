.DEFAULT_GOAL := build
GOBIN ?= $(shell go env GOPATH)/bin

ifeq ($(filter-out /,$(abspath $(GOBIN))),)
$(error GOBIN is '$(GOBIN)'; it must name a real directory)
endif

# the claude shim runs from the checkout (it needs its node_modules), so the
# installed binary is stamped with where to find it; claude.shim overrides.
SHIM_DIR := $(CURDIR)/shim/claude
LDFLAGS := -X git.hq.quigley.com/research/roundhouse/internal/config.defaultClaudeShim=$(SHIM_DIR)

.PHONY: build test e2e clean frontend frontend-test shim

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

clean:
	go clean ./...
	rm -f "$(GOBIN)"/*
	rm -rf ui/dist ui/node_modules shim/claude/node_modules
