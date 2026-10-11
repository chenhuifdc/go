// contacts.js - 联系人管理页面
const ContactsPage = {
    contacts: [],
    tags: [],
    currentPage: 1,
    pageSize: 20,
    total: 0,
    searchQuery: '',
    selectedTag: '',

    async init() {
        console.log('[ContactsPage] 初始化联系人管理页面');
        await this.loadTags();
        await this.loadContacts();
        this.bindEvents();
    },

    async loadTags() {
        try {
            this.tags = await api.getTags();
            this.renderTagFilter();
        } catch (e) {
            console.error('加载标签失败:', e);
        }
    },

    async loadContacts() {
        try {
            const params = new URLSearchParams({
                page: this.currentPage,
                page_size: this.pageSize,
            });
            if (this.searchQuery) params.append('search', this.searchQuery);
            if (this.selectedTag) params.append('tag', this.selectedTag);

            const res = await api.getContacts(params.toString());
            this.contacts = res.contacts || [];
            this.total = res.total || 0;
            this.renderContacts();
            this.renderPagination();
        } catch (e) {
            console.error('加载联系人失败:', e);
            app.showToast('加载联系人失败: ' + e.message, 'error');
        }
    },

    renderTagFilter() {
        const container = document.getElementById('contactTagFilter');
        if (!container) return;
        container.innerHTML = `
            <option value="">全部标签</option>
            ${this.tags.map(t => `<option value="${t.name}">${t.name}</option>`).join('')}
        `;
    },

    renderContacts() {
        const tbody = document.querySelector('#contactsTable tbody');
        if (!tbody) return;
        tbody.innerHTML = this.contacts.map(c => `
            <tr>
                <td>${c.name || '-'}</td>
                <td>${c.phone || '-'}</td>
                <td>${c.jid}</td>
                <td>${c.tags || '-'}</td>
                <td>${c.notes ? c.notes.substring(0, 30) + '...' : '-'}</td>
                <td>${c.source || '-'}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="ContactsPage.editContact('${c.jid}')">编辑</button>
                    <button class="btn btn-danger btn-sm" onclick="ContactsPage.deleteContact('${c.jid}')">删除</button>
                </td>
            </tr>
        `).join('');
    },

    renderPagination() {
        const container = document.getElementById('contactsPagination');
        if (!container) return;
        const totalPages = Math.ceil(this.total / this.pageSize);
        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }
        let html = '';
        for (let i = 1; i <= totalPages; i++) {
            if (i === 1 || i === totalPages || (i >= this.currentPage - 1 && i <= this.currentPage + 1)) {
                html += `<button class="btn ${i === this.currentPage ? 'btn-primary' : 'btn-secondary'}" onclick="ContactsPage.goPage(${i})">${i}</button>`;
            } else if (i === this.currentPage - 2 || i === this.currentPage + 2) {
                html += '<span>...</span>';
            }
        }
        container.innerHTML = html;
    },

    goPage(page) {
        this.currentPage = page;
        this.loadContacts();
    },

    bindEvents() {
        const searchInput = document.getElementById('contactSearch');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.searchQuery = e.target.value;
                    this.currentPage = 1;
                    this.loadContacts();
                }, 300);
            });
        }

        const tagFilter = document.getElementById('contactTagFilter');
        if (tagFilter) {
            tagFilter.addEventListener('change', (e) => {
                this.selectedTag = e.target.value;
                this.currentPage = 1;
                this.loadContacts();
            });
        }

        const addBtn = document.getElementById('addContactBtn');
        if (addBtn) {
            addBtn.addEventListener('click', () => this.openModal());
        }

        const modalClose = document.getElementById('contactModalClose');
        if (modalClose) {
            modalClose.addEventListener('click', () => this.closeModal());
        }

        const modalOverlay = document.getElementById('modalOverlay');
        if (modalOverlay) {
            modalOverlay.addEventListener('click', () => this.closeModal());
        }

        const form = document.getElementById('contactForm');
        if (form) {
            form.addEventListener('submit', (e) => this.saveContact(e));
        }
    },

    openModal(contact = null) {
        const modal = document.getElementById('contactModal');
        const form = document.getElementById('contactForm');
        const title = document.getElementById('contactModalTitle');

        if (contact) {
            title.textContent = '编辑联系人';
            form.jid.value = contact.jid;
            form.jid.disabled = true;
            form.name.value = contact.name || '';
            form.phone.value = contact.phone || '';
            form.tags.value = contact.tags || '';
            form.notes.value = contact.notes || '';
            form.source.value = contact.source || '';
        } else {
            title.textContent = '添加联系人';
            form.reset();
            form.jid.disabled = false;
        }

        modal.classList.add('active');
        document.getElementById('modalOverlay').classList.add('active');
    },

    closeModal() {
        document.getElementById('contactModal').classList.remove('active');
        document.getElementById('modalOverlay').classList.remove('active');
    },

    async editContact(jid) {
        const contact = this.contacts.find(c => c.jid === jid);
        if (contact) this.openModal(contact);
    },

    async deleteContact(jid) {
        if (!confirm('确定要删除这个联系人吗？')) return;
        try {
            await api.deleteContact(jid);
            app.showToast('删除成功', 'success');
            this.loadContacts();
        } catch (e) {
            app.showToast('删除失败: ' + e.message, 'error');
        }
    },

    async saveContact(e) {
        e.preventDefault();
        const form = e.target;
        const data = {
            jid: form.jid.value,
            name: form.name.value,
            phone: form.phone.value,
            tags: form.tags.value,
            notes: form.notes.value,
            source: form.source.value,
        };

        try {
            const isEdit = form.jid.disabled;
            if (isEdit) {
                await api.updateContact(data.jid, data);
            } else {
                await api.createContact(data);
            }
            this.closeModal();
            app.showToast(isEdit ? '更新成功' : '创建成功', 'success');
            this.loadContacts();
        } catch (ex) {
            app.showToast('保存失败: ' + ex.message, 'error');
        }
    }
};

document.addEventListener('DOMContentLoaded', () => ContactsPage.init());
window.ContactsPage = ContactsPage;