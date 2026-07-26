const dropzone = document.getElementById('dropzone');
const fileInput = document.getElementById('file-input');
const selectedFileNode = document.getElementById('selected-file');
const deployButton = document.getElementById('deploy-btn');
const cancelButton = document.getElementById('cancel-btn');
const resultNode = document.getElementById('result');
let selectedFile = null;
let activeRequest = null;

function chooseFile() {
    fileInput.click();
}

function stageFile(file) {
    if (!/\.(html?|zip)$/i.test(file.name)) {
        showResult('err', '仅支持 .html、.htm 和 .zip 文件。');
        return;
    }
    selectedFile = file;
    selectedFileNode.hidden = false;
    selectedFileNode.textContent = `已选择：${file.name} · ${Deployer.fmtSize(file.size)}`;
    const titleInput = document.getElementById('title-input');
    if (!titleInput.value.trim()) {
        titleInput.value = file.name.replace(/\.(html?|zip)$/i, '').replace(/[-_]+/g, ' ');
    }
    deployButton.disabled = false;
    resultNode.hidden = true;
}

dropzone.addEventListener('click', chooseFile);
dropzone.addEventListener('keydown', event => {
    if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        chooseFile();
    }
});
fileInput.addEventListener('change', () => {
    if (fileInput.files.length) stageFile(fileInput.files[0]);
});
for (const eventName of ['dragenter', 'dragover']) {
    dropzone.addEventListener(eventName, event => {
        event.preventDefault();
        dropzone.classList.add('dragover');
    });
}
for (const eventName of ['dragleave', 'drop']) {
    dropzone.addEventListener(eventName, event => {
        event.preventDefault();
        dropzone.classList.remove('dragover');
    });
}
dropzone.addEventListener('drop', event => {
    if (event.dataTransfer.files.length) stageFile(event.dataTransfer.files[0]);
});

function showResult(kind, text, link) {
    resultNode.hidden = false;
    resultNode.className = `result ${kind}`;
    resultNode.replaceChildren(document.createTextNode(text));
    if (link) {
        const anchor = document.createElement('a');
        anchor.href = link;
        anchor.target = '_blank';
        anchor.rel = 'noopener noreferrer';
        anchor.textContent = link;
        resultNode.append(document.createElement('br'), anchor);
    }
}

async function uploadSelectedFile() {
    if (!selectedFile || activeRequest) return;
    const overwrite = document.getElementById('overwrite-input').checked;
    if (overwrite && !confirm('确认允许覆盖同名项目？旧版本会保留在回收站 7 天。')) return;

    const token = await Deployer.requireToken();
    if (!token) return;
    const form = new FormData();
    form.append('name', document.getElementById('name-input').value.trim());
    form.append('title', document.getElementById('title-input').value.trim());
    form.append('description', document.getElementById('description-input').value.trim());
    form.append('overwrite', overwrite ? 'true' : 'false');
    form.append('file', selectedFile);

    const request = new XMLHttpRequest();
    activeRequest = request;
    deployButton.disabled = true;
    cancelButton.hidden = false;
    showResult('loading', `正在上传 ${selectedFile.name}… 0%`);

    request.open('POST', '/api/upload');
    request.setRequestHeader('Authorization', `Bearer ${token}`);
    request.upload.addEventListener('progress', event => {
        if (event.lengthComputable) {
            const percent = Math.min(100, Math.round(event.loaded / event.total * 100));
            showResult('loading', `正在上传并部署 ${selectedFile.name}… ${percent}%`);
        }
    });
    request.addEventListener('load', async () => {
        let body = {};
        try { body = JSON.parse(request.responseText); } catch {}
        if (request.status === 201) {
            const url = Deployer.safeURL(body.url);
            showResult('ok', '部署成功，项目地址：', url);
            selectedFile = null;
            selectedFileNode.hidden = true;
            fileInput.value = '';
            document.getElementById('name-input').value = '';
            document.getElementById('title-input').value = '';
            document.getElementById('description-input').value = '';
            await Deployer.loadSites();
            if (Deployer.historyLoaded) await Deployer.loadTrash();
        } else {
            if (request.status === 401 || request.status === 429) Deployer.clearToken();
            showResult('err', body.error || `上传失败：HTTP ${request.status}`);
        }
    });
    request.addEventListener('error', () => showResult('err', '网络错误，上传未完成。'));
    request.addEventListener('abort', () => showResult('err', '上传已取消。'));
    request.addEventListener('loadend', () => {
        activeRequest = null;
        deployButton.disabled = !selectedFile;
        cancelButton.hidden = true;
    });
    request.send(form);
}

deployButton.addEventListener('click', uploadSelectedFile);
cancelButton.addEventListener('click', () => {
    if (activeRequest) activeRequest.abort();
});
