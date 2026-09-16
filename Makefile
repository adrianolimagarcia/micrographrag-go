GO_TAGS=sqlite_fts5

.PHONY: test build demo fmt vet

test:
	go test -tags '$(GO_TAGS)' ./...

build:
	CGO_ENABLED=1 go build -tags '$(GO_TAGS)' ./...

demo:
	CGO_ENABLED=1 go run -tags '$(GO_TAGS)' ./cmd/demo

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

vet:
	go vet -tags '$(GO_TAGS)' ./...
