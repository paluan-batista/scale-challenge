.DEFAULT_GOAL := verify

.PHONY: verify format-check vet unit integration race acceptance clean-clone

verify: format-check vet unit integration race acceptance clean-clone

format-check:
	@unformatted="$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))"; test -z "$$unformatted" || { echo "Run gofmt on:"; echo "$$unformatted"; exit 1; }

vet:
	go vet ./...

unit:
	go test -count=1 ./...

integration:
	go test -count=1 -tags=integration ./internal/...

race:
	go test -count=1 -race ./...

acceptance:
	go test -count=1 -tags=acceptance ./tests/acceptance/...

clean-clone:
	./scripts/validate-clean-worktree.sh
