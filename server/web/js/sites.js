// sites.js — Token 管理、站点列表渲染与删除。
// 暴露 window.Deployer 供 upload.js 复用。

const TOKEN_KEY = 'deploy_token';

const Deployer = {
    getToken() {
        let t = localStorage.getItem(TOKEN_KEY);
        if (!t) t = this.promptToken();
        return t || '';
    },

    promptToken() {
        const t = window.prompt('请输入上传 Token：', localStorage.getItem(TOKEN_KEY) || '');
        if (t !== null) {
            localStorage.setItem(TOKEN_KEY, t.trim());
            return t.trim();
        }
        return localStorage.getItem(TOKEN_KEY) || '';
    },

    authHeaders() {
        return { 'Authorization': 'Bearer ' + this.getToken() };
    },

    fmtSize(n) {
        if (n < 1024) return n + ' B';
        if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
        return (n / 1024 / 1024).toFixed(1) + ' MB';
    },

    async loadSites(retried) {
        const list = document.getElementById('site-list');
        try {
            const res = await fetch('/api/sites', { headers: this.authHeaders() });
            if (res.status === 401) {
                // 浏览器里可能存着旧 Token：自动弹窗重填一次，仍失败才提示。
                if (!retried) {
                    this.promptToken();
                    return this.loadSites(true);
                }
                list.innerHTML = '<p class="empty">Token 无效，点击右上角 🔑 重新输入。</p>';
                return;
            }
            const sites = await res.json();
            this.renderSites(sites);
        } catch (e) {
            list.innerHTML = '<p class="empty">加载失败：' + e.message + '</p>';
        }
    },

    renderSites(sites) {
        const list = document.getElementById('site-list');
        if (!sites || sites.length === 0) {
            list.innerHTML = '<p class="empty">还没有部署任何站点。</p>';
            return;
        }
        list.innerHTML = sites.map(s => `
            <div class="site-card">
                <div class="site-meta">
                    <div class="site-name">${escapeHtml(s.name)}</div>
                    <div class="site-sub">${s.files} 个文件 · ${this.fmtSize(s.size)} · ${new Date(s.createdAt).toLocaleString()}</div>
                </div>
                <div class="site-actions">
                    <a class="btn-open" href="${s.url}" target="_blank" rel="noopener">打开 ↗</a>
                    <button class="btn-del" data-name="${escapeHtml(s.name)}">删除</button>
                </div>
            </div>`).join('');

        list.querySelectorAll('.btn-del').forEach(btn => {
            btn.addEventListener('click', () => this.deleteSite(btn.dataset.name));
        });
    },

    async deleteSite(name) {
        if (!confirm(`确定删除站点 "${name}"？此操作不可恢复。`)) return;
        const res = await fetch('/api/sites/' + encodeURIComponent(name), {
            method: 'DELETE',
            headers: this.authHeaders(),
        });
        if (res.ok || res.status === 204) {
            this.loadSites();
        } else {
            const body = await res.json().catch(() => ({}));
            alert('删除失败：' + (body.error || res.status));
        }
    },
};

function escapeHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

window.Deployer = Deployer;

document.getElementById('token-btn').addEventListener('click', () => {
    Deployer.promptToken();
    Deployer.loadSites();
});
document.getElementById('refresh-btn').addEventListener('click', () => Deployer.loadSites());

Deployer.loadSites();
