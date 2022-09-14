.PHONY: install
install:
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
		go install golang.org/x/tools/cmd/goimports@latest
		go install golang.org/x/vuln/cmd/govulncheck@latest
		# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
		# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

.PHONY: lint
lint:
		goimports -local github.com/Laisky/go-ramjet -w .
		go mod tidy
		go vet
		gofmt -s -w .
		govulncheck ./...
		golangci-lint run -c .golangci.lint.yml

gen:
	go get github.com/vektah/gqlparser/v2@v2.1.0
	go get github.com/99designs/gqlgen
	go run github.com/99designs/gqlgen

test:
	@tox --recreate
	@tox

changelog: CHANGELOG.md
	sh ./.scripts/generate_changelog.sh
