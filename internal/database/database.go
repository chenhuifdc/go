package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"whatsapp-ops/internal/config"
	"go.uber.org/zap"
)

var (
	MainDB *sql.DB
	mu     sync.RWMutex
	accountDBs = make(map[string]*sql.DB)
)

func Init() error {
	cfg := config.Get()

	// 主数据库
	var err error
	MainDB, err = openDB(cfg.Database.MainDBPath, cfg.Database.EnableWAL)
	if err != nil {
		return fmt.Errorf("open main db: %w", err)
	}

	MainDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	MainDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	MainDB.SetConnMaxLifetime(time.Hour)

	// 初始化表
	if err := migrate(MainDB); err != nil {
		return fmt.Errorf("migrate main db: %w", err)
	}

	zap.L().Info("主数据库初始化完成", zap.String("path", cfg.Database.MainDBPath))
	return nil
}

func openDB(path string, wal bool) (*sql.DB, error) {
	// 确保目录存在
	// 这里不创建目录，由 config.ensureDirs 处理

	dsn := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000", path)
	if wal {
		dsn += "&_journal_mode=WAL"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		// 账号表
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			name TEXT,
			phone TEXT,
			jid TEXT,
			proxy_url TEXT,
			status TEXT DEFAULT 'disconnected',
			qr_code TEXT,
			qr_expires_at INTEGER,
			last_seen INTEGER,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 客户表
		`CREATE TABLE IF NOT EXISTS contacts (
			jid TEXT PRIMARY KEY,
			name TEXT,
			phone TEXT,
			avatar TEXT,
			tags TEXT DEFAULT '[]',
			notes TEXT,
			source TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 标签字典
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			color TEXT DEFAULT '#409EFF',
			created_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 跟进记录
		`CREATE TABLE IF NOT EXISTS followups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contact_jid TEXT NOT NULL REFERENCES contacts(jid) ON DELETE CASCADE,
			content TEXT NOT NULL,
			operator TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 群组表
		`CREATE TABLE IF NOT EXISTS groups (
			jid TEXT PRIMARY KEY,
			name TEXT,
			owner_jid TEXT,
			participants_count INTEGER DEFAULT 0,
			is_announce_only BOOLEAN DEFAULT 0,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 群成员表
		`CREATE TABLE IF NOT EXISTS group_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_jid TEXT NOT NULL REFERENCES groups(jid) ON DELETE CASCADE,
			user_jid TEXT NOT NULL,
			name TEXT,
			phone TEXT,
			is_admin BOOLEAN DEFAULT 0,
			is_super_admin BOOLEAN DEFAULT 0,
			joined_at INTEGER,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			UNIQUE(group_jid, user_jid)
		);`,

		// 消息记录
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			chat_jid TEXT NOT NULL,
			sender_jid TEXT,
			type TEXT,
			content TEXT,
			timestamp INTEGER,
			is_from_me BOOLEAN DEFAULT 0,
			is_group BOOLEAN DEFAULT 0,
			quoted_msg_id TEXT,
			media_url TEXT,
			media_type TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 群发任务
		`CREATE TABLE IF NOT EXISTS broadcast_tasks (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			name TEXT,
			message_content TEXT,
			message_type TEXT DEFAULT 'text',
			group_jids TEXT, -- JSON array
			mention_all BOOLEAN DEFAULT 0,
			interval_ms INTEGER DEFAULT 2000,
			status TEXT DEFAULT 'pending', -- pending, running, completed, failed, paused
			total_groups INTEGER DEFAULT 0,
			sent_count INTEGER DEFAULT 0,
			failed_count INTEGER DEFAULT 0,
			error_msg TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			started_at INTEGER,
			completed_at INTEGER
		);`,

		// 群发日志
		`CREATE TABLE IF NOT EXISTS broadcast_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL REFERENCES broadcast_tasks(id) ON DELETE CASCADE,
			group_jid TEXT NOT NULL,
			status TEXT, -- success, failed
			error_msg TEXT,
			message_id TEXT,
			sent_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 风控事件
		`CREATE TABLE IF NOT EXISTS risk_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id TEXT NOT NULL,
			group_jid TEXT,
			user_jid TEXT,
			level INTEGER, -- 1, 2, 3
			rule_id TEXT,
			rule_name TEXT,
			matched_content TEXT,
			action TEXT, -- delete, warn, kick, log
			action_status TEXT, -- success, failed
			message_id TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 风控规则
		`CREATE TABLE IF NOT EXISTS risk_rules (
			id TEXT PRIMARY KEY,
			level INTEGER NOT NULL, -- 1, 2, 3
			name TEXT NOT NULL,
			pattern TEXT, -- 关键词/正则
			rule_type TEXT, -- keyword, regex, model
			action TEXT DEFAULT 'warn', -- delete, warn, kick, log
			enabled BOOLEAN DEFAULT 1,
			priority INTEGER DEFAULT 0,
			description TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 翻译缓存
		`CREATE TABLE IF NOT EXISTS translation_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_text TEXT NOT NULL,
			target_lang TEXT NOT NULL,
			translated_text TEXT NOT NULL,
			model TEXT,
			created_at INTEGER DEFAULT (strftime('%s','now')),
			UNIQUE(source_text, target_lang)
		);`,

		// 设置键值表
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER DEFAULT (strftime('%s','now'))
		);`,

		// 索引
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_jid, timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_account_time ON messages(account_id, timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_phone ON contacts(phone);`,
		`CREATE INDEX IF NOT EXISTS idx_followups_contact ON followups(contact_jid, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_group_members_group ON group_members(group_jid);`,
		`CREATE INDEX IF NOT EXISTS idx_risk_events_account_time ON risk_events(account_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_broadcast_logs_task ON broadcast_logs(task_id);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %s: %w", strings.TrimSpace(stmt)[:80], err)
		}
	}

	// 插入默认标签
	defaultTags := []struct{ name, color string }{
		{"VIP", "#F56C6C"},
		{"意向客户", "#E6A23C"},
		{"已成交", "#67C23A"},
		{"流失", "#909399"},
		{"重点跟进", "#409EFF"},
	}
	for _, t := range defaultTags {
		db.Exec(`INSERT OR IGNORE INTO tags(name, color) VALUES(?, ?)`, t.name, t.color)
	}

	// 插入默认风控规则示例
	defaultRules := []struct{ id, name, pattern, rtype, action string; level int }{
		{"l1_spam_001", "广告链接", `(?i)(https?://|www\.)\S+`, "regex", "warn", 1},
		{"l1_spam_002", "微信号/QQ号", `(?i)(微信|vx|wx|QQ)\s*[:\-]?\s*[a-zA-Z0-9_-]{5,}`, "regex", "warn", 1},
		{"l2_politics_001", "政治敏感", `敏感词1|敏感词2|敏感词3`, "keyword", "delete", 2},
		{"l2_porn_001", "色情内容", `色情|黄色|成人`, "keyword", "delete", 2},
	}
	for _, r := range defaultRules {
		db.Exec(`INSERT OR IGNORE INTO risk_rules(id, level, name, pattern, rule_type, action, enabled) VALUES(?, ?, ?, ?, ?, ?, 1)`,
			r.id, r.level, r.name, r.pattern, r.rtype, r.action)
	}

	return nil
}

// GetAccountDB 获取/创建账号专用会话数据库
func GetAccountDB(accountID string) (*sql.DB, error) {
	mu.RLock()
	db, ok := accountDBs[accountID]
	mu.RUnlock()
	if ok {
		return db, nil
	}

	mu.Lock()
	defer mu.Unlock()
	// 双重检查
	if db, ok = accountDBs[accountID]; ok {
		return db, nil
	}

	cfg := config.Get()
	path := filepath.Join(cfg.Database.SessionsDir, fmt.Sprintf("session_%s.db", accountID))
	db, err := openDB(path, cfg.Database.EnableWAL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	accountDBs[accountID] = db
	return db, nil
}

func CloseAccountDB(accountID string) {
	mu.Lock()
	defer mu.Unlock()
	if db, ok := accountDBs[accountID]; ok {
		db.Close()
		delete(accountDBs, accountID)
	}
}

func CloseAll() {
	if MainDB != nil {
		MainDB.Close()
	}
	mu.Lock()
	for _, db := range accountDBs {
		db.Close()
	}
	accountDBs = make(map[string]*sql.DB)
	mu.Unlock()
}

// 通用查询辅助
func QueryRow(query string, args ...interface{}) *sql.Row {
	return MainDB.QueryRow(query, args...)
}

func Query(query string, args ...interface{}) (*sql.Rows, error) {
	return MainDB.Query(query, args...)
}

func Exec(query string, args ...interface{}) (sql.Result, error) {
	return MainDB.Exec(query, args...)
}

func Transaction(fn func(*sql.Tx) error) error {
	tx, err := MainDB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}