/**
 * tools.js — 工具数据源
 *
 * 每个工具对象字段说明：
 *   id      {string}  唯一标识，用于生成 DOM id
 *   title   {string}  卡片标题
 *   desc    {string}  一句话描述
 *   href    {string}  工具页面路径
 *   status  {string}  "done" | "pending"
 *   model   {string?} 可选，生成该工具所用的模型名
 *   prompt  {string?} 可选，status 为 "done" 时必填
 */
export const TOOLS = [
    {
        id: 'receipt',
        title: '小票物理模拟',
        desc: '基于 Three.js + Verlet 积分的交互式小票，支持抓取、拖拽、弯曲，热敏纸质感。',
        href: 'receipt-physics.html',
        status: 'done',
        model: 'Claude Sonnet 4.6',
        prompt: `请创建一个基于 Three.js + WebGL 的交互式 receipt UI，小票应模拟真实纸张物理，而不是普通平面动画。
必须满足以下约束：
- 使用 Verlet integration 或粒子约束系统实现纸张模拟
- 小票可被鼠标左键抓取、拖拽、弯曲、折叠，并产生自然摆动、褶皱、回弹
- 顶部边缘必须整条固定或整条约束，始终保持笔直水平
- 严禁只固定顶部 3 个点（左/中/右）
- 严禁顶部中间出现凹陷、悬挂、下垂或"晾衣绳"效果
- 小票主体下半部分必须仍然保持柔软纸张形变
- 材质表现应像热敏纸，而不是布料、塑料或金属
- 小票表面要有搞笑的 3D 图形术语购物清单
- 背景为白色，带轻微阴影和高光，整体有真实重量感
- 输出完整 HTML + CSS + JS，可直接浏览器运行
- 同时进行性能优化，保证桌面浏览器流畅运行

请先说明实现思路，再输出完整代码。`,
    },
    {
        id: 'ai-answer',
        title: 'AI 洞察档案',
        desc: '暗黑科技风格的问答档案，收录关于未来的残酷真相与高级洞察，支持卡片导出。',
        href: 'ai_anwser.html',
        status: 'pending',
    },
    {
        id: 'zen',
        title: 'Zen 禅意计时',
        desc: '极简主义的时间记录器，记录你坚持习惯的每一天，随时间变化的主题色。',
        href: 'zen.html',
        status: 'pending',
    },
    {
        id: 'password',
        title: '密码生成器',
        desc: '客户端安全密码生成，自定义长度与字符类型，一键生成高强度随机密码。',
        href: 'password_create.html',
        status: 'pending',
    },
    {
        id: 'how-to-learn',
        title: '认知科学学习法',
        desc: '给大脑装系统的说明书，基于认知科学的高效学习流程图解。',
        href: 'how_to_learn.html',
        status: 'pending',
    },
    {
        id: 'rsvp',
        title: 'RSVP 速读训练',
        desc: '基于 RSVP（快速序列视觉呈现）的速读工具，消除眼球移动，突破阅读速度瓶颈。',
        href: 'RSVP.html',
        status: 'pending',
    },
    {
        id: 'url-opener',
        title: '批量网址打开器',
        desc: '粘贴多个链接，一键打开所有目标页面，批量信息获取。',
        href: 'url_opener.html',
        status: 'pending',
    },
    {
        id: 'apimart-image',
        title: 'APImart 生图',
        desc: '基于 APImart gpt-image-2 接口的网页生图工具，支持文生图与多图参考、自定义比例与并发生成。',
        href: 'apimart-image.html',
        status: 'pending',
    },
    {
        id: 'lakehouse-arch',
        title: '湖仓一体架构图',
        desc: '火山引擎数据湖仓架构图，清晰展示从数据接入、处理到分析的全链路流程。',
        href: 'bytedance_lakehouse_architecture.html',
        status: 'done',
        model: 'Gemini 3 Flash',
        prompt: `请根据上传的火山引擎数据湖仓架构图图片，使用 HTML 和 CSS 完整复现该架构示意图。
要求：
1. 视觉还原：高度还原图片中的配色（针对数据接入、处理、分析、生态各阶段使用不同色系）、布局及组件样式。
2. 响应式布局：使用 Flexbox 或 Grid 布局，确保在不同屏幕宽度下架构逻辑清晰。
3. 代码质量：使用语义化 HTML 结构，CSS 变量定义常用圆角、边距及颜色，支持暗色模式切换。
4. 交互体验：容器应支持横向滚动以适配窄屏。`,
    },
];
