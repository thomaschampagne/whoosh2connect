set shell := ["sh", "-cu"]

binary := "whoosh2connect"
pkg := "./..."

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
commit := `git rev-parse --short HEAD 2>/dev/null || echo none`
ldflags := "-s -w -X whoosh2connect/internal/version.Version=" + version + " -X whoosh2connect/internal/version.Commit=" + commit

default: test

# --- checks ---------------------------------------------------------------

test:
    go test -race {{pkg}}

vet:
    go vet {{pkg}}
    GOOS=windows go vet {{pkg}}

lint:
    golangci-lint run

fmt:
    gofmt -s -w .

fmt-check:
    #!/bin/sh
    out=$(gofmt -s -l .)
    if [ -n "$out" ]; then
        echo "gofmt needed on:"; echo "$out"; exit 1
    fi

# --- build ----------------------------------------------------------------

# build the host binary into bin/whoosh2connect
build:
    go build -ldflags "{{ldflags}}" -o bin/{{binary}} ./cmd/{{binary}}

# run the CLI from bin/ so the log lands next to the binary (e.g. just run --version)
run *args: build
    ./bin/{{binary}} {{args}}

tidy:
    go mod tidy

# Local: formats, then checks + builds.
sanity: fmt test vet build

# CI: check-only, no rewrites.
ci: fmt-check vet test build

clean:
    rm -rf bin dist

# --- release (needs zip + sha256sum + tar: Ubuntu CI + local) -------------

# Build every platform into dist/ with VERSION baked into the binary.
release version:
    #!/usr/bin/env bash
    set -euo pipefail
    rm -rf dist && mkdir dist
    commit=$(git rev-parse --short HEAD 2>/dev/null || echo none)
    ld="-s -w -X whoosh2connect/internal/version.Version={{version}} -X whoosh2connect/internal/version.Commit=$commit"
    build() {
        GOOS=$1 GOARCH=$2 CGO_ENABLED=0 go build -trimpath -ldflags "$ld" -o "dist/$3" ./cmd/{{binary}}
    }
    build windows amd64 {{binary}}.exe
    build linux amd64 {{binary}}-linux-amd64
    build darwin amd64 {{binary}}-darwin-amd64
    build darwin arm64 {{binary}}-darwin-arm64
    (cd dist && zip -q {{binary}}-windows-amd64.zip {{binary}}.exe)
    (cd dist && tar -czf {{binary}}-linux-amd64.tar.gz {{binary}}-linux-amd64 && rm {{binary}}-linux-amd64)
    (cd dist && tar -czf {{binary}}-darwin-amd64.tar.gz {{binary}}-darwin-amd64 && rm {{binary}}-darwin-amd64)
    (cd dist && tar -czf {{binary}}-darwin-arm64.tar.gz {{binary}}-darwin-arm64 && rm {{binary}}-darwin-arm64)
    (cd dist && sha256sum {{binary}}.exe {{binary}}-windows-amd64.zip {{binary}}-linux-amd64.tar.gz {{binary}}-darwin-amd64.tar.gz {{binary}}-darwin-arm64.tar.gz > checksums.txt)

# --- scoop (local + CI) ---------------------------------------------------

# Render the Scoop manifest for VERSION + exe SHA256 to stdout.
scoop-manifest version hash:
    #!/usr/bin/env bash
    # Same recipe runs locally and in the release workflow bucket job. VERSION is
    # the bare semantic-release tag (no "v"); SHA256 is the released
    # whoosh2connect.exe hash from that release's checksums.txt asset.
    set -euo pipefail
    version='{{version}}'
    hash='{{hash}}'
    cat <<EOF
    {
        "version": "$version",
        "description": "Download MyWhoosh cycling rides and upload them to Garmin Connect.",
        "homepage": "https://github.com/thomaschampagne/whoosh2connect",
        "license": "NOASSERTION",
        "architecture": {
            "64bit": {
                "url": "https://github.com/thomaschampagne/whoosh2connect/releases/download/$version/whoosh2connect.exe",
                "hash": "$hash"
            }
        },
        "bin": "whoosh2connect.exe",
        "shortcuts": [["whoosh2connect.exe", "whoosh2connect"]],
        "checkver": "github",
        "autoupdate": {
            "architecture": {
                "64bit": {
                    "url": "https://github.com/thomaschampagne/whoosh2connect/releases/download/\$version/whoosh2connect.exe"
                }
            },
            "hash": {
                "url": "\$baseurl/checksums.txt"
            }
        }
    }
    EOF

# Local end-to-end: build release assets and render dist/whoosh2connect.json.
scoop-release version: (release version)
    #!/usr/bin/env bash
    set -euo pipefail
    sha=$(awk '$2 == "whoosh2connect.exe" { print $1 }' dist/checksums.txt)
    [ -n "$sha" ] || { echo "no whoosh2connect.exe hash in dist/checksums.txt"; exit 1; }
    just scoop-manifest {{version}} "$sha" > dist/whoosh2connect.json
    echo "wrote dist/whoosh2connect.json"

# --- deps -----------------------------------------------------------------

update-deps:
    go get -u ./...
    go mod tidy
