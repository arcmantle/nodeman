BINARY_NAME := nodeman
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
BUILD_DIR := dist

.PHONY: all build clean install setup release

# Build for current platform
build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/nodeman

# Build for all supported platforms
all: clean
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64  ./cmd/nodeman
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64  ./cmd/nodeman
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64   ./cmd/nodeman
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64   ./cmd/nodeman
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/nodeman

# Tagged release build (requires git tag)
release:
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "Error: no git tag found. Create a tag first: git tag v1.0.0"; \
		exit 1; \
	fi
	@echo "Building release $(VERSION)..."
	$(MAKE) all
	@echo "Release binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

# Install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/nodeman

# Build, install, and run setup
setup: install
	$(BINARY_NAME) setup

clean:
	rm -rf $(BUILD_DIR)

# Run tests
test:
	go test ./...

# Run go vet
vet:
	go vet ./...
