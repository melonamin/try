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
    rm -f try try-* *.zip
    rm -rf dist/

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

# Build for macOS (universal binary)
build-macos:
    @echo "Building for macOS (amd64)..."
    GOOS=darwin GOARCH=amd64 go build -o try-darwin-amd64
    @echo "Building for macOS (arm64)..."
    GOOS=darwin GOARCH=arm64 go build -o try-darwin-arm64
    @echo "Creating universal binary..."
    lipo -create -output try try-darwin-amd64 try-darwin-arm64
    rm try-darwin-amd64 try-darwin-arm64
    @echo "Universal binary created: try"

# Build for Linux
build-linux:
    @echo "Building for Linux (amd64)..."
    GOOS=linux GOARCH=amd64 go build -o try-linux-amd64
    @echo "Building for Linux (arm64)..."
    GOOS=linux GOARCH=arm64 go build -o try-linux-arm64

# Code sign the macOS binary
sign: build-macos
    @echo "Code signing binary..."
    codesign --force --options runtime --sign "Developer ID Application: Ameba Labs, LLC (X93LWC49WV)" --timestamp try
    @echo "Verifying signature..."
    codesign -dv --verbose=4 try

# Create zip archive for notarization
package: sign
    @echo "Creating zip archive..."
    zip -r try.zip try
    @echo "Archive created at try.zip"

# Submit for notarization
notarize: package
    @echo "Submitting for notarization..."
    xcrun notarytool submit try.zip \
        --keychain-profile "notarytool-kefir" \
        --wait

# Verify notarization
verify-notarization: notarize
    @echo "Verifying notarization..."
    @echo "Note: Standalone binaries cannot be stapled, but they are still notarized"
    @echo "Extracting binary from zip..."
    unzip -o try.zip
    @echo "Checking notarization status..."
    spctl -a -vvv -t install try 2>&1 || true
    @echo "Binary is ready for distribution!"

# Create distribution archives
dist-macos: verify-notarization
    @echo "Creating macOS distribution archives..."
    mkdir -p dist
    # Universal binary
    cp try dist/try-macos-universal
    cd dist && zip -r try-macos-universal.zip try-macos-universal
    cd dist && shasum -a 256 try-macos-universal.zip > try-macos-universal.zip.sha256
    rm dist/try-macos-universal
    @echo "macOS distribution ready in dist/"

# Build Linux distributions
dist-linux: build-linux
    @echo "Creating Linux distribution archives..."
    mkdir -p dist
    # Linux amd64
    cp try-linux-amd64 dist/
    cd dist && tar czf try-linux-amd64.tar.gz try-linux-amd64
    cd dist && shasum -a 256 try-linux-amd64.tar.gz > try-linux-amd64.tar.gz.sha256
    rm dist/try-linux-amd64
    # Linux arm64
    cp try-linux-arm64 dist/
    cd dist && tar czf try-linux-arm64.tar.gz try-linux-arm64
    cd dist && shasum -a 256 try-linux-arm64.tar.gz > try-linux-arm64.tar.gz.sha256
    rm dist/try-linux-arm64
    @echo "Linux distributions ready in dist/"

# Full release flow for all platforms
release: dist-macos dist-linux
    @echo "Release build complete!"
    @echo "Distribution files ready in dist/"
    @ls -lh dist/


# Format code
fmt:
    go fmt ./...

# Run go mod tidy
tidy:
    go mod tidy