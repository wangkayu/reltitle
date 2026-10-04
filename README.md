# Reltitle

Anime and film release-title parser command-line tool written in Go.
The CLI prints the complete parse result as JSON.

## Usage

Requires Go 1.22 or newer and the `configs/` directory.

```bash
go run ./cmd/reltitle -title "[LoliHouse] Sample Title - 03 [1080p]"
```

The title can also be supplied as a positional argument or through standard input.

```bash
echo "[LoliHouse] Sample Title - 03 [1080p]" | go run ./cmd/reltitle
```

Use a different configuration directory with `-config`.

## Build

```bash
go build -o reltitle ./cmd/reltitle
```

For Linux x86-64:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reltitle ./cmd/reltitle
```

## Test

```bash
go test ./...
```
