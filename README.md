# Prompt Library — 用提示词复现工具

一个静态工具集，每个工具都附带生成它的大模型提示词。复制提示词，粘贴给 Claude、GPT 或 Gemini，即可得到同类工具的完整代码。

🔗 **在线访问**：通过 Nginx Proxy Manager 代理对外提供服务

---

## 工具列表

| 工具 | 描述 | 提示词 |
|------|------|--------|
| [小票物理模拟](html/receipt-physics.html) | Three.js + Verlet 积分的交互式热敏纸小票，支持抓取拖拽弯曲 | ✅ 有（Claude Sonnet 4.6） |
| [AI 洞察档案](html/ai_anwser.html) | 暗黑科技风格的问答档案，支持卡片导出 | 🕐 待补充 |
| [Zen 禅意计时](html/zen.html) | 极简习惯打卡记录器，随时间变化的主题色 | 🕐 待补充 |
| [密码生成器](html/password_create.html) | 客户端安全密码生成，自定义长度与字符类型 | 🕐 待补充 |
| [认知科学学习法](html/how_to_learn.html) | 基于认知科学的高效学习流程图解 | 🕐 待补充 |
| [RSVP 速读训练](html/RSVP.html) | 快速序列视觉呈现速读工具，消除眼球移动 | 🕐 待补充 |
| [批量网址打开器](html/url_opener.html) | 粘贴多个链接，一键批量打开 | 🕐 待补充 |
| [Spark SQL 转换器](html/spark-sql-converter.html) | Spark SQL 转 Scala 代码，适配 Zeppelin | 🕐 待补充 |
| [湖仓一体架构图](html/bytedance_lakehouse_architecture.html) | 火山引擎数据湖仓架构图，分为接入、处理、分析与生态集成阶段 | ✅ 有（Gemini 3 Flash） |

---

## 如何贡献提示词

如果你用大模型复现了某个工具，欢迎提交对应的提示词：

1. Fork 本仓库
2. 编辑 `html/data/tools.js`，找到对应的工具对象
3. 将 `status` 改为 `"done"`，填入 `model` 和 `prompt` 字段
4. 提交 Pull Request，标题格式：`feat: 补充 [工具名] 提示词`

```js
// 示例：为某工具补充提示词
{
    id: 'zen',
    title: 'Zen 禅意计时',
    // ...
    status: 'done',          // 从 'pending' 改为 'done'
    model: 'Claude Sonnet',  // 填写使用的模型
    prompt: `你的提示词...`, // 粘贴提示词内容
},
```

或者直接 [提交 Issue](https://github.com/dengshu2/static-site/issues) 附上提示词文本。

---

## 技术栈

- [static-web-server](https://github.com/static-web-server/static-web-server) `v2.41.0` — 高性能轻量级静态文件服务器
- Docker Compose — 容器化部署
- Nginx Proxy Manager — 反向代理与 HTTPS

---

## 项目结构

```
.
├── docker-compose.yml    # Docker Compose 配置
├── html/                 # 静态文件目录
│   ├── index.html        # 首页骨架（~60 行）
│   ├── css/
│   │   ├── index.css     # 首页专属样式
│   │   └── styles.css    # 公共样式（工具页使用）
│   ├── js/
│   │   └── app.js        # 首页渲染与交互逻辑
│   ├── data/
│   │   └── tools.js      # ← 工具数据源，新增工具只改这里
│   ├── receipt-physics.html
│   ├── ai_anwser.html
│   ├── zen.html
│   ├── password_create.html
│   ├── how_to_learn.html
│   ├── RSVP.html
│   ├── url_opener.html
│   └── spark-sql-converter.html
├── .gitignore
└── README.md
```

---

## 本地部署

### 前置条件

- Docker + Docker Compose
- 外部网络 `npm-network`（由 Nginx Proxy Manager 创建）

### 启动

```bash
docker compose up -d
```

服务监听 `127.0.0.1:8080`，通过 Nginx Proxy Manager 配置域名代理后对外访问。

### 停止

```bash
docker compose down
```

### 添加新工具

1. 在 `html/` 下创建新的 `.html` 文件
2. 在 `html/data/tools.js` 末尾追加一个工具对象（首页自动渲染，无需改 HTML）
3. 无需重启服务，文件直接生效（目录已挂载为只读 volume）
4. **清理缓存**：若修改了 `tools.js` 或 `app.js` 后首页未更新，需在 HTML 引入处增加版本号（如 `?v=...`）或强制刷新浏览器缓存。

---

## 配置说明

通过 `docker-compose.yml` 中的环境变量调整服务行为：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `SERVER_LOG_LEVEL` | 日志级别（error/warn/info/debug） | `info` |
| `SERVER_DIRECTORY_LISTING` | 启用目录浏览 | `false` |
| `SERVER_COMPRESSION` | 启用 gzip 压缩 | `true` |
| `SERVER_CACHE_CONTROL_HEADERS` | 启用缓存控制头 | `true` |

完整配置项：https://static-web-server.net/configuration/environment-variables/
