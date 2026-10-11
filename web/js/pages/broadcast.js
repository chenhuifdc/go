// broadcast.js - 群发任务页面
const BroadcastPage = {
    tasks: [],
    currentPage: 1,
    pageSize: 20,
    total: 0,
    searchQuery: '',

    async init() {
        console.log('[BroadcastPage] 初始化群发任务页面');
        await this.loadTasks();
        this.bindEvents();
    },

    async loadTasks() {
        try {
            const params = new URLSearchParams({
                page: this.currentPage,
                page_size: this.pageSize,
            });
            if (this.searchQuery) params.append('search', this.searchQuery);

            const res = await api.getBroadcastTasks(params.toString());
            this.tasks = res.tasks || [];
            this.total = res.total || 0;
            this.renderTasks();
            this.renderPagination();
        } catch (e) {
            console.error('加载群发任务失败:', e);
            app.showToast('加载群发任务失败: ' + e.message, 'error');
        }
    },

    renderTasks() {
        const tbody = document.querySelector('#broadcastTasksTable tbody');
        if (!tbody) return;
        tbody.innerHTML = this.tasks.map(t => `
            <tr>
                <td>${t.id}</td>
                <td>${t.name || '-'}</td>
                <td>${t.account_id || '-'}</td>
                <td>${t.status}</td>
                <td>${t.total_targets || 0}</td>
                <td>${t.sent_count || 0}</td>
                <td>${t.failed_count || 0}</td>
                <td>${new Date(t.created_at * 1000).toLocaleString()}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="BroadcastPage.viewProgress('${t.id}')" ${t.status !== 'running' && t.status !== 'pending' ? 'disabled' : ''}>进度</button>
                    <button class="btn btn-secondary btn-sm" onclick="BroadcastPage.cancelTask('${t.id}')" ${t.status !== 'running' && t.status !== 'pending' ? 'disabled' : ''}>取消</button>
                </td>
            </tr>
        `).join('');
    },

    renderPagination() {
        const container = document.getElementById('broadcastPagination');
        if (!container) return;
        const totalPages = Math.ceil(this.total / this.pageSize);
        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }
        let html = '';
        for (let i = 1; i <= totalPages; i++) {
            if (i === 1 || i === totalPages || (i >= this.currentPage - 1 && i <= this.currentPage + 1)) {
                html += `<button class="btn ${i === this.currentPage ? 'btn-primary' : 'btn-secondary'}" onclick="BroadcastPage.goPage(${i})">${i}</button>`;
            } else if (i === this.currentPage - 2 || i === this.currentPage + 2) {
                html += '<span>...</span>';
            }
        }
        container.innerHTML = html;
    },

    goPage(page) {
        this.currentPage = page;
        this.loadTasks();
    },

    bindEvents() {
        const searchInput = document.getElementById('broadcastSearch');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.searchQuery = e.target.value;
                    this.currentPage = 1;
                    this.loadTasks();
                }, 300);
            });
        }

        const createBtn = document.getElementById('createBroadcastBtn');
        if (createBtn) {
            createBtn.addEventListener('click', () => this.openModal());
        }

        const modalClose = document.getElementById('broadcastModalClose');
        if (modalClose) {
            modalClose.addEventListener('click', () => this.closeModal());
        }

        const modalOverlay = document.getElementById('modalOverlay');
        if (modalOverlay) {
            modalOverlay.addEventListener('click', () => this.closeModal());
        }

        const form = document.getElementById('broadcastForm');
        if (form) {
            form.addEventListener('submit', (e) => this.saveTask(e));
        }

        // 加载账号列表到下拉框
        this.loadAccountsForSelect();
    },

    async loadAccountsForSelect() {
        try {
            const res = await api.getAccounts();
            const accounts = res.accounts || [];
            const select = document.getElementById('broadcastAccount');
            if (select) {
                select.innerHTML = accounts.filter(a => a.status === 'connected').map(a =>
                    `<option value="${a.id}">${a.name} (${a.phone || '未登录'})</option>`
                ).join('');
            }
        } catch (e) {
            console.error('加载账号列表失败:', e);
        }
    },

    openModal() {
        const modal = document.getElementById('broadcastModal');
        const form = document.getElementById('broadcastForm');
        form.reset();
        document.getElementById('broadcastModalTitle').textContent = '创建群发任务';
        modal.classList.add('active');
        document.getElementById('modalOverlay').classList.add('active');
        this.loadAccountsForSelect();
    },

    closeModal() {
        document.getElementById('broadcastModal').classList.remove('active');
        document.getElementById('modalOverlay').classList.remove('active');
    },

    async saveTask(e) {
        e.preventDefault();
        const form = e.target;
        const data = {
            name: form.name.value,
            account_id: form.account_id.value,
            content: form.content.value,
            targets: form.targets.value.split('\n').map(s => s.trim()).filter(s => s),
            interval_min: parseInt(form.interval_min.value) || 1,
            interval_max: parseInt(form.interval_max.value) || 3,
            mention_all: form.mention_all.checked,
        };

        if (data.targets.length === 0) {
            app.showToast('请至少输入一个目标', 'warning');
            return;
        }

        try {
            await api.createBroadcastTask(data);
            this.closeModal();
            app.showToast('创建群发任务成功', 'success');
            this.loadTasks();
        } catch (ex) {
            app.showToast('创建失败: ' + ex.message, 'error');
        }
    },

    async viewProgress(id) {
        try {
            const res = await api.getBroadcastProgress(id);
            this.showProgressModal(id, res);
        } catch (e) {
            app.showToast('加载进度失败: ' + e.message, 'error');
        }
    },

    showProgressModal(id, progress) {
        let modal = document.getElementById('progressModal');
        if (!modal) {
            modal = document.createElement('div');
            modal.id = 'progressModal';
            modal.className = 'modal';
            modal.style.maxWidth = '600px';
            modal.innerHTML = `
                <div class="modal-header">
                    <h3>群发进度: <span id="progressTaskId"></span></h3>
                    <button class="modal-close" id="progressModalClose">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="progress-bar" style="height: 10px; background: #e5e7eb; border-radius: 5px; overflow: hidden; margin-bottom: 15px;">
                        <div id="progressBarFill" style="height: 100%; background: #2563eb; width: 0%; transition: width 0.3s;"></div>
                    </div>
                    <div style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-bottom: 15px;">
                        <div><strong>总计:</strong> <span id="progressTotal">0</span></div>
                        <div><strong>已发送:</strong> <span id="progressSent">0</span></div>
                        <div><strong>失败:</strong> <span id="progressFailed">0</span></div>
                    </div>
                    <div class="table-container" style="max-height: 300px;">
                        <table class="table">
                            <thead><tr><th>目标</th><th>状态</th><th>错误信息</th></tr></thead>
                            <tbody id="progressDetails"></tbody>
                        </table>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
            document.getElementById('progressModalClose').addEventListener('click', () => {
                modal.classList.remove('active');
                document.getElementById('modalOverlay').classList.remove('active');
            });
        }
        document.getElementById('progressTaskId').textContent = id;
        const pct = progress.total_targets > 0 ? (progress.sent_count / progress.total_targets * 100).toFixed(1) : 0;
        document.getElementById('progressBarFill').style.width = pct + '%';
        document.getElementById('progressTotal').textContent = progress.total_targets;
        document.getElementById('progressSent').textContent = progress.sent_count;
        document.getElementById('progressFailed').textContent = progress.failed_count;
        document.getElementById('progressDetails').innerHTML = (progress.details || []).map(d => `
            <tr>
                <td>${d.target}</td>
                <td><span class="badge badge-${d.status === 'sent' ? 'success' : 'danger'}">${d.status}</span></td>
                <td>${d.error || '-'}</td>
            </tr>
        `).join('');
        modal.classList.add('active');
        document.getElementById('modalOverlay').classList.add('active');
    },

    async cancelTask(id) {
        if (!confirm('确定要取消这个群发任务吗？')) return;
        try {
            await api.cancelBroadcastTask(id);
            app.showToast('任务已取消', 'success');
            this.loadTasks();
        } catch (e) {
            app.showToast('取消失败: ' + e.message, 'error');
        }
    }
};

document.addEventListener('DOMContentLoaded', () => BroadcastPage.init());
window.BroadcastPage = BroadcastPage;