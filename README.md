# Static Site

使用 Docker Compose 部署的静态网站服务。

## 技术栈

- [static-web-server](https://github.com/static-web-server/static-web-server) - 高性能、轻量级的静态文件服务器
- Docker Compose

## 项目结构

```
.
├── docker-compose.yml    # Docker Compose 配置
├── html/                 # 静态文件目录
│   ├── index.html        # 首页
│   └── ...               # 其他页面
├── .gitignore
└── README.md
```

## 快速开始

### 启动服务

```bash
docker compose up -d
```

### 停止服务

```bash
docker compose down
```

## 访问页面

服务启动后，通过以下规则访问页面：

| 文件路径 | 访问 URL |
|---------|----------|
| `html/index.html` | http://localhost:8080/ |
| `html/about.html` | http://localhost:8080/about.html |
| `html/docs/guide.html` | http://localhost:8080/docs/guide.html |
| `html/assets/style.css` | http://localhost:8080/assets/style.css |

### 访问规则

- **根路径 `/`** → 自动加载 `html/index.html`
- **其他路径** → 直接映射到 `html/` 目录下的对应文件
- 支持子目录，如 `html/docs/api.html` → `/docs/api.html`

### 示例

```bash
# 添加新页面
echo "<h1>About</h1>" > html/about.html

# 访问
curl http://localhost:8080/about.html
```

## 配置说明

可以通过修改 `docker-compose.yml` 中的环境变量来配置服务器：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `SERVER_LOG_LEVEL` | 日志级别 | `info` |
| `SERVER_DIRECTORY_LISTING` | 启用目录列表 | `false` |
| `SERVER_COMPRESSION` | 启用 gzip 压缩 | `true` |
| `SERVER_CACHE_CONTROL_HEADERS` | 缓存控制头 | `true` |

更多配置选项请参考: https://static-web-server.net/configuration/environment-variables/
