# tdload 开发文档

自托管 Telegram 批量下载与监控控制台。后端 Go 单体集成 [iyear/tdl](https://github.com/iyear/tdl)（`tdl/core` + `gotd`），前端 Vue 控制台；Docker 部署到 amd64 / arm64 Linux。仓库内 `xtools/` 仅作产品与部署参考，不进入运行时依赖。

许可证：**AGPL-3.0**（与 tdl 一致）。

相关文档：

- [tdl 官方文档](https://docs.iyear.me/tdl/)
- [tdl 下载指南](https://docs.iyear.me/tdl/guide/download/)
- 参考样板：[xtools/README.md](../xtools/README.md)

---

## 1. 目标与范围

### 1.1 目标

控制台以 **Telegram 账号为中心**：先同步对话与收藏，再在「频道」继续批量下载、「收藏」补齐缺失、「任务」处理消息链接、「监听」增量入队；全程 **自动跳过已下载**（`media_index` + 磁盘存在性），进度经 SSE 推送。

| 能力 | 说明 |
|------|------|
| 鉴权 | 控制台 JWT + Telegram session |
| **Telegram 页** | 登录 / 退出；**同步**已加入频道·群组·收藏缓存，并刷新自定义频道元数据；展示 **频道（对话）数**、**收藏数** |
| **收藏页** | 整库同步到 `我的收藏/`；无扫描水位，只补缺失（`skip_same`） |
| **频道页** | 已加入对话 + 自定义频道；覆盖进度、同步最新、继续下载；可调扫描水位；失败重试仅 ids、不推进水位 |
| **任务页** | 消息链接入队（仅 `source=url`）；刷新 / 清除已完成 / 删除单项（无全部暂停/开始） |
| **资源库** | 已下载索引、筛选、预览；扫盘只维护索引，**不改水位** |
| **监听** | 频道 / 群 / 收藏 / 自定义频道增量入队；内容类型筛选（全部 / 媒体 / 图片 / 视频）；添加前同步最新消息 ID |
| 部署 | 单镜像 Docker，`linux/amd64` / `linux/arm64`；本机前后端分离调试 |

**落盘目录（相对 `download_dir`）：**

- 收藏：`我的收藏/`
- 其它消息：`{频道id}-{频道名称}/`（与 Worker `chatFolderName` 一致；名称需安全化）

### 1.2 分期

| 阶段 | 内容 | 状态 |
|------|------|------|
| **一期** | 对话/收藏同步 + 频道工作台 + 消息任务 + 收藏同步 + 队列/SSE + 资源库 | ✅ |
| **二期** | 监听增量自动入队（复用同一任务模型与去重） | ✅（M5） |

### 1.3 一期非目标

上传、转发、多用户、桌面端、内置 MySQL。

---

## 2. 已确认技术决策

| 项 | 决策 |
|----|------|
| 后端 | Go 单体：HTTP API + Worker + Watcher，单二进制 |
| Telegram | `github.com/iyear/tdl/core` + `gotd`；下载对齐 tdl `pkg/downloader` 的 Iter / Progress |
| 前端 | Vue 3 + Vite + Naive UI + Pinia + Vue Router（布局对齐 xtools；深色 + 粉主色 `#f472b6`） |
| 数据库 | SQLite（文件落在配置卷，单容器友好） |
| 配置 | YAML 运行时配置 + 环境变量机密（对齐 xtools） |
| 鉴权 | 单用户 JWT Bearer；媒体流可用短时 ticket（勿把会话 JWT 放进 URL） |
| 参考代码 | `xtools/` 只读参考，禁止 `import` / 拷贝进构建依赖 |
| 开源 | 整仓 AGPL-3.0 |

---

## 3. 总体架构

```mermaid
flowchart LR
  Browser[Vue_Console] -->|JWT_REST_SSE| API[Go_API]
  API --> Auth[JWT_Auth]
  API --> TG[Telegram_Session]
  API --> Queue[Task_Worker]
  TG --> Core[tdl_core_gotd]
  Queue --> Core
  Queue --> Disk[Download_Volume]
  API --> DB[(SQLite)]
  Watch[Channel_Watcher] --> Queue
  Watch --> Core
```

### 3.1 进程模型

一个进程内包含：

1. **API**：REST + SSE + 静态前端托管  
2. **Worker**：消费 `queued` 任务，调用 tdl 下载，写盘与更新 DB  
3. **Watcher**：监听对话增量，命中后入队  

关机时 drain 进行中任务（有超时上限），未完成任务下次启动重新入队。

### 3.2 数据流（下载）

1. 用户在 Web 粘贴 `t.me/...` 链接、开始收藏同步，或在频道/监听入队  
2. API 解析为 `task` + `task_items`，状态 `queued`  
3. Worker 取任务 → 构建 Iter → `Downloader.Download`  
4. `Progress` 回调 → 内存事件总线 → SSE `/api/events`  
5. 完成后写 `media_index`，状态 `done` / `failed`

### 3.3 数据流（监听）

1. 用户选择对话写入 `watched_chats`（加入前同步当前最新 message id 为水位，不立刻下载）
2. Watcher 按间隔扫描，取 `(水位, 最新]` 增量，并按 `filter_json.contentType` 过滤附件类型
3. 有命中媒体 → 自动创建 `watch` / `watch_saved` 任务入队
4. 后续与 Worker 相同（`skip_same` 去重）

---

## 4. 仓库布局

```
tdload/
├── xtools/                      # 参考样板（勿依赖）
├── docs/
│   └── DEVELOPMENT.md           # 本文档
├── cmd/
│   └── tdload/
│       └── main.go
├── internal/
│   ├── api/                     # HTTP handlers / 路由
│   ├── auth/                    # JWT、ticket、管理员引导
│   ├── config/                  # YAML + env
│   ├── db/                      # SQLite schema / 列迁移
│   ├── tg/                      # session、登录、同步、下载、内容类型
│   ├── worker/                  # 任务调度与槽位
│   ├── watcher/                 # 频道/群/收藏/自定义频道监听
│   ├── progress/                # SSE 事件总线
│   ├── library/                 # 已下载索引扫盘
│   ├── static/                  # WEB_DIR 静态资源
│   └── version/
├── web/                         # Vue 控制台
├── Dockerfile
├── docker-compose.yml
├── .dockerignore
├── .env.example
├── config.example.yaml
├── go.mod
├── LICENSE                      # AGPL-3.0
├── NOTICE                       # 第三方（含 tdl）声明
└── README.md
```

---

## 5. 配置设计

持久化运行时配置为 YAML；数据库路径、管理员、JWT 等用环境变量。首次启动若 `jwt_secret` 为空则自动生成并写回 YAML。

### 5.1 `config.example.yaml`

```yaml
bind: "0.0.0.0:3030"
download_dir: "./downloads"
web_dir: "./web/dist"
db_path: "./config/tdload.db"
session_dir: "./config/session"

# Telegram API。留空时启动写入 Desktop 公开凭证（与 tdl 相同）
app_id: 2040
app_hash: "b18441a1ff607e10a989891a5462e627"

threads: 8          # 单任务分片线程 -t
concurrency: 4      # 并发任务数 -l
skip_same: true     # --skip-same
group_album: true   # --group
rewrite_ext: false  # --rewrite-ext
takeout: true       # --takeout（仅大批量下载；解析/同步频道不用）
no_image: false     # 界面无图模式（资源库预览占位）
watch_interval_minutes: 30
template: "{{DialogID }}-{{MessageID }}-{{FileName }}"

proxy: ""           # 例: socks5://127.0.0.1:1080
jwt_secret: ""
```

容器首次生成配置时会改成 `/tdload/...` 与 `BIND=0.0.0.0:3080`。

### 5.2 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `ADMIN_USERNAME` | 是 | 控制台管理员，首次启动建号 |
| `ADMIN_PASSWORD` | 是 | 同上；已有用户后不覆盖 |
| `BIND` | 否 | 进程默认 `0.0.0.0:3030`；Docker 镜像内置 `0.0.0.0:3080`；本地调试可用 `0.0.0.0:3000` |
| `CONFIG_PATH` | 否 | 默认 `/tdload/config/config.yaml` |
| `WEB_DIR` | 否 | 前端静态目录 |
| `TG_APP_ID` / `TG_APP_HASH` | 否 | 覆盖 YAML；不填则用 Desktop 公开凭证 |
| `PROXY` | 否 | 覆盖 YAML 中的代理 |
| `JWT_SECRET` | 否 | 空则自动生成写回 YAML |

机密与 session **禁止**打进镜像层，只走挂载卷。

---

## 6. 数据模型

SQLite。迁移用编号 SQL 或轻量迁移库（如 `golang-migrate` / 自研 embed）。

### 6.1 `users`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| username | TEXT UNIQUE | |
| password_hash | TEXT | argon2id / bcrypt |
| created_at | TEXT | RFC3339 |

单用户：仅支持一个管理员账号。

### 6.2 `tg_accounts`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| label | TEXT | 显示名 |
| phone | TEXT | 可空（QR 登录） |
| user_id | INTEGER | Telegram user id |
| session_file | TEXT | 相对 `session_dir` 的路径 |
| status | TEXT | `pending` / `active` / `expired` / `error` |
| proxy | TEXT | 可覆盖全局 proxy |
| last_error | TEXT | |
| created_at / updated_at | TEXT | |

一期可先支持单账号；表结构预留多账号。

### 6.3 `tasks`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| source | TEXT | `url` / `saved_all` / `chat_continue` / `chat_batch` / `watch` / `watch_saved` 等 |
| title | TEXT | 展示标题 |
| status | TEXT | `queued` / `running` / `paused` / `done` / `failed` / `cancelled` |
| tg_account_id | INTEGER FK | |
| options_json | TEXT | 覆盖全局 threads/concurrency 等 |
| total_bytes | INTEGER | |
| done_bytes | INTEGER | |
| total_files | INTEGER | |
| done_files | INTEGER | |
| speed_bps | INTEGER | 最近采样 |
| error | TEXT | |
| created_at / started_at / finished_at | TEXT | |

### 6.4 `task_items`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| task_id | INTEGER FK | |
| chat_id | INTEGER | |
| message_id | INTEGER | |
| file_name | TEXT | |
| size | INTEGER | |
| status | TEXT | `pending` / `downloading` / `done` / `skipped` / `failed` |
| local_path | TEXT | |
| error | TEXT | |

唯一约束建议：`(task_id, chat_id, message_id)`。

### 6.5 `media_index`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| chat_id | INTEGER | |
| message_id | INTEGER | |
| file_name | TEXT | |
| size | INTEGER | |
| local_path | TEXT | |
| mime | TEXT | |
| downloaded_at | TEXT | |

唯一约束：`(chat_id, message_id, size)`，配合 `skip_same`。

### 6.6 `task_logs`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| task_id | INTEGER FK | |
| level | TEXT | `info` / `warn` / `error` |
| message | TEXT | |
| created_at | TEXT | |

### 6.7 `watched_chats`

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| tg_account_id | INTEGER FK | |
| chat_id | INTEGER | |
| chat_title | TEXT | |
| enabled | INTEGER | 0/1 |
| last_message_id | INTEGER | 监听水位 |
| filter_json | TEXT | `{ "contentType": "all\|media\|image\|video" }` |
| last_run_at / next_run_at | TEXT | 上次 / 下次调度 |
| created_at / updated_at | TEXT | |

### 6.8 `tg_dialogs`（一期 · 缓存）

Telegram 页同步已加入对话后写入；自定义频道单独插入，`is_custom=1`，同步已加入列表时不会被删掉。若用户后来加入该频道，UPSERT 会合并并清掉自定义标记。

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| tg_account_id | INTEGER FK | |
| chat_id | INTEGER | Bot API 风格 id（频道多为 `-100…`） |
| title | TEXT | 显示名 |
| username | TEXT | `@` 可空 |
| kind | TEXT | `channel` / `supergroup` / `group` / `custom` |
| message_count | INTEGER | 对话消息总数（能取则填；取不到可为 -1） |
| downloaded_count | INTEGER | 来自 `media_index` 聚合 |
| last_message_id | INTEGER | 对话最新消息 id（覆盖进度用） |
| synced_at | TEXT | 上次同步时间 |
| is_custom | INTEGER | 1 = 手动添加的未加入公开频道 |

唯一约束：`(tg_account_id, chat_id)`。列表排序：`is_custom ASC, title`（自定义排在已加入之后）。

### 6.9 `saved_messages_cache`（一期 · 收藏）

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK | |
| tg_account_id | INTEGER FK | |
| message_id | INTEGER | Saved Messages 内 id |
| chat_id | INTEGER | 源对话 id（转发来源，可 0） |
| synced_at | TEXT | |

唯一约束：`(tg_account_id, message_id)`。收藏总数 = 行数；已下载数 = 与 `media_index`（收藏专用 `chat_id` 约定，见 §7.2）交集。

### 6.10 `chat_download_state`（一期 · 可选）

按对话记录批量下载扫描水位，便于频道详情「继续下载」与列表覆盖进度。

| 列 | 类型 | 说明 |
|----|------|------|
| tg_account_id | INTEGER FK | |
| chat_id | INTEGER | |
| last_downloaded_message_id | INTEGER | 扫描水位（已扫到的最大 message id） |
| batch_size | INTEGER | 每批条数，默认 100 |
| updated_at | TEXT | |

主键：`(tg_account_id, chat_id)`。

### 6.11 `chat_labels`

解析后的频道/群显示名缓存（不依赖 `tg_dialogs`，同步对话不会清掉）。

| 列 | 类型 | 说明 |
|----|------|------|
| chat_id | INTEGER PK | |
| title / username | TEXT | |
| updated_at | TEXT | |

---

## 7. Web 功能分期

布局对齐 xtools（深色 + 粉主色 `#f472b6`、宽侧栏 / 窄屏顶栏下拉）；SSE 进度 + 任务列表轮询兜底；设置写回 YAML。侧栏活跃任务数按 kind 分徽标（消息 / 收藏 / 频道），5s 轮询 `GET /api/dashboard`。

### 7.1 侧栏导航（目标态）

**当前侧栏：** 仪表盘 · Telegram · **收藏** · **频道** · 任务 · 监听 · **资源库** · 设置

| 路由名 | 页面 | 说明 |
|--------|------|------|
| `dashboard` | 仪表盘 | 队列 / 磁盘 / TG 摘要 |
| `telegram` | Telegram | 登录 + **同步** + 计数 |
| `saved` | 收藏 | 收藏同步（补缺失） |
| `channels` | 频道 | 已加入 + 自定义；覆盖进度；新增 / 同步 / 下载 |
| `channel-detail` | 频道详情 | 覆盖进度 +「继续下载」+ 水位调整；自定义可删除 |
| `tasks` | 任务 | 消息链接下载（仅 message） |
| `watch` | 监听 | 增量自动入队；内容类型筛选 |
| `library` | 资源库 | 索引浏览、筛选、预览、扫盘 |
| `settings` | 设置 | 下载参数、目录、proxy、监听间隔等 |

兼容：`/tasks?tab=saved` → `/saved`；`/tasks?tab=channel` → `/channels`。

### 7.2 各页能力（详细）

**Telegram**

- 配置 / 登录：验证码、2FA、退出 Telegram（无二维码登录）
- **同步**：拉取可访问 **频道 + 群组** 写入 `tg_dialogs`（只替换 `is_custom=0`）；拉取 **Saved Messages** 写入 `saved_messages_cache`；随后刷新自定义频道标题 / 最新消息（`messages.getPeerDialogs`，不走 takeout / getHistory）
- 展示：**频道（含群与自定义）数**、**收藏数**；上次同步时间；失败原因

**收藏**

- 入队：「开始同步」→ `source=saved_all`；落盘 `{download_dir}/我的收藏/`
- **先拉后下**：任务启动后先拉取最新收藏列表（`phase=listing`，刷新「收藏数」，「已同步」为 —），再按缓存全量下载（`phase=downloading`）
- **无扫描水位**：靠 `skip_same` + `media_index` / 磁盘跳过已有
- 列表：同步记录（收藏数 / 已同步 / 失败 / 时间）；「清除完成」；单项可停止并删除
- 工具栏：**刷新 / 清除完成 / 开始同步**（有进行中同步时禁用开始）；**无**全部暂停/开始
- 分页：`GET /api/tasks?kind=saved`；SSE 经共享 `useAppEvents`

**频道**

- 列表：标题、`@username`、**类型**（频道粉 / 自定义绿 / 超级群蓝 / 群组黄）、**覆盖进度**、已下载数、状态
- 工具栏：**新增**（`@名称` 或频道 ID）+ **刷新**；操作列：**同步**（最新消息）+ **下载**
- 新增：解析公开频道/群；去重（chat id / 用户名）；未加入的私有频道、无权限会报中文错误；写入 `is_custom=1`，排在已加入之后
- 详情工作台：覆盖进度条、每批条数（`chat_download_state.batch_size`，默认 **100**）、「继续下载」
- **扫描水位**：`POST /api/channels/{chatId}/scan-cursor`，`mode=set` 或 `mode=align`
- 进行中批次可暂停 / 继续 / 取消；**重试失败** → `Mode=ids`，不推进扫描水位；可清除已完成批次
- 自定义频道详情：**删除**（仅 `is_custom=1`）后回列表
- 同频道同时只跑一批；继续下载与水位逻辑对已加入 / 自定义一致
- 监听水位与频道扫描水位分离；去重共用 `media_index`

**任务（消息下载）**

- 仅消息链接：多行粘贴 `t.me/...` → `source=url`（控制台已移除「频道下载」Tab）
- **列表：每条消息一行**（频道、消息 ID、文件名、类型、状态）；分页 `pageSize=50`
- 顶部：**刷新**（无全部暂停 / 全部开始）
- 操作：删除单项、「清除已完成」；SSE + 轮询兜底
- `POST /api/tasks/pause-all`、`start-all` 与 `source=chat_batch` 仍保留在 API，控制台不再提供入口

**资源库**

- **频道筛选**：顶栏下拉；选项第一项固定 **「我的收藏」**，其余为频道/群
- **媒体类型筛选**：全部 / **图片** / **视频**
- **预览**：列表缩略图 / 视频封面（`GET /api/library/{id}/thumb`）；点击浏览页加载原文件（`/file`，视频 Range）
- 分页、本地路径、删索引（可选删文件）
- **扫盘补索引**：只维护 `media_index`（增删失效项）；API 返回 `cursorsUpdated` **恒为 0**，**不改** `chat_download_state` / 收藏水位

**监听**

- 设置：`watch_interval_minutes`（默认 30，范围 10–300）
- 下拉：已加入 + 自定义频道 +「我的收藏」（已监听排除）；选项只显示标题 + 类型标签（收藏粉 / 自定义绿 / 频道粉 / 超级群蓝 / 群组黄），不带 `@用户名`
- 添加时可选手内容类型：全部 / 媒体 / 图片 / 视频；添加前同步该频道最新消息 ID 作为水位，**不立刻下载**
- 列表：频道（带类型标签）、内容类型、已下载、最新消息、上次运行、下次运行；「我的收藏」置顶
- 到期后下载 `(水位, 最新]`（含端点），按内容类型过滤附件，`skip_same` 去重；成功后推进监听水位
- **无**扩展名黑白名单、最小大小、关键词过滤
- 任务 source：`watch` / `watch_saved`

### 7.3 当前实现与目标差距

| 目标 | 现状 |
|------|------|
| 侧栏：收藏 / 频道 / 任务 / 监听 / 资源库 | ✅ |
| Telegram 刷新 + 计数 | ✅ |
| 频道列表 + 自定义频道 + 同步 / 继续下载 + 水位调整 | ✅ |
| 任务页仅消息下载 | ✅（无频道下载 Tab；收藏独立为 `/saved`） |
| 收藏同步 + `我的收藏/`（补缺失、无水位） | ✅ |
| 跳过已下载 | ✅ |
| 资源库筛选 / 预览 / 扫盘（不改水位） | ✅ |
| 监听增量入队 + 内容类型筛选 + 自定义频道 | ✅ |
| Takeout / rewrite_ext | ✅ |
| 关于（AGPL） | ✅ |
| Docker 多架构镜像 | ✅（`wannayoung/tdload`，BIND `3080`） |
| JSON 导出入队、二维码登录、任务日志 API、资源库搜索框等 | 可选后续 |

### 7.4 前端目录

```
web/
├── package.json
├── vite.config.ts          # dev proxy → 后端
├── src/
│   ├── api/                # http.ts + types
│   ├── stores/             # auth
│   ├── router/
│   ├── layouts/            # AppLayout（侧栏徽标按 kind）
│   ├── views/              # Login / Dashboard / Telegram / Saved / Channels /
│   │                       # ChannelDetail / Tasks / Watch / Library / Settings
│   ├── composables/        # useAppEvents（共享 SSE）、useMobile、useNoImage
│   └── main.ts
```

本地：浏览器打开 `http://127.0.0.1:3080`（Vite）；Docker：`http://127.0.0.1:3080`（同源托管静态资源）。

---

## 8. API

统一前缀 `/api`。除 `/health`、`/auth/login` 外需 `Authorization: Bearer <jwt>`。

### 8.1 公共

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 存活探测 |
| POST | `/api/auth/login` | `{username,password}` → `{token}` |
| GET | `/api/auth/me` | 当前用户 |
| POST | `/api/auth/ticket` | 短时 ticket（文件流 / SSE 备用） |
| GET | `/api/events` | SSE：`taskId/phase/done/total/speed/...` |
| GET | `/api/dashboard` | 队列 / 磁盘 / 账号摘要；含 `tasksMessageActive` / `tasksSavedActive` / `tasksChannelActive`（侧栏徽标） |
| GET/PUT | `/api/settings` | 读写运行时配置 |

### 8.2 Telegram 登录与同步

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| GET | `/api/tg/status` | 会话是否有效 | 已实现 |
| POST | `/api/tg/login/send_code` | 手机号发验证码 | 已实现 |
| POST | `/api/tg/login/sign_in` | 验证码 / 2FA | 已实现 |
| POST | `/api/tg/login/qr/start` | 返回 QR payload | 待定（可选） |
| GET | `/api/tg/login/qr/poll` | 轮询扫码结果 | 待定（可选） |
| POST | `/api/tg/logout` | 清除 session | 已实现 |
| POST | `/api/tg/credentials/desktop` | 写入 Desktop 公开 API 凭证 | 已实现 |
| POST | `/api/tg/sync/dialogs` | 刷新频道·群组 → `tg_dialogs` | 已实现（经 `/api/tg/sync`） |
| POST | `/api/tg/sync/saved` | 刷新收藏 → `saved_messages_cache` | 已实现（经 `/api/tg/sync`） |
| POST | `/api/tg/sync` | dialogs + saved 一次调用 | 已实现 |
| GET | `/api/tg/summary` | `{ dialogCount, savedCount, syncedAt }` | 已实现 |

实现应对齐 tdl 登录流程（见 [tdl 登录文档](https://docs.iyear.me/tdl/guide/login/)），session 文件写入 `session_dir`。同步已加入对话用 `messages.GetDialogs`；收藏用 Saved Messages 历史；自定义频道用 `messages.getPeerDialogs` 刷新最新消息。

### 8.3 频道

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| GET | `/api/channels` | 分页列表：`kind`、`isCustom`、覆盖水位、已下载数、状态 | 已实现 |
| POST | `/api/channels` | 新增自定义频道：`{ chat }`（`@名称` 或 ID）；已存在 409；无权/私有 400 | 已实现 |
| GET | `/api/channels/{chatId}` | 单对话详情 | 已实现 |
| DELETE | `/api/channels/{chatId}` | 仅删除自定义频道 | 已实现 |
| POST | `/api/channels/{chatId}/sync` | 刷新标题 / 用户名 / 最新消息 ID | 已实现 |
| GET | `/api/channels/{chatId}/download` | 下载工作台摘要：覆盖进度、进行中任务、历史批次、`defaultBatchSize`、`isCustom` | 已实现 |
| POST | `/api/channels/{chatId}/continue` | body `{ count }`：从水位向前扫一批入队；成功后记忆 `batch_size` | 已实现 |
| POST | `/api/channels/{chatId}/scan-cursor` | body `{ mode: "set", messageId }` 或 `{ mode: "align" }`：调整扫描水位 | 已实现 |
| DELETE | `/api/channels/{chatId}/batches/completed` | 清除该频道已结束续下批次 | 已实现 |

### 8.4 任务（一期）

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| GET | `/api/tasks` | 分页；**`kind`** 见下 | 已实现 |
| POST | `/api/tasks` | 创建（见下方 `source`） | 已实现（url / saved_all / chat_*） |
| POST | `/api/tasks/batch` | 批量 URL / JSON | 待定 |
| GET | `/api/tasks/{id}` | 详情；频道类返回 **聚合 counts**，不含 items 全量 | 已实现 |
| GET | `/api/tasks/{id}/items` | **仅 message/saved** 任务：单条 item 分页，默认 `pageSize=50` | 已实现 |
| POST | `/api/tasks/{id}/pause` | 暂停 | 已实现 |
| POST | `/api/tasks/{id}/resume` | 从 paused 继续 | 已实现 |
| POST | `/api/tasks/{id}/cancel` | 取消（`cancelled`） | 已实现 |
| POST | `/api/tasks/{id}/retry` | 整任务重试（保留兼容） | 已实现 |
| POST | `/api/tasks/{id}/retry-failed` | **仅 failed items** 重试；频道任务走 `Mode=ids`，不推进扫描水位 | 已实现 |
| DELETE | `/api/tasks/{id}` | 删除 | 已实现 |
| POST | `/api/tasks/pause-all` | | 已实现 |
| POST | `/api/tasks/start-all` | | 已实现 |
| DELETE | `/api/tasks/completed` | 清理已结束任务；可选 `?kind=saved` | 已实现 |
| GET | `/api/tasks/{id}/logs` | 日志 | 待定 |

**`GET /api/tasks` 查询：**

| 参数 | 说明 |
|------|------|
| `kind=message` | `source=url`；**pageSize 默认 50** |
| `kind=saved` | `source` 为收藏相关；**pageSize 默认 50** |
| `kind=channel` | `source` 为 `chat_continue` / `chat_range` / `watch`；**pageSize 默认 20** |
| `kind=channel_batch` | `source=chat_batch`（API 仍支持，控制台无入口） |
| `page` / `pageSize` | 可覆盖默认值 |

**频道任务列表项扩展字段（聚合自 `task_items`）：**

```json
{
  "id": 12,
  "title": "某频道批量",
  "chatId": -100123,
  "status": "running",
  "progressDone": 120,
  "progressTotal": 500,
  "itemCounts": {
    "pending": 10,
    "downloading": 2,
    "done": 480,
    "skipped": 5,
    "failed": 3
  }
}
```

message/saved 列表项可带 `itemsPreview`（当前页关联 items）或前端再调 `/items`。

**`source` 与产品入口对应：**

| source | 入口 | 说明 |
|--------|------|------|
| `url` | 任务 · 消息下载 | 多行 `t.me` 链接 |
| `saved_all` | 收藏页 · 开始同步 | 缓存全量 message id，`skip_same` 跳过已有；`out_subdir`: `我的收藏` |
| `chat_continue` | 频道详情 · 继续下载 | 指定 `chat_id` + `count`，从水位向前扫 |
| `chat_batch` | （API 仍支持，控制台无入口） | `chat_id` + `from_message_id` + `count` |
| `json` | （可选后续） | 导出 JSON |
| `watch` / `watch_saved` | 监听自动入队 | 增量区间 |

**创建 body 示例：**

```json
{
  "source": "url",
  "urls": [
    "https://t.me/telegram/193",
    "https://t.me/c/1697797156/151"
  ],
  "options": { "threads": 8, "group_album": true }
}
```

```json
{
  "source": "saved_all",
  "options": { "out_subdir": "我的收藏" }
}
```

```json
{
  "source": "chat_batch",
  "chat_id": -1001234567890,
  "from_message_id": 100,
  "count": 50
}
```

```json
{
  "source": "chat_continue",
  "chat_id": -1001234567890
}
```

### 8.5 资源库

| 方法 | 路径 | 说明 | 状态 |
|------|------|------|------|
| GET | `/api/library/filters` | 频道下拉：**第一项「我的收藏」**，其余对话（id + 标题） | 已实现 |
| GET | `/api/library` | 列表；见下方 query | 已实现 |
| POST | `/api/library/sync` | 扫盘补索引；只维护 `media_index`，`cursorsUpdated` 恒为 0 | 已实现 |
| DELETE | `/api/library/{id}` | 删索引（query: `delete_file=1`） | 已实现 |
| GET | `/api/library/{id}/file` | 原文件流；浏览页原图 / 视频 Range 播放 | 已实现 |
| GET | `/api/library/{id}/thumb` | 列表缩略图（图片缩放；视频 ffmpeg 封面，可缓存） | 已实现 |
| GET | `/api/library/{id}/thumb` | 可选：视频首帧 / 大图缩略（无则 404，前端用 mime 图标兜底） | 待定 |
| GET | `/api/about` | 版本号、许可证、源码链接（AGPL） | 已实现 |

**`GET /api/library` query：**

| 参数 | 说明 |
|------|------|
| `chat` | `saved` = 我的收藏；或具体 `chat_id`；空 = 全部 |
| `mediaType` | `all` / `image` / `video` |
| `q` | 文件名关键词 |
| `page` / `pageSize` | 分页 |

列表项含：`id`, `chatId`, `chatTitle`, `messageId`, `fileName`, `mime`, `size`, `localPath`, `previewUrl`（或相对 path 供前端拼 ticket）。

### 8.6 监听

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/watch` | 监听列表（「我的收藏」置顶；含 `kind` / `isCustom` / `contentType`） |
| GET | `/api/watch/candidates` | 可添加的频道（已监听排除；含自定义；收藏置顶） |
| POST | `/api/watch` | body `{ chatId, contentType }`；添加前同步最新消息 ID 为水位，不立刻下载 |
| DELETE | `/api/watch/{id}` | 移除监听 |

设置项：`watchIntervalMinutes`（10–300，默认 30）。

### 8.7 SSE 事件形状

**任务级（频道详情 / 收藏同步等任务进度；message 也可用）：**

```json
{
  "type": "task_progress",
  "taskId": 12,
  "kind": "channel",
  "phase": "downloading",
  "done": 120,
  "total": 500,
  "itemCounts": { "pending": 10, "downloading": 2, "done": 480, "skipped": 5, "failed": 3 },
  "doneBytes": 1048576,
  "totalBytes": 10485760,
  "speed": 2097152
}
```

**单条级（仅 message / saved Tab 订阅）：**

```json
{
  "type": "task_item_progress",
  "taskId": 12,
  "itemId": 901,
  "chatId": -100123,
  "messageId": 456,
  "status": "downloading",
  "doneBytes": 1024,
  "totalBytes": 4096
}
```

另可推送 `task_status`、`watch_hit`。前端共享 `useAppEvents`（单路 `/api/events`）：仪表盘 / 任务 / 收藏 / 频道详情订阅；频道详情处理频道进度，消息与收藏处理 `task_item_progress` + 可选任务总进度。轮询作兜底。

---

## 9. tdl 集成要点

### 9.1 依赖

```bash
go get github.com/iyear/tdl/core@v0.20.3
# 按需使用与 tdl 同版本的 gotd、以及下载进度相关包
```

锁定 `go.mod` 版本；升级 tdl 时回归登录与下载两条链路。`tdl/core` 是官方推荐给扩展/库使用的入口（见 [tdl-extension-template](https://github.com/iyear/tdl-extension-template)）。

### 9.2 下载封装（`internal/tg` + `internal/worker`）

- 输入：已登录 client、消息 Iter（来自 URL / 频道范围 / watch）
- 调用对齐 [tdl Downloader](https://pkg.go.dev/github.com/iyear/tdl/pkg/downloader)：`New(opts)` → `Download(ctx, limit)`
- 实现 `Progress`：
  - `OnAdd` → 插入 / 更新 `task_items`
  - `OnDownload` → 更新字节进度 → `progress` 总线
  - `OnDone` → 写 `media_index` 或记录错误
- 参数映射：

| 配置 | tdl CLI |
|------|---------|
| threads | `-t` |
| concurrency | `-l` |
| download_dir | `-d` |
| skip_same | `--skip-same` |
| group_album | `--group` |
| rewrite_ext | `--rewrite-ext` |
| takeout | `--takeout` |
| template | `--template` |
| proxy | 全局 / 账号代理 |

### 9.3 入队源解析

支持官方客户端「复制链接」格式，例如（详见[下载文档](https://docs.iyear.me/tdl/guide/download/)）：

- `https://t.me/telegram/193`
- `https://t.me/c/1697797156/151`
- 话题 / 评论链等变体

JSON：Telegram Desktop 导出或 tdl export 的消息 JSON。

### 9.4 Session 与安全

- 路径：`{session_dir}/account_{id}.json`（gotd session）
- 卷挂载：`/tdload/config` → 含 `config.yaml`、`tdload.db`、`session/`
- 容器只读根文件系统可选；session 目录必须可写
- 控制台密码与 TG session 分离：控制台管 Web，TG 管协议

### 9.5 Flood wait 与限流

- 捕获 flood wait：按服务器要求 sleep，任务保持 `running`，日志打 warn
- **大批量下载**可走 takeout（配置项默认开启）；**解析 / 新增 / 同步频道**只用普通 API + `messages.getPeerDialogs`，禁止为此申请 takeout 或扫全量 `getHistory`
- `concurrency` 默认不宜过高；设置页可调
- 可恢复错误自动重试；不可恢复标 `failed`
- 暂停：cancel 当前任务 context，items 未完成保持可续传

### 9.6 不要做的事

- 不要 `exec` 外部 `tdl` CLI 作为主路径（进度与生命周期难控）；以库调用为主
- 不要把 `app_hash`、session 打进前端或镜像

---

## 10. Worker 设计

对齐 xtools worker 思路（进程内队列、槽位、超时、drain）：

- 启动：扫描 `queued` / 异常中断的 `running` → 重新入队
- 槽位：`concurrency`（配置项），硬上限防止误配
- 单任务超时：可配置，默认 4h
- 关机：`CancellationToken` / `context` cancel → drain 30s → 强制 abort
- 每账号可绑独立 proxy（二期多账号时按 lane 隔离）

伪代码：

```go
for {
  select {
  case <-shutdown.Done():
    drainInFlight()
    return
  case taskID := <-queue:
    if !acquireSlot() { requeue(taskID); continue }
    go func() {
      defer releaseSlot()
      runTask(ctx, taskID) // downloader + progress
    }()
  }
}
```

---

## 11. Watcher 设计

1. 对每个 `enabled` 的 `watched_chats` 按 `watch_interval_minutes` 扫描增量  
2. 区间为 `(last_message_id, 最新]`（含端点）；收藏走 `saved_messages_cache` 区间  
3. 按 `filter_json.contentType`（全部 / 媒体 / 图片 / 视频）过滤附件  
4. 有命中则创建 `source=watch` / `watch_saved` 任务入队  
5. **不做**扩展名 / 大小 / 关键词过滤；去重仅靠 `skip_same` + `media_index`  
6. 入队后更新调度时间；下载成功后由 Worker 推进监听水位（与频道扫描水位分离）  
7. SSE 推送 `watch_hit` 

注意：监听是「发现 + 入队」，实际下载仍走 Worker，避免两套下载逻辑。

---

## 12. 本地调试

### 12.1 前置

- Go 1.26+（以 `go.mod` 为准）
- Node.js 20+、pnpm
- Telegram API 可不填（Desktop 公开凭证）；遇 `API_ID_PUBLISHED_FLOOD` 再申请自己的 `api_id`

### 12.2 步骤

```bash
cp config.example.yaml config/config.yaml
cp .env.example .env
# 编辑 .env：ADMIN_*、TG_APP_ID、TG_APP_HASH
# 编辑 config.yaml：download_dir 可用 ./downloads

mkdir -p config/session downloads

# 终端 1：API（本地监听 3000）
export BIND=0.0.0.0:3000
export CONFIG_PATH=./config/config.yaml
export WEB_DIR=./web/dist
go run ./cmd/tdload

# 终端 2：前端
cd web
pnpm install
pnpm dev
```

浏览器：<http://127.0.0.1:3080>（Vite）；也可先 `pnpm build` 后只用后端 `3000` 同源访问。

### 12.3 Vite 代理示例

```ts
// web/vite.config.ts
server: {
  port: 3080,
  proxy: {
    '/api': 'http://127.0.0.1:3000',
  },
}
```

### 12.4 调试建议

- TG 登录优先在本机跑通，再进 Docker（session 可复制到卷）
- 用少量公开频道消息链接验证下载与 SSE
- `journal` / stdout 使用结构化日志（`log/slog` 或 zap）

---

## 13. Docker 与多架构

### 13.1 镜像设计（多阶段）

仓库已有多阶段 `Dockerfile`（`CGO_ENABLED=0` + 纯 Go SQLite）：

1. **web**：`node:22-alpine` → `pnpm build` → `dist/`  
2. **builder**：`golang:1.26-bookworm` → `go build -o /out/tdload ./cmd/tdload`  
3. **runtime**：`debian:bookworm-slim` + `ca-certificates` → 拷贝二进制与 `web/dist`

环境默认：

```
BIND=0.0.0.0:3080
CONFIG_PATH=/tdload/config/config.yaml
WEB_DIR=/app/web
```

目录：

```
/tdload/config/     # config.yaml、tdload.db、session/
/tdload/downloads/  # 下载文件
/app/tdload         # 二进制
/app/web            # 前端静态文件
```

### 13.2 多架构构建

```bash
docker buildx create --use --name tdload-builder || true
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t wannayoung/tdload:latest \
  --push .
```

本机验证：

```bash
docker build -t tdload:local .
docker compose up -d --build
```

### 13.3 `docker-compose.yml`

```yaml
services:
  tdload:
    image: wannayoung/tdload:latest
    build: .
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

不内置数据库容器；SQLite 文件在 `./data/config/tdload.db`。

### 13.4 SQLite 与 CGO

当前镜像使用 **纯 Go** SQLite 驱动（`CGO_ENABLED=0`），便于交叉编译 amd64/arm64。

---

## 14. 安全与合规

### 14.1 AGPL-3.0

- 仓库根目录放置 `LICENSE`（AGPL-3.0）与 `NOTICE`（标明使用 [iyear/tdl](https://github.com/iyear/tdl) 等）
- 若提供网络服务（含私有部署对外的 Web UI），需向使用者提供**对应版本**的完整源码获取方式（例如：关于页链接到 Git 标签 / 附带 `SOURCES` 卷）
- 分发 Docker 镜像时，镜像说明或文档中写明源码地址与版本标签

### 14.2 其它

- 遵守 Telegram 服务条款；控制请求频率，避免滥用
- 控制台仅单用户，仍需强密码；建议反代 HTTPS
- 不在日志中打印验证码、session 明文、JWT

---

## 15. 实现里程碑

按依赖顺序推进；每阶段可独立演示。  
**进度同步：** 下文 checkbox 随实现勾选；前端壳与主题已在 M0/M3 交叉推进。

### 模块与页面总览（对照实现）

| 模块 | 职责 | 状态 |
|------|------|------|
| `cmd/tdload` + `internal/config` | 启动、YAML/env、目录 | 已完成 |
| `internal/db` | SQLite schema / 列迁移 | 已完成 |
| `internal/auth` + `/api/auth/*` | JWT / ticket / 管理员引导 | 已完成 |
| `internal/api` | REST 路由 | 已完成 |
| `internal/static` | SPA 静态托管 | 已完成 |
| `internal/tg` | 登录、同步、下载、自定义频道快照 | 已完成 |
| `internal/worker` + `progress` | 下载与 SSE | 已完成 |
| `internal/watcher` | 监听增量入队 + 内容类型 | 已完成 |
| `web` 壳 | 深色 + 粉色高亮、宽窄屏布局、@vicons/ionicons5 | 已完成壳 |
| Docker | 多架构镜像 | **已完成**（`wannayoung/tdload`，BIND `3080`） |

**前端导航：** 仪表盘 · Telegram · 收藏 · 频道 · 任务 · 监听 · 资源库 · 设置

**当前侧栏：** 同上（收藏 / 频道 / 任务按 kind 显示活跃数徽标）

### M0 — 仓库骨架

- [x] `go mod` 初始化、`cmd/tdload`、健康检查、配置加载
- [x] SQLite 迁移、`users` 与 `ADMIN_*` 引导
- [x] JWT 登录、静态资源托管
- [x] `LICENSE` / `NOTICE` / 根 README
- [x] 表结构预建：`tg_accounts` / `tasks` / `task_items` / `media_index` / `task_logs` / `watched_chats`
- [x] `/api/dashboard`、`/api/settings`、`/api/tg/status`（占位）

### M1 — Telegram 会话

- [x] app_id/hash 配置（环境变量 + 设置页 / Telegram 页可写）
- [x] 验证码登录 + session 持久化（`config/session/account_1.json`）
- [ ]（可选）二维码登录
- [x] `/api/tg/status` 真实探活、logout
- [x] 前端 Telegram 页：凭证、发码、验证码/2FA、退出

### M2 — 下载闭环

- [x] URL 入队 API（`POST /api/tasks`）
- [x] Worker + gotd/tdl 解析链接下载 + Progress → SSE
- [x] 暂停 / 重试 / 失败原因
- [x] 设置页读写 YAML（下载参数已接到 Worker）
- [ ] JSON 导出入队（后续）

### M3 — 前端壳（已完成）

- [x] Vue 脚手架（Vue3 + Vite + Naive UI + Pinia + @vicons/ionicons5）
- [x] 深色模式 + 粉色主色（`#f472b6`）；宽屏侧栏 / 窄屏顶栏下拉（≤1000px）
- [x] 登录、仪表盘、TG、任务（链接入队）、设置页
- [x] 任务真实列表 + SSE 进度 + 轮询兜底

### M3.5 — 产品对齐（频道 · 任务 · 同步）（已完成）

依赖 M1 + M2 Worker；按 §7.2 验收。

**后端**

- [x] 迁移：`tg_dialogs`、`saved_messages_cache`、`chat_download_state`（§6.8–6.10）
- [x] `POST /api/tg/sync`、`GET /api/tg/summary`
- [x] `GET /api/channels`（含已下载数、扫描水位）
- [x] `POST /api/channels/{chatId}/scan-cursor`（set / align）
- [x] Worker：`source=saved_all`（输出 `我的收藏/`；无水位，只补缺失）
- [x] Worker：`chat_batch` / `chat_continue` / `ids`（频道历史扫描 + 失败重试不抬水位）
- [x] 收藏 `media_index` 使用 **self user id** 作为 `chat_id`（`tg.FavoritesChatID`）
- [x] 任务 API：`kind` 分页、`itemCounts`、`/resume` `/cancel` `/retry-failed`、`/tasks/items`
- [x] 资源库 API：`/library/filters`、chat / mediaType、文件流预览；扫盘不改水位

**前端**

- [x] 侧栏：收藏 · 频道 · 任务 · 监听 · 资源库（活跃徽标按 kind）
- [x] Telegram 页：同步 + 计数
- [x] `ChannelsView.vue` + `ChannelDetailView.vue`（继续下载 + 水位对话框）
- [x] `TasksView.vue`：仅消息下载；`SavedView.vue`：独立收藏同步
- [x] `LibraryView.vue`：筛选 + 图/视频预览
- [x] `useAppEvents`：共享单路 SSE（避免浏览器连接数耗尽）

### M3.6 — 频道批量 Worker（已完成）

- [x] `internal/tg`：按 `chat_id` 拉历史、按范围/续下/ids 生成 jobs
- [x] Worker 更新 `task_items` 并仅推送 **task_progress + itemCounts**
- [x] 频道工作台：覆盖进度 +「继续下载」；batch_size 持久化；监听不抬高频道扫描水位

### M4 — Docker（已完成）

- [x] 多阶段 Dockerfile（`CGO_ENABLED=0`）
- [x] compose（`.env.example` + `3080`）
- [x] buildx amd64/arm64 发布（`wannayoung/tdload`）
- [x] 卷权限与 session 持久化（`/tdload/config`、`/tdload/downloads`）

### M5 — 监听

- [x] `watched_chats` API（列表 / 候选 / 添加 / 删除）
- [x] Watcher 定时扫描 + 水位（含端点区间）+ 入队 `watch` / `watch_saved`
- [x] 设置页监听间隔；监听页（收藏置顶；类型标签；内容类型）
- [x] SSE `watch_hit`；`filter_json.contentType`：全部 / 媒体 / 图片 / 视频
- [x] 候选含自定义频道；添加前同步最新消息 ID

### M6 — 硬化

- [x] takeout、rewrite_ext 接入下载路径；资源库扫盘补索引 / 删索引（不改水位）
- [x] 任务 pause-all / start-all（**仅 API**；任务页已去掉入口）
- [x] 自定义频道：新增 / 去重 / 权限报错 / 同步最新 / 删除；对话同步保留 `is_custom`
- [ ] 结构化日志、基础 metrics（可选）
- [x] 关于信息：设置页版本号 + 源码链接（AGPL，`GET /api/about`）

---

## 16. 风险与约束

| 风险 | 应对 |
|------|------|
| tdl 内部 API 变更 | 锁定次版本；升级列 checklist 回归 |
| FloodWait / 限速 | 退避、takeout、降并发、代理 |
| Premium 带宽差异 | 文档说明速度上限取决于账号 |
| Session 损坏 | status=`expired`，引导重新登录 |
| 大文件磁盘满 | 仪表盘磁盘监控；任务失败可辨识 |
| AGPL 传染 | 整仓同许可；不链闭源专有模块 |
| 交叉编译 SQLite | 优先纯 Go 驱动 |

---

## 17. 与 xtools 的对照（方便抄作业）

| 能力 | xtools | tdload |
|------|--------|--------|
| 语言 | Rust + Axum | Go |
| 下载引擎 | xchina-core | tdl/core + gotd |
| DB | MySQL 外置 | SQLite 内置文件 |
| 前端 | Vue3 + Naive UI | 同左 |
| 进度 | SSE | 同左 |
| 配置 | YAML + MYSQL_/ADMIN_ | YAML + ADMIN_/TG_ |
| 镜像 | 多阶段 amd64/arm64 | 同左；运行镜像含 ffmpeg（资源库视频封面） |
| 参考路径 | 整个 `xtools/` | 只读，不 import |

可重点参考的 xtools 文件：

- 部署与环境：[xtools/README.md](../xtools/README.md)、[xtools/Dockerfile](../xtools/Dockerfile)、[xtools/docker-compose.yml](../xtools/docker-compose.yml)
- API 路由形状：[xtools/crates/xtools/src/api/mod.rs](../xtools/crates/xtools/src/api/mod.rs)
- Worker 槽位与 drain：[xtools/crates/xtools/src/worker.rs](../xtools/crates/xtools/src/worker.rs)
- 前端栈：[xtools/web/package.json](../xtools/web/package.json)

---

## 18. 下一步

**当前：** 一期 + 监听 + 自定义频道均已落地。主线可视为完成。

建议顺序：

1. ~~M1 Telegram 登录~~ ✅（QR 未做）
2. ~~M2 链接下载闭环~~ ✅
3. ~~**M3.5 / M3.6** 频道工作台 + 收藏独立页 + 消息任务~~ ✅
4. ~~**M4** Docker 多架构~~ ✅（`wannayoung/tdload`）
5. ~~**M5** 监听自动入队 + 内容类型~~ ✅
6. ~~**M6** 扫盘 / takeout / 自定义频道 / 关于页~~ ✅（结构化日志/metrics 可选）

可选后续：二维码登录、JSON 导出入队、资源库缩略图 / 关键词搜索 UI、任务日志 API。
