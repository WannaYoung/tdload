# TDLoad

**中文** | [English](README_EN.md)

自托管 Telegram 批量下载与监听控制台（单用户 Web）。后端 Go + [iyear/tdl](https://github.com/iyear/tdl) / gotd，前端 Vue 3 + Naive UI（深色 + 粉主色）。界面支持**中文 / 英文**（默认跟随浏览器语言，可切换并写入本地存储）。

**镜像：** [wannayoung/tdload](https://hub.docker.com/r/wannayoung/tdload)（`linux/amd64` · `linux/arm64`）  
**源码：** [github.com/WannaYoung/tdload](https://github.com/WannaYoung/tdload)

## 功能

侧栏：**仪表盘 · Telegram · 收藏 · 频道 · 任务 · 监听 · 资源库 · 设置**

- **控制台登录**：JWT；管理员由环境变量 `ADMIN_*` 在首次启动创建
- **Telegram**：Desktop 公开 API 凭证（与 tdl 相同）；验证码 / 二步验证；同步已加入对话与收藏缓存，并刷新自定义频道的标题 / 最新消息
- **收藏**：整库同步到 `我的收藏/`（先拉取列表再下载）；只补缺失（`skip_same` + `media_index` / 磁盘去重），无扫描水位
- **频道**：已加入对话 + **自定义频道**（`@名称` 或频道 ID）；列表看覆盖进度与彩色状态文字，可同步最新消息；详情可调整 / 对齐扫描水位，按批「继续下载」（每批是媒体条数，可按全部 / 媒体 / 图片 / 视频筛选）；失败项可重试（不推进水位）；自定义频道可删除
- **任务**：粘贴 `t.me` 链接入队；单项删除 / 清除完成；进度 SSE（与收藏 / 频道详情共用单路连接）
- **监听**：按间隔轮询已监听频道 / 群 / 收藏 / 自定义频道，增量自动入队；可按内容类型（全部 / 媒体 / 图片 / 视频）筛选
- **资源库**：按频道 / 类型浏览；图片本地缩放，视频封面用 Telegram document thumb（下载时写入缓存，浏览缺失时补拉）；扫盘只维护索引，**不改**频道或收藏水位
- **设置**：文件名模板、单文件线程数、并行文件数、相册整组、Takeout、代理、无图模式、监听间隔等
- **界面语言**：中文 / English；首次按浏览器语言（`zh*` → 中文，其它 → 英文），顶栏或登录页可切换，偏好保存在浏览器 `localStorage`

落盘目录：

- 收藏：`{download_dir}/我的收藏/{模板名}`
- 其它：`{download_dir}/{频道ID}-{频道名称}/{模板名}`

默认下载目录 `./downloads`（容器内 `/tdload/downloads`）。

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

示例 `compose.yml`（仅拉镜像部署；仓库根目录的 [docker-compose.yml](docker-compose.yml) 另含 `build: .` 便于本地构建）：

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

同目录准备 `.env`（管理员与代理从这里注入，勿提交真实 `.env`）：

```bash
cp .env.example .env
# 必填：ADMIN_USERNAME / ADMIN_PASSWORD
# 可选：PROXY=socks5://… 或 http://…

docker compose pull
docker compose up -d
# 或从本仓库构建：docker compose up -d --build
```

| 项 | 容器 | 说明 |
|---|---|---|
| 控制台 | `3080` | <http://127.0.0.1:3080> |
| 配置 | `/tdload/config` | 映射 `./data/config`（`config.yaml`、`tdload.db`、`session/`） |
| 下载 | `/tdload/downloads` | 映射 `./data/downloads` |
| 管理员 | `ADMIN_USERNAME` / `ADMIN_PASSWORD` | **必填** |
| 代理 | `PROXY` | 可选，覆盖配置文件中的代理 |

标签示例：`wannayoung/tdload:latest`、`wannayoung/tdload:0.1.6`。

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
