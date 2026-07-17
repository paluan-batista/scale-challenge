.DEFAULT_GOAL := verify

.PHONY: verify format-check vet unit integration race acceptance

verify: format-check vet unit integration race acceptance

format-check:
	@unformatted="$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))"; test -z "$$unformatted" || { echo "Run gofmt on:"; echo "$$unformatted"; exit 1; }

vet:
	go vet ./...

unit:
	go test ./...

integration:
	go test -tags=integration ./internal/...

race:
	go test -race ./...

acceptance:
	go test -tags=acceptance ./tests/acceptance/...
