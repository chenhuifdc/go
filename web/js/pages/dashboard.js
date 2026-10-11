// dashboard.js - 仪表盘页面
const DashboardPage = {
    stats: {},
    recentEvents: [],
    accountStatus: {},

    async init() {
        console.log('[DashboardPage] 初始化仪表盘页面');
        await this.loadData();
        this.bindEvents();
        // 启动定时刷新
        this.refreshInterval = setInterval(() => this.loadData(), 30000);
    },

    async loadData() {
        try {
            const [statsRes, eventsRes, accountsRes] = await Promise.all([
                api.getStatsOverview(),
                api.getRiskEvents('page=1&page_size=10'),
                api.getAccounts(),
            ]);

            this.stats = statsRes || {};
            this.recentEvents = (eventsRes.events || []).slice(0, 10);
            this.accountStatus = {};
            (accountsRes.accounts || []).forEach(a => {
                this.accountStatus[a.id] = a.status;
            });

            this.renderStats();
            this.renderRecentEvents();
            this.renderAccountStatus();
        } catch (e) {
            console.error('加载仪表盘数据失败:', e);
        }
    },

    renderStats() {
        const container = document.getElementById('statsGrid');
        if (!container) return;
        container.innerHTML = `
            <div class="stat-card">
                <h3>在线账号</h3>
                <span class="stat-value" style="color: #22c55e;">${this.stats.online_accounts || 0}</span>
                <span class="stat-label">共 ${this.stats.total_accounts || 0} 个账号</span>
            </div>
            <div class="stat-card">
                <h3>今日消息</h3>
                <span class="stat-value">${this.stats.today_messages || 0}</span>
                <span class="stat-label">总计 ${this.stats.total_messages || 0} 条</span>
            </div>
            <div class="stat-card">
                <h3>监控群组</h3>
                <span class="stat-value">${this.stats.monitored_groups || 0}</span>
                <span class="stat-label">总成员 ${this.stats.total_group_members || 0} 人</span>
            </div>
            <div class="stat-card">
                <h3>风控拦截</h3>
                <span class="stat-value" style="color: #ef4444;">${this.stats.risk_blocked_today || 0}</span>
                <span class="stat-label">总计 ${this.stats.total_risk_events || 0} 次</span>
            </div>
        `;
    },

    renderRecentEvents() {
        const container = document.getElementById('recentEvents');
        if (!container) return;
        if (this.recentEvents.length === 0) {
            container.innerHTML = '<div class="empty-state">暂无风控事件</div>';
            return;
        }
        container.innerHTML = this.recentEvents.map(e => `
            <div class="event-item">
                <div class="event-time">${new Date(e.timestamp * 1000).toLocaleTimeString()}</div>
                <div class="event-content">
                    <span class="badge badge-${this.getLevelBadgeClass(e.level)}">${this.getLevelLabel(e.level)}</span>
                    <span class="event-account">${e.account_id || 'N/A'}</span>
                    <span class="event-jid">${e.jid}</span>
                    <span class="event-action">${e.action_taken || '记录'}</span>
                </div>
                <div class="event-preview">${e.message_content ? e.message_content.substring(0, 80) : '无内容'}</div>
            </div>
        `).join('');
    },

    getLevelBadgeClass(level) {
        switch (level) {
            case 'L1': return 'danger';
            case 'L2': return 'warning';
            case 'L3': return 'info';
            default: return 'secondary';
        }
    },

    getLevelLabel(level) {
        switch (level) {
            case 'L1': return 'L1 垃圾';
            case 'L2': return 'L2 敏感';
            case 'L3': return 'L3 行为';
            default: return level;
        }
    },

    renderAccountStatus() {
        const container = document.getElementById('accountStatusList');
        if (!container) return;
        const accounts = Object.entries(this.accountStatus);
        if (accounts.length === 0) {
            container.innerHTML = '<div class="empty-state">暂无账号，请前往账号管理添加</div>';
            return;
        }
        container.innerHTML = accounts.map(([id, status]) => `
            <div class="account-status-item">
                <span class="status-dot ${status === 'connected' ? 'online' : status === 'connecting' ? 'connecting' : 'offline'}"></span>
                <span class="account-id">${id}</span>
                <span class="account-status ${status}">${this.getStatusLabel(status)}</span>
            </div>
        `).join('');
    },

    getStatusLabel(status) {
        switch (status) {
            case 'connected': return '在线';
            case 'connecting': return '连接中';
            case 'disconnected': return '离线';
            case 'qr_pending': return '待扫码';
            default: return status;
        }
    },

    bindEvents() {
        // 手动刷新按钮
        const refreshBtn = document.getElementById('refreshDashboardBtn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => this.loadData());
        }
    },

    destroy() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
    }
};

document.addEventListener('DOMContentLoaded', () => DashboardPage.init());
window.DashboardPage = DashboardPage;