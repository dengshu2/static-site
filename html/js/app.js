import { TOOLS } from '../data/tools.js?v=20260417_2';

// ── Render ──────────────────────────────────────────────────────────────────

/**
 * 根据工具数据生成一张卡片的 HTML 字符串。
 * @param {object} tool
 * @returns {string}
 */
function renderCard(tool) {
    const statusBadge = tool.status === 'done'
        ? `<span class="badge badge-done">有提示词</span>`
        : `<span class="badge badge-pending">待补充</span>`;

    const modelBadge = tool.model
        ? `<span class="badge badge-model">${tool.model}</span>`
        : '';

    const promptContent = tool.status === 'done' && tool.prompt
        ? `
        <div class="prompt-body" id="prompt-${tool.id}" role="region" aria-labelledby="toggle-${tool.id}">
            <div class="prompt-actions">
                <button class="btn-copy" data-target="text-${tool.id}">复制</button>
            </div>
            <pre class="prompt-pre" id="text-${tool.id}">${escapeHtml(tool.prompt)}</pre>
        </div>`
        : `
        <p class="prompt-pending" id="prompt-${tool.id}" role="region" aria-labelledby="toggle-${tool.id}">
            提示词待补充——如果你用大模型复现了这个工具，欢迎在 GitHub 提交提示词。
        </p>`;

    return `
    <article class="card" role="listitem">
        <div class="card-header">
            <div class="card-meta">
                <div class="card-title-row">
                    <span class="card-title">${tool.title}</span>
                    ${statusBadge}
                    ${modelBadge}
                </div>
                <p class="card-desc">${tool.desc}</p>
            </div>
            <div class="card-actions">
                <a href="${tool.href}" class="btn-tool">打开工具 ↗</a>
            </div>
        </div>
        <div class="prompt-section">
            <button class="prompt-toggle" aria-expanded="false"
                    aria-controls="prompt-${tool.id}" id="toggle-${tool.id}">
                <span>查看提示词</span>
                <svg class="toggle-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor"
                     stroke-width="1.5" aria-hidden="true">
                    <path d="M4 6l4 4 4-4"/>
                </svg>
            </button>
            ${promptContent}
        </div>
    </article>`;
}

/**
 * 转义 HTML 特殊字符，防止提示词内容中的 < > & 破坏 DOM。
 * @param {string} str
 * @returns {string}
 */
function escapeHtml(str) {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// ── Mount ────────────────────────────────────────────────────────────────────

function mountToolList() {
    const container = document.getElementById('tool-list');
    if (!container) return;
    container.innerHTML = TOOLS.map(renderCard).join('');
}

// ── Events ───────────────────────────────────────────────────────────────────

function bindToggle() {
    document.querySelectorAll('.prompt-toggle').forEach(btn => {
        const targetId = btn.getAttribute('aria-controls');
        const panel = document.getElementById(targetId);
        const label = btn.querySelector('span');

        btn.addEventListener('click', () => {
            const isOpen = btn.getAttribute('aria-expanded') === 'true';
            btn.setAttribute('aria-expanded', String(!isOpen));
            panel.classList.toggle('is-open', !isOpen);
            label.textContent = !isOpen ? '收起提示词' : '查看提示词';
        });
    });
}

function bindCopy() {
    document.querySelectorAll('.btn-copy').forEach(btn => {
        btn.addEventListener('click', async () => {
            const targetId = btn.dataset.target;
            const pre = document.getElementById(targetId);
            if (!pre) return;

            try {
                await navigator.clipboard.writeText(pre.textContent.trim());
                btn.textContent = '已复制 ✓';
                btn.classList.add('copied');
                setTimeout(() => {
                    btn.textContent = '复制';
                    btn.classList.remove('copied');
                }, 2000);
            } catch {
                // Fallback: select text
                const sel = window.getSelection();
                const range = document.createRange();
                range.selectNodeContents(pre);
                sel.removeAllRanges();
                sel.addRange(range);
            }
        });
    });
}

// ── Init ─────────────────────────────────────────────────────────────────────

mountToolList();
bindToggle();
bindCopy();
