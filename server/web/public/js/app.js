const list = document.getElementById('site-list');
const count = document.getElementById('site-count');
const searchInput = document.getElementById('search-input');
const sortSelect = document.getElementById('sort-select');
const clearSearch = document.getElementById('clear-search');
let allSites = [];

function fmtSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function fmtNumber(value) {
    return new Intl.NumberFormat('zh-CN', { notation: value >= 10000 ? 'compact' : 'standard' }).format(value || 0);
}

function fmtDate(value) {
    if (!value) return '尚未更新';
    return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(new Date(value));
}

function safeURL(raw) {
    try {
        const value = new URL(raw, location.origin);
        return value.protocol === 'https:' || value.hostname === 'localhost' || value.hostname === '127.0.0.1'
            ? value.href
            : '';
    } catch {
        return '';
    }
}

function makeButton(label, className, action) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = className;
    button.textContent = label;
    button.addEventListener('click', action);
    return button;
}

function fact(text) {
    const node = document.createElement('span');
    node.textContent = text;
    return node;
}

function renderSites() {
    const query = searchInput.value.trim().toLocaleLowerCase('zh-CN');
    const filtered = allSites.filter(site => {
        const searchable = `${site.title || ''} ${site.description || ''} ${site.name}`.toLocaleLowerCase('zh-CN');
        return !query || searchable.includes(query);
    });
    filtered.sort((a, b) => {
        if (sortSelect.value === 'views') return (b.views || 0) - (a.views || 0) || a.name.localeCompare(b.name);
        if (sortSelect.value === 'name') return (a.title || a.name).localeCompare(b.title || b.name, 'zh-CN');
        if (sortSelect.value === 'size') return (b.size || 0) - (a.size || 0);
        return new Date(b.updatedAt || b.createdAt) - new Date(a.updatedAt || a.createdAt);
    });

    list.replaceChildren();
    count.textContent = query ? `找到 ${filtered.length} 个，共 ${allSites.length} 个项目` : `${allSites.length} 个公开项目`;
    clearSearch.hidden = !query;
    if (!filtered.length) {
        const empty = document.createElement('p');
        empty.className = 'empty';
        const title = document.createElement('strong');
        title.textContent = query ? '没有找到匹配项目' : '还没有部署项目';
        empty.append(title, document.createTextNode(query ? '换一个关键词试试。' : '完成第一次部署后，项目会出现在这里。'));
        list.append(empty);
        return;
    }
    for (const site of filtered) list.append(siteCard(site));
}

function siteCard(site) {
    const url = safeURL(site.url);
    const card = document.createElement('article');
    card.className = 'site-card';

    const top = document.createElement('div');
    top.className = 'card-top';
    const slug = document.createElement('p');
    slug.className = 'site-slug';
    slug.textContent = `/s/${site.name}/`;
    const views = document.createElement('span');
    views.className = 'site-views';
    views.textContent = `${fmtNumber(site.views)} 次访问`;
    top.append(slug, views);

    const title = document.createElement('h3');
    title.className = 'site-name';
    title.textContent = site.title || site.name;
    const description = document.createElement('p');
    description.className = 'site-description';
    description.textContent = site.description || '这个项目还没有添加简介。';

    const facts = document.createElement('div');
    facts.className = 'site-facts';
    facts.append(
        fact(`${site.files} 个文件`),
        fact(fmtSize(site.size)),
        fact(`更新于 ${fmtDate(site.updatedAt || site.createdAt)}`),
    );

    const actions = document.createElement('div');
    actions.className = 'site-actions';
    const open = document.createElement('a');
    open.className = 'primary-btn';
    open.href = url;
    open.target = '_blank';
    open.rel = 'noopener noreferrer';
    open.textContent = '打开项目 ↗';
    const copy = makeButton('复制链接', 'soft-btn', async () => {
        try {
            await navigator.clipboard.writeText(url);
            copy.textContent = '已复制';
            setTimeout(() => { copy.textContent = '复制链接'; }, 1500);
        } catch {
            copy.textContent = '复制失败';
        }
    });
    actions.append(open, copy);
    card.append(top, title, description, facts, actions);
    return card;
}

async function loadCatalog() {
    count.textContent = '正在读取项目…';
    try {
        const [sitesResponse, analyticsResponse] = await Promise.all([
            fetch('/api/sites', { cache: 'no-store' }),
            fetch('/api/analytics', { cache: 'no-store' }),
        ]);
        if (!sitesResponse.ok) throw new Error(`HTTP ${sitesResponse.status}`);
        allSites = await sitesResponse.json();
        renderSites();
        if (analyticsResponse.ok) {
            const analytics = await analyticsResponse.json();
            document.getElementById('stat-sites').textContent = fmtNumber(analytics.totalSites);
            document.getElementById('stat-views').textContent = fmtNumber(analytics.totalViews);
            document.getElementById('stat-size').textContent = fmtSize(analytics.totalBytes);
        }
    } catch {
        list.replaceChildren();
        const error = document.createElement('p');
        error.className = 'error';
        error.textContent = '项目目录加载失败，请稍后刷新。';
        list.append(error);
        count.textContent = '加载失败';
    }
}

async function loadConfig() {
    try {
        const response = await fetch('/api/config', { cache: 'no-store' });
        if (!response.ok) return;
        const config = await response.json();
        document.getElementById('admin-link').href = safeURL(config.adminURL) || '/admin/';
    } catch {
        // 保留同域 /admin/ 回退地址。
    }
}

searchInput.addEventListener('input', renderSites);
sortSelect.addEventListener('change', renderSites);
clearSearch.addEventListener('click', () => {
    searchInput.value = '';
    searchInput.focus();
    renderSites();
});
document.getElementById('refresh-btn').addEventListener('click', loadCatalog);
loadConfig();
loadCatalog();
