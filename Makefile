.PHONY: all clean lint test test-acceptance test-conformance test-fuzz test-mutation test-race

# Per-fuzzer time budget for `make test-fuzz`. CI uses 60s; pass e.g.
# FUZZTIME=10s for a quicker local smoke run.
FUZZTIME ?= 60s

all: clean lint test test-acceptance test-conformance test-fuzz test-mutation test-race

clean:
	@rm -rf *.out test_mutation.json

lint:
	@gofmt_unformatted=$$(gofmt -l . 2>/dev/null | grep -v '^testdata/' || true); \
	test -z "$$gofmt_unformatted" || (echo "files not formatted:" && echo "$$gofmt_unformatted" && exit 1)
	go vet ./...
	go mod verify
	go tool golangci-lint run ./...
	go tool go-licenses check ./...
	go tool govulncheck ./...

test:
	@go test -v -count=1 -coverprofile=test.out ./...
	@go tool cover -func=test.out | tail -1

test-acceptance:
	@go test -v -count=1 -run TestAcceptance -coverprofile=test_acceptance.out ./...
	@go tool cover -func=test_acceptance.out | tail -1

test-conformance:
	@go test -v -count=1 -run TestDialectCompat -coverprofile=test_conformance.out ./...
	@go tool cover -func=test_conformance.out | tail -1

test-fuzz:
	@go test -fuzz=FuzzScanner -fuzztime=$(FUZZTIME) -run=^$$ .
	@go test -fuzz=FuzzParser -fuzztime=$(FUZZTIME) -run=^$$ .
	@go test -fuzz=FuzzExpand -fuzztime=$(FUZZTIME) -run=^$$ .
	@go test -fuzz=FuzzRoundTrip -fuzztime=$(FUZZTIME) -run=^$$ .
	@go test -fuzz=FuzzMarshalEdit -fuzztime=$(FUZZTIME) -run=^$$ .

test-mutation:
	@go tool github.com/go-gremlins/gremlins/cmd/gremlins unleash --config .gremlins.yaml

test-race:
	@go test -race -count=1 -coverprofile=test_race.out ./...
	@go tool cover -func=test_race.out | tail -1
