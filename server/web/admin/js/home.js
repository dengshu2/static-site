// The workbench: the numbers, publishing, and every project (tap one to manage it).

import { api } from './api.js';
import { Deploy } from './deploy.js';
import { cover, fmtDate, fmtNumber, fmtSize, h, icon } from '../ui.js';

export class Home {
    constructor({ config, onLogout }) {
        const $ = (id) => document.getElementById(id);
        this.el = { overview: $('overview'), list: $('sites'), count: $('sites-count'), search: $('sites-search'), sort: $('sites-sort') };
        this.sites = [];
        this.sort = 'updated';
        this.poll = null;
        this.deploy = new Deploy({
            config,
            sites: () => this.sites,
            onPublished: () => this.refresh(),
        });
        $('logout').addEventListener('click', onLogout);
        let timer;
        this.el.search.addEventListener('input', () => {
            clearTimeout(timer);
            timer = setTimeout(() => this.render(), 120);
        });
        for (const btn of this.el.sort.querySelectorAll('[data-sort]')) {
            btn.addEventListener('click', () => {
                this.sort = btn.dataset.sort;
                for (const b of this.el.sort.querySelectorAll('[data-sort]')) b.setAttribute('aria-pressed', String(b === btn));
                this.render();
            });
        }
    }

    async refresh() {
        if (!this.sites.length) {
            this.el.list.replaceChildren(h('li', { class: 'q-skeleton', 'aria-hidden': 'true' }));
            this.el.list.firstChild.style.height = '180px';
        }
        try {
            const [sites, stats] = await Promise.all([api('/api/sites'), api('/api/analytics').catch(() => null)]);
            this.sites = sites;
            if (stats) this.renderOverview(stats);
            this.render();
            this.watchCovers();
        } catch (err) {
            if (err.status === 401) return;
            this.el.list.replaceChildren(h('li', { class: 'q-state' },
                h('b', { text: '项目列表加载失败' }), h('span', { text: err.message }),
                h('button', { class: 'q-btn q-btn--sm', type: 'button', text: '重试', onclick: () => this.refresh() })));
        }
    }

    /** New projects get their cover a few seconds later; check back until they have. */
    watchCovers() {
        clearTimeout(this.poll);
        if (!this.sites.some((s) => s.coverPending)) return;
        this.poll = setTimeout(async () => {
            try {
                this.sites = await api('/api/sites');
                this.render();
            } catch { /* try again on the next refresh */ }
            this.watchCovers();
        }, 3000);
    }

    renderOverview(s) {
        const tile = (label, value, sub) => h('div', { class: 'q-tile' },
            h('span', { class: 'q-tile-label', text: label }),
            h('span', { class: 'q-tile-value', text: value }),
            h('span', { class: 'q-tile-sub', text: sub }));
        this.el.overview.replaceChildren(
            tile('项目', fmtNumber(s.totalSites), s.updatedThisWeek ? `本周更新 ${s.updatedThisWeek} 个` : '本周没有更新'),
            tile('访问', fmtNumber(s.totalViews), `今天 ${fmtNumber(s.viewsToday)}`),
            tile('占用', fmtSize(s.totalBytes), `${fmtNumber(s.totalFiles)} 个文件`));
    }

    render() {
        const q = this.el.search.value.trim().toLocaleLowerCase('zh-CN');
        const list = this.sites.filter((s) => !q || `${s.title} ${s.description} ${s.name}`.toLocaleLowerCase('zh-CN').includes(q));
        list.sort((a, b) => {
            if (this.sort === 'views') return (b.views || 0) - (a.views || 0) || a.name.localeCompare(b.name);
            if (this.sort === 'name') return (a.title || a.name).localeCompare(b.title || b.name, 'zh-CN');
            if (this.sort === 'size') return (b.size || 0) - (a.size || 0);
            return new Date(b.updatedAt || b.createdAt) - new Date(a.updatedAt || a.createdAt);
        });
        this.el.count.textContent = q ? `${list.length} / ${this.sites.length}` : `${this.sites.length} 个`;
        if (!list.length) {
            this.el.list.replaceChildren(h('li', { class: 'q-state' },
                h('b', { text: q ? '没有匹配的项目' : '还没有项目' }),
                h('span', { text: q ? '换个关键词试试' : '在上面发布第一个项目' })));
            return;
        }
        this.el.list.replaceChildren(...list.map((site) => h('li', { class: 'q-row site-row' },
            h('a', { href: `#site/${encodeURIComponent(site.name)}` },
                cover(site, { pending: site.coverPending }),
                h('div', { class: 'q-row-main' },
                    h('span', { text: site.title || site.name }),
                    h('span', { text: `${fmtDate(site.updatedAt || site.createdAt)} 更新 · ${fmtNumber(site.views)} 次访问 · ${fmtSize(site.size)}` })),
                icon('chevron', 'chev')))));
    }
}
