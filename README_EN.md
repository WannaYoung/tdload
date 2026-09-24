# TDLoad

[中文](README.md) | **English**

Self-hosted Telegram batch download and watch console (single-user web app). Backend: Go + [iyear/tdl](https://github.com/iyear/tdl) / gotd. Frontend: Vue 3 + Naive UI (dark theme, pink accent).

**Image:** [wannayoung/tdload](https://hub.docker.com/r/wannayoung/tdload) (`linux/amd64` · `linux/arm64`)  
**Source:** [github.com/WannaYoung/tdload](https://github.com/WannaYoung/tdload)

## Features

Sidebar: **Dashboard · Telegram · Saved Messages · Channels · Tasks · Watch · Library · Settings**

- **Console login**: JWT; admin account is created on first start from `ADMIN_*` env vars
- **Telegram**: Desktop public API credentials (same as tdl); code / 2FA; sync joined dialogs and Saved Messages cache; refresh custom channel title / latest message
- **Saved Messages**: full sync into `我的收藏/` (list first, then download); fill gaps only (`skip_same` + `media_index` / on-disk dedupe); no scan cursor
- **Channels**: joined dialogs + **custom channels** (`@username` or channel ID); coverage progress and status text; sync latest message; adjust / align scan cursor; batch “continue download” (batch size = media count; filter all / media / photo / video); retry failed items without advancing cursor; delete custom channels
- **Tasks**: paste `t.me` links to enqueue; delete items / clear completed; progress over SSE (shared with Saved / Channel detail)
- **Watch**: poll watched channels / groups / Saved / custom channels on an interval; incremental enqueue; content-type filter (all / media / photo / video)
- **Library**: browse by channel / type; local image thumbnails; video covers from Telegram document thumbs (cached on download, backfilled when missing); disk scan updates index only and **does not** change channel or Saved cursors
- **Settings**: filename template, threads per file, parallel files, album grouping, Takeout, proxy, no-image mode, watch interval, etc.

On-disk layout:

- Saved: `{download_dir}/我的收藏/{template name}`
- Other: `{download_dir}/{channelID}-{channelTitle}/{template name}`

Default download dir: `./downloads` (in container: `/tdload/downloads`).

## Sessions

| Session | Lifetime |
|------|------|
| Console login (JWT) | **30 days** |
| Telegram session | **Long-lived** (invalidated by in-app logout, phone “terminate sessions”, or account issues) |

Restarting the process usually keeps Telegram logged in (session files live under `session/` in the config directory).

## Docker

Images support `linux/amd64` and `linux/arm64`; Docker picks the matching arch on pull. Built-in defaults:

- `BIND=0.0.0.0:3080`
- `CONFIG_PATH=/tdload/config/config.yaml`
- `WEB_DIR=/app/web`

On first start, if the config file is missing, one is generated for container paths (downloads at `/tdload/downloads`; DB and session under `/tdload/config/`).

Example `compose.yml` (pull-only deploy; the repo [docker-compose.yml](docker-compose.yml) also includes `build: .` for local builds):

```yaml
services:
  tdload:
    image: wannayoung/tdload:latest
    container_name: tdload
    restart: unless-stopped
    ports:
      - "3080:3080"
    environment:
      ADMIN_USERNAME: ${ADMIN_USERNAME:?set ADMIN_USERNAME in .env}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD:?set ADMIN_PASSWORD in .env}
      PROXY: ${PROXY:-}
    volumes:
      - ./data/config:/tdload/config
      - ./data/downloads:/tdload/downloads
```

Put a `.env` next to it (admin and proxy are injected from here; never commit a real `.env`):

```bash
cp .env.example .env
# Required: ADMIN_USERNAME / ADMIN_PASSWORD
# Optional: PROXY=socks5://… or http://…

docker compose pull
docker compose up -d
# Or build from this repo: docker compose up -d --build
```

| Item | Container | Notes |
|---|---|---|
| Console | `3080` | <http://127.0.0.1:3080> |
| Config | `/tdload/config` | maps to `./data/config` (`config.yaml`, `tdload.db`, `session/`) |
| Downloads | `/tdload/downloads` | maps to `./data/downloads` |
| Admin | `ADMIN_USERNAME` / `ADMIN_PASSWORD` | **required** |
| Proxy | `PROXY` | optional; overrides proxy in config |

Tags: `wannayoung/tdload:latest`, `wannayoung/tdload:0.1.5`.

## Local development

```bash
cp .env.example .env                 # set ADMIN_*; optional PROXY / TG_*
cp config.example.yaml config/config.yaml

# Terminal 1: backend (default listen 3000; override with BIND in .env)
export BIND=0.0.0.0:3000 CONFIG_PATH=./config/config.yaml WEB_DIR=./web/dist
go run ./cmd/tdload

# Terminal 2: frontend (Vite proxies to the backend)
cd web && pnpm install && pnpm dev
```

Open the Vite URL (usually <http://127.0.0.1:3080>). Admin credentials are the `ADMIN_*` values in `.env`.

If `TG_APP_ID` / `TG_APP_HASH` are empty, Desktop public credentials are written on start. If you hit `API_ID_PUBLISHED_FLOOD`, use your own api_id from [my.telegram.org](https://my.telegram.org).

## License

AGPL-3.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
