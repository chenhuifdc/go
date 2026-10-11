// risk.js - 风控中心页面
const RiskPage = {
    events: [],
    rules: [],
    currentEventPage: 1,
    eventPageSize: 20,
    eventTotal: 0,
    eventSearchQuery: '',
    eventLevelFilter: '',

    async init() {
        console.log('[RiskPage] 初始化风控中心页面');
        await Promise.all([this.loadRules(), this.loadEvents()]);
        this.bindEvents();
    },

    async loadRules() {
        try {
            this.rules = await api.getRiskRules();
            this.renderRules();
        } catch (e) {
            console.error('加载风控规则失败:', e);
            app.showToast('加载风控规则失败: ' + e.message, 'error');
        }
    },

    async loadEvents() {
        try {
            const params = new URLSearchParams({
                page: this.currentEventPage,
                page_size: this.eventPageSize,
            });
            if (this.eventSearchQuery) params.append('search', this.eventSearchQuery);
            if (this.eventLevelFilter) params.append('level', this.eventLevelFilter);

            const res = await api.getRiskEvents(params.toString());
            this.events = res.events || [];
            this.eventTotal = res.total || 0;
            this.renderEvents();
            this.renderEventPagination();
        } catch (e) {
            console.error('加载风控事件失败:', e);
            app.showToast('加载风控事件失败: ' + e.message, 'error');
        }
    },

    renderRules() {
        const tbody = document.querySelector('#riskRulesTable tbody');
        if (!tbody) return;
        tbody.innerHTML = this.rules.map(r => `
            <tr>
                <td>${r.id}</td>
                <td><span class="badge badge-${this.getLevelBadgeClass(r.level)}">${this.getLevelLabel(r.level)}</span></td>
                <td>${r.pattern}</td>
                <td>${r.action}</td>
                <td>${r.enabled ? '<span class="badge badge-success">启用</span>' : '<span class="badge badge-secondary">禁用</span>'}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="RiskPage.editRule('${r.id}')">编辑</button>
                    <button class="btn btn-danger btn-sm" onclick="RiskPage.deleteRule('${r.id}')">删除</button>
                </td>
            </tr>
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

    renderEvents() {
        const tbody = document.querySelector('#riskEventsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = this.events.map(e => `
            <tr>
                <td>${new Date(e.timestamp * 1000).toLocaleString()}</td>
                <td><span class="badge badge-${this.getLevelBadgeClass(e.level)}">${this.getLevelLabel(e.level)}</span></td>
                <td>${e.account_id || '-'}</td>
                <td>${e.jid}</td>
                <td>${e.matched_rule || '-'}</td>
                <td>${e.action_taken || '-'}</td>
                <td>${e.message_content ? e.message_content.substring(0, 50) + '...' : '-'}</td>
            </tr>
        `).join('');
    },

    renderEventPagination() {
        const container = document.getElementById('riskEventsPagination');
        if (!container) return;
        const totalPages = Math.ceil(this.eventTotal / this.eventPageSize);
        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }
        let html = '';
        for (let i = 1; i <= totalPages; i++) {
            if (i === 1 || i === totalPages || (i >= this.currentEventPage - 1 && i <= this.currentEventPage + 1)) {
                html += `<button class="btn ${i === this.currentEventPage ? 'btn-primary' : 'btn-secondary'}" onclick="RiskPage.goEventPage(${i})">${i}</button>`;
            } else if (i === this.currentEventPage - 2 || i === this.currentEventPage + 2) {
                html += '<span>...</span>';
            }
        }
        container.innerHTML = html;
    },

    goEventPage(page) {
        this.currentEventPage = page;
        this.loadEvents();
    },

    bindEvents() {
        // 事件搜索
        const searchInput = document.getElementById('riskEventSearch');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.eventSearchQuery = e.target.value;
                    this.currentEventPage = 1;
                    this.loadEvents();
                }, 300);
            });
        }

        // 事件等级筛选
        const levelFilter = document.getElementById('riskEventLevelFilter');
        if (levelFilter) {
            levelFilter.addEventListener('change', (e) => {
                this.eventLevelFilter = e.target.value;
                this.currentEventPage = 1;
                this.loadEvents();
            });
        }

        // 规则模态框
        const addRuleBtn = document.getElementById('addRiskRuleBtn');
        if (addRuleBtn) {
            addRuleBtn.addEventListener('click', () => this.openRuleModal());
        }

        const modalClose = document.getElementById('riskRuleModalClose');
        if (modalClose) {
            modalClose.addEventListener('click', () => this.closeRuleModal());
        }

        const modalOverlay = document.getElementById('modalOverlay');
        if (modalOverlay) {
            modalOverlay.addEventListener('click', () => this.closeRuleModal());
        }

        const form = document.getElementById('riskRuleForm');
        if (form) {
            form.addEventListener('submit', (e) => this.saveRule(e));
        }

        // 重新加载词库按钮
        const reloadBtn = document.getElementById('reloadRiskWordsBtn');
        if (reloadBtn) {
            reloadBtn.addEventListener('click', () => this.reloadWords());
        }

        // 导入/导出词库
        const importBtn = document.getElementById('importRiskWordsBtn');
        if (importBtn) {
            importBtn.addEventListener('click', () => this.importWords());
        }

        const exportBtn = document.getElementById('exportRiskWordsBtn');
        if (exportBtn) {
            exportBtn.addEventListener('click', () => this.exportWords());
        }
    },

    openRuleModal(rule = null) {
        const modal = document.getElementById('riskRuleModal');
        const form = document.getElementById('riskRuleForm');
        const title = document.getElementById('riskRuleModalTitle');

        if (rule) {
            title.textContent = '编辑风控规则';
            form.id.value = rule.id;
            form.level.value = rule.level;
            form.pattern.value = rule.pattern;
            form.action.value = rule.action;
            form.enabled.checked = rule.enabled;
        } else {
            title.textContent = '添加风控规则';
            form.reset();
            form.id.value = '';
        }

        modal.classList.add('active');
        document.getElementById('modalOverlay').classList.add('active');
    },

    closeRuleModal() {
        document.getElementById('riskRuleModal').classList.remove('active');
        document.getElementById('modalOverlay').classList.remove('active');
    },

    editRule(id) {
        const rule = this.rules.find(r => r.id === id);
        if (rule) this.openRuleModal(rule);
    },

    async deleteRule(id) {
        if (!confirm('确定要删除这个风控规则吗？')) return;
        try {
            await api.deleteRiskRule(id);
            app.showToast('删除成功', 'success');
            this.loadRules();
        } catch (e) {
            app.showToast('删除失败: ' + e.message, 'error');
        }
    },

    async saveRule(e) {
        e.preventDefault();
        const form = e.target;
        const data = {
            level: form.level.value,
            pattern: form.pattern.value,
            action: form.action.value,
            enabled: form.enabled.checked,
        };

        if (!data.pattern) {
            app.showToast('请输入匹配模式', 'warning');
            return;
        }

        try {
            const isEdit = !!form.id.value;
            if (isEdit) {
                await api.updateRiskRule(form.id.value, data);
            } else {
                await api.createRiskRule(data);
            }
            this.closeRuleModal();
            app.showToast(isEdit ? '更新成功' : '创建成功', 'success');
            this.loadRules();
        } catch (ex) {
            app.showToast('保存失败: ' + ex.message, 'error');
        }
    },

    async reloadWords() {
        try {
            await api.reloadRiskWords();
            app.showToast('词库已重新加载', 'success');
        } catch (e) {
            app.showToast('重新加载失败: ' + e.message, 'error');
        }
    },

    importWords() {
        const input = document.createElement('input');
        input.type = 'file';
        input.accept = '.txt,.csv';
        input.onchange = async (e) => {
            const file = e.target.files[0];
            if (!file) return;
            const text = await file.text();
            const words = text.split('\n').map(l => l.trim()).filter(l => l);
            try {
                // 这里需要后端支持导入 API，暂时提示
                app.showToast('导入功能待后端实现', 'warning');
            } catch (ex) {
                app.showToast('导入失败: ' + ex.message, 'error');
            }
        };
        input.click();
    },

    exportWords() {
        try {
            // 从后端获取词库并导出
            app.showToast('导出功能待后端实现', 'warning');
        } catch (e) {
            app.showToast('导出失败: ' + e.message, 'error');
        }
    }
};

document.addEventListener('DOMContentLoaded', () => RiskPage.init());
window.RiskPage = RiskPage;