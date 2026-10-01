// Small DOM and formatting helpers shared by the catalog and the admin page.

/** Creates an element; "on…" props become listeners, class/text/dataset/hidden are special. */
export function h(tag, props = {}, ...children) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(props)) {
        if (v === undefined || v === null || v === false) continue;
        if (k === 'class') el.className = v;
        else if (k === 'text') el.textContent = v;
        else if (k === 'dataset') Object.assign(el.dataset, v);
        else if (k === 'hidden') el.hidden = Boolean(v);
        else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
        else el.setAttribute(k, v === true ? '' : v);
    }
    el.append(...children.flat().filter((c) => c !== null && c !== undefined && c !== false));
    return el;
}

const SVG = 'http://www.w3.org/2000/svg';

/** An icon from the page's inline sprite. */
export function icon(name, cls = '') {
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('class', `q-i ${cls}`.trim());
    svg.setAttribute('aria-hidden', 'true');
    const use = document.createElementNS(SVG, 'use');
    use.setAttribute('href', `#i-${name}`);
    svg.append(use);
    return svg;
}

/** A brief message at the bottom; with `action`, it carries one button (e.g. 撤销). */
export function toast(message, { type = 'info', action, duration = 3000 } = {}) {
    const host = document.getElementById('toasts');
    if (!host) return () => {};
    let closed = false;
    const el = h('div', { class: `q-toast${type === 'error' ? ' q-toast--error' : ''}`, role: type === 'error' ? 'alert' : 'status' }, h('span', { text: message }));
    const close = () => {
        if (closed) return;
        closed = true;
        clearTimeout(timer);
        el.classList.add('is-leaving');
        setTimeout(() => el.remove(), 200);
    };
    if (action) el.append(h('button', { type: 'button', text: action.label, onclick: () => { action.onClick(); close(); } }));
    host.append(el);
    const timer = setTimeout(close, duration);
    return close;
}

export function fmtSize(bytes = 0) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
    return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

export function fmtNumber(n = 0) {
    return new Intl.NumberFormat('zh-CN', { notation: n >= 10000 ? 'compact' : 'standard' }).format(n);
}

export function fmtDate(value, withTime = false) {
    if (!value) return '';
    const d = new Date(value);
    const now = new Date();
    const opts = d.getFullYear() === now.getFullYear() ? { month: 'numeric', day: 'numeric' } : { year: 'numeric', month: 'numeric', day: 'numeric' };
    if (withTime) Object.assign(opts, { hour: '2-digit', minute: '2-digit' });
    return new Intl.DateTimeFormat('zh-CN', opts).format(d);
}

/** Only links to https pages (or the local machine) are used as hrefs. */
export function safeURL(raw) {
    try {
        const u = new URL(raw, location.origin);
        if (u.protocol === 'https:' || u.hostname === 'localhost' || u.hostname === '127.0.0.1') return u.href;
    } catch { /* fall through */ }
    return '';
}

/** Copies text; the button briefly shows a check mark. */
export async function copyText(text, btn) {
    try {
        await navigator.clipboard.writeText(text);
    } catch {
        toast('复制失败', { type: 'error' });
        return;
    }
    toast('已复制链接');
    const use = btn?.querySelector('use');
    if (!use) return;
    const prev = use.getAttribute('href');
    use.setAttribute('href', '#i-check');
    setTimeout(() => use.setAttribute('href', prev), 1400);
}

/** The cover, or the title's first letter on a plain tile until there is one. */
export function cover(site, { pending = false } = {}) {
    const box = h('span', { class: 'cover' });
    if (site.cover) {
        box.append(h('img', { src: site.cover, alt: '', loading: 'lazy', decoding: 'async' }));
    } else {
        const first = [...(site.title || site.name || '·').trim()][0] || '·';
        box.append(h('span', { class: 'initial', text: pending ? '…' : first.toUpperCase() }));
    }
    return box;
}
