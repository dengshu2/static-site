// Every earlier version and deleted project still kept: restore one, or
// delete it for good (asked twice).

import { api, confirmTwice } from './api.js';
import { fmtDate, fmtNumber, fmtSize, h, toast } from '../ui.js';

const REASON = { overwritten: '被替换的版本', deleted: '已删除' };

export class History {
    constructor({ config, onChanged }) {
        const $ = (id) => document.getElementById(id);
        this.el = { list: $('history'), search: $('history-search'), filter: $('history-filter'), note: $('history-note') };
        this.days = config.trashDays || 7;
        this.onChanged = onChanged;
        this.items = [];
        this.live = new Set();
        this.filter = 'all';
        this.el.note.textContent = `被替换下来的版本和删除的项目保留 ${this.days} 天，到期自动清除。`;
        let timer;
        this.el.search.addEventListener('input', () => {
            clearTimeout(timer);
            timer = setTimeout(() => this.render(), 120);
        });
        this.el.search.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.el.search.value) {
                e.preventDefault(); // Esc clears the search before it leaves the page
                this.el.search.value = '';
                this.render();
            }
        });
        for (const btn of this.el.filter.querySelectorAll('[data-filter]')) {
            btn.addEventListener('click', () => {
                this.filter = btn.dataset.filter;
                for (const b of this.el.filter.querySelectorAll('[data-filter]')) b.setAttribute('aria-pressed', String(b === btn));
                this.render();
            });
        }
    }

    async show() {
        const skel = h('li', { class: 'q-skeleton', 'aria-hidden': 'true' });
        skel.style.height = '160px';
        this.el.list.replaceChildren(skel);
        try {
            const [items, sites] = await Promise.all([api('/api/trash'), api('/api/sites')]);
            this.items = items;
            this.live = new Set(sites.map((s) => s.name));
            this.render();
        } catch (err) {
            if (err.status === 401) return;
            this.el.list.replaceChildren(h('li', { class: 'q-state' },
                h('b', { text: '加载失败' }), h('span', { text: err.message }),
                h('button', { class: 'q-btn q-btn--sm', type: 'button', text: '重试', onclick: () => this.show() })));
        }
    }

    render() {
        const q = this.el.search.value.trim().toLocaleLowerCase('zh-CN');
        const items = this.items.filter((it) => (this.filter === 'all' || it.reason === this.filter) &&
            (!q || `${it.site.title} ${it.site.name}`.toLocaleLowerCase('zh-CN').includes(q)));
        if (!items.length) {
            this.el.list.replaceChildren(h('li', { class: 'q-state' },
                h('b', { text: q || this.filter !== 'all' ? '没有符合条件的记录' : '历史是空的' }),
                h('span', { text: '替换或删除项目后，旧版本会出现在这里' })));
            return;
        }
        this.el.list.replaceChildren(...items.map((it) => this.row(it)));
    }

    row(it) {
        const left = Math.max(0, Math.ceil((new Date(it.deletedAt).getTime() + this.days * 864e5 - Date.now()) / 864e5));
        const exists = this.live.has(it.site.name);
        const restore = exists
            ? confirmTwice(h('button', { class: 'q-btn q-btn--sm', type: 'button' }, '恢复'), () => this.restore(it, true), '替换当前版本？')
            : h('button', { class: 'q-btn q-btn--sm', type: 'button', onclick: () => this.restore(it, false) }, '恢复');
        const purge = confirmTwice(h('button', { class: 'q-btn q-btn--sm q-btn--quiet', type: 'button' }, '永久删除'), () => this.purge(it), '再点一次删除');
        return h('li', { class: 'q-row version' },
            h('div', { class: 'q-row-main' },
                h('span', { text: it.site.title || it.site.name }),
                h('span', { text: `${REASON[it.reason] || '旧版本'} · ${fmtDate(it.deletedAt, true)} · ${fmtNumber(it.site.files)} 个文件，${fmtSize(it.site.size)} · 还剩 ${left} 天` })),
            h('div', { class: 'row-actions' }, restore, purge));
    }

    async restore(it, replace) {
        try {
            await api(`/api/trash/${encodeURIComponent(it.id)}/restore${replace ? '?replace=1' : ''}`, { method: 'POST' });
            toast(replace ? '已恢复；被替换下来的版本也在历史里' : `已恢复“${it.site.title}”`);
            this.onChanged();
            this.show();
        } catch (err) {
            toast(err.message, { type: 'error' });
        }
    }

    async purge(it) {
        try {
            await api(`/api/trash/${encodeURIComponent(it.id)}`, { method: 'DELETE' });
            this.items = this.items.filter((x) => x.id !== it.id);
            this.render();
            toast('已永久删除');
        } catch (err) {
            toast(err.message, { type: 'error' });
        }
    }
}
