# Build the try binary
build:
    go build -o try

# Run try with optional arguments
run *args:
    go run . {{args}}

# Install to ~/.local/bin
install: build
    mkdir -p ~/.local/bin
    cp try ~/.local/bin/
    @echo "Installed to ~/.local/bin/try"

# Clean build artifacts
clean:
    rm -f try

# Run with help flag
help:
    go run . --help

# Build for multiple platforms
build-all:
    GOOS=darwin GOARCH=amd64 go build -o try-darwin-amd64
    GOOS=darwin GOARCH=arm64 go build -o try-darwin-arm64
    GOOS=linux GOARCH=amd64 go build -o try-linux-amd64
    GOOS=linux GOARCH=arm64 go build -o try-linux-arm64
    @echo "Built for all platforms"

# Format code
fmt:
    go fmt ./...

# Run go mod tidy
tidy:
    go mod tidy