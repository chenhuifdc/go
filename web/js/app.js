// app.js - 主应用初始化

document.addEventListener('DOMContentLoaded', () => {
    // 初始化主标签导航
    initTabs();
    
    // 初始化刷新按钮
    document.getElementById('refreshBtn').addEventListener('click', () => {
        refreshCurrentPanel();
        showToast('数据已刷新', 'success');
    });

    // 初始化各页面
    initDashboard();
    initAccounts();
    initCRM();
    initGroups();
    initBroadcast();
    initRisk();
    initSettings();

    // 健康检查
    setInterval(async () => {
        try {
            const status = await api.healthCheck();
            const dot = document.querySelector('.status-indicator .dot');
            if (dot && status && status.status === 'ok') {
                dot.classList.add('online');
            } else {
                dot.classList.remove('online');
            }
        } catch (e) {
            console.warn('健康检查失败:', e);
        }
    }, 15000);

    // 加载初始数据
    loadDashboardData();
    loadAccounts();
    loadContacts();
    loadGroups();
    loadRiskEvents();
    loadTranslationSettings();

    // 显示欢迎提示
    showToast('欢迎使用 WhatsApp Ops 运营平台', 'info');
});

function initTabs() {
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            const target = tab.getAttribute('data-tab');
            document.querySelectorAll('.tab').forEach(t => {
                t.classList.remove('active');
                t.setAttribute('aria-selected', 'false');
            });
            document.querySelectorAll('.tab-panel').forEach(panel => panel.classList.remove('active'));
            tab.classList.add('active');
            tab.setAttribute('aria-selected', 'true');
            document.getElementById('panel-' + target).classList.add('active');
            
            // 刷新对应面板数据
            refreshPanel(target);
        });
    });
}

function refreshPanel(panelName) {
    switch (panelName) {
        case 'dashboard': loadDashboardData(); break;
        case 'accounts': loadAccounts(); break;
        case 'crm': loadContacts(); break;
        case 'groups': loadGroups(); break;
        case 'broadcast': loadBroadcastTasks(); break;
        case 'risk': loadRiskEvents(); break;
        case 'settings': loadTranslationSettings(); break;
    }
}

function refreshCurrentPanel() {
    const activeTab = document.querySelector('.tab.active');
    if (activeTab) {
        refreshPanel(activeTab.getAttribute('data-tab'));
    }
}

function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.style.animation = 'slideIn 0.3s ease-out reverse';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

// ====== 数据加载辅助 ======

function renderTable(tableId, data, columns) {
    const tbody = document.querySelector(`#${tableId} tbody`);
    if (!tbody) return;
    tbody.innerHTML = '';
    if (!data || data.length === 0) {
        tbody.innerHTML = '<tr><td colspan="100" style="text-align:center;color:#999;padding:20px;">暂无数据</td></tr>';
        return;
    }
    
    data.forEach(item => {
        const tr = document.createElement('tr');
        columns.forEach(col => {
            const td = document.createElement('td');
            td.textContent = (item[col.key] !== undefined && item[col.key] !== null) ? item[col.key] : '-';
            tr.appendChild(td);
        });
        tbody.appendChild(tr);
    });
}

function formatDate(ts) {
    if (!ts) return '-';
    const date = new Date(typeof ts === 'number' ? ts * 1000 : ts);
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function renderStatus(status) {
    const colors = {
        'connected': '#4ade80',
        'connecting': '#f59e0b',
        'disconnected': '#ef4444',
        'qr_ready': '#0ea5e9',
        'error': '#ef4444',
        'banned': '#ef4444',
    };
    return `<span style="color:${colors[status] || '#999'}">●</span> ${status}`;
}

// 简单数据占位（各页面 JS 会覆盖具体实现）
function loadDashboardData() {
    // 占位：各页面的 JS 会替换具体实现
    document.getElementById('statsGrid').innerHTML = `
        <div class="stat-card"><h3>在线账号</h3><span class="stat-value">0</span></div>
        <div class="stat-card"><h3>今日消息</h3><span class="stat-value">0</span></div>
        <div class="stat-card"><h3>监控群组</h3><span class="stat-value">0</span></div>
        <div class="stat-card"><h3>风控拦截</h3><span class="stat-value">0</span></div>
    `;
}

function loadAccounts() {
    api.getAccounts().then(data => {
        const tbody = document.querySelector('#accountsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        if (!Array.isArray(data) || data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;padding:20px;">暂无账号，请添加</td></tr>';
            return;
        }
        data.forEach(acc => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${acc.name || '-'}</td>
                <td>${acc.phone || '-'}</td>
                <td>${acc.proxy || '直连'}</td>
                <td>${acc.status}</td>
                <td>${acc.last_seen ? new Date(acc.last_seen * 1000).toLocaleString('zh-CN') : '-'}</td>
                <td>${acc.has_qr ? '<span style="color:#2563eb;">● 已生成</span>' : '-'}</td>
                <td>
                    <button onclick="startAccount('${acc.id}')" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;">启动</button>
                    <button onclick="stopAccount('${acc.id}')" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;">停止</button>
                    <button onclick="deleteAccount('${acc.id}')" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;background:#ef4444;">删除</button>
                </td>
            `;
            tbody.appendChild(tr);
        });
    }).catch(err => {
        console.error('加载账号失败:', err);
        showToast('加载账号失败', 'error');
    });
}

function loadContacts() {
    api.getContacts().then(data => {
        const tbody = document.querySelector('#contactsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        if (!Array.isArray(data) || data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;padding:20px;">暂无客户数据</td></tr>';
            return;
        }
        data.forEach(contact => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${contact.name || '-'}</td>
                <td>${contact.phone || '-'}</td>
                <td>${contact.tags ? '<span style="font-size:11px;background:#e0e7ff;padding:2px 6px;border-radius:10px;color:#3730a3;">标签</span>' : '-'}</td>
                <td>${contact.source || '-'}</td>
                <td>-</td>
                <td>-</td>
                <td>
                    <button onclick="editContact('${contact.jid}')" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;">编辑</button>
                </td>
            `;
            tbody.appendChild(tr);
        });
    }).catch(err => {
        console.error('加载客户失败:', err);
        showToast('加载客户失败', 'error');
    });
}

function loadGroups() {
    api.getGroups().then(data => {
        const tbody = document.querySelector('#groupsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        if (!Array.isArray(data) || data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#999;padding:20px;">暂无群组数据</td></tr>';
            return;
        }
        data.forEach(group => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${group.name || '-'}</td>
                <td>${group.owner_jid || '-'}</td>
                <td>${group.participants_count || 0}</td>
                <td>${group.is_announce_only ? '<span style="color:#f59e0b;">仅管理员发言</span>' : '<span style="color:#10b981;">开放</span>'}</td>
                <td>${group.created_at ? new Date(group.created_at * 1000).toLocaleString('zh-CN') : '-'}</td>
                <td>
                    <button onclick="toggleGroupPrivacy('${group.jid}', ${group.is_announce_only ? 0 : 1})" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;">${group.is_announce_only ? '打开群聊' : '关闭群聊'}</button>
                    <button onclick="exportGroupMembers('${group.jid}')" class="btn btn-secondary" style="padding:4px 8px;font-size:12px;">导出成员</button>
                </td>
            `;
            tbody.appendChild(tr);
        });
    }).catch(err => {
        console.error('加载群组失败:', err);
        showToast('加载群组失败', 'error');
    });
}

function loadRiskEvents() {
    api.getRiskEvents().then(data => {
        const tbody = document.querySelector('#riskEventsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        if (!Array.isArray(data) || data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;color:#999;padding:20px;">暂无风控事件</td></tr>';
            return;
        }
        data.slice(0, 50).forEach(event => {
            const tr = document.createElement('tr');
            const levelColors = { 1: '#ef4444', 2: '#f97316', 3: '#f59e0b' };
            tr.innerHTML = `
                <td>${event.created_at ? new Date(event.created_at * 1000).toLocaleString('zh-CN') : '-'}</td>
                <td>${event.account_id || '-'}</td>
                <td>${event.group_jid ? event.group_jid.split('@')[0] : '-'}</td>
                <td>${event.user_jid ? event.user_jid.split('@')[0] : '-'}</td>
                <td><span style="font-weight:bold;color:${levelColors[event.level] || '#999'}">L${event.level}</span></td>
                <td>${event.rule_name || '-'}</td>
                <td><span style="font-size:11px;color:#666;max-width:200px;display:inline-block;overflow:hidden;text-overflow:ellipsis;">${event.matched_content || '-'}</span></td>
                <td>${event.action || '-'}</td>
                <td>${event.action_status || '-'}</td>
            `;
            tbody.appendChild(tr);
        });
    }).catch(err => {
        console.error('加载风控事件失败:', err);
        showToast('加载风控事件失败', 'error');
    });
}

function loadTranslationSettings() {
    api.getTranslationSettings().then(data => {
        document.getElementById('transEnabled').checked = data.enabled || false;
        document.getElementById('transProvider').value = data.provider || 'openai_compatible';
        document.getElementById('transBaseUrl').value = data.base_url || '';
        document.getElementById('transApiKey').value = data.api_key ? '********' : '';
        document.getElementById('transModel').value = data.model || 'gpt-4o-mini';
        document.getElementById('transTargetLang').value = data.target_lang || 'zh-CN';
        document.getElementById('transPrompt').value = data.prompt_template || '';
    }).catch(err => {
        console.error('加载翻译设置失败:', err);
    });
}

function loadBroadcastTasks() {
    // 占位
    document.querySelector('#broadcastTable tbody').innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;padding:20px;">功能开发中</td></tr>';
}

// ====== 交互函数（全局可访问） ======
window.startAccount = async (id) => {
    try {
        await api.startAccount(id);
        showToast('账号启动中，请扫码登录', 'info');
        setTimeout(() => loadAccounts(), 1000);
    } catch (e) {
        showToast('启动失败: ' + e.message, 'error');
    }
};

window.stopAccount = async (id) => {
    try {
        await api.stopAccount(id);
        showToast('账号已停止', 'success');
        loadAccounts();
    } catch (e) {
        showToast('停止失败: ' + e.message, 'error');
    }
};

window.deleteAccount = async (id) => {
    if (!confirm('确定要删除此账号吗？')) return;
    try {
        await api.deleteAccount(id);
        showToast('账号已删除', 'success');
        loadAccounts();
    } catch (e) {
        showToast('删除失败: ' + e.message, 'error');
    }
};

window.toggleGroupPrivacy = async (jid, isAnnounceOnly) => {
    try {
        // 这里简化处理，实际需要调用对应 API
        showToast(isAnnounceOnly ? '群聊已打开' : '群聊已关闭（仅管理员发言）', 'success');
        loadGroups();
    } catch (e) {
        showToast('操作失败: ' + e.message, 'error');
    }
};

window.exportGroupMembers = async (jid) => {
    showToast('成员导出功能开发中...', 'info');
};

window.editContact = async (jid) => {
    showToast('编辑功能开发中...', 'info');
};