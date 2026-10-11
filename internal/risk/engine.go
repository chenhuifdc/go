package risk

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"whatsapp-ops/internal/config"
	"whatsapp-ops/internal/database"

	"go.uber.org/zap"
)

type RiskLevel int

const (
	Level1 RiskLevel = iota + 1 // 垃圾广告
	Level2                      // 敏感违规
	Level3                      // 可疑行为
)

type ActionType string

const (
	ActionDelete ActionType = "delete"
	ActionWarn   ActionType = "warn"
	ActionKick   ActionType = "kick"
	ActionLog    ActionType = "log"
)

type RiskRule struct {
	ID          string     `json:"id"`
	Level       RiskLevel  `json:"level"`
	Name        string     `json:"name"`
	Pattern     string     `json:"pattern,omitempty"` // 关键词/正则
	RuleType    string     `json:"rule_type"`         // keyword, regex, model
	Action      ActionType `json:"action"`            // delete, warn, kick, log
	Enabled     bool       `json:"enabled"`
	Priority    int        `json:"priority"`
	Description string     `json:"description,omitempty"`
}

type RiskEvent struct {
	ID             int64      `json:"id"`
	AccountID      string     `json:"account_id"`
	GroupJID       string     `json:"group_jid,omitempty"`
	UserJID        string     `json:"user_jid,omitempty"`
	Level          RiskLevel  `json:"level"`
	RuleID         string     `json:"rule_id,omitempty"`
	RuleName       string     `json:"rule_name,omitempty"`
	MatchedContent string     `json:"matched_content,omitempty"`
	Action         ActionType `json:"action"`
	ActionStatus   string     `json:"action_status,omitempty"`
	MessageID      string     `json:"message_id,omitempty"`
	CreatedAt      int64      `json:"created_at"`
}

type BehaviorStats struct {
	MsgCount1Min     int
	MsgCount5Min     int
	MentionCount1Min int
	LastMsgTime      time.Time
	LastMentionTime  time.Time
	RecentMessages   []string // 用于相似度检测
	mu               sync.RWMutex
}

// 全局引擎实例
var defaultEngine *Engine

// Engine 风控引擎结构
type Engine struct {
	l1Matcher     *ACAutomaton
	l2Matcher     *ACAutomaton
	compiledRegex []*regexpRegexp
	modelEnabled  bool
	rulesByLevel  map[RiskLevel][]*RiskRule
	ruleMap       map[string]*RiskRule
	behaviorStats map[string]map[string]*BehaviorStats // accountID -> groupJID/userJID -> stats
	behaviorMu    sync.RWMutex
	mu            sync.RWMutex
}

// Init 初始化风控引擎（保持兼容性）
func Init() error {
	cfg := config.Get()
	if !cfg.Risk.Enabled {
		zap.L().Info("风控引擎已禁用")
		return nil
	}

	engine := &Engine{
		rulesByLevel:  make(map[RiskLevel][]*RiskRule),
		ruleMap:       make(map[string]*RiskRule),
		behaviorStats: make(map[string]map[string]*BehaviorStats),
	}

	// 加载规则
	if err := engine.loadRulesFromDB(); err != nil {
		return err
	}
	// 加载词库
	if err := engine.loadWordLists(); err != nil {
		return err
	}
	// 初始化AC自动机
	if err := engine.initACAutomaton(); err != nil {
		return err
	}

	defaultEngine = engine

	zap.L().Info("风控引擎初始化完成",
		zap.Int("rules", len(engine.ruleMap)),
		zap.Int("l1_patterns", engine.l1Matcher.PatternCount()),
		zap.Int("l2_patterns", engine.l2Matcher.PatternCount()))
	return nil
}

// GetEngine 获取全局风控引擎实例
func GetEngine() *Engine {
	return defaultEngine
}

// ruleMap 返回所有规则的扁平映射（兼容旧代码）
func (e *Engine) GetRuleMap() map[string]*RiskRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ruleMap
}

// GetL1Matcher 返回 L1 匹配器
func (e *Engine) GetL1Matcher() *ACAutomaton {
	return e.l1Matcher
}

