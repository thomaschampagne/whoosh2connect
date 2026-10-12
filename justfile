set shell := ["bash", "-cu"]

binary := "whoosh2connect"
pkg := "./..."

version := if "{{shell('git describe --tags --always --dirty 2>/dev/null || echo dev')}}" == "" { "dev" } else { "{{shell('git describe --tags --always --dirty 2>/dev/null || echo dev')}}" }
commit := "{{shell('git rev-parse --short HEAD 2>/dev/null || echo none')}}"
ldflags := "-s -w -X whoosh2connect/internal/version.Version=" + version + " -X whoosh2connect/internal/version.Commit=" + commit

# build the binary into bin/whoosh2connect
build:
    go build -ldflags "{{ldflags}}" -o bin/{{binary}} ./cmd/{{binary}}

# run the CLI (pass args after --, e.g. just run -- --version)
run *args:
    go run ./cmd/{{binary}} {{args}}

# run the test suite
test:
    go test {{pkg}}

# run go vet
vet:
    go vet {{pkg}}

# run golangci-lint
lint:
    golangci-lint run

# gofmt the tree
fmt:
    gofmt -s -w .

# tidy modules
tidy:
    go mod tidy

# remove build artifacts
clean:
    rm -rf bin

# default recipe
default: test
