# build the steamduck binary (no cgo; the webview loads kwakore's libwebview.so)
build: build-store
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo development)" -o steamduck ./cmd/steamduck

test: build-store
    go vet ./...
    go test ./...

# cross-build for the Deck from any machine
deck: build-store
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/steamduck-linux-amd64 ./cmd/steamduck

# the store UI in a browser against the local kwakore daemon
dev-store: build-store
    go run ./cmd/steamduck store --serve 127.0.0.1:8080

# single-file napplet and compatibility artifacts embedded by the Go binary
build-store:
    npm run build

# Rebuild and launch the real trusted napplet (not the browser compatibility UI).
store *args: build
    ./steamduck store {{args}}

test-store:
    npm run check
