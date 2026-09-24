# syntax=docker/dockerfile:1

FROM node:22-alpine AS web
WORKDIR /web
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ .
RUN pnpm build

FROM golang:1.26-bookworm AS builder
WORKDIR /src
ENV CGO_ENABLED=0
ARG VERSION=0.1.5
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w -X tdload/internal/version.Version=${VERSION}" -o /out/tdload ./cmd/tdload

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /tdload/config/session /tdload/downloads /app/web
WORKDIR /app
COPY --from=builder /out/tdload /app/tdload
COPY --from=web /web/dist /app/web
ENV BIND=0.0.0.0:3080
ENV CONFIG_PATH=/tdload/config/config.yaml
ENV WEB_DIR=/app/web
EXPOSE 3080
CMD ["/app/tdload"]
