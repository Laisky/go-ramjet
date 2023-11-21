.PHONY: install
install:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
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
	golangci-lint run -c .golangci.lint.yml
	govulncheck ./...

.PHONY: changelog
changelog:
	./.scripts/generate_changelog.sh

.PHONY: gen
gen:
	sass ./internal/tasks/gptchat/templates/scss