// GetL2Matcher 返回 L2 匹配器
func (e *Engine) GetL2Matcher() *ACAutomaton {
	return e.l2Matcher
}

// GetCompiledRegex 返回编译后的正则
func (e *Engine) GetCompiledRegex() []*regexpRegexp {
	return e.compiledRegex
}

// IsModelEnabled 返回模型是否启用
func (e *Engine) IsModelEnabled() bool {
	return e.modelEnabled
}

// GetRulesByLevel 返回按层级分组的规则
func (e *Engine) GetRulesByLevel() map[RiskLevel][]*RiskRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rulesByLevel
}

func (e *Engine) loadRulesFromDB() error {
	rows, err := database.Query(`
		SELECT id, level, name, pattern, rule_type, action, enabled, priority, description
		FROM risk_rules ORDER BY level, priority DESC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	e.rulesByLevel = make(map[RiskLevel][]*RiskRule)
	e.ruleMap = make(map[string]*RiskRule)
	for rows.Next() {
		var id string
		var level int
		var name, pattern, ruleType, action string
		var enabled bool
		var priority int
		var description sql.NullString
		if err := rows.Scan(&id, &level, &name, &pattern, &ruleType, &action, &enabled, &priority, &description); err != nil {
			continue
		}
		rule := &RiskRule{
			ID:          id,
			Level:       RiskLevel(level),
			Name:        name,
			Pattern:     pattern,
			RuleType:    ruleType,
			Action:      ActionType(action),
			Enabled:     enabled,
			Priority:    priority,
			Description: description.String,
		}
		if !rule.Enabled {
			continue
		}
		e.ruleMap[id] = rule
		e.rulesByLevel[rule.Level] = append(e.rulesByLevel[rule.Level], rule)
	}
	// 按优先级排序
	for level := range e.rulesByLevel {
		sort.Slice(e.rulesByLevel[level], func(i, j int) bool {
			return e.rulesByLevel[level][i].Priority > e.rulesByLevel[level][j].Priority
		})
	}

	// 编译正则规则
	e.compiledRegex = nil
	for _, rule := range e.ruleMap {
		if rule.RuleType == "regex" && rule.Pattern != "" {
			if re, err := regexp.Compile(rule.Pattern); err == nil {
				e.compiledRegex = append(e.compiledRegex, &regexpRegexp{
					re:     re,
					ruleID: rule.ID,
				})
			} else {
				zap.L().Warn("编译正则规则失败", zap.String("rule_id", rule.ID), zap.Error(err))
			}
		}
	}

	return nil
}

func (e *Engine) loadWordLists() error {
	cfg := config.Get()
	wordsDir := cfg.Risk.WordsDir
	if wordsDir == "" {
		return nil
	}

	// L1: 垃圾广告词库
	l1Path := filepath.Join(wordsDir, "l1_spam.txt")
	if data, err := os.ReadFile(l1Path); err == nil {
		lines := strings.Split(string(data), "\n")
		var patterns []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			patterns = append(patterns, line)
		}
		// TODO: 添加到 L1 matcher
	}

	// L2: 敏感词库
	l2Path := filepath.Join(wordsDir, "l2_sensitive.txt")
	if data, err := os.ReadFile(l2Path); err == nil {
		lines := strings.Split(string(data), "\n")
		var patterns []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			patterns = append(patterns, line)
		}
		// TODO: 添加到 L2 matcher
	}

	return nil
}

func (e *Engine) initACAutomaton() error {
	// TODO: 实现 Aho-Corasick 自动机构建
	// 这里使用占位实现
	e.l1Matcher = &ACAutomaton{}
	e.l2Matcher = &ACAutomaton{}
	return nil
}

// CheckMessage 检查消息，返回匹配的规则和建议动作
func (e *Engine) CheckMessage(accountID, groupJID, userJID, content string, msgID string) ([]*RiskMatch, *BehaviorMatch) {
	if e == nil || !config.Get().Risk.Enabled {
		return nil, nil
	}

	var riskMatches []*RiskMatch
	var behaviorMatch *BehaviorMatch

	// L1: 垃圾广告
	if config.Get().Risk.L1Enabled {
		if matches := e.l1Matcher.Match(content); len(matches) > 0 {
			for _, m := range matches {
				if rule, ok := e.ruleMap[m.RuleID]; ok {
					riskMatches = append(riskMatches, &RiskMatch{
						Rule:     rule,
						Matched:  m.Text,
						Position: m.Position,
					})
				}
			}
		}
	}

	// L2: 敏感违规
	if config.Get().Risk.L2Enabled {
		if matches := e.l2Matcher.Match(content); len(matches) > 0 {
			for _, m := range matches {
				if rule, ok := e.ruleMap[m.RuleID]; ok {
					riskMatches = append(riskMatches, &RiskMatch{
						Rule:     rule,
						Matched:  m.Text,
						Position: m.Position,
					})
				}
			}
		}
		// 正则规则匹配
		for _, rr := range e.compiledRegex {
			if loc := rr.re.FindStringIndex(content); loc != nil {
				if rule, ok := e.ruleMap[rr.ruleID]; ok {
					riskMatches = append(riskMatches, &RiskMatch{
						Rule:     rule,
						Matched:  content[loc[0]:loc[1]],
						Position: loc[0],
					})
				}
			}
		}
		// TODO: 可选的模型检测
	}

	// L3: 行为分析
	if config.Get().Risk.L3Enabled {
		if bm := e.checkBehavior(accountID, groupJID, userJID, content); bm != nil {
			behaviorMatch = bm
		}
	}

	return riskMatches, behaviorMatch
}

// checkBehavior 行为分析
func (e *Engine) checkBehavior(accountID, groupJID, userJID, content string) *BehaviorMatch {
	e.behaviorMu.Lock()
	defer e.behaviorMu.Unlock()

	// 获取或创建统计对象
	accStats, ok := e.behaviorStats[accountID]
	if !ok {
		accStats = make(map[string]*BehaviorStats)
		e.behaviorStats[accountID] = accStats
	}
	var targetKey string
	if groupJID != "" {
		targetKey = "g:" + groupJID
	} else {
		targetKey = "u:" + userJID
	}
	stats, ok := accStats[targetKey]
	if !ok {
		stats = &BehaviorStats{
			RecentMessages: make([]string, 0, 10),
		}
		accStats[targetKey] = stats
	}

	now := time.Now()
	stats.mu.Lock()
	defer stats.mu.Unlock()

	// 计数（这里简化，实际需要维护时间队列）
	stats.MsgCount1Min++
	if strings.Contains(content, "@all") || strings.Contains(content, "@everyone") {
		stats.MentionCount1Min++
	}
	stats.LastMsgTime = now

	// 保存最近消息用于相似度检测
	stats.RecentMessages = append(stats.RecentMessages, content)
	if len(stats.RecentMessages) > 10 {
		stats.RecentMessages = stats.RecentMessages[1:]
	}

	// 检查阈值
	cfg := config.Get()
	if stats.MsgCount1Min > cfg.Risk.L3MaxMsgPerMin {
		return &BehaviorMatch{
			Type:        "flood",
			Description: fmt.Sprintf("1分钟内发送%d条消息（阈值：%d）", stats.MsgCount1Min, cfg.Risk.L3MaxMsgPerMin),
			Count:       stats.MsgCount1Min,
			Threshold:   cfg.Risk.L3MaxMsgPerMin,
		}
	}
	if stats.MentionCount1Min > cfg.Risk.L3MaxMentionPerMin {
		return &BehaviorMatch{
			Type:        "mention_spam",
			Description: fmt.Sprintf("1分钟内@%d次（阈值：%d）", stats.MentionCount1Min, cfg.Risk.L3MaxMentionPerMin),
			Count:       stats.MentionCount1Min,
			Threshold:   cfg.Risk.L3MaxMentionPerMin,
		}
	}
	// TODO: 相似度检测（重复内容）
	// TODO: 频繁进出群检测

	return nil
}

// RiskMatch 风控匹配结果
type RiskMatch struct {
	Rule     *RiskRule
	Matched  string
	Position int
}

// BehaviorMatch 行为匹配结果
type BehaviorMatch struct {
	Type        string
	Description string
	Count       int
	Threshold   int
}

// LogRiskEvent 记录风控事件
func LogRiskEvent(accountID, groupJID, userJID, messageID, content string, matches []*RiskMatch, bm *BehaviorMatch) error {
	cfg := config.Get()
	if !cfg.Risk.Enabled {
		return nil
	}

	var level RiskLevel
	var ruleID, ruleName, matchedContent string
	var action ActionType
	var actionStatus string = "pending"

	if len(matches) > 0 {
		// 按级别和优先级排序
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Rule.Level != matches[j].Rule.Level {
				return matches[i].Rule.Level < matches[j].Rule.Level
			}
			return matches[i].Rule.Priority > matches[j].Rule.Priority
		})
		level = matches[0].Rule.Level
		ruleID = matches[0].Rule.ID
		ruleName = matches[0].Rule.Name
		matchedContent = matches[0].Matched
		action = matches[0].Rule.Action
	} else if bm != nil {
		level = Level2 // 行为异常归为L2
		ruleID = "behavior_" + bm.Type
		ruleName = "行为异常: " + bm.Type
		matchedContent = bm.Description
		action = ActionType(cfg.Risk.DefaultAction)
	} else {
		return nil
	}

	_, err := database.Exec(`
		INSERT INTO risk_events(
			account_id, group_jid, user_jid, level, rule_id, rule_name,
			matched_content, action, action_status, message_id, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s','now'))
	`, accountID, groupJID, userJID, level, ruleID, ruleName, matchedContent,
		action, actionStatus, messageID)
	return err
}

// ExecuteAction 执行风控动作
func (e *Engine) ExecuteAction(accountID, groupJID, userJID, messageID, content string, matches []*RiskMatch, bm *BehaviorMatch) error {
	if !config.Get().Risk.Enabled || !config.Get().Risk.AutoAction {
		return nil // 仅记录，不执行
	}

	var action ActionType
	var reason string

	// 确定动作：取最高级别的动作
	if len(matches) > 0 {
		// 按级别和优先级排序
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Rule.Level != matches[j].Rule.Level {
				return matches[i].Rule.Level < matches[j].Rule.Level // 越小级别越高（1最高）
			}
			return matches[i].Rule.Priority > matches[j].Rule.Priority
		})
		action = matches[0].Rule.Action
		reason = fmt.Sprintf("%s: %s", matches[0].Rule.Name, matches[0].Matched)
	} else if bm != nil {
		// 行为匹配，使用默认动作
		action = ActionType(config.Get().Risk.DefaultAction)
		reason = bm.Description
	} else {
		return nil
	}

	// 检查白名单（TODO）
	// if isWhitelisted(accountID, userJID) {
	// 	return nil
	// }

	// 执行动作
	var err error
	switch action {
	case ActionDelete:
		err = e.deleteMessage(accountID, groupJID, messageID)
	case ActionWarn:
		err = e.warnUser(accountID, groupJID, userJID, reason)
	case ActionKick:
		err = e.kickUser(accountID, groupJID, userJID)
	case ActionLog:
		// 仅记录，不执行
	}

	// 记录事件
	if err := e.LogRiskEvent(accountID, groupJID, userJID, messageID, content, matches, bm); err != nil {
		zap.L().Error("记录风控事件失败", zap.Error(err))
	}
	return err
}

// ReloadRules 重新加载规则
func (e *Engine) ReloadRules() error {
	return e.loadRulesFromDB()
}

// ReloadWordLists 重新加载词库
func (e *Engine) ReloadWordLists() error {
	return e.loadWordLists()
}

// InitACAutomaton 初始化 AC 自动机（用于外部调用）
func (e *Engine) InitACAutomaton() error {
	return e.initACAutomaton()
}

// LoadExampleWords 加载示例词库（用于测试）
func LoadExampleWords() {
	if defaultEngine == nil {
		return
	}
	// L1 示例
	defaultEngine.l1Matcher.AddPattern("http://", "l1_link_001")
	defaultEngine.l1Matcher.AddPattern("https://", "l1_link_002")
	defaultEngine.l1Matcher.AddPattern("微信号", "l1_wx_001")
	defaultEngine.l1Matcher.AddPattern("QQ号", "l1_qq_001")
	defaultEngine.l1Matcher.AddPattern("暗号", "l1_code_001")
	defaultEngine.l1Matcher.Build()

	// L2 示例
	defaultEngine.l2Matcher.AddPattern("敏感词1", "l2_sens_001")
	defaultEngine.l2Matcher.AddPattern("敏感词2", "l2_sens_002")
	defaultEngine.l2Matcher.AddPattern("敏感词3", "l2_sens_003")
	defaultEngine.l2Matcher.Build()
}

// deleteMessage 撤回消息
func (e *Engine) deleteMessage(accountID, groupJID, messageID string) error {
	// TODO: 使用 whatsmeow 撤回消息
	// 需要通过事件总线或其他方式调用 whatsapp manager
	return nil
}

// warnUser 警告用户（私聊发送警告）
func (e *Engine) warnUser(accountID, groupJID, userJID, reason string) error {
	// TODO: 发送私聊消息
	// 需要通过事件总线或其他方式调用 whatsapp manager
	return nil
}

// kickUser 踢出群聊
func (e *Engine) kickUser(accountID, groupJID, userJID string) error {
	// TODO: 使用 whatsmeow 踢人
	// 需要通过事件总线或其他方式调用 whatsapp manager
	return nil
}

// LogRiskEvent 记录风控事件
func (e *Engine) LogRiskEvent(accountID, groupJID, userJID, messageID, content string, matches []*RiskMatch, bm *BehaviorMatch) error {
	cfg := config.Get()
	if !cfg.Risk.Enabled {
		return nil
	}

	var level RiskLevel
	var ruleID, ruleName, matchedContent string
	var action ActionType
	var actionStatus string = "pending"

	if len(matches) > 0 {
		// 按级别和优先级排序
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Rule.Level != matches[j].Rule.Level {
				return matches[i].Rule.Level < matches[j].Rule.Level
			}
			return matches[i].Rule.Priority > matches[j].Rule.Priority
		})
		level = matches[0].Rule.Level
		ruleID = matches[0].Rule.ID
		ruleName = matches[0].Rule.Name
		matchedContent = matches[0].Matched
		action = matches[0].Rule.Action
	} else if bm != nil {
		level = Level2 // 行为异常归为L2
		ruleID = "behavior_" + bm.Type
		ruleName = "行为异常: " + bm.Type
		matchedContent = bm.Description
		action = ActionType(cfg.Risk.DefaultAction)
	} else {
		return nil
	}

	_, err := database.Exec(`
		INSERT INTO risk_events(
			account_id, group_jid, user_jid, level, rule_id, rule_name,
			matched_content, action, action_status, message_id, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s','now'))
	`, accountID, groupJID, userJID, level, ruleID, ruleName, matchedContent,
		action, actionStatus, messageID)
	return err
}

func boolToStr(b bool) string {
	if b {
		return "success"
	}
	return "failed"
}

// ====== 辅助类型和工具 ======

// Aho-Corasick 自动机占位
type ACAutomaton struct {
	patterns []string
	mu       sync.RWMutex
}

func (a *ACAutomaton) AddPattern(p string, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.patterns = append(a.patterns, p)
}

func (a *ACAutomaton) Build() {
	// TODO: 实际构建
}

func (a *ACAutomaton) Match(text string) []MatchResult {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// TODO: 实际匹配
	return nil
}

func (a *ACAutomaton) PatternCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.patterns)
}

type MatchResult struct {
	RuleID   string
	Text     string
	Position int
}

type regexpRegexp struct {
	re     *regexp.Regexp
	ruleID string
}

// TODO: 实现模型检测（可选）
