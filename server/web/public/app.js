// The public catalog: every project as a cover card, with search and sorting.

import { copyText, cover, fmtDate, fmtNumber, h, icon, safeURL, toast } from './ui.js';

const $ = (id) => document.getElementById(id);
const grid = $('grid');
const search = $('search');
let sites = [];
let sort = 'updated';

function render() {
    const q = search.value.trim().toLocaleLowerCase('zh-CN');
    const list = sites.filter((s) => !q || `${s.title} ${s.description} ${s.name}`.toLocaleLowerCase('zh-CN').includes(q));
    list.sort((a, b) => {
        if (sort === 'views') return (b.views || 0) - (a.views || 0) || a.name.localeCompare(b.name);
        if (sort === 'name') return (a.title || a.name).localeCompare(b.title || b.name, 'zh-CN');
        return new Date(b.updatedAt || b.createdAt) - new Date(a.updatedAt || a.createdAt);
    });
    if (!list.length) {
        grid.replaceChildren(h('div', { class: 'q-state' },
            h('b', { text: q ? '没有匹配的项目' : '还没有项目' }),
            h('span', { text: q ? '换个关键词试试' : '发布的项目会出现在这里' })));
        return;
    }
    grid.replaceChildren(...list.map(card));
}

function card(site) {
    const url = safeURL(site.url);
    const copy = h('button', {
        class: 'q-play q-play--sm', type: 'button', title: '复制链接', 'aria-label': `复制“${site.title}”的链接`,
        onclick: () => copyText(url, copy),
    }, icon('copy'));
    return h('article', { class: 'card' },
        h('a', { class: 'card-link', href: url, target: '_blank', rel: 'noopener noreferrer' },
            cover(site),
            h('h2', { class: 'card-title', text: site.title || site.name }),
            site.description ? h('p', { class: 'card-desc', text: site.description }) : null),
        h('div', { class: 'card-foot' },
            h('span', { class: 'q-st', text: `${fmtDate(site.updatedAt || site.createdAt)} 更新 · ${fmtNumber(site.views)} 次访问` }),
            url ? copy : null));
}

async function load() {
    grid.replaceChildren(...Array.from({ length: 4 }, () => h('span', { class: 'q-skeleton skel', 'aria-hidden': 'true' })));
    try {
        const [list, stats] = await Promise.all([
            fetch('/api/sites', { cache: 'no-store' }).then((r) => (r.ok ? r.json() : Promise.reject(r.status))),
            fetch('/api/analytics', { cache: 'no-store' }).then((r) => (r.ok ? r.json() : null)).catch(() => null),
        ]);
        sites = list;
        if (stats) {
            $('stats').textContent = `${fmtNumber(stats.totalSites)} 个项目 · 共 ${fmtNumber(stats.totalViews)} 次访问` +
                (stats.updatedThisWeek ? ` · 本周更新 ${fmtNumber(stats.updatedThisWeek)} 个` : '');
        }
        render();
    } catch {
        grid.replaceChildren(h('div', { class: 'q-state' },
            h('b', { text: '加载失败' }), h('span', { text: '请稍后刷新页面' }),
            h('button', { class: 'q-btn q-btn--sm', type: 'button', text: '重试', onclick: load })));
        toast('项目列表加载失败', { type: 'error' });
    }
}

let timer;
search.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(render, 120);
});
for (const btn of $('sort').querySelectorAll('[data-sort]')) {
    btn.addEventListener('click', () => {
        sort = btn.dataset.sort;
        for (const b of $('sort').querySelectorAll('[data-sort]')) b.setAttribute('aria-pressed', String(b === btn));
        render();
    });
}
load();
