# Drop & Deploy — 上传即部署

把一个 `.html` 文件或 `.zip` 压缩包拖进网页，立即部署成一个可访问的静态站点。
私有自托管，Go 单二进制 + Docker，零外部依赖。

---

## 它能做什么

- **拖拽上传**：HTML 单文件，或包含 `index.html` 的 zip 压缩包
- **即时部署**：每次上传生成一个独立站点 `/s/<name>/`，互不覆盖
- **站点管理**：网页内查看 / 打开 / 删除已部署站点
- **私有鉴权**：上传/管理接口由 Bearer Token 保护，公网只能只读访问已部署站点

---

## 架构

```
                    ┌─────────────────────────────────────┐
   Caddy  ──反代──► │  deployer (Go 单二进制, scratch 镜像) │
  (HTTPS)           │                                      │
                    │  GET  /              落地页(内嵌)     │  公开
                    │  GET  /s/<name>/*    已部署站点       │  公开
                    │  POST /api/upload    上传+解压+部署    │  需 Token
                    │  GET  /api/sites     站点列表          │  需 Token
                    │  DELETE /api/sites/<name>  删除站点    │  需 Token
                    └──────────────┬───────────────────────┘
                                   │ 可写卷
                            ./data/sites/<name>/...
                            ./data/meta.json
```

- **后端**：纯标准库 Go（Go 1.22 `ServeMux` 路由 + `embed` 内嵌前端 + `archive/zip` 安全解压）
- **前端**：`server/web/` 下纯静态三件套，编译时打进二进制
- **数据**：仅 `./data` 一个可写卷，站点文件与元数据都落在这里

---

## 项目结构

```
.
├── docker-compose.yml      # 容器编排（接入 Caddy 的 proxy-network）
├── .env.example            # DEPLOY_TOKEN 模板
├── server/                 # Go 后端
│   ├── main.go             # 启动、路由、Token 中间件
│   ├── config.go           # 环境变量配置
│   ├── upload.go           # 上传处理：判类型 / 命名 / 原子发布
│   ├── unzip.go            # 安全解压（防 Zip-Slip / zip bomb）
│   ├── store.go            # 站点元数据与命名工具
│   ├── sites.go            # 列表 / 删除接口
│   ├── Dockerfile          # 多阶段构建 → scratch
│   └── web/                # 前端（编译时内嵌）
│       ├── index.html
│       ├── css/app.css
│       └── js/{upload,sites}.js
└── data/                   # 运行时数据（已 gitignore）
```

---

## 部署

### 前置条件

- Docker + Docker Compose
- 外部网络 `proxy-network`（与 Caddy 共享）

### 启动

```bash
# 1. 设置上传 Token（私有部署的唯一凭证）
cp .env.example .env
sed -i "s/changeme/$(openssl rand -hex 16)/" .env

# 2. 构建并启动
docker compose up -d
```

服务监听 `127.0.0.1:8080`，通过 Caddy 反代对外。首次打开网页，点右上角 🔑 填入 `.env` 里的 Token。

### 停止

```bash
docker compose down
```

---

## 配置

`docker-compose.yml` 环境变量：

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DEPLOY_TOKEN` | 上传/管理接口的 Bearer Token（**必填**） | — |
| `DATA_DIR` | 数据根目录 | `/data` |
| `MAX_UPLOAD_MB` | 单次上传体积上限（MB） | `50` |
| `MAX_UNZIP_MB` | zip 解压后总大小上限（MB），防 zip bomb | `200` |

---

## API（命令行上传）

```bash
TOKEN=$(grep DEPLOY_TOKEN .env | cut -d= -f2)

# 上传单文件
curl -H "Authorization: Bearer $TOKEN" \
     -F file=@page.html -F name=my-page \
     http://127.0.0.1:8080/api/upload

# 上传 zip（根目录需含 index.html）
curl -H "Authorization: Bearer $TOKEN" \
     -F file=@site.zip -F name=my-site -F overwrite=true \
     http://127.0.0.1:8080/api/upload
```

成功返回 `201` 与站点信息；同名且未勾选覆盖返回 `409`。

---

## 安全说明

- 上传/管理接口全部需要 Token；公网只能只读访问 `/s/<name>/`
- zip 解压有三道防线：路径穿越（Zip-Slip）拦截、解压总大小封顶、目录创建限制在站点目录内
- 上传体积与解压大小均有上限，防止资源耗尽
- 容器以非 root（`uid=gid=1001`，与宿主用户对齐）运行，监听非特权端口 `8080`，数据卷文件归宿主用户所有
  - 首次部署或更换宿主用户时，确保 `./data` 属主为该 uid：
    `docker run --rm -v "$PWD/data:/data" alpine chown -R 1001:1001 /data`
