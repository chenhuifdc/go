// api.js - 统一 API 调用封装
const API_BASE = '/api';

const api = {
    // 账号相关
    async getAccounts() {
        return this.request('/accounts');
    },
    async getAccount(id) {
        return this.request(`/accounts/${id}`);
    },
    async createAccount(data) {
        return this.request('/accounts', { method: 'POST', body: JSON.stringify(data) });
    },
    async updateAccount(id, data) {
        return this.request(`/accounts/${id}`, { method: 'PUT', body: JSON.stringify(data) });
    },
    async deleteAccount(id) {
        return this.request(`/accounts/${id}`, { method: 'DELETE' });
    },
    async connectAccount(id) {
        return this.request(`/accounts/${id}/connect`, { method: 'POST' });
    },
    async disconnectAccount(id) {
        return this.request(`/accounts/${id}/disconnect`, { method: 'POST' });
    },
    async refreshQR(id) {
        return this.request(`/accounts/${id}/qr/refresh`, { method: 'POST' });
    },
    async getAccountQR(id) {
        return this.request(`/accounts/${id}/qr`);
    },
    async getAccountStatus(id) {
        return this.request(`/accounts/${id}/status`);
    },

    // CRM 相关
    async getContacts(query = '') {
        const url = query ? `/crm/contacts?search=${encodeURIComponent(query)}` : '/crm/contacts';
        return this.request(url);
    },
    async createContact(data) {
        return this.request('/crm/contacts', { method: 'POST', body: JSON.stringify(data) });
    },
    async updateContact(jid, data) {
        return this.request(`/crm/contacts/${jid}`, { method: 'PUT', body: JSON.stringify(data) });
    },
    async deleteContact(jid) {
        return this.request(`/crm/contacts/${jid}`, { method: 'DELETE' });
    },
    async getTags() {
        return this.request('/crm/tags');
    },
    async createTag(data) {
        return this.request('/crm/tags', { method: 'POST', body: JSON.stringify(data) });
    },
    async updateTag(id, data) {
        return this.request(`/crm/tags/${id}`, { method: 'PUT', body: JSON.stringify(data) });
    },
    async deleteTag(id) {
        return this.request(`/crm/tags/${id}`, { method: 'DELETE' });
    },
    async getFollowups() {
        return this.request('/crm/followups');
    },
    async createFollowup(data) {
        return this.request('/crm/followups', { method: 'POST', body: JSON.stringify(data) });
    },
    async deleteFollowup(id) {
        return this.request(`/crm/followups/${id}`, { method: 'DELETE' });
    },

    // 群组相关
    async getGroups(query = '') {
        const url = query ? `/groups?search=${encodeURIComponent(query)}` : '/groups';
        return this.request(url);
    },
    async getGroup(jid) {
        return this.request(`/groups/${jid}`);
    },
    async getGroupMembers(jid) {
        return this.request(`/groups/${jid}/members`);
    },
    async toggleGroupPrivacy(jid) {
        return this.request(`/groups/${jid}/toggle`, { method: 'POST' });
    },
    async exportGroupMembers(jid) {
        return this.request(`/groups/${jid}/export`, { method: 'POST' });
    },
    async kickGroupMember(jid, member) {
        return this.request(`/groups/${jid}/kick`, { method: 'POST', body: JSON.stringify({ member }) });
    },
    async promoteGroupMember(jid, member) {
        return this.request(`/groups/${jid}/promote`, { method: 'POST', body: JSON.stringify({ member }) });
    },
    async demoteGroupMember(jid, member) {
        return this.request(`/groups/${jid}/demote`, { method: 'POST', body: JSON.stringify({ member }) });
    },

    // 群发相关
    async getBroadcastTasks() {
        return this.request('/broadcast');
    },
    async createBroadcastTask(data) {
        return this.request('/broadcast', { method: 'POST', body: JSON.stringify(data) });
    },
    async getBroadcastTask(id) {
        return this.request(`/broadcast/${id}`);
    },
    async updateBroadcastTask(id, data) {
        return this.request(`/broadcast/${id}`, { method: 'PUT', body: JSON.stringify(data) });
    },
    async deleteBroadcastTask(id) {
        return this.request(`/broadcast/${id}`, { method: 'DELETE' });
    },
    async startBroadcastTask(id) {
        return this.request(`/broadcast/${id}/start`, { method: 'POST' });
    },
    async pauseBroadcastTask(id) {
        return this.request(`/broadcast/${id}/pause`, { method: 'POST' });
    },
    async cancelBroadcastTask(id) {
        return this.request(`/broadcast/${id}/cancel`, { method: 'POST' });
    },
    async getBroadcastProgress(id) {
        return this.request(`/broadcast/${id}/progress`);
    },
    async getBroadcastTaskLogs(id) {
        return this.request(`/broadcast/${id}/logs`);
    },

    // 风控相关
    async getRiskEvents(params = {}) {
        const query = new URLSearchParams(params).toString();
        const url = query ? `/risk/events?${query}` : '/risk/events';
        return this.request(url);
    },
    async getRiskRules() {
        return this.request('/risk/rules');
    },
    async createRiskRule(data) {
        return this.request('/risk/rules', { method: 'POST', body: JSON.stringify(data) });
    },
    async updateRiskRule(id, data) {
        return this.request(`/risk/rules/${id}`, { method: 'PUT', body: JSON.stringify(data) });
    },
    async deleteRiskRule(id) {
        return this.request(`/risk/rules/${id}`, { method: 'DELETE' });
    },
    async getRiskWords() {
        return this.request('/risk/words');
    },
    async uploadRiskWords(data) {
        return this.request('/risk/words', { method: 'POST', body: JSON.stringify(data) });
    },
    async deleteRiskWords(data) {
        return this.request('/risk/words', { method: 'DELETE', body: JSON.stringify(data) });
    },

    // 翻译相关
    async translateText(text, targetLang) {
        return this.request('/translate', { method: 'POST', body: JSON.stringify({ text, target_lang: targetLang }) });
    },
    async batchTranslate(texts, targetLang) {
        return this.request('/translate/batch', { method: 'POST', body: JSON.stringify({ texts, target_lang: targetLang }) });
    },
    async getTranslationSettings() {
        return this.request('/translate/settings');
    },
    async updateTranslationSettings(data) {
        return this.request('/translate/settings', { method: 'PUT', body: JSON.stringify(data) });
    },

    // 配置相关
    async getConfig() {
        return this.request('/config');
    },
    async updateConfig(data) {
        return this.request('/config', { method: 'PUT', body: JSON.stringify(data) });
    },
    async getConfigExample() {
        return this.request('/config/example');
    },

    // 统计/监控
    async getStatsOverview() {
        return this.request('/stats/overview');
    },
    async getAccountStats() {
        return this.request('/stats/accounts');
    },
    async getMessageStats() {
        return this.request('/stats/messages');
    },
    async getRiskStats() {
        return this.request('/stats/risk');
    },

    // 健康检查
    async healthCheck() {
        return this.request('/health');
    },

    // 核心请求方法
    async request(path, options = {}) {
        const url = path.startsWith('http') ? path : API_BASE + path;
        const defaultOptions = {
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' }
        };
        const finalOptions = { ...defaultOptions, ...options, headers: { ...defaultOptions.headers, ...(options.headers || {}) } };
        
        try {
            const response = await fetch(url, finalOptions);
            const contentType = response.headers.get('content-type');
            
            if (response.status >= 200 && response.status < 300) {
                if (contentType && contentType.includes('application/json')) {
                    return await response.json();
                }
                return { success: true, status: response.status };
            }
            
            let errorMessage = `请求失败 (${response.status})`;
            if (contentType && contentType.includes('application/json')) {
                const errorData = await response.json();
                errorMessage = errorData.error || errorData.message || errorMessage;
            }
            throw new Error(errorMessage);
        } catch (error) {
            console.error('API 请求失败:', url, error);
            throw error;
        }
    }
};

window.api = api;