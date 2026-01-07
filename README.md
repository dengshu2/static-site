# Static Web Server Demo

使用 Docker Compose 部署的静态网页服务器项目。

## 技术栈

- [static-web-server](https://github.com/static-web-server/static-web-server) - 高性能、轻量级的静态文件服务器
- Docker Compose

## 项目结构

```
.
├── docker-compose.yml    # Docker Compose 配置
├── html/                 # 静态文件目录
│   └── index.html        # 首页
├── .gitignore
└── README.md
```

## 快速开始

### 启动服务

```bash
docker compose up -d
```

### 访问网站

打开浏览器访问: http://localhost:8080

### 停止服务

```bash
docker compose down
```

## 添加更多页面

将你的 HTML、CSS、JavaScript 和其他静态文件放入 `html/` 目录即可。

## 配置说明

可以通过修改 `docker-compose.yml` 中的环境变量来配置服务器：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `SERVER_LOG_LEVEL` | 日志级别 | `info` |
| `SERVER_DIRECTORY_LISTING` | 启用目录列表 | `false` |
| `SERVER_COMPRESSION` | 启用 gzip 压缩 | `true` |
| `SERVER_CACHE_CONTROL_HEADERS` | 缓存控制头 | `true` |

更多配置选项请参考: https://static-web-server.net/configuration/environment-variables/
