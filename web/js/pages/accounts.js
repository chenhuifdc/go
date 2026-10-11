// 账号管理页面
let accountsCache = [];

async function loadAccounts() {
    try {
        const data = await api.getAccounts();
        accountsCache = data;
        renderAccounts(data);
    } catch (e) {
        console.error('加载账号失败:', e);
        showToast('加载账号失败: ' + e.message, 'error');
    }
}

function renderAccounts(accounts) {
    const container = document.getElementById('accountsGrid');
    if (!container) return;

    if (accounts.length === 0) {
        container.innerHTML = `
            <div class="empty-state">
                <i class="icon">📱</i>
                <p>暂无账号，点击右上角添加</p>
            </div>
        `;
        return;
    }

    container.innerHTML = accounts.map(acc => `
        <div class="account-card" data-id="${acc.id}">
            <div class="account-header">
                <div>
                    <h4>${escapeHtml(acc.name || '未命名')}</h4>
                    <span class="account-id">${escapeHtml(acc.id)}</span>
                </div>
                <span class="badge ${getStatusClass(acc.status)}">${escapeHtml(acc.status)}</span>
            </div>
            <div class="account-info">
                ${acc.jid ? `<div class="info-row"><span class="label">JID:</span><span>${escapeHtml(acc.jid)}</span></div>` : ''}
                ${acc.proxy ? `<div class="info-row"><span class="label">代理:</span><span>${escapeHtml(acc.proxy)}</span></div>` : ''}
                <div class="info-row"><span class="label">创建时间:</span><span>${formatTime(acc.created_at)}</span></div>
                <div class="info-row"><span class="label">最后在线:</span><span>${acc.last_seen ? formatTime(acc.last_seen) : '从未'}</span></div>
            </div>
            <div class="account-qr" id="qr-${acc.id}" style="display: none;">
                <img id="qr-img-${acc.id}" alt="QR Code">
                <p class="qr-tip">请使用 WhatsApp 扫码登录，二维码每 30 秒自动刷新</p>
            </div>
            <div class="account-actions">
                <button class="btn btn-secondary btn-sm" onclick="toggleQR('${acc.id}')" id="qr-btn-${acc.id}">
                    ${accountsCache.find(a => a.id === acc.id)?.qr_code ? '隐藏二维码' : '显示二维码'}
                </button>
                <button class="btn btn-primary btn-sm" onclick="connectAccount('${acc.id}')" id="connect-btn-${acc.id}" ${acc.status === 'connected' ? 'disabled' : ''}>连接</button>
                <button class="btn btn-warning btn-sm" onclick="disconnectAccount('${acc.id}')" id="disconnect-btn-${acc.id}" ${acc.status !== 'connected' ? 'disabled' : ''}>断开</button>
                <button class="btn btn-secondary btn-sm" onclick="refreshQR('${acc.id}')" id="refresh-btn-${acc.id}">刷新二维码</button>
                <button class="btn btn-danger btn-sm" onclick="deleteAccount('${acc.id}')">删除</button>
            </div>
        </div>
    `).join('');
}

function getStatusClass(status) {
    switch (status) {
        case 'connected': return 'badge-success';
        case 'connecting': return 'badge-warning';
        case 'disconnected': return 'badge-secondary';
        default: return 'badge-secondary';
    }
}

function toggleQR(accountId) {
    const qrDiv = document.getElementById(`qr-${accountId}`);
    const btn = document.getElementById(`qr-btn-${accountId}`);
    if (qrDiv.style.display === 'none') {
        qrDiv.style.display = 'block';
        btn.textContent = '隐藏二维码';
        startQRPolling(accountId);
    } else {
        qrDiv.style.display = 'none';
        btn.textContent = '显示二维码';
        stopQRPolling(accountId);
    }
}

let qrPollingIntervals = {};

function startQRPolling(accountId) {
    if (qrPollingIntervals[accountId]) return;
    // 立即获取一次
    fetchQR(accountId);
    // 每 2 秒轮询
    qrPollingIntervals[accountId] = setInterval(() => fetchQR(accountId), 2000);
}

function stopQRPolling(accountId) {
    if (qrPollingIntervals[accountId]) {
        clearInterval(qrPollingIntervals[accountId]);
        delete qrPollingIntervals[accountId];
    }
}

async function fetchQR(accountId) {
    try {
        const data = await api.getAccount(accountId);
        if (data.qr_code) {
            const img = document.getElementById(`qr-img-${accountId}`);
            if (img) {
                img.src = 'data:image/png;base64,' + data.qr_code;
            }
        }
    } catch (e) {
        console.error('获取二维码失败:', e);
    }
}

async function connectAccount(accountId) {
    try {
        await api.connectAccount(accountId);
        showToast('连接请求已发送', 'success');
        loadAccounts();
    } catch (e) {
        showToast('连接失败: ' + e.message, 'error');
    }
}

async function disconnectAccount(accountId) {
    if (!confirm('确定要断开该账号连接吗？')) return;
    try {
        await api.disconnectAccount(accountId);
        showToast('已断开连接', 'success');
        loadAccounts();
    } catch (e) {
        showToast('断开失败: ' + e.message, 'error');
    }
}

async function refreshQR(accountId) {
    try {
        await api.refreshQR(accountId);
        showToast('二维码刷新请求已发送', 'success');
        // 立即获取新二维码
        setTimeout(() => fetchQR(accountId), 500);
    } catch (e) {
        showToast('刷新失败: ' + e.message, 'error');
    }
}

async function deleteAccount(accountId) {
    if (!confirm('确定要删除该账号吗？此操作不可恢复。')) return;
    try {
        await api.deleteAccount(accountId);
        showToast('账号已删除', 'success');
        loadAccounts();
    } catch (e) {
        showToast('删除失败: ' + e.message, 'error');
    }
}

function showAddAccountModal() {
    const modal = document.getElementById('accountModal');
    const form = document.getElementById('accountForm');
    form.reset();
    document.getElementById('accountModalTitle').textContent = '添加账号';
    document.getElementById('accountId').value = '';
    modal.classList.add('show');
}

async function editAccount(accountId) {
    const acc = accountsCache.find(a => a.id === accountId);
    if (!acc) return;

    const modal = document.getElementById('accountModal');
    const form = document.getElementById('accountForm');
    document.getElementById('accountModalTitle').textContent = '编辑账号';
    document.getElementById('accountId').value = acc.id;
    document.getElementById('accountName').value = acc.name || '';
    document.getElementById('accountProxy').value = acc.proxy || '';
    modal.classList.add('show');
}

async function saveAccount(e) {
    e.preventDefault();
    const form = e.target;
    const id = document.getElementById('accountId').value;
    const name = document.getElementById('accountName').value;
    const proxy = document.getElementById('accountProxy').value;

    try {
        if (id) {
            await api.updateAccount(id, { name, proxy });
            showToast('账号已更新', 'success');
        } else {
            await api.createAccount({ name, proxy });
            showToast('账号已创建', 'success');
        }
        closeModal('accountModal');
        loadAccounts();
    } catch (err) {
        showToast('保存失败: ' + err.message, 'error');
    }
}

// 页面初始化
function initAccounts() {
    loadAccounts();

    // 表单提交
    document.getElementById('accountForm')?.addEventListener('submit', saveAccount);

    // 点击模态框外部关闭
    document.getElementById('accountModal')?.addEventListener('click', (e) => {
        if (e.target.id === 'accountModal') closeModal('accountModal');
    });
}

// 导出到全局
window.initAccounts = initAccounts;
window.loadAccounts = loadAccounts;
window.showAddAccountModal = showAddAccountModal;
window.editAccount = editAccount;
window.connectAccount = connectAccount;
window.disconnectAccount = disconnectAccount;
window.refreshQR = refreshQR;
window.deleteAccount = deleteAccount;
window.toggleQR = toggleQR;