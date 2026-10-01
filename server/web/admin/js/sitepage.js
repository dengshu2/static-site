// One project: its cover, address and visits; editing the title and
// description; a new version; deleting (with undo); and its earlier versions.

import { api, confirmTwice, upload } from './api.js';
import { copyText, cover, fmtDate, fmtNumber, fmtSize, h, icon, safeURL, toast } from '../ui.js';

const REASON = { overwritten: '被替换的版本', deleted: '删除前的版本' };

export class SitePage {
    constructor({ config, onChanged, onGone }) {
        this.body = document.getElementById('site-body');
        this.open = document.getElementById('site-open');
        this.trashDays = config.trashDays || 7;
        this.onChanged = onChanged;
        this.onGone = onGone;
        this.name = null;
        this.poll = null;
        this.job = null;
    }

    async show(name, { quiet = false } = {}) {
        this.name = name;
        clearTimeout(this.poll);
        if (!quiet) {
            const skel = h('div', { class: 'q-skeleton', 'aria-hidden': 'true' });
            skel.style.aspectRatio = '16 / 10';
            this.body.replaceChildren(skel);
        }
        let d;
        try {
            d = await api(`/api/sites/${encodeURIComponent(name)}`);
        } catch (err) {
            if (this.name !== name || err.status === 401) return;
            this.body.replaceChildren(h('div', { class: 'q-state' }, h('b', { text: '打不开这个项目' }), h('span', { text: err.message })));
            return;
        }
        if (this.name !== name) return;
        document.title = `${d.site.title} · Drop & Deploy`;
        this.render(d);
        if (d.site.coverPending) this.poll = setTimeout(() => this.show(name, { quiet: true }), 3000);
    }

    render(d) {
        const s = d.site;
        const url = safeURL(s.url);
        this.open.href = url || '#';
        const copy = h('button', { class: 'q-play q-play--sm', type: 'button', 'aria-label': '复制链接', title: '复制链接', onclick: () => copyText(url, copy) }, icon('copy'));
        const retake = h('button', { class: 'q-btn q-btn--sm retake', type: 'button', onclick: () => this.retake(retake) },
            icon('camera', 'q-i--sm'), s.coverPending ? '截图中…' : '重新截图');
        retake.disabled = s.coverPending;

        this.body.replaceChildren(
            h('div', { class: 'site-head' },
                h('div', { class: 'cover-wrap' }, cover(s, { pending: s.coverPending }), retake),
                h('h1', { text: s.title || s.name }),
                h('div', { class: 'site-url' }, h('a', { href: url, target: '_blank', rel: 'noopener', text: url.replace(/^https?:\/\//, '') }), copy)),
            this.actions(s),
            this.editor(s),
            this.visits(s, d.daily),
            h('section', { class: 'q-sec' },
                h('h3', { text: '详情' }),
                h('dl', { class: 'facts' },
                    h('dt', { text: '站点名' }), h('dd', { text: s.name }),
                    h('dt', { text: '文件' }), h('dd', { text: `${fmtNumber(s.files)} 个，共 ${fmtSize(s.size)}` }),
                    h('dt', { text: '首次发布' }), h('dd', { text: fmtDate(s.createdAt, true) }),
                    h('dt', { text: '最近更新' }), h('dd', { text: fmtDate(s.updatedAt, true) }),
                    h('dt', { text: '最近访问' }), h('dd', { text: s.lastViewedAt ? fmtDate(s.lastViewedAt, true) : '还没有' }))),
            this.versions(d.versions),
        );
    }

    actions(s) {
        const file = h('input', { type: 'file', accept: '.html,.htm,.zip', hidden: true });
        const replace = h('button', { class: 'q-btn q-btn--sm', type: 'button', onclick: () => (this.job ? this.job.abort() : file.click()) },
            icon('upload', 'q-i--sm'), '上传新版本');
        file.addEventListener('change', () => file.files[0] && this.replace(s, file.files[0], replace));
        const del = h('button', { class: 'q-btn q-btn--sm q-btn--quiet', type: 'button', onclick: () => this.remove(s) }, icon('trash', 'q-i--sm'), '删除');
        return h('div', { class: 'q-actions' }, replace, file, del);
    }

    editor(s) {
        const title = h('input', { class: 'q-field', maxlength: '120', value: s.title || '' });
        const desc = h('textarea', { class: 'q-field', rows: '3', maxlength: '400', placeholder: '一两句话说明这个项目' });
        desc.value = s.description || '';
        const save = h('button', { class: 'q-btn q-btn--sm q-btn--primary', type: 'submit', disabled: true }, '保存');
        const dirty = () => { save.disabled = title.value.trim() === (s.title || '') && desc.value.trim() === (s.description || ''); };
        title.addEventListener('input', dirty);
        desc.addEventListener('input', dirty);
        const form = h('form', { class: 'q-sec edit', novalidate: true },
            h('h3', { text: '标题和简介' }),
            h('label', {}, h('span', { class: 'q-field-label', text: '标题' }), title),
            h('label', {}, h('span', { class: 'q-field-label', text: '简介' }), desc),
            h('div', { class: 'q-actions' }, save));
        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            if (save.disabled) return;
            save.disabled = true;
            save.textContent = '保存中…';
            try {
                await api(`/api/sites/${encodeURIComponent(s.name)}`, { method: 'PATCH', body: { title: title.value.trim(), description: desc.value.trim() } });
                toast('已保存');
                this.onChanged();
                this.show(s.name, { quiet: true });
            } catch (err) {
                toast(err.message, { type: 'error' });
                save.disabled = false;
            } finally {
                save.textContent = '保存';
            }
        });
        return form;
    }

    visits(s, daily) {
        const max = Math.max(1, ...daily.map((d) => d.views));
        const month = daily.reduce((n, d) => n + d.views, 0);
        const bars = h('div', { class: 'bars', role: 'img', 'aria-label': `近 30 天共 ${month} 次访问` }, ...daily.map((d) => {
            const bar = h('i', { class: d.views ? '' : 'zero', title: `${d.date}：${d.views} 次` });
            bar.style.height = `${Math.max(3, (d.views / max) * 100)}%`;
            return bar;
        }));
        return h('section', { class: 'q-sec visits' },
            h('div', { class: 'sec-head' }, h('h3', { text: '访问' }), h('span', { class: 'q-meta', text: `近 30 天 ${fmtNumber(month)} 次 · 累计 ${fmtNumber(s.views)} 次` })),
            bars,
            h('div', { class: 'bars-axis' }, h('span', { text: fmtDate(daily[0].date) }), h('span', { text: '今天' })));
    }

    versions(items) {
        const sec = h('section', { class: 'q-sec' }, h('div', { class: 'sec-head' },
            h('h3', { text: '历史版本' }), h('span', { class: 'q-meta', text: `保留 ${this.trashDays} 天` })));
        if (!items.length) {
            sec.append(h('p', { class: 'q-empty', text: '还没有被替换下来的版本。上传新版本后，当前版本会保留在这里。' }));
            return sec;
        }
        sec.append(h('ul', { class: 'q-rows' }, ...items.map((v) => {
            const restore = confirmTwice(h('button', { class: 'q-btn q-btn--sm', type: 'button' }, '恢复这个版本'),
                () => this.restore(v), '再点一次，替换当前版本');
            return h('li', { class: 'q-row version' },
                h('div', { class: 'q-row-main' },
                    h('span', { text: `${REASON[v.reason] || '旧版本'} · ${fmtDate(v.deletedAt, true)}` }),
                    h('span', { text: `${v.site.title} · ${fmtNumber(v.site.files)} 个文件，${fmtSize(v.site.size)}` })),
                restore);
        })));
        return sec;
    }

    async retake(btn) {
        btn.disabled = true;
        try {
            await api(`/api/sites/${encodeURIComponent(this.name)}/cover`, { method: 'POST' });
            toast('正在重新截图，几秒后更新');
            this.show(this.name, { quiet: true });
        } catch (err) {
            toast(err.message, { type: 'error' });
            btn.disabled = false;
        }
    }

    async replace(s, file, btn) {
        if (!/\.(html?|zip)$/i.test(file.name)) {
            toast('只支持 .html、.htm 和 .zip 文件', { type: 'error' });
            return;
        }
        const form = new FormData();
        form.append('name', s.name);
        form.append('title', '');
        form.append('description', '');
        form.append('overwrite', 'true');
        form.append('file', file);
        const label = btn.lastChild;
        this.job = upload(form, (p) => { label.textContent = p < 1 ? `上传中 ${Math.round(p * 100)}%（点此取消）` : '正在发布…'; });
        try {
            await this.job.done;
            toast(`已更新，旧版本保留 ${this.trashDays} 天`);
            this.onChanged();
            this.show(s.name, { quiet: true });
        } catch (err) {
            if (err.status !== -1) toast(err.message, { type: 'error' });
        } finally {
            this.job = null;
            label.textContent = '上传新版本';
        }
    }

    async remove(s) {
        let res;
        try {
            res = await api(`/api/sites/${encodeURIComponent(s.name)}`, { method: 'DELETE' });
        } catch (err) {
            toast(err.message, { type: 'error' });
            return;
        }
        this.onChanged();
        this.onGone();
        toast(`已删除“${s.title}”`, {
            duration: 6000,
            action: {
                label: '撤销',
                onClick: async () => {
                    try {
                        await api(`/api/trash/${encodeURIComponent(res.trashId)}/restore`, { method: 'POST' });
                        toast('已恢复');
                        this.onChanged();
                    } catch (err) {
                        toast(err.message, { type: 'error' });
                    }
                },
            },
        });
    }

    async restore(v) {
        try {
            await api(`/api/trash/${encodeURIComponent(v.id)}/restore?replace=1`, { method: 'POST' });
            toast('已恢复；被替换下来的版本也在历史里');
            this.onChanged();
            this.show(v.site.name, { quiet: true });
        } catch (err) {
            toast(err.message, { type: 'error' });
        }
    }
}
