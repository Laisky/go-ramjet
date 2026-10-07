.PHONY: install
install:
	go install golang.org/x/tools/cmd/goimports@v0.51.0
	go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.PHONY: lint
lint:
	goimports -local github.com/Laisky/go-ramjet -w .
	go mod tidy
	gofmt -s -w .
	go vet ./...
	golangci-lint run -c .golangci.yml
	govulncheck ./...

.PHONY: changelog
changelog:
	./.scripts/generate_changelog.sh

.PHONY: gen
gen:
	@echo "No legacy SCSS to generate - templates moved to SPA"

.PHONY: frontend-install
frontend-install:
	corepack enable
	pnpm -C web install --frozen-lockfile

.PHONY: frontend-build
frontend-build: frontend-install
	pnpm -C web build

.PHONY: frontend-test
frontend-test: frontend-install
	pnpm -C web test

.PHONY: dev
dev: frontend-install
	pnpm -C web run dev:proxy

.PHONY: build
build: frontend-build
# 	go build

.PHONY: format
format:
	pnpm -C web exec prettier --write .
