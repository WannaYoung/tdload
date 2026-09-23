# TDLoad

自托管 Telegram 批量下载与监听控制台（单用户 Web）。后端 Go + [iyear/tdl](https://github.com/iyear/tdl) / gotd，前端 Vue 3 + Naive UI。

**开发文档：** [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)  
**镜像：** [wannayoung/tdload](https://hub.docker.com/r/wannayoung/tdload)（`linux/amd64` · `linux/arm64`）

仓库内 [`xtools/`](xtools/) 仅为参考样板，不参与本项目构建。

## 功能

- **控制台登录**：JWT；管理员由环境变量 `ADMIN_*` 在首次启动创建
- **Telegram 登录**：默认 Desktop 公开 API 凭证（与 tdl 相同）；支持验证码 / 二步验证；session 持久化
- **频道**：同步对话列表；按 message id 批量补齐或续下未下载媒体
- **任务**：消息链接入队下载；收藏夹整库同步；暂停 / 开始 / 进度（SSE）
- **监听**：按间隔轮询已监听频道，新增消息自动入队（可过滤）
- **资源库**：按频道 / 类型浏览、预览、扫盘补索引
- **设置**：文件名模板、并发与线程、相册整组、Takeout、代理、无图模式等

落盘目录：`{download_dir}/{频道ID}-{频道名称}/{模板名}`（默认下载目录 `./downloads` 或容器内 `/tdload/downloads`）。

## 会话

| 会话 | 时长 |
|------|------|
| 控制台登录（JWT） | **30 天** |
| Telegram session | **长期有效**（应用内退出、手机踢设备或账号异常才会失效） |

重启进程一般不会掉 Telegram 登录（session 在配置目录的 `session/`）。

## Docker 部署

镜像支持 `linux/amd64` 与 `linux/arm64`，pull 时按机器架构自动选择。镜像内置：

- `BIND=0.0.0.0:3080`
- `CONFIG_PATH=/tdload/config/config.yaml`
- `WEB_DIR=/app/web`

首次启动若配置文件不存在，会按容器路径自动生成（下载目录 `/tdload/downloads`，数据库与 session 在 `/tdload/config/`）。

```bash
cp .env.example .env
# 必填：ADMIN_USERNAME / ADMIN_PASSWORD
# 可选：PROXY=socks5://… 或 http://…

docker compose pull
docker compose up -d
# 或本地构建：docker compose up -d --build
```

| 项 | 容器 | 说明 |
|---|---|---|
| 控制台 | `3080` | <http://127.0.0.1:3080> |
| 配置 | `/tdload/config` | 映射 `./data/config`（`config.yaml`、`tdload.db`、`session/`） |
| 下载 | `/tdload/downloads` | 映射 `./data/downloads` |
| 管理员 | `ADMIN_USERNAME` / `ADMIN_PASSWORD` | **必填** |
| 代理 | `PROXY` | 可选，覆盖配置文件中的代理 |

标签示例：`wannayoung/tdload:latest`、`wannayoung/tdload:0.1.0`。

## 本地调试

```bash
cp .env.example .env                 # 填 ADMIN_*；可选 PROXY / TG_*
cp config.example.yaml config/config.yaml

# 终端 1：后端（本地默认听 3000，可用 .env 的 BIND 覆盖）
export BIND=0.0.0.0:3000 CONFIG_PATH=./config/config.yaml WEB_DIR=./web/dist
go run ./cmd/tdload

# 终端 2：前端（Vite 代理到后端）
cd web && pnpm install && pnpm dev
```

浏览器打开 Vite 地址（一般为 <http://127.0.0.1:3080>），管理员账号见 `.env` 中的 `ADMIN_*`。

不填 `TG_APP_ID` / `TG_APP_HASH` 时启动会写入 Desktop 公开凭证；若遇 `API_ID_PUBLISHED_FLOOD`，再改用自己的 api_id。

## 许可证

AGPL-3.0（见 [LICENSE](LICENSE)、[NOTICE](NOTICE)）
