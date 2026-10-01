# Drop & Deploy

自托管静态项目目录与部署工具。访客无需 Token 即可搜索和浏览项目；管理者可以上传 HTML/ZIP、维护项目标题与简介、查看访问统计、覆盖、删除和恢复旧版本。

## 页面与权限

| 路径 | 用途 | 权限 |
|---|---|---|
| `/` | 公开项目目录 | 公开只读 |
| `/admin/` | 上传和管理页面 | 页面公开，操作需要登录 |
| `/s/<name>/` | 已部署静态项目 | 公开只读 |
| `/covers/<name>.jpg` | 项目封面截图 | 公开只读 |
| `GET /api/sites` | 项目列表（公开目录不含文件数和体积） | 公开只读 |
| `GET /api/analytics` | 项目数、容量和匿名访问汇总 | 公开只读 |
| `POST /api/session` / `DELETE /api/session` | 登录（换取 30 天会话）/ 退出 | 管理页面 |
| `POST /api/upload` | 上传并部署 | 登录或 Bearer Token |
| `GET /api/sites/<name>` | 一个项目：近 30 天每日访问、历史版本 | 登录或 Bearer Token |
| `PATCH /api/sites/<name>` | 修改项目标题和简介 | 登录或 Bearer Token |
| `POST /api/sites/<name>/cover` | 重新截封面 | 登录或 Bearer Token |
| `DELETE /api/sites/<name>` | 移入回收站 | 登录或 Bearer Token |
| `GET /api/trash` | 查看历史版本与回收站 | 登录或 Bearer Token |
| `POST /api/trash/<id>/restore` | 恢复；加 `?replace=1` 时替换同名的当前版本（当前版本进入历史） | 登录或 Bearer Token |
| `DELETE /api/trash/<id>` | 永久删除 | 登录或 Bearer Token |

## 登录

管理页面输入一次 Token，换成一个 30 天有效的会话 Cookie：`HttpOnly`、`SameSite=Strict`、只属于管理端域名（HTTPS 下带 `__Host-` 前缀）。页面脚本读不到它，也从不保存 Token。Cookie 内容是过期时间加 HMAC 签名，密钥由 `DEPLOY_TOKEN` 派生：服务重启不会退出登录，**更换 Token 会让所有会话立即失效**。

管理端（deploy）和上传内容（pages）属于同一个站点（同一个注册域名），SameSite 挡不住从上传页面发往管理端的请求，所以凭 Cookie 发起的写操作还必须来自管理页面本身（`Origin` 或 `Sec-Fetch-Site: same-origin`），否则返回 403。脚本和命令行照旧用 `Authorization: Bearer <Token>`。

## 项目封面

每个项目在发布、替换或恢复后自动截一张首屏（1280×800 缩到 800×500），公开目录用它做封面。截图由单独的 `shot` 容器完成（`shot/` 目录，headless Chrome 加中文和 emoji 字体），deployer 本身仍是不带浏览器的小镜像：

- `shot` 只在单独的 `shot` 网络里，能访问 deployer 和外网，碰不到 `proxy-network` 上的其他服务，也不持有任何密钥；只接受 `/s/<name>/` 形式的路径。
- 它通过 `INTERNAL_HOST`（`static-deployer`）访问项目：这个主机名只返回项目内容，截图不计入访问量。
- 截图失败会在 10 秒、1 分钟、5 分钟后重试；之后每小时补一次所有还没有封面的项目。没有封面的项目显示标题首字母。
- 封面存在 `data/covers/`，随 `data/` 一起备份；地址带版本号，可以长期缓存。

## 前端结构

无构建步骤，全部通过 `//go:embed` 打包进二进制：

```text
server/web/shared/quiet.css       共用设计规范 Quiet UI（复制进来的版本，不要直接改）
server/web/shared/site.css        两端共用的补充：自托管字体、应用标识颜色、封面
server/web/shared/ui.js           两端共用的 DOM 和格式化工具
server/web/shared/fonts/*.woff2   自托管的 Figtree 拉丁子集
server/web/public/                公开目录（封面卡片、搜索、排序）
server/web/admin/                 管理端：登录、工作台、项目页（#site/<name>）、历史（#history）
```

`web/shared` 不单独挂路由：公开端和管理端各自的静态文件系统在未命中时回落到它，因此 `/quiet.css`、`/fonts/figtree-v1.woff2` 与 `/admin/quiet.css`、`/admin/fonts/figtree-v1.woff2` 指向同一份内嵌字节。两个 Origin 的 CSP 只允许本站资源（外加 Cloudflare 自动注入的统计脚本），字体必须各自提供，不能跨域共享。

界面遵循 Quiet UI：不用弹窗、下拉菜单和浏览器确认框；需要确认的不可撤销操作改成"再点一次确认"，删除可以撤销。自带字体只包含拉丁字母、数字和标点，中文回落到系统字体（PingFang SC / 微软雅黑 / Noto Sans SC）。`.woff2` 按文件名缓存一年（`immutable`），其余页面与脚本仍然每次重新验证，**更换字体文件时必须同时改文件名**。

## 安全边界

- 公开 API 不返回原始上传文件名，也不包含任何管理操作。
- 访问统计只记录项目、日期和次数，不保存访客 IP、Cookie 或其他个人信息。
- 管理 Token 使用固定时间摘要比较，失败请求按客户端 IP 限速；登录后的会话见上文"登录"。
- 上传文件先流式写入 `/data/tmp`，不依赖 scratch 镜像中的系统 `/tmp`。
- ZIP 拒绝路径穿越、反斜杠路径、符号链接、特殊文件、重复路径和过深目录。
- 同时限制上传体积、实际解压体积、ZIP 文件数、站点数和全部站点总容量。
- 覆盖时旧版本先进入回收站；新文件和元数据保存失败会自动回滚。
- 删除和覆盖的旧版本默认保留 7 天，可以从管理页恢复。
- `/s/` 及缺少 `index.html` 的目录不会生成服务器目录列表。
- 管理 UI 使用 CSP、禁止被 iframe 嵌入，并包含基础安全响应头。
- 容器以非 root 用户运行，根文件系统只读，移除全部 Linux capabilities，并配置健康检查和资源限制。

生产环境使用三个独立 Origin：`site.dengshu.ovh` 提供公开目录，`deploy.dengshu.ovh` 提供管理端，`pages.dengshu.ovh` 承载用户上传内容。上传页面因此无法读取管理端内存中的 Token。旧的 `/admin/` 和 `/s/<name>/` 地址由 Caddy 永久重定向到对应新域名。

## 启动

```bash
cp .env.example .env
sed -i "s/changeme/$(openssl rand -hex 24)/" .env
chmod 600 .env
docker compose up -d --build
```

公开目录：`https://site.dengshu.ovh/`

管理页面：`https://deploy.dengshu.ovh/`

项目内容：`https://pages.dengshu.ovh/s/<name>/`

## 配置

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `DEPLOY_TOKEN` | 无 | 管理 Token，至少 7 个字符；公开部署建议使用 24 位以上强随机值 |
| `DATA_DIR` | `/data` | 数据根目录 |
| `PUBLIC_HOST` | `site.dengshu.ovh` | 公开目录 Host |
| `ADMIN_HOST` | 同公开 Host | 管理端 Host |
| `CONTENT_HOST` | 同公开 Host | 上传内容 Host |
| `PUBLIC_BASE_URL` | 按公开 Host 生成 | 公开页面绝对地址 |
| `ADMIN_BASE_URL` | 按管理 Host 生成 | 管理页面绝对地址 |
| `CONTENT_BASE_URL` | 按内容 Host 生成 | 项目链接前缀 |
| `TZ` | `Asia/Shanghai` | 访问统计的每日归档时区 |
| `MAX_UPLOAD_MB` | `50` | 单文件上传上限 |
| `MAX_UNZIP_MB` | `200` | ZIP 实际解压总大小上限 |
| `MAX_TOTAL_MB` | `10240` | 全部在线项目总容量 |
| `MAX_ZIP_FILES` | `5000` | 单个 ZIP 最大条目数 |
| `MAX_PATH_DEPTH` | `20` | ZIP 最大目录深度 |
| `MAX_SITE_NAME_LEN` | `63` | 站点名长度上限 |
| `MAX_SITES` | `1000` | 在线站点数量上限 |
| `TRASH_RETENTION_HOURS` | `168` | 回收站保留时间 |
| `SHOT_URL` | 空 | 截图服务地址（如 `http://static-shot:9000`）；空则不生成封面 |
| `INTERNAL_HOST` | 空 | 截图服务访问本服务用的主机名（如 `static-deployer`），只返回项目内容、不计访问 |

