package broadcast

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsapp-ops/internal/config"
	"whatsapp-ops/internal/database"
	"whatsapp-ops/internal/risk"
	"whatsapp-ops/internal/whatsapp"
	"go.uber.org/zap"
)

// Scheduler 群发调度器
type Scheduler struct {
	waMgr     *whatsapp.Manager
	riskEngine *risk.Engine
	cfg       *config.BroadcastConfig
	logger    *zap.Logger

	mu          sync.Mutex
	running     bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
	activeTasks map[string]*taskRunner // taskID -> runner
}

// taskRunner 单个任务的执行器
type taskRunner struct {
	taskID     string
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	progress   *Progress
	mu         sync.Mutex
	stopped    bool
}

// Progress 任务进度
type Progress struct {
	TaskID      string  `json:"task_id"`
	Total       int     `json:"total"`
	Sent        int     `json:"sent"`
	Failed      int     `json:"failed"`
	Pending     int     `json:"pending"`
	Percent     float64 `json:"percent"`
	Status      string  `json:"status"` // running, paused, completed, cancelled, failed
	CurrentJID  string  `json:"current_jid"`
	UpdatedAt   int64   `json:"updated_at"`
	ErrorMsg    string  `json:"error_msg,omitempty"`
}

// NewScheduler 创建调度器
func NewScheduler(waMgr *whatsapp.Manager, riskEngine *risk.Engine, logger *zap.Logger) *Scheduler {
	cfg := config.Get()
	return &Scheduler{
		waMgr:      waMgr,
		riskEngine: riskEngine,
		cfg:        &cfg.Broadcast,
		logger:     logger,
		stopCh:     make(chan struct{}),
		activeTasks: make(map[string]*taskRunner),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}
	s.running = true
	s.logger.Info("群发调度器已启动")
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	close(s.stopCh)
	s.stopCh = make(chan struct{})
	s.running = false

	// 取消所有运行中的任务
	for _, runner := range s.activeTasks {
		runner.cancel()
	}
	s.logger.Info("群发调度器已停止")
}

// ExecuteTask 执行群发任务
func (s *Scheduler) ExecuteTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.activeTasks[taskID]; exists {
		return fmt.Errorf("任务已在执行中")
	}

	// 读取任务详情
	task, err := s.getTask(taskID)
	if err != nil {
		return fmt.Errorf("读取任务失败: %w", err)
	}

	if task == nil {
		return fmt.Errorf("任务不存在")
	}

	if task.Status != "pending" && task.Status != "paused" {
		return fmt.Errorf("任务状态不允许执行: %s", task.Status)
	}

	// 更新状态为运行中
	if err := s.updateTaskStatus(taskID, "running", ""); err != nil {
		return err
	}

	// 创建任务运行器
	ctx, cancel := context.WithCancel(context.Background())
	runner := &taskRunner{
		taskID:  taskID,
		ctx:     ctx,
		cancel:  cancel,
		progress: &Progress{
			TaskID:    taskID,
			Total:     len(task.Targets),
			Pending:   len(task.Targets),
			Status:    "running",
			UpdatedAt: time.Now().UnixMilli(),
		},
	}

	s.activeTasks[taskID] = runner

	// 启动执行协程
	runner.wg.Add(1)
	go s.runTask(runner, task)

	return nil
}

// runTask 执行任务主循环
func (s *Scheduler) runTask(runner *taskRunner, task *BroadcastTask) {
	defer runner.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.activeTasks, runner.taskID)
		s.mu.Unlock()
	}()

	// 随机化目标顺序（避免检测）
	targets := make([]string, len(task.Targets))
	copy(targets, task.Targets)
	rand.Shuffle(len(targets), func(i, j int) {
		targets[i], targets[j] = targets[j], targets[i]
	})

	for _, target := range targets {
		select {
		case <-runner.ctx.Done():
			s.updateProgress(runner, func(p *Progress) {
				p.Status = "cancelled"
			})
			s.updateTaskStatus(runner.taskID, "cancelled", "")
			return
		default:
		}

		// 检查暂停
		s.mu.Lock()
		paused := false
		if t, err := s.getTask(runner.taskID); err == nil && t != nil {
			paused = t.Status == "paused"
		}
		s.mu.Unlock()

		if paused {
			time.Sleep(2 * time.Second)
			continue
		}

		// 发送单条消息
		s.sendToTarget(runner, task, target)

		// 间隔延迟
		interval := s.cfg.DefaultInterval
		if task.IntervalMs > 0 {
			interval = task.IntervalMs
		}
		// 添加随机抖动 ±20%
		jitter := interval / 5
		actualInterval := interval - jitter + rand.Intn(2*jitter)
		time.Sleep(time.Duration(actualInterval) * time.Millisecond)
	}

	// 任务完成
	finalStatus := "completed"
	errMsg := ""
	s.mu.Lock()
	p := runner.progress
	s.mu.Unlock()

	if p.Failed > 0 && p.Sent == 0 {
		finalStatus = "failed"
		errMsg = "所有消息发送失败"
	} else if p.Failed > 0 {
		finalStatus = "completed_with_errors"
		errMsg = fmt.Sprintf("部分失败: %d/%d", p.Failed, p.Total)
	}

	s.updateProgress(runner, func(p *Progress) {
		p.Status = finalStatus
		p.ErrorMsg = errMsg
	})
	s.updateTaskStatus(runner.taskID, finalStatus, errMsg)

	s.logger.Info("群发任务完成", zap.String("task_id", runner.taskID),
		zap.Int("sent", p.Sent), zap.Int("failed", p.Failed))
}

// sendToTarget 发送消息给单个目标
func (s *Scheduler) sendToTarget(runner *taskRunner, task *BroadcastTask, target string) {
	s.updateProgress(runner, func(p *Progress) {
		p.CurrentJID = target
		p.Pending--
	})

	// 获取账号客户端
	acc, err := s.waMgr.GetAccount(task.AccountID)
	if err != nil || acc == nil || acc.Client == nil {
		s.recordFailure(runner, target, "账号未连接")
		return
	}

	// 风控检查
	if s.riskEngine != nil && task.Content != "" {
		check := s.riskEngine.CheckMessage(target, task.Content, task.AccountID)
		if check.Action != risk.ActionNone {
			s.recordFailure(runner, target, fmt.Sprintf("风控拦截: %s", check.Action))
			return
		}
	}

	// 发送消息（重试逻辑）
	var lastErr error
	for attempt := 0; attempt <= s.cfg.RetryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(s.cfg.RetryInterval) * time.Second)
		}

		err = acc.SendSimpleText(target, task.Content)
		if err == nil {
			s.recordSuccess(runner, target)
			return
		}
		lastErr = err
		s.logger.Warn("发送失败，准备重试",
			zap.String("target", target),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	s.recordFailure(runner, target, lastErr.Error())
}

// sendMessage 实际发送消息
func (s *Scheduler) sendMessage(acc *whatsapp.Account, target, content string) error {
	// 解析目标 JID
	jid := target
	if !strings.Contains(target, "@") {
		// 如果是纯数字，转为 JID
		jid = target + "@s.whatsapp.net"
	}

	// 这里调用 whatsmeow 发送
	// 实际实现需要在 whatsapp manager 中暴露发送方法
	return acc.SendMessage(jid, content)
}

// recordSuccess 记录成功
func (s *Scheduler) recordSuccess(runner *taskRunner, target string) {
	s.updateProgress(runner, func(p *Progress) {
		p.Sent++
		p.Percent = float64(p.Sent+p.Failed) / float64(p.Total) * 100
	})
	s.appendLog(runner.taskID, target, "success", "")
}

// recordFailure 记录失败
func (s *Scheduler) recordFailure(runner *taskRunner, target, errMsg string) {
	s.updateProgress(runner, func(p *Progress) {
		p.Failed++
		p.Percent = float64(p.Sent+p.Failed) / float64(p.Total) * 100
	})
	s.appendLog(runner.taskID, target, "failed", errMsg)
}

