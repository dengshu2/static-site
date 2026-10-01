// Talking to the server. The session is an HttpOnly cookie the browser sends
// on its own; nothing here ever sees or stores the token.

export class ApiError extends Error {
    constructor(message, status) {
        super(message);
        this.status = status;
    }
}

let signedOut = () => {};

/** Called when the server says the session is over. */
export function whenSignedOut(fn) {
    signedOut = fn;
}

export async function api(path, { method = 'GET', body } = {}) {
    let res;
    try {
        res = await fetch(path, {
            method,
            cache: 'no-store',
            credentials: 'same-origin',
            headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
    } catch {
        throw new ApiError('网络连接失败，请检查网络后重试', 0);
    }
    if (res.status === 401 && path !== '/api/session') {
        signedOut();
        throw new ApiError('登录已过期，请重新登录', 401);
    }
    if (!res.ok) {
        let message = `请求失败（${res.status}）`;
        try { message = (await res.json()).error || message; } catch { /* not JSON */ }
        throw new ApiError(message, res.status);
    }
    return res.status === 204 ? null : res.json();
}

/** Uploads a project with progress. Returns { done: Promise<site>, abort }. */
export function upload(form, onProgress) {
    const xhr = new XMLHttpRequest();
    const done = new Promise((resolve, reject) => {
        xhr.open('POST', '/api/upload');
        xhr.upload.addEventListener('progress', (e) => {
            if (e.lengthComputable) onProgress(e.loaded / e.total);
        });
        xhr.addEventListener('load', () => {
            let body = {};
            try { body = JSON.parse(xhr.responseText); } catch { /* not JSON */ }
            if (xhr.status === 201) {
                resolve(body);
                return;
            }
            if (xhr.status === 401) signedOut();
            reject(new ApiError(body.error || `上传失败（${xhr.status}）`, xhr.status));
        });
        xhr.addEventListener('error', () => reject(new ApiError('网络错误，上传没有完成', 0)));
        xhr.addEventListener('abort', () => reject(new ApiError('已取消上传', -1)));
        xhr.send(form);
    });
    return { done, abort: () => xhr.abort() };
}

/** The server's rule for site names, so the page can show the address first. */
export function slugify(s) {
    return s.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

/**
 * Asks for a second click before something that cannot be undone: the button
 * turns red and says so for a few seconds; a second click runs `action`.
 */
export function confirmTwice(btn, action, label = '再点一次确认') {
    const original = btn.textContent;
    let armed = false;
    let timer;
    btn.addEventListener('click', () => {
        if (!armed) {
            armed = true;
            btn.classList.add('is-arming');
            btn.textContent = label;
            timer = setTimeout(() => {
                armed = false;
                btn.classList.remove('is-arming');
                btn.textContent = original;
            }, 3000);
            return;
        }
        clearTimeout(timer);
        armed = false;
        btn.classList.remove('is-arming');
        btn.textContent = original;
        action();
    });
    return btn;
}