## ZIP 结构

支持两种常见布局：

```text
index.html
assets/...
```

或单层包装目录：

```text
dist/
  index.html
  assets/...
```

HTML 内的资源应使用相对路径。根绝对路径 `/assets/...` 会指向内容域的根目录，而不是当前项目目录。

## 命令行上传

```bash
TOKEN=$(sed -n 's/^DEPLOY_TOKEN=//p' .env)

curl -H "Authorization: Bearer $TOKEN" \
  -F name=my-page \
  -F overwrite=false \
  -F file=@page.html \
  http://127.0.0.1:8080/api/upload
```

## 备份与恢复

创建权限为 `600` 的压缩备份和 SHA-256 校验文件：

```bash
./scripts/backup.sh
```

默认保留 7 天，可通过 `BACKUP_DIR` 和 `BACKUP_RETENTION_DAYS` 调整。恢复前先停止服务并再次备份当前数据：

```bash
docker compose down
mv data "data.before-restore.$(date +%s)"
tar -xzf backups/static-site-data-YYYYMMDDTHHMMSSZ.tar.gz
docker compose up -d
```

仓库附带 systemd 定时任务模板，安装后每天自动备份：

```bash
sudo cp ops/static-site-backup.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now static-site-backup.timer
```

## 测试

项目使用 Go 1.26.5：

```bash
cd server && go test ./... && go vet ./...
cd shot && go test ./... && go vet ./...
```

截图服务的浏览器测试需要 Chrome，没设 `CHROME_PATH` 时跳过；可以先 `go test -c`，再在 `shot` 镜像里运行。CI 还会检查 Go 格式、浏览器 JavaScript 语法和两个 Docker 镜像的构建。

## 生产域名拓扑

Cloudflare 使用代理 A 记录：

```text
site.dengshu.ovh    -> <源站 IP>
deploy.dengshu.ovh  -> <源站 IP>
pages.dengshu.ovh   -> <源站 IP>
```

Compose 配置：

```yaml
PUBLIC_HOST: site.dengshu.ovh
ADMIN_HOST: deploy.dengshu.ovh
CONTENT_HOST: pages.dengshu.ovh
PUBLIC_BASE_URL: https://site.dengshu.ovh
ADMIN_BASE_URL: https://deploy.dengshu.ovh
CONTENT_BASE_URL: https://pages.dengshu.ovh
```

Caddy 为三个域名分别代理到 `static-deployer:8080`。应用按 Host 只暴露对应的公开、管理或内容路由。

## 项目目录与访问统计

- 公开目录以封面卡片展示，支持按标题、简介或站点名搜索，并可按更新时间、访问量和名称排序；文件数、体积只在管理端显示。
- 管理端每个项目有单独的页面：封面和重新截图、近 30 天每日访问、修改资料、上传新版本、删除（可撤销）以及历史版本恢复。
- 项目标题和简介可以在上传时填写，也可以在管理页后续修改。
- 访问量统计 HTML 页面和目录首页的成功访问；脚本、样式、图片以及不存在的页面不会计数。
- `data/stats.json` 保存累计访问、最后访问时间和最近 90 天的每日汇总，并随 `data/` 一起备份。
- 覆盖部署会保留原项目的创建时间、资料和访问量；覆盖前内容进入“版本历史与回收站”。