// updateProgress 更新进度（线程安全）
func (s *Scheduler) updateProgress(runner *taskRunner, fn func(*Progress)) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.stopped {
		return
	}
	fn(runner.progress)
	runner.progress.UpdatedAt = time.Now().UnixMilli()
}

// GetProgress 获取进度
func (s *Scheduler) GetProgress(taskID string) *Progress {
	s.mu.Lock()
	runner, exists := s.activeTasks[taskID]
	s.mu.Unlock()

	if !exists {
		// 从数据库读取
		return s.getProgressFromDB(taskID)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.progress
}

// PauseTask 暂停任务
func (s *Scheduler) PauseTask(taskID string) error {
	s.mu.Lock()
	runner, exists := s.activeTasks[taskID]
	s.mu.Unlock()

	if exists {
		s.updateProgress(runner, func(p *Progress) {
			p.Status = "paused"
		})
	}

	return s.updateTaskStatus(taskID, "paused", "")
}

// ResumeTask 恢复任务
func (s *Scheduler) ResumeTask(taskID string) error {
	s.mu.Lock()
	runner, exists := s.activeTasks[taskID]
	s.mu.Unlock()

	if exists {
		s.updateProgress(runner, func(p *Progress) {
			p.Status = "running"
		})
	}

	return s.updateTaskStatus(taskID, "running", "")
}

// CancelTask 取消任务
func (s *Scheduler) CancelTask(taskID string) error {
	s.mu.Lock()
	runner, exists := s.activeTasks[taskID]
	s.mu.Unlock()

	if exists {
		runner.cancel()
	}

	return s.updateTaskStatus(taskID, "cancelled", "")
}

// 数据库操作辅助方法

type BroadcastTask struct {
	ID          string
	AccountID   string
	Name        string
	Content     string
	ContentType string // text, image, video, document
	MediaURL    string
	Targets     []string
	IntervalMs  int
	Status      string
	CreatedAt   int64
	UpdatedAt   int64
}

func (s *Scheduler) getTask(taskID string) (*BroadcastTask, error) {
	row := database.QueryRow(`
		SELECT id, account_id, name, content, content_type, media_url, targets, interval_ms, status, created_at, updated_at
		FROM broadcast_tasks WHERE id = ?`, taskID)

	var task BroadcastTask
	var targetsJSON string
	err := row.Scan(&task.ID, &task.AccountID, &task.Name, &task.Content,
		&task.ContentType, &task.MediaURL, &targetsJSON, &task.IntervalMs,
		&task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(targetsJSON), &task.Targets)
	return &task, nil
}

func (s *Scheduler) updateTaskStatus(taskID, status, errMsg string) error {
	_, err := database.Exec(`
		UPDATE broadcast_tasks SET status = ?, error_msg = ?, updated_at = ?
		WHERE id = ?`, status, errMsg, time.Now().UnixMilli(), taskID)
	return err
}

func (s *Scheduler) appendLog(taskID, target, status, errMsg string) {
	_, _ = database.Exec(`
		INSERT INTO broadcast_logs (task_id, target_jid, status, error_msg, created_at)
		VALUES (?, ?, ?, ?, ?)`, taskID, target, status, errMsg, time.Now().UnixMilli())
}

func (s *Scheduler) getProgressFromDB(taskID string) *Progress {
	row := database.QueryRow(`
		SELECT total, sent, failed, pending, status, updated_at
		FROM broadcast_tasks WHERE id = ?`, taskID)

	var p Progress
	p.TaskID = taskID
	err := row.Scan(&p.Total, &p.Sent, &p.Failed, &p.Pending, &p.Status, &p.UpdatedAt)
	if err != nil {
		return &Progress{TaskID: taskID, Status: "unknown"}
	}
	p.Percent = float64(p.Sent+p.Failed) / float64(p.Total) * 100
	return &p
}

// GetActiveTasks 获取运行中的任务列表
func (s *Scheduler) GetActiveTasks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.activeTasks))
	for id := range s.activeTasks {
		ids = append(ids, id)
	}
	return ids
}

// IsRunning 检查任务是否在运行
func (s *Scheduler) IsRunning(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.activeTasks[taskID]
	return exists
}