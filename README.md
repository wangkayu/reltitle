# Title Parser

Anime and film release-title parser command-line tool written in Go.

## Usage

Requires Go 1.22 or newer and the `configs/` directory.

```bash
go run ./cmd/parse -title "[LoliHouse] Sample Title - 03 [1080p]"
```

The title can also be supplied as a positional argument or through standard input.

```bash
echo "[LoliHouse] Sample Title - 03 [1080p]" | go run ./cmd/parse
```

Use a different configuration directory with `-config`.

## Build

```bash
go build -o title-parser ./cmd/parse
```

For Linux x86-64:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o title-parser ./cmd/parse
```

## Test

```bash
go test ./...
```
