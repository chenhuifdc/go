// 本地状态存储
const store = {
    get(key, defaultVal = null) {
        try {
            const val = localStorage.getItem('waops_' + key);
            return val ? JSON.parse(val) : defaultVal;
        } catch {
            return defaultVal;
        }
    },
    set(key, val) {
        try {
            localStorage.setItem('waops_' + key, JSON.stringify(val));
        } catch (e) {
            console.warn('存储失败:', e);
        }
    }
};
window.store = store;
