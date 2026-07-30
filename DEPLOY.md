# Deployment

The service requires the Linux binary and the complete `configs/` directory.
The public routes are `GET /`, `POST /api/v1/content` and `GET /health`.

## Build a Linux amd64 Package

Run these commands from the repository root:

```bash
mkdir -p dist/title-parser
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags='-s -w' -o dist/title-parser/title-parser ./cmd/server
cp -R configs dist/title-parser/configs
cp deploy/title-parser.service dist/title-parser/title-parser.service
cp deploy/title-parser.nginx.conf dist/title-parser/title-parser.nginx.conf
tar -C dist/title-parser -czf dist/title-parser-linux-amd64.tar.gz .
```

Generated files under `dist/` are ignored by Git.

## systemd

On the Linux server:

```bash
mkdir -p /app/title-parser
tar -xzf title-parser-linux-amd64.tar.gz -C /app/title-parser
useradd --system --home /app/title-parser --shell /usr/sbin/nologin title-parser
chown -R title-parser:title-parser /app/title-parser
cp /app/title-parser/title-parser.service /etc/systemd/system/title-parser.service
systemctl daemon-reload
systemctl enable --now title-parser
systemctl status title-parser
curl http://127.0.0.1:8080/health
```

If the service account already exists, `useradd` can be skipped. Logs are
available through:

```bash
journalctl -u title-parser -n 100 --no-pager
```

## Nginx

Copy the example configuration and replace its placeholder domain:

```bash
cp /app/title-parser/title-parser.nginx.conf /etc/nginx/sites-available/title-parser
ln -s /etc/nginx/sites-available/title-parser /etc/nginx/sites-enabled/title-parser
nginx -t
systemctl reload nginx
```

## Docker

```bash
docker build -t title-parser .
docker run --rm -p 18080:8080 title-parser
```

Open `http://localhost:18080/` or request `/health` to verify the container.
