# tdload

自托管 Telegram 批量下载与监控控制台（Web）：登录管理、任务队列、进度监控；二期支持频道/群监听自动入队。后端 Go + [iyear/tdl](https://github.com/iyear/tdl)，前端 Vue（深色 + 粉色高亮，布局对齐 xtools）；Docker 支持 `linux/amd64` / `linux/arm64`。

**开发文档（架构、API、分期、清单）：** [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)

仓库内 [`xtools/`](xtools/) 为参考样板，不参与本项目构建。

## 当前进度

- **已具备**：Telegram **同步频道/收藏**、**频道**页、任务 **三 Tab**、**收藏下载**（`downloads/我的收藏`）、**资源库**筛选与图/视频预览、消息链接下载、SSE
- **待完成**：**频道批量 Worker**（`chat_batch` / `chat_continue`），见 [docs/DEVELOPMENT.md M3.6](docs/DEVELOPMENT.md)
- **下一步**：M3.6 → Docker（M4）→ 监听（M5）

## 会话能保存多久？

| 会话 | 时长 |
|------|------|
| 控制台登录（JWT） | **30 天**，过期需重新登录网页 |
| Telegram（session 文件） | **长期有效**，直到你在本应用点「退出 Telegram」、或在手机 Telegram「设备」里踢掉、或账号异常失效 |

重启 `tdload` 进程一般**不会**掉 Telegram 登录（session 在 `config/session/`）。

## 本地调试

```bash
cp .env.example .env          # 填 ADMIN_*、可选 TG_*
cp config.example.yaml config/config.yaml

# 终端 1
export BIND=0.0.0.0:3000 CONFIG_PATH=./config/config.yaml WEB_DIR=./web/dist
go run ./cmd/tdload

# 终端 2
cd web && pnpm install && pnpm dev
```

浏览器打开 <http://127.0.0.1:3080>，默认管理员 `wannayoung` / `52111314`（见 `.env`，登录页已预填）。

### 测试 Telegram 登录（M1）

1. **重启后端**后会自动写入 Desktop 公开凭证（`api_id=2040`，与 tdl 相同），不用再去 my.telegram.org  
2. 打开控制台 → **Telegram**，应显示「Desktop 公开凭证」  
3. 输入手机号（如 `+86138...`）→ 发送验证码 → 登录  
4. 可选：设置里填 `socks5://127.0.0.1:1080` 后再发码  

若遇 `API_ID_PUBLISHED_FLOOD`，再在 Telegram 页改用自己的 api_id。

### 测试下载（M2）

1. **重启后端**以加载 Worker  
2. 确认 Telegram 已登录  
3. 打开 **任务**，粘贴一条公开频道消息链接，例如 `https://t.me/telegram/193`（每行一条）  
4. 点「开始下载」，应看到进度；完成后文件在设置里的下载目录（默认 `./downloads`）  
5. 可测：暂停、重试、删除、清理已完成  

许可证：AGPL-3.0（见 [LICENSE](LICENSE)、[NOTICE](NOTICE)）
