// groups.js - 群组管理页面
const GroupsPage = {
    groups: [],
    currentPage: 1,
    pageSize: 20,
    total: 0,
    searchQuery: '',

    async init() {
        console.log('[GroupsPage] 初始化群组管理页面');
        await this.loadGroups();
        this.bindEvents();
    },

    async loadGroups() {
        try {
            const params = new URLSearchParams({
                page: this.currentPage,
                page_size: this.pageSize,
            });
            if (this.searchQuery) params.append('search', this.searchQuery);

            const res = await api.getGroups(params.toString());
            this.groups = res.groups || [];
            this.total = res.total || 0;
            this.renderGroups();
            this.renderPagination();
        } catch (e) {
            console.error('加载群组失败:', e);
            app.showToast('加载群组失败: ' + e.message, 'error');
        }
    },

    renderGroups() {
        const tbody = document.querySelector('#groupsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = this.groups.map(g => `
            <tr>
                <td>${g.subject || '-'}</td>
                <td>${g.jid}</td>
                <td>${g.participants_count || 0}</td>
                <td>${g.is_announce ? '<span class="badge badge-info">仅公告</span>' : '<span class="badge badge-success">开放</span>'}</td>
                <td>${new Date(g.created_at * 1000).toLocaleString()}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="GroupsPage.viewMembers('${g.jid}')">成员</button>
                    <button class="btn btn-secondary btn-sm" onclick="GroupsPage.toggleAnnounce('${g.jid}', ${!g.is_announce})">
                        ${g.is_announce ? '开放聊天' : '仅公告'}
                    </button>
                    <button class="btn btn-secondary btn-sm" onclick="GroupsPage.exportMembers('${g.jid}')">导出</button>
                </td>
            </tr>
        `).join('');
    },

    renderPagination() {
        const container = document.getElementById('groupsPagination');
        if (!container) return;
        const totalPages = Math.ceil(this.total / this.pageSize);
        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }
        let html = '';
        for (let i = 1; i <= totalPages; i++) {
            if (i === 1 || i === totalPages || (i >= this.currentPage - 1 && i <= this.currentPage + 1)) {
                html += `<button class="btn ${i === this.currentPage ? 'btn-primary' : 'btn-secondary'}" onclick="GroupsPage.goPage(${i})">${i}</button>`;
            } else if (i === this.currentPage - 2 || i === this.currentPage + 2) {
                html += '<span>...</span>';
            }
        }
        container.innerHTML = html;
    },

    goPage(page) {
        this.currentPage = page;
        this.loadGroups();
    },

    bindEvents() {
        const searchInput = document.getElementById('groupSearch');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.searchQuery = e.target.value;
                    this.currentPage = 1;
                    this.loadGroups();
                }, 300);
            });
        }
    },

    async viewMembers(jid) {
        try {
            const res = await api.getGroupMembers(jid);
            const members = res.members || [];
            this.showMembersModal(jid, members);
        } catch (e) {
            app.showToast('加载成员失败: ' + e.message, 'error');
        }
    },

    showMembersModal(jid, members) {
        let modal = document.getElementById('groupMembersModal');
        if (!modal) {
            modal = document.createElement('div');
            modal.id = 'groupMembersModal';
            modal.className = 'modal';
            modal.innerHTML = `
                <div class="modal-header">
                    <h3>群组成员: <span id="modalGroupJid"></span></h3>
                    <button class="modal-close" id="membersModalClose">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="table-container">
                        <table class="table">
                            <thead>
                                <tr><th>JID</th><th>姓名</th><th>角色</th></tr>
                            </thead>
                            <tbody id="membersTableBody"></tbody>
                        </table>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
            document.getElementById('membersModalClose').addEventListener('click', () => {
                modal.classList.remove('active');
                document.getElementById('modalOverlay').classList.remove('active');
            });
        }
        document.getElementById('modalGroupJid').textContent = jid;
        document.getElementById('membersTableBody').innerHTML = members.map(m => `
            <tr>
                <td>${m.jid}</td>
                <td>${m.name || '-'}</td>
                <td>${m.is_admin ? '管理员' : (m.is_super_admin ? '超管' : '成员')}</td>
            </tr>
        `).join('');
        modal.classList.add('active');
        document.getElementById('modalOverlay').classList.add('active');
    },

    async toggleAnnounce(jid, enable) {
        try {
            await api.setGroupAnnounce(jid, enable);
            app.showToast(enable ? '已设置为仅公告模式' : '已开放群聊', 'success');
            this.loadGroups();
        } catch (e) {
            app.showToast('操作失败: ' + e.message, 'error');
        }
    },

    async exportMembers(jid) {
        try {
            const res = await api.exportGroupMembers(jid);
            const csv = 'JID,Name,Phone,IsAdmin\n' + res.members.map(m =>
                `${m.jid},"${m.name || ''}",${m.phone || ''},${m.is_admin}`
            ).join('\n');
            const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
            const link = document.createElement('a');
            link.href = URL.createObjectURL(blob);
            link.download = `group_${jid.replace('@g.us', '')}_members.csv`;
            link.click();
            app.showToast('导出成功', 'success');
        } catch (e) {
            app.showToast('导出失败: ' + e.message, 'error');
        }
    }
};

document.addEventListener('DOMContentLoaded', () => GroupsPage.init());
window.GroupsPage = GroupsPage;