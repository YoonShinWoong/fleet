BIN  := bin/keepty
LD   := -ldflags "-X github.com/YoonShinWoong/keepty/internal/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)"

.PHONY: build install test vet run clean dist
build:   ; go build $(LD) -o $(BIN) ./cmd/keepty
install: ; go install $(LD) ./cmd/keepty
test:    ; go test ./...
vet:     ; go vet ./...
run: build ; ./$(BIN) daemon
clean:   ; rm -rf bin
dist:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build $(LD) -o dist/keepty-darwin-arm64 ./cmd/keepty
	GOOS=darwin GOARCH=amd64 go build $(LD) -o dist/keepty-darwin-amd64 ./cmd/keepty
	GOOS=linux  GOARCH=arm64 go build $(LD) -o dist/keepty-linux-arm64 ./cmd/keepty
	shasum -a 256 dist/*
