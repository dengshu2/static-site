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
            ? { year: 'numeric', month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }
            : { year: 'numeric', month: 'numeric', day: 'numeric' };
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
            document.getElementById('stats').textContent = [
                `${this.fmtNumber(analytics.totalSites)} 个项目`,
                `${this.fmtNumber(analytics.totalViews)} 次访问（今天 ${this.fmtNumber(analytics.viewsToday)}）`,
                `${this.fmtSize(analytics.totalBytes)} / ${this.fmtNumber(analytics.totalFiles)} 个文件`,
                `过去 7 天更新 ${this.fmtNumber(analytics.updatedThisWeek)} 个`,
            ].join(' · ');
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
        // 未搜索时项目总数已经出现在页首统计里，这里留空。
        document.getElementById('site-count').textContent = query ? `匹配 ${sites.length} / ${this.sites.length}` : '';
        if (!sites.length) {
            list.append(this.message(query ? '没有找到匹配项目。' : '还没有部署任何项目。', 'empty'));
            return;
        }
        for (const site of sites) list.append(this.siteRow(site));
    },

    siteRow(site) {
        const url = this.safeURL(site.url);
        const row = document.createElement('article');
        row.className = 'row';

        const main = document.createElement('div');
        main.className = 'row-main';
        const title = document.createElement('h3');
        title.className = 'row-title';
        title.textContent = site.title || site.name;
        main.append(title);
        if (site.description) {
            const description = document.createElement('p');
            description.className = 'row-desc';
            description.textContent = site.description;
            main.append(description);
        }
        main.append(this.metaLine([
            [`/s/${site.name}/`, 'row-slug'],
            [`${site.files} 个文件`],
            [this.fmtSize(site.size)],
            [this.fmtDate(site.updatedAt || site.createdAt)],
        ]));

        const side = document.createElement('div');
        side.className = 'row-side';
        const views = document.createElement('span');
        views.textContent = `${this.fmtNumber(site.views)} 次访问`;

        const actions = document.createElement('span');
        actions.className = 'actions';
        const open = document.createElement('a');
        open.className = 'link';
        open.href = url;
        open.target = '_blank';
        open.rel = 'noopener noreferrer';
        open.textContent = '打开';
        const copy = this.button('复制', 'link', async () => {
            try {
                await navigator.clipboard.writeText(url);
                copy.textContent = '已复制';
            } catch {
                copy.textContent = '复制失败';
            }
            setTimeout(() => { copy.textContent = '复制'; }, 1500);
        });
        actions.append(
            open,
            copy,
            this.button('编辑', 'link', () => this.openMetadata(site)),
            this.button('删除', 'link danger', () => this.deleteSite(site.name)),
        );
        side.append(views, actions);

        row.append(main, side);
        return row;
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
        document.getElementById('trash-count').textContent = query || filter !== 'all'
            ? `匹配 ${items.length} / ${this.trashItems.length}`
            : `${this.trashItems.length} 条历史记录，默认保留 7 天`;
        if (!items.length) {
            list.append(this.message(query || filter !== 'all' ? '没有匹配的历史记录。' : '版本历史为空。', 'empty'));
            return;
        }
        for (const item of items) list.append(this.trashRow(item));
    },

    trashRow(item) {
        const row = document.createElement('article');
        row.className = 'row';

        const main = document.createElement('div');
        main.className = 'row-main';
        const title = document.createElement('h3');
        title.className = 'row-title';
        title.textContent = item.site.title || item.site.name;
        main.append(title, this.metaLine([
            [item.reason === 'overwritten' ? '覆盖前版本' : '已删除项目', 'row-tag'],
            [`/s/${item.site.name}/`, 'row-slug'],
            [`${item.site.files} 个文件`],
            [this.fmtSize(item.site.size)],
            [`${this.fmtDate(item.deletedAt, true)} 移入`],
        ]));

        const side = document.createElement('div');
        side.className = 'row-side';
        const actions = document.createElement('span');
        actions.className = 'actions';
        actions.append(
            this.button('恢复', 'link', () => this.restoreTrash(item.id)),
            this.button('永久删除', 'link danger', () => this.purgeTrash(item.id, item.site.name)),
        );
        side.append(actions);

        row.append(main, side);
        return row;
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

    metaLine(entries) {
        const line = document.createElement('div');
        line.className = 'row-meta';
        for (const [text, className] of entries) {
            const node = document.createElement('span');
            if (className) node.className = className;
            node.textContent = text;
            line.append(node);
        }
        return line;
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
