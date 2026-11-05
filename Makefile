.PHONY: check check-tools test-unit test-e2e test-integration test-resolvers test-race coverage fuzz-smoke lint build run lab
.NOTPARALLEL:

export GOCACHE ?= $(CURDIR)/var/cache/go-build
export GOPATH ?= $(CURDIR)/var/gopath
export GOMODCACHE ?= $(CURDIR)/var/cache/go-mod
export GOTMPDIR ?= $(CURDIR)/var/tmp
export GOLANGCI_LINT_CACHE ?= $(CURDIR)/var/cache/golangci-lint
export BUILDX_CONFIG ?= $(CURDIR)/var/cache/buildx
export EPOCHALYPSE_BUILD_COMMIT := $(shell scripts/build-commit.sh)
VERSION_FLAGS = -X github.com/Crank-Git/epochalypse/internal/version.Commit=$(EPOCHALYPSE_BUILD_COMMIT)

$(shell mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(GOTMPDIR)" "$(GOLANGCI_LINT_CACHE)" "$(BUILDX_CONFIG)")

check: check-tools
	$(MAKE) test-unit test-integration test-e2e test-resolvers test-race coverage fuzz-smoke lint build

check-tools:
	@test "$$(go version | awk '{print $$3}')" = go1.27.3 || { echo 'Install Go 1.27.3.' >&2; exit 1; }
	@golangci-lint version | grep -q 'version 2.15.0 ' || { echo 'Install golangci-lint 2.15.0.' >&2; exit 1; }
	@python3 --version

test-unit:
	python3 scripts/go_tests.py -count=1 ./...
	python3 -m unittest discover -s scripts -p 'test_coverage.py'
	python3 -m unittest discover -s scripts -p 'test_checks.py'
	python3 -m unittest discover -s scripts -p 'test_go_tests.py'

test-e2e:
	python3 -m unittest discover -s scripts -p 'test_e2e.py'
	python3 -m unittest discover -s scripts -p 'test_browser.py'
	python3 -m unittest discover -s scripts -p 'test_lab.py'
	python3 -m unittest discover -s scripts -p 'test_backup.py'

test-integration:
	python3 scripts/go_tests.py -count=1 ./tests/integration

test-resolvers:
	python3 scripts/go_tests.py -tags=resolvers -count=1 ./tests/resolvers

test-race:
	python3 scripts/go_tests.py -race -count=1 ./...

coverage:
	mkdir -p .test-artifacts
	python3 scripts/go_tests.py -count=1 -coverpkg=github.com/Crank-Git/epochalypse/... -covermode=atomic -coverprofile=.test-artifacts/unit.out ./...
	$(MAKE) test-e2e
	python3 scripts/coverage_gate.py .test-artifacts/unit.out .test-artifacts/cli.out .test-artifacts/browser.out .test-artifacts/backup.out

fuzz-smoke:
	go test ./internal/command -run='^$$' -fuzz=FuzzRun -fuzztime=30s -parallel=2

lint: check-tools
	@test -z "$$(gofmt -l $$(find cmd internal web tests -name '*.go' 2>/dev/null))" || { echo 'Run gofmt on the reported Go files.' >&2; exit 1; }
	go vet ./...
	golangci-lint run ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(VERSION_FLAGS)' -o bin/epochalypse-linux-amd64 ./cmd/epochalypse
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '$(VERSION_FLAGS)' -o bin/epochalypse-linux-arm64 ./cmd/epochalypse

run:
	go run -ldflags '$(VERSION_FLAGS)' ./cmd/epochalypse server --config "$${EPOCHALYPSE_CONFIG:-config/local.example.json}"

lab:
	docker compose -f deploy/lab/compose.yaml up --build --detach --wait --wait-timeout 60

.PHONY: demo clean-demo
demo:
	go run ./cmd/epochalypse demo

clean-demo:
	go run ./cmd/epochalypse clean-demo --namespace "$(NAMESPACE)"
