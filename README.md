# Title Parser

Anime and film release-title parser written in Go. It includes an HTTP API,
a simple web page and a command-line tool.

## Run

Requires Go 1.22 or newer.

```bash
go run ./cmd/server
```

The default address is `http://localhost:8080`. Set `PORT`, `CONFIG_DIR` and
`GIN_MODE` to override the runtime defaults.

```bash
PORT=18080 GIN_MODE=release go run ./cmd/server
```

## HTTP API

- `GET /` - single-page parser UI
- `POST /api/v1/content` - compact parse result
- `GET /health` - health check

Request:

```bash
curl -X POST http://localhost:8080/api/v1/content \
  -H 'content-type: application/json' \
  -d '{"title":"[LoliHouse] Sample Title - 03 [1080p][HEVC][AAC]"}'
```

Response fields are omitted when no value is available:

```json
{
  "content": "Sample Title",
  "groups": ["LoliHouse"],
  "episode": {"number": "3"},
  "media": {
    "resolutions": ["1080p"],
    "video_codecs": ["HEVC"],
    "audio_tracks": [{"codec": "AAC"}]
  }
}
```

## CLI

```bash
go run ./cmd/parse -title "[LoliHouse] Sample Title - 03 [1080p]"
```

## Test

```bash
go test ./...
```

See `DEPLOY.md` for deployment instructions.
