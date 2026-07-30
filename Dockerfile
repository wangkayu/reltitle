FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY pkg ./pkg
COPY configs ./configs
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/title-parser ./cmd/server

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/title-parser ./title-parser
COPY --from=build /src/configs ./configs
ENV CONFIG_DIR=/app/configs
ENV GIN_MODE=release
ENV PORT=8080
EXPOSE 8080
CMD ["/app/title-parser"]
