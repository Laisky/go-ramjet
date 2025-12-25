.PHONY: install
install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.4.0

	go install golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	# go install go.uber.org/nilaway/cmd/nilaway@latest
	# go install github.com/mitranim/gow@latest
	# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
	# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

.PHONY: lint
lint:
	goimports -local github.com/Laisky/go-ramjet -w .
	go mod tidy
	gofmt -s -w .
	go vet
	# nilaway ./...
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
	corepack enable || true
	pnpm -C web install

.PHONY: frontend-build
frontend-build: frontend-install
	pnpm -C web build

.PHONY: frontend-test
frontend-test: frontend-install
	pnpm -C web test

.PHONY: build
build: frontend-build
# 	go build
