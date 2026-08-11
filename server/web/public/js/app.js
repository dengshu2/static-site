const list = document.getElementById('site-list');
const count = document.getElementById('site-count');
const stats = document.getElementById('stats');
const searchInput = document.getElementById('search-input');
const sortSelect = document.getElementById('sort-select');
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
    return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'numeric', day: 'numeric' }).format(new Date(value));
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

function meta(text, className) {
    const node = document.createElement('span');
    if (className) node.className = className;
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
    // 未搜索时项目总数已经出现在页首统计里，这里留空。
    count.textContent = query ? `匹配 ${filtered.length} / ${allSites.length}` : '';
    if (!filtered.length) {
        const empty = document.createElement('p');
        empty.className = 'empty';
        empty.textContent = query ? '没有匹配的项目，换一个关键词试试。' : '还没有部署项目。完成第一次部署后，项目会出现在这里。';
        list.append(empty);
        return;
    }
    for (const site of filtered) list.append(siteRow(site));
}

function siteRow(site) {
    const url = safeURL(site.url);
    const row = document.createElement('article');
    row.className = 'row';

    const main = document.createElement('div');
    main.className = 'row-main';

    const title = document.createElement('h3');
    title.className = 'row-title';
    const link = document.createElement('a');
    link.href = url;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.textContent = site.title || site.name;
    title.append(link);
    main.append(title);

    if (site.description) {
        const description = document.createElement('p');
        description.className = 'row-desc';
        description.textContent = site.description;
        main.append(description);
    }

    const facts = document.createElement('div');
    facts.className = 'row-meta';
    facts.append(
        meta(`/s/${site.name}/`, 'row-slug'),
        meta(`${site.files} 个文件`),
        meta(fmtSize(site.size)),
        meta(fmtDate(site.updatedAt || site.createdAt)),
    );
    main.append(facts);

    const side = document.createElement('div');
    side.className = 'row-side';
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'link copy';
    copy.textContent = '复制链接';
    copy.addEventListener('click', async () => {
        try {
            await navigator.clipboard.writeText(url);
            copy.textContent = '已复制';
        } catch {
            copy.textContent = '复制失败';
        }
        copy.dataset.done = '1';
        setTimeout(() => {
            copy.textContent = '复制链接';
            delete copy.dataset.done;
        }, 1500);
    });
    side.append(meta(`${fmtNumber(site.views)} 次访问`), copy);

    row.append(main, side);
    return row;
}

async function loadCatalog() {
    count.textContent = '正在读取…';
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
            stats.textContent = [
                `${fmtNumber(analytics.totalSites)} 个项目`,
                `${fmtNumber(analytics.totalViews)} 次访问`,
                fmtSize(analytics.totalBytes),
            ].join(' · ');
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
document.getElementById('refresh-btn').addEventListener('click', loadCatalog);
loadConfig();
loadCatalog();
