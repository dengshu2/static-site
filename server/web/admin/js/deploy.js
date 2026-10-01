// Publishing a new project: drop or pick a file, check the details, publish.
// The page shows the address the project will get and asks before replacing
// a project that already has that name.

import { slugify, upload } from './api.js';
import { fmtSize, safeURL, toast } from '../ui.js';

export class Deploy {
    constructor({ config, sites, onPublished }) {
        const $ = (id) => document.getElementById(id);
        this.el = {
            form: $('deploy-form'), file: $('file-input'), drop: $('drop'), dropTitle: $('drop-title'), dropSub: $('drop-sub'),
            fields: $('deploy-fields'), title: $('title-input'), desc: $('desc-input'), name: $('name-input'), nameHint: $('name-hint'),
            note: $('overwrite-note'), noteText: $('overwrite-text'), overwrite: $('overwrite-input'),
            status: $('deploy-status'), reset: $('deploy-reset'), submit: $('deploy-submit'),
        };
        this.maxMB = config.maxUploadMB || 50;
        this.trashDays = config.trashDays || 7;
        this.sites = sites;          // () => the current project list
        this.onPublished = onPublished;
        this.file = null;
        this.job = null;
        this.el.dropSub.textContent = `HTML 单文件，或包含 index.html 的 ZIP，最大 ${this.maxMB} MB`;
        this.bind();
    }

    bind() {
        const { drop, file, form, name, overwrite, reset, title } = this.el;
        drop.addEventListener('click', () => file.click());
        file.addEventListener('change', () => file.files[0] && this.stage(file.files[0]));
        for (const type of ['dragenter', 'dragover']) {
            drop.addEventListener(type, (e) => { e.preventDefault(); drop.classList.add('is-over'); });
        }
        for (const type of ['dragleave', 'drop']) {
            drop.addEventListener(type, (e) => { e.preventDefault(); drop.classList.remove('is-over'); });
        }
        drop.addEventListener('drop', (e) => e.dataTransfer.files[0] && this.stage(e.dataTransfer.files[0]));
        // Dropping anywhere else on the page must not open the file instead.
        window.addEventListener('dragover', (e) => e.preventDefault());
        window.addEventListener('drop', (e) => e.preventDefault());
        name.addEventListener('input', () => this.sync());
        title.addEventListener('input', () => this.sync());
        overwrite.addEventListener('change', () => this.sync());
        reset.addEventListener('click', () => (this.job ? this.job.abort() : this.clear()));
        form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.publish();
        });
    }

    stage(file) {
        if (!/\.(html?|zip)$/i.test(file.name)) {
            toast('只支持 .html、.htm 和 .zip 文件', { type: 'error' });
            return;
        }
        if (file.size > this.maxMB * 1024 * 1024) {
            toast(`文件超过 ${this.maxMB} MB 上限`, { type: 'error' });
            return;
        }
        this.file = file;
        this.el.dropTitle.textContent = file.name;
        this.el.dropSub.textContent = `${fmtSize(file.size)} · 点击换一个`;
        this.el.fields.hidden = false;
        if (!this.el.title.value.trim()) {
            this.el.title.value = file.name.replace(/\.(html?|zip)$/i, '').replace(/[-_]+/g, ' ').trim();
        }
        this.sync();
        this.el.title.focus();
    }

    /** The address preview, the replace warning and whether Publish is allowed. */
    sync() {
        const { name, nameHint, note, noteText, overwrite, submit, status } = this.el;
        const raw = name.value.trim();
        const slug = slugify(raw);
        const existing = slug ? this.sites().find((s) => s.name === slug) : null;
        if (raw && !slug) {
            nameHint.textContent = '站点名要包含英文字母或数字';
            nameHint.classList.add('is-bad');
        } else {
            nameHint.classList.remove('is-bad');
            nameHint.textContent = slug ? `/s/${slug}/` : '';
        }
        note.hidden = !existing;
        if (existing) noteText.textContent = `已有同名项目“${existing.title}”。发布会替换它，当前版本保留 ${this.trashDays} 天，可以恢复。`;
        else overwrite.checked = false;
        submit.disabled = Boolean(this.job) || !this.file || (raw && !slug) || (existing && !overwrite.checked);
        if (!this.job) status.textContent = '';
    }

    async publish() {
        if (this.el.submit.disabled) return;
        const { title, desc, name, overwrite, status, submit, reset } = this.el;
        const form = new FormData();
        form.append('name', name.value.trim());
        form.append('title', title.value.trim());
        form.append('description', desc.value.trim());
        form.append('overwrite', overwrite.checked ? 'true' : 'false');
        form.append('file', this.file);

        this.job = upload(form, (p) => {
            status.textContent = p < 1 ? `上传中 ${Math.round(p * 100)}%` : '正在发布…';
        });
        submit.disabled = true;
        submit.textContent = '发布中…';
        reset.textContent = '取消上传';
        status.textContent = '上传中 0%';
        try {
            const site = await this.job.done;
            this.job = null;
            this.clear();
            const url = safeURL(site.url);
            toast(`已发布：${site.title}`, url ? { action: { label: '打开', onClick: () => window.open(url, '_blank', 'noopener') }, duration: 6000 } : {});
            this.onPublished(site);
        } catch (err) {
            this.job = null;
            if (err.status !== -1) toast(err.message, { type: 'error' });
            status.textContent = '';
        } finally {
            submit.textContent = '发布';
            reset.textContent = '取消';
            this.sync();
        }
    }

    clear() {
        const { file, title, desc, name, overwrite, fields, dropTitle, dropSub } = this.el;
        this.file = null;
        file.value = '';
        title.value = '';
        desc.value = '';
        name.value = '';
        overwrite.checked = false;
        fields.hidden = true;
        dropTitle.textContent = '拖入文件，或点击选择';
        dropSub.textContent = `HTML 单文件，或包含 index.html 的 ZIP，最大 ${this.maxMB} MB`;
        this.sync();
    }
}

