DEFAULT=test

.PHONY: test
test:
	go test -race -v ./...

# TODO replace with go tool once go.mod is set to 1.24
.PHONY: get-goimports
get-goimports:
	@go install golang.org/x/tools/cmd/goimports@latest

.PHONY: fmt
fmt: get-goimports
	gofmt -w -s ./
	goimports -w ./

