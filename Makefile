BIN  := bin/fleet
LD   := -ldflags "-X github.com/YoonShinWoong/fleet/internal/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)"

.PHONY: build install test vet run clean dist
build:   ; go build $(LD) -o $(BIN) ./cmd/fleet
install: ; go install $(LD) ./cmd/fleet
test:    ; go test ./...
vet:     ; go vet ./...
run: build ; ./$(BIN) daemon
clean:   ; rm -rf bin
dist:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build $(LD) -o dist/fleet-darwin-arm64 ./cmd/fleet
	GOOS=darwin GOARCH=amd64 go build $(LD) -o dist/fleet-darwin-amd64 ./cmd/fleet
	GOOS=linux  GOARCH=arm64 go build $(LD) -o dist/fleet-linux-arm64 ./cmd/fleet
	shasum -a 256 dist/*
