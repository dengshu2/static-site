// upload.js — 拖拽 / 选择文件、调用 /api/upload、展示结果。
// 依赖 window.Deployer（见 sites.js）。

const dropzone = document.getElementById('dropzone');
const fileInput = document.getElementById('file-input');
const result = document.getElementById('result');

// ── 触发文件选择 ──────────────────────────────────────────────
document.getElementById('browse-btn').addEventListener('click', () => fileInput.click());
dropzone.addEventListener('click', (e) => {
    if (e.target.id !== 'browse-btn') fileInput.click();
});
fileInput.addEventListener('change', () => {
    if (fileInput.files.length) upload(fileInput.files[0]);
});

// ── 拖拽 ──────────────────────────────────────────────────────
['dragenter', 'dragover'].forEach(ev =>
    dropzone.addEventListener(ev, (e) => { e.preventDefault(); dropzone.classList.add('dragover'); }));
['dragleave', 'drop'].forEach(ev =>
    dropzone.addEventListener(ev, (e) => { e.preventDefault(); dropzone.classList.remove('dragover'); }));
dropzone.addEventListener('drop', (e) => {
    const f = e.dataTransfer.files[0];
    if (f) upload(f);
});

// ── 上传 ──────────────────────────────────────────────────────
async function upload(file) {
    const okExt = /\.(html?|zip)$/i.test(file.name);
    if (!okExt) {
        show('err', '仅支持 .html / .htm / .zip 文件');
        return;
    }

    const form = new FormData();
    form.append('file', file);
    form.append('name', document.getElementById('name-input').value.trim());
    form.append('overwrite', document.getElementById('overwrite-input').checked ? 'true' : 'false');

    show('loading', `正在上传并部署 ${file.name} …`);

    try {
        const res = await fetch('/api/upload', {
            method: 'POST',
            headers: Deployer.authHeaders(),
            body: form,
        });
        const body = await res.json().catch(() => ({}));

        if (res.status === 201) {
            show('ok', `✅ 部署成功！访问：<a href="${body.url}" target="_blank" rel="noopener">${location.origin}${body.url}</a>`);
            document.getElementById('name-input').value = '';
            fileInput.value = '';
            Deployer.loadSites();
        } else if (res.status === 409) {
            show('err', `⚠️ ${body.error}（可勾选「同名时覆盖」后重试）`);
        } else if (res.status === 401) {
            show('err', '❌ Token 无效，点击右上角 🔑 重新设置。');
        } else {
            show('err', '❌ ' + (body.error || ('上传失败：' + res.status)));
        }
    } catch (e) {
        show('err', '❌ 网络错误：' + e.message);
    }
}

function show(kind, html) {
    result.hidden = false;
    result.className = 'result ' + kind;
    result.innerHTML = html;
}
