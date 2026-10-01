import { api, whenSignedOut } from './api.js';
import { History } from './history.js';
import { Home } from './home.js';
import { SitePage } from './sitepage.js';

// Full pages switched by the hash, no pop-ups: the workbench (#), one project
// (#site/<name>) and the history (#history). Back and Esc return to where
// you were, with the scroll position kept.

const $ = (id) => document.getElementById(id);
const views = { login: $('view-login'), home: $('view-home'), site: $('view-site'), history: $('view-history') };
let config = {};
let started = false;
let home;
let sitePage;
let historyPage;
let current = null;
let moves = 0;
const scroll = {};

function showOnly(name) {
    for (const [key, el] of Object.entries(views)) el.hidden = key !== name;
}

function showLogin() {
    current = 'login';
    showOnly('login');
    document.title = '登录 · Drop & Deploy';
    $('login-token').value = '';
    $('login-token').focus();
}

whenSignedOut(showLogin);

$('login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const input = $('login-token');
    const submit = $('login-submit');
    const error = $('login-error');
    if (!input.value.trim()) {
        error.textContent = '请输入 Token';
        error.hidden = false;
        input.focus();
        return;
    }
    submit.disabled = true;
    submit.textContent = '登录中…';
    error.hidden = true;
    try {
        await api('/api/session', { method: 'POST', body: { token: input.value.trim() } });
        input.value = '';
        start();
    } catch (err) {
        error.textContent = err.message;
        error.hidden = false;
        input.select();
    } finally {
        submit.disabled = false;
        submit.textContent = '登录';
    }
});

function start() {
    if (!started) {
        started = true;
        home = new Home({ config, onLogout: logout });
        sitePage = new SitePage({ config, onChanged: () => home.refresh(), onGone: () => leave() });
        historyPage = new History({ config, onChanged: () => home.refresh() });
        window.addEventListener('hashchange', () => {
            moves = Math.max(0, moves + (location.hash ? 1 : -1));
            route();
        });
        for (const btn of document.querySelectorAll('[data-back]')) btn.addEventListener('click', leave);
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && current !== 'home' && current !== 'login' && !e.defaultPrevented) leave();
        });
    }
    current = null;
    route();
    home.refresh();
}

function route() {
    const hash = decodeURIComponent(location.hash.slice(1));
    if (hash.startsWith('site/')) show('site', hash.slice(5));
    else show(hash === 'history' ? 'history' : 'home');
}

function show(name, arg) {
    const from = current;
    if (from && from !== 'login') scroll[from] = window.scrollY;
    showOnly(name);
    current = name;
    const el = views[name];
    if (from && from !== 'login' && from !== name) {
        const cls = name === 'home' ? 'q-enter-back' : 'q-enter';
        el.classList.add(cls);
        el.addEventListener('animationend', () => el.classList.remove(cls), { once: true });
    }
    if (name === 'home') {
        document.title = '管理 · Drop & Deploy';
        window.scrollTo({ top: scroll.home || 0 });
    } else if (name === 'site') {
        sitePage.show(arg);
        window.scrollTo({ top: 0 });
    } else {
        historyPage.show();
        document.title = '历史版本 · Drop & Deploy';
        window.scrollTo({ top: 0 });
    }
}

function leave() {
    if (moves > 0) window.history.back();
    else location.hash = '';
}

async function logout() {
    try { await api('/api/session', { method: 'DELETE' }); } catch { /* signed out either way */ }
    location.hash = '';
    showLogin();
}

async function boot() {
    try {
        config = await (await fetch('/api/config', { cache: 'no-store' })).json();
    } catch { /* defaults */ }
    if (config.publicURL) $('catalog-link').href = config.publicURL;
    const session = await api('/api/session').catch(() => null);
    if (session?.signedIn) start();
    else showLogin();
}

boot();
