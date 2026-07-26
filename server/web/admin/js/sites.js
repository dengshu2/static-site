const Deployer = {
    token: '',
    tokenWaiter: null,
    sites: [],
    trashItems: [],
    historyLoaded: false,

    fmtSize(bytes) {
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
        return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
    },

    fmtNumber(value) {
        return new Intl.NumberFormat('zh-CN', { notation: value >= 10000 ? 'compact' : 'standard' }).format(value || 0);
    },

    fmtDate(value, includeTime = false) {
        if (!value) return '暂无记录';
        const options = includeTime
            ? { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
            : { year: 'numeric', month: 'short', day: 'numeric' };
        return new Intl.DateTimeFormat('zh-CN', options).format(new Date(value));
    },

    safeURL(raw) {
        try {
            const value = new URL(raw, location.origin);
            if (value.protocol === 'https:' || value.hostname === 'localhost' || value.hostname === '127.0.0.1') return value.href;
        } catch {
            // 下方统一返回空字符串。
        }
        return '';
    },

    async requireToken() {
        if (this.token) return this.token;
        if (this.tokenWaiter) return this.tokenWaiter;
        const dialog = document.getElementById('token-dialog');
        const input = document.getElementById('token-input');
        this.tokenWaiter = new Promise(resolve => {
            const finish = value => {
                this.token = value;
                this.tokenWaiter = null;
                input.value = '';
                if (dialog.open) dialog.close();
                this.updateTokenButton();
                resolve(value);
            };
            document.getElementById('token-form').onsubmit = event => {
                event.preventDefault();
                finish(input.value.trim());
            };
            document.getElementById('token-cancel').onclick = () => finish('');
            dialog.oncancel = event => {
                event.preventDefault();
                finish('');
            };
            dialog.showModal();
            input.focus();
        });
        return this.tokenWaiter;
    },

    clearToken() {
        this.token = '';
        this.updateTokenButton();
    },

    updateTokenButton() {
        document.getElementById('token-btn').textContent = this.token ? '清除 Token' : '设置 Token';
    },

    async authorizedFetch(url, options = {}) {
        const token = await this.requireToken();
        if (!token) throw new Error('已取消 Token 输入');
        const headers = new Headers(options.headers || {});
        headers.set('Authorization', `Bearer ${token}`);
        const response = await fetch(url, { ...options, headers, cache: 'no-store' });
        if (response.status === 401 || response.status === 429) this.clearToken();
        return response;
    },

    async loadAnalytics() {
        try {
            const response = await fetch('/api/analytics', { cache: 'no-store' });
            if (!response.ok) return;
            const analytics = await response.json();
            document.getElementById('metric-sites').textContent = this.fmtNumber(analytics.totalSites);
            document.getElementById('metric-views').textContent = this.fmtNumber(analytics.totalViews);
            document.getElementById('metric-today').textContent = `今天 ${this.fmtNumber(analytics.viewsToday)} 次`;
            document.getElementById('metric-size').textContent = this.fmtSize(analytics.totalBytes);
            document.getElementById('metric-files').textContent = `共 ${this.fmtNumber(analytics.totalFiles)} 个文件`;
            document.getElementById('metric-updated').textContent = this.fmtNumber(analytics.updatedThisWeek);
        } catch {
            // 概览失败不影响主要管理流程。
        }
    },

    async loadSites() {
        const list = document.getElementById('site-list');
        document.getElementById('site-count').textContent = '正在读取…';
        try {
            const response = await fetch('/api/sites', { cache: 'no-store' });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            this.sites = await response.json();
            this.renderSites();
            await this.loadAnalytics();
        } catch {
            list.replaceChildren(this.message('项目列表加载失败，请稍后刷新。', 'error'));
            document.getElementById('site-count').textContent = '加载失败';
        }
    },

    renderSites() {
        const query = document.getElementById('admin-search-input').value.trim().toLocaleLowerCase('zh-CN');
        const sort = document.getElementById('admin-sort-select').value;
        const sites = this.sites.filter(site => {
            const searchable = `${site.title || ''} ${site.description || ''} ${site.name}`.toLocaleLowerCase('zh-CN');
            return !query || searchable.includes(query);
        });
        sites.sort((a, b) => {
            if (sort === 'views') return (b.views || 0) - (a.views || 0) || a.name.localeCompare(b.name);
            if (sort === 'name') return (a.title || a.name).localeCompare(b.title || b.name, 'zh-CN');
            return new Date(b.updatedAt || b.createdAt) - new Date(a.updatedAt || a.createdAt);
        });

        const list = document.getElementById('site-list');
        list.replaceChildren();
        document.getElementById('site-count').textContent = query
            ? `找到 ${sites.length} 个，共 ${this.sites.length} 个项目`
            : `${this.sites.length} 个在线项目`;
        if (!sites.length) {
            list.append(this.message(query ? '没有找到匹配项目。' : '还没有部署任何项目。', 'empty'));
            return;
        }
        for (const site of sites) list.append(this.siteCard(site));
    },

    siteCard(site) {
        const url = this.safeURL(site.url);
        const card = document.createElement('article');
        card.className = 'site-card';

        const main = document.createElement('div');
        main.className = 'site-card-main';
        const meta = document.createElement('div');
        meta.className = 'site-meta';
        const title = document.createElement('h3');
        title.className = 'site-name';
        title.textContent = site.title || site.name;
        const slug = document.createElement('p');
        slug.className = 'site-slug';
        slug.textContent = `/s/${site.name}/`;
        const description = document.createElement('p');
        description.className = 'site-description';
        description.textContent = site.description || '尚未添加项目简介。';
        const sub = document.createElement('p');
        sub.className = 'site-sub';
        sub.textContent = `${site.files} 个文件 · ${this.fmtSize(site.size)} · 更新于 ${this.fmtDate(site.updatedAt || site.createdAt)}`;
        meta.append(title, slug, description, sub);
        const views = document.createElement('span');
        views.className = 'view-chip';
        views.textContent = `${this.fmtNumber(site.views)} 次访问`;
        main.append(meta, views);

        const actions = document.createElement('div');
        actions.className = 'site-actions';
        const open = document.createElement('a');
        open.className = 'primary-btn';
        open.href = url;
        open.target = '_blank';
        open.rel = 'noopener noreferrer';
        open.textContent = '打开 ↗';
        const copy = this.button('复制链接', 'soft-btn', async () => {
            try {
                await navigator.clipboard.writeText(url);
                copy.textContent = '已复制';
                setTimeout(() => { copy.textContent = '复制链接'; }, 1500);
            } catch {
                copy.textContent = '复制失败';
            }
        });
        actions.append(
            open,
            copy,
            this.button('编辑资料', 'ghost-btn', () => this.openMetadata(site)),
            this.button('删除', 'danger-btn', () => this.deleteSite(site.name)),
        );
        card.append(main, actions);
        return card;
    },

    openMetadata(site) {
        const dialog = document.getElementById('metadata-dialog');
        document.getElementById('metadata-form').dataset.site = site.name;
        document.getElementById('metadata-slug').textContent = `/s/${site.name}/`;
        document.getElementById('metadata-title-input').value = site.title || site.name;
        document.getElementById('metadata-description-input').value = site.description || '';
        dialog.showModal();
        document.getElementById('metadata-title-input').focus();
    },

    async saveMetadata(name, title, description) {
        const response = await this.authorizedFetch(`/api/sites/${encodeURIComponent(name)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ title, description }),
        });
        if (!response.ok) {
            const body = await response.json().catch(() => ({}));
            throw new Error(body.error || `HTTP ${response.status}`);
        }
        document.getElementById('metadata-dialog').close();
        await this.loadSites();
    },

    async deleteSite(name) {
        if (!confirm(`确定把项目“${name}”移入回收站？`)) return;
        try {
            const response = await this.authorizedFetch(`/api/sites/${encodeURIComponent(name)}`, { method: 'DELETE' });
            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error || `HTTP ${response.status}`);
            }
            await this.loadSites();
            if (this.historyLoaded) await this.loadTrash();
        } catch (error) {
            alert(`删除失败：${error.message}`);
        }
    },

    async loadTrash() {
        const list = document.getElementById('trash-list');
        list.hidden = false;
        list.replaceChildren(this.message('正在读取版本历史…', 'empty'));
        try {
            const response = await this.authorizedFetch('/api/trash');
            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error || `HTTP ${response.status}`);
            }
            this.trashItems = await response.json();
            this.historyLoaded = true;
            document.getElementById('trash-btn').textContent = '刷新历史';
            this.renderTrash();
        } catch (error) {
            list.replaceChildren(this.message(`版本历史加载失败：${error.message}`, 'error'));
        }
    },

    renderTrash() {
        const list = document.getElementById('trash-list');
        const query = document.getElementById('history-search-input').value.trim().toLocaleLowerCase('zh-CN');
        const filter = document.getElementById('history-filter-select').value;
        const items = this.trashItems.filter(item => {
            const searchable = `${item.site.title || ''} ${item.site.name}`.toLocaleLowerCase('zh-CN');
            return (!query || searchable.includes(query)) && (filter === 'all' || item.reason === filter);
        });
        list.replaceChildren();
        document.getElementById('trash-count').textContent = `显示 ${items.length} 条，共 ${this.trashItems.length} 条历史记录；默认保留 7 天。`;
        if (!items.length) {
            list.append(this.message(query || filter !== 'all' ? '没有匹配的历史记录。' : '版本历史为空。', 'empty'));
            return;
        }
        for (const item of items) list.append(this.trashCard(item));
    },

    trashCard(item) {
        const card = document.createElement('article');
        card.className = 'history-card';
        const badge = document.createElement('span');
        badge.className = `history-badge${item.reason === 'deleted' ? ' deleted' : ''}`;
        badge.textContent = item.reason === 'overwritten' ? '覆盖前版本' : '已删除项目';
        const title = document.createElement('h3');
        title.textContent = item.site.title || item.site.name;
        const slug = document.createElement('p');
        slug.textContent = `/s/${item.site.name}/`;
        const details = document.createElement('p');
        details.textContent = `${item.site.files} 个文件 · ${this.fmtSize(item.site.size)} · 进入历史于 ${this.fmtDate(item.deletedAt, true)}`;
        const actions = document.createElement('div');
        actions.className = 'site-actions';
        actions.append(
            this.button('恢复此版本', 'primary-btn', () => this.restoreTrash(item.id)),
            this.button('永久删除', 'danger-btn', () => this.purgeTrash(item.id, item.site.name)),
        );
        card.append(badge, title, slug, details, actions);
        return card;
    },

    async restoreTrash(id) {
        try {
            const response = await this.authorizedFetch(`/api/trash/${encodeURIComponent(id)}/restore`, { method: 'POST' });
            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error || `HTTP ${response.status}`);
            }
            await Promise.all([this.loadSites(), this.loadTrash()]);
        } catch (error) {
            alert(`恢复失败：${error.message}`);
        }
    },

    async purgeTrash(id, name) {
        if (!confirm(`永久删除“${name}”的这条历史记录？此操作无法恢复。`)) return;
        try {
            const response = await this.authorizedFetch(`/api/trash/${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (!response.ok && response.status !== 204) throw new Error(`HTTP ${response.status}`);
            await this.loadTrash();
        } catch (error) {
            alert(`永久删除失败：${error.message}`);
        }
    },

    button(label, className, action) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = className;
        button.textContent = label;
        button.addEventListener('click', action);
        return button;
    },

    message(text, className) {
        const node = document.createElement('p');
        node.className = className;
        node.textContent = text;
        return node;
    },
};

window.Deployer = Deployer;

document.getElementById('token-btn').addEventListener('click', async () => {
    if (Deployer.token) {
        Deployer.clearToken();
    } else {
        await Deployer.requireToken();
    }
});
document.getElementById('refresh-btn').addEventListener('click', () => Deployer.loadSites());
document.getElementById('trash-btn').addEventListener('click', () => Deployer.loadTrash());
document.getElementById('admin-search-input').addEventListener('input', () => Deployer.renderSites());
document.getElementById('admin-sort-select').addEventListener('change', () => Deployer.renderSites());
document.getElementById('history-search-input').addEventListener('input', () => {
    if (Deployer.historyLoaded) Deployer.renderTrash();
});
document.getElementById('history-filter-select').addEventListener('change', () => {
    if (Deployer.historyLoaded) Deployer.renderTrash();
});
document.getElementById('metadata-cancel').addEventListener('click', () => {
    document.getElementById('metadata-dialog').close();
});
document.getElementById('metadata-form').addEventListener('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    try {
        await Deployer.saveMetadata(
            form.dataset.site,
            document.getElementById('metadata-title-input').value.trim(),
            document.getElementById('metadata-description-input').value.trim(),
        );
    } catch (error) {
        alert(`保存失败：${error.message}`);
    }
});

fetch('/api/config', { cache: 'no-store' })
    .then(response => response.ok ? response.json() : null)
    .then(config => {
        if (config) document.getElementById('public-link').href = Deployer.safeURL(config.publicURL) || '/';
    })
    .catch(() => {});

Deployer.loadSites();
