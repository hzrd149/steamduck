# build the steamduck binary (no cgo; the webview loads kwakore's libwebview.so)
build:
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo development)" -o steamduck ./cmd/steamduck

test:
    go vet ./...
    go test ./...

# cross-build for the Deck from any machine
deck:
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/steamduck-linux-amd64 ./cmd/steamduck

# the store UI in a browser against the local kwakore daemon
dev-store:
    go run ./cmd/steamduck store --serve 127.0.0.1:8080
