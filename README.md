# Reltitle

Anime and film release-title parser command-line tool written in Go.
The CLI prints the complete parse result as JSON.

## Usage

Requires Go 1.22 or newer. The default configuration is embedded in the
binary, so no external files are needed:

```bash
go run ./cmd/reltitle -title "[LoliHouse] Sample Title - 03 [1080p]"
```

The title can also be supplied as a positional argument or through standard input.

```bash
echo "[LoliHouse] Sample Title - 03 [1080p]" | go run ./cmd/reltitle
```

To run against a modified copy of the configuration instead of the embedded
one, point `-config` at its directory:

```bash
./reltitle -config ./configs -title "[LoliHouse] Sample Title - 03 [1080p]"
```

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

## Versioning / Releases

Releases are tagged `vMAJOR.MINOR.PATCH` and documented in
[CHANGELOG.md](CHANGELOG.md). Because reltitle is used as a library, the version
number describes the compatibility of its output: a patch release (`0.0.x`) adds
dictionary entries or fixes parsing defects without changing the output shape, so
consumers need no code change, while a minor release (`0.x.0`) changes the output
shape — a field or an attribute dimension is added, renamed, or removed — and may
require consumers to adapt. `1.0.0` follows once the output shape is stable.

## Serve mode

`reltitle serve` runs a long-lived process that answers one JSON request per
line, for programs that parse many titles:

```bash
printf '%s\n' '{"id":1,"op":"hello"}' '{"id":2,"op":"parse","title":"[Group] Show - 03 [1080p]"}' | ./reltitle serve
```

The protocol is `rt-serve-1`: `hello` reports the protocol, the build version
and line, and the dictionary digest; `vocabulary` lists the canonical values of
every attribute dimension; `parse` returns the same result the one-shot CLI
prints. Malformed input is answered with an error and does not stop the
process.

## Update channel

Consumers that run the binary as a separate process can update it without
rebuilding: every release publishes linux/amd64 and linux/arm64 binaries, a
sha256 and size for each, and a manifest to the `update-channel` branch (test
builds of `main` go to `update-channel-test`). The manifest lives at
`lines/<major.minor>/channel.json`; an updater reads it and the binaries it
names over HTTPS from this repository, checks each binary against its sha256
and size, and refuses any path that leaves the channel directory. The channel
is not separately signed: its authenticity is the repository's, so only push to
these branches from the release workflow. GitHub Release assets carry the same
binaries for people, but the branch files are what an updater reads.
