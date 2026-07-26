# Drop & Deploy

自托管静态项目目录与部署工具。访客无需 Token 即可搜索和浏览项目；管理者可以上传 HTML/ZIP、维护项目标题与简介、查看访问统计、覆盖、删除和恢复旧版本。

## 页面与权限

| 路径 | 用途 | 权限 |
|---|---|---|
| `/` | 公开项目目录 | 公开只读 |
| `/admin/` | 上传和管理页面 | 页面公开，写操作需要 Token |
| `/s/<name>/` | 已部署静态项目 | 公开只读 |
| `GET /api/sites` | 安全的公开项目元数据 | 公开只读 |
| `GET /api/analytics` | 项目数、容量和匿名访问汇总 | 公开只读 |
| `POST /api/upload` | 上传并部署 | Bearer Token |
| `PATCH /api/sites/<name>` | 修改项目标题和简介 | Bearer Token |
| `DELETE /api/sites/<name>` | 移入回收站 | Bearer Token |
| `GET /api/trash` | 查看回收站 | Bearer Token |
| `POST /api/trash/<id>/restore` | 恢复项目 | Bearer Token |
| `DELETE /api/trash/<id>` | 永久删除 | Bearer Token |

Token 只保存在管理页面的 JavaScript 内存中，不会写入 Cookie、`localStorage` 或 `sessionStorage`。刷新或关闭页面后自动清除。

## 安全边界

- 公开 API 不返回原始上传文件名，也不包含任何管理操作。
- 访问统计只记录项目、日期和次数，不保存访客 IP、Cookie 或其他个人信息。
- 管理 Token 使用固定时间摘要比较，失败请求按客户端 IP 限速。
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

默认保留 14 天，可通过 `BACKUP_DIR` 和 `BACKUP_RETENTION_DAYS` 调整。恢复前先停止服务并再次备份当前数据：

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
cd server
go test ./...
go test -race ./...
go vet ./...
```

CI 还会检查 Go 格式、浏览器 JavaScript 语法和 Docker 镜像构建。

## 生产域名拓扑

Cloudflare 使用代理 A 记录：

```text
site.dengshu.ovh    -> 151.245.106.129
deploy.dengshu.ovh  -> 151.245.106.129
pages.dengshu.ovh   -> 151.245.106.129
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

- 公开目录支持按标题、简介或站点名搜索，并可按更新时间、访问量、名称和体积排序。
- 项目标题和简介可以在上传时填写，也可以在管理页后续修改。
- 访问量统计 HTML 页面和目录首页的成功访问；脚本、样式、图片以及不存在的页面不会计数。
- `data/stats.json` 保存累计访问、最后访问时间和最近 90 天的每日汇总，并随 `data/` 一起备份。
- 覆盖部署会保留原项目的创建时间、资料和访问量；覆盖前内容进入“版本历史与回收站”。
