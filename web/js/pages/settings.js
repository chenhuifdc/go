// settings.js - 系统设置页面
const SettingsPage = {
    config: {},

    async init() {
        console.log('[SettingsPage] 初始化系统设置页面');
        await this.loadConfig();
        this.bindEvents();
    },

    async loadConfig() {
        try {
            this.config = await api.getSettings();
            this.renderConfig();
        } catch (e) {
            console.error('加载设置失败:', e);
            app.showToast('加载设置失败: ' + e.message, 'error');
        }
    },

    renderConfig() {
        // 翻译设置
        const t = this.config.translation || {};
        this.setValue('translationBaseUrl', t.base_url || '');
        this.setValue('translationModel', t.model || '');
        this.setValue('translationPrompt', t.prompt_template || '');

        // 群发设置
        const b = this.config.broadcast || {};
        this.setValue('broadcastDefaultIntervalMin', b.default_interval_min || 1);
        this.setValue('broadcastDefaultIntervalMax', b.default_interval_max || 3);
        this.setValue('broadcastMaxRetries', b.max_retries || 3);
        this.setValue('broadcastRetryDelay', b.retry_delay || 5);

        // 风控设置
        const r = this.config.risk || {};
        this.setValue('riskL1Enabled', r.l1_enabled !== false);
        this.setValue('riskL2Enabled', r.l2_enabled !== false);
        this.setValue('riskL3Enabled', r.l3_enabled !== false);
        this.setValue('riskL3WindowSeconds', r.l3_window_seconds || 60);
        this.setValue('riskL3MaxMessages', r.l3_max_messages || 20);
        this.setValue('riskL3MaxMentions', r.l3_max_mentions || 5);

        // 调度器设置
        const s = this.config.scheduler || {};
        this.setValue('schedulerEnabled', s.enabled !== false);
        this.setValue('schedulerInterval', s.interval || 60);
    },

    setValue(id, value) {
        const el = document.getElementById(id);
        if (el) {
            if (el.type === 'checkbox') {
                el.checked = value;
            } else {
                el.value = value;
            }
        }
    },

    getValue(id) {
        const el = document.getElementById(id);
        if (!el) return null;
        if (el.type === 'checkbox') {
            return el.checked;
        }
        return el.value;
    },

    bindEvents() {
        const form = document.getElementById('settingsForm');
        if (form) {
            form.addEventListener('submit', (e) => this.saveConfig(e));
        }

        const reloadBtn = document.getElementById('reloadConfigBtn');
        if (reloadBtn) {
            reloadBtn.addEventListener('click', () => this.loadConfig());
        }

        // 测试翻译连接
        const testTranslationBtn = document.getElementById('testTranslationBtn');
        if (testTranslationBtn) {
            testTranslationBtn.addEventListener('click', () => this.testTranslation());
        }
    },

    async saveConfig(e) {
        e.preventDefault();
        const data = {
            translation: {
                base_url: this.getValue('translationBaseUrl'),
                model: this.getValue('translationModel'),
                prompt_template: this.getValue('translationPrompt'),
            },
            broadcast: {
                default_interval_min: parseInt(this.getValue('broadcastDefaultIntervalMin')) || 1,
                default_interval_max: parseInt(this.getValue('broadcastDefaultIntervalMax')) || 3,
                max_retries: parseInt(this.getValue('broadcastMaxRetries')) || 3,
                retry_delay: parseInt(this.getValue('broadcastRetryDelay')) || 5,
            },
            risk: {
                l1_enabled: this.getValue('riskL1Enabled'),
                l2_enabled: this.getValue('riskL2Enabled'),
                l3_enabled: this.getValue('riskL3Enabled'),
                l3_window_seconds: parseInt(this.getValue('riskL3WindowSeconds')) || 60,
                l3_max_messages: parseInt(this.getValue('riskL3MaxMessages')) || 20,
                l3_max_mentions: parseInt(this.getValue('riskL3MaxMentions')) || 5,
            },
            scheduler: {
                enabled: this.getValue('schedulerEnabled'),
                interval: parseInt(this.getValue('schedulerInterval')) || 60,
            },
        };

        try {
            await api.updateSettings(data);
            app.showToast('设置已保存', 'success');
            this.loadConfig();
        } catch (ex) {
            app.showToast('保存失败: ' + ex.message, 'error');
        }
    },

    async testTranslation() {
        const baseUrl = this.getValue('translationBaseUrl');
        const model = this.getValue('translationModel');
        const apiKey = this.getValue('translationApiKey'); // 从环境变量或配置获取

        if (!baseUrl || !model) {
            app.showToast('请先填写翻译 API 地址和模型', 'warning');
            return;
        }

        try {
            const res = await api.translate({
                text: 'Hello, this is a test message.',
                target_lang: 'zh-CN',
            });
            app.showToast('翻译测试成功: ' + res.translated_text, 'success');
        } catch (e) {
            app.showToast('翻译测试失败: ' + e.message, 'error');
        }
    }
};

document.addEventListener('DOMContentLoaded', () => SettingsPage.init());
window.SettingsPage = SettingsPage;