package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"whatsapp-ops/internal/config"
	"whatsapp-ops/internal/database"
	"whatsapp-ops/internal/events"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type AccountStatus string

const (
	StatusDisconnected AccountStatus = "disconnected"
	StatusConnecting   AccountStatus = "connecting"
	StatusQRReady      AccountStatus = "qr_ready"
	StatusConnected    AccountStatus = "connected"
	StatusBanned       AccountStatus = "banned"
	StatusError        AccountStatus = "error"
)

type Account struct {
	ID          string
	Name        string
	Phone       string
	JID         *types.JID
	ProxyURL    string
	Status      AccountStatus
	Client      *whatsmeow.Client
	Store       *sqlstore.Container
	QRCode      string // base64 PNG
	QRExpiresAt int64
	LastSeen    time.Time
	mu          sync.RWMutex
	cancelFunc  context.CancelFunc
	ctx         context.Context
	reconnectCh chan struct{}
	cfg         *config.Config
}

func (a *Account) setStatus(s AccountStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Status = s
	a.LastSeen = time.Now()
	// 持久化到主数据库
	go database.Exec(
		`UPDATE accounts SET status=?, last_seen=?, updated_at=strftime('%s','now') WHERE id=?`,
		s, a.LastSeen.Unix(), a.ID,
	)
	// 广播状态变更
	go events.BroadcastEvent("account_status", map[string]interface{}{
		"account_id": a.ID,
		"name":       a.Name,
		"status":     string(s),
		"jid":        a.JID,
	})
}

func (a *Account) getStatus() AccountStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status
}

func (a *Account) setQR(qr string, expires int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.QRCode = qr
	a.QRExpiresAt = expires
	go database.Exec(
		`UPDATE accounts SET qr_code=?, qr_expires_at=?, updated_at=strftime('%s','now') WHERE id=?`,
		qr, expires, a.ID,
	)
}

func (a *Account) clearQR() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.QRCode = ""
	a.QRExpiresAt = 0
	go database.Exec(
		`UPDATE accounts SET qr_code='', qr_expires_at=0, updated_at=strftime('%s','now') WHERE id=?`,
		a.ID,
	)
}

type Manager struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	cfg      *config.Config
	eventBus chan interface{} // 统一事件总线
}

// 全局管理器实例
var WaMgr *Manager

func NewManager() *Manager {
	mgr := &Manager{
		accounts: make(map[string]*Account),
		cfg:      config.Get(),
		eventBus: make(chan interface{}, 1000),
	}
	WaMgr = mgr
	return mgr
}

func (m *Manager) CreateAccount(id, name, proxyURL string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.accounts[id]; exists {
		return nil, fmt.Errorf("account %s already exists", id)
	}
	if len(m.accounts) >= m.cfg.WhatsApp.MaxAccounts {
		return nil, fmt.Errorf("max accounts limit reached")
	}

	acc := &Account{
		ID:          id,
		Name:        name,
		ProxyURL:    proxyURL,
		Status:      StatusDisconnected,
		reconnectCh: make(chan struct{}, 1),
		cfg:         m.cfg,
	}
	m.accounts[id] = acc

	// 持久化
	_, err := database.Exec(
		`INSERT INTO accounts(id, name, proxy_url, status, created_at, updated_at) VALUES(?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))`,
		id, name, proxyURL, StatusDisconnected,
	)
	return acc, err
}

func (m *Manager) GetAccount(id string) (*Account, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.accounts[id]
	return acc, ok
}

func (m *Manager) ListAccounts() []*Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Account, 0, len(m.accounts))
	for _, a := range m.accounts {
		list = append(list, a)
	}
	return list
}

func (m *Manager) DeleteAccount(id string) error {
	m.mu.Lock()
	acc, ok := m.accounts[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("account not found")
	}
	// 停止连接
	acc.Stop()
	database.CloseAccountDB(id)
	// 删除会话文件
	sessionPath := filepath.Join(m.cfg.Database.SessionsDir, fmt.Sprintf("session_%s.db", id))
	os.Remove(sessionPath)
	os.Remove(sessionPath + "-wal")
	os.Remove(sessionPath + "-shm")

	m.mu.Lock()
	delete(m.accounts, id)
	m.mu.Unlock()

	_, err := database.Exec(`DELETE FROM accounts WHERE id=?`, id)
	return err
}

func (m *Manager) LoadAccounts() error {
	rows, err := database.Query(`SELECT id, name, phone, jid, proxy_url, status FROM accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, phone, jidStr, proxy, status string
		if err := rows.Scan(&id, &name, &phone, &jidStr, &proxy, &status); err != nil {
			continue
		}
		acc := &Account{
			ID:          id,
			Name:        name,
			Phone:       phone,
			ProxyURL:    proxy,
			Status:      AccountStatus(status),
			reconnectCh: make(chan struct{}, 1),
			cfg:         m.cfg,
		}
		if jidStr != "" {
			if j, err := types.ParseJID(jidStr); err == nil {
				acc.JID = &j
			}
		}
		m.mu.Lock()
		m.accounts[id] = acc
		m.mu.Unlock()
	}
	return nil
}

// StartAccount 启动账号连接
func (a *Account) Start() error {
	if a.getStatus() == StatusConnecting || a.getStatus() == StatusConnected {
		return nil // 已经在连接中或已连接
	}
	a.setStatus(StatusConnecting)

	// 创建会话存储
	sessionDB, err := database.GetAccountDB(a.ID)
	if err != nil {
		a.setStatus(StatusError)
		return fmt.Errorf("session db: %w", err)
	}

	container := sqlstore.NewWithDB(sessionDB, "sqlite", waLog.Stdout("Database", "DEBUG", true))
	a.Store = container

	// 获取设备存储
	deviceStore, err := container.GetFirstDevice(a.ctx)
	if err != nil {
		a.setStatus(StatusError)
		return fmt.Errorf("get device: %w", err)
	}

	// 创建客户端
	clientLog := waLog.Stdout("Client", "INFO", true)
	a.Client = whatsmeow.NewClient(deviceStore, clientLog)

	// 配置代理
	// TODO: 根据 whatsmeow API 实现代理配置
	_ = a.ProxyURL

	// 事件处理
	a.ctx, a.cancelFunc = context.WithCancel(context.Background())
	go a.eventLoop()

	// 连接
	if err := a.Client.Connect(); err != nil {
		a.setStatus(StatusError)
		return fmt.Errorf("connect: %w", err)
	}

	// 检查是否已登录
	if a.Client.Store.ID == nil {
		// 需要扫码
		a.startQRRefresh()
	} else {
		a.setStatus(StatusConnected)
		a.JID = a.Client.Store.ID
		a.Phone = a.JID.User
		// 更新数据库
		database.Exec(`UPDATE accounts SET phone=?, jid=?, updated_at=strftime('%s','now') WHERE id=?`,
			a.Phone, a.JID.String(), a.ID)
		// 同步联系人/群组
		go a.syncContactsAndGroups()
	}

	return nil
}

func (a *Account) startQRRefresh() {
	a.setStatus(StatusQRReady)
	qrChan, _ := a.Client.GetQRChannel(a.ctx)
	go func() {
		for {
			select {
			case evt := <-qrChan:
				if evt.Event == "code" {
					// 生成二维码图片
					var pngBytes []byte
					pngBytes, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					if err != nil {
						zap.L().Error("生成二维码失败", zap.Error(err))
						continue
					}
					b64 := base64.StdEncoding.EncodeToString(pngBytes)
					a.setQR("data:image/png;base64,"+b64, time.Now().Add(45*time.Second).Unix())
				} else if evt.Event == "success" {
					a.clearQR()
					a.setStatus(StatusConnected)
					a.JID = a.Client.Store.ID
					a.Phone = a.JID.User
					database.Exec(`UPDATE accounts SET phone=?, jid=?, updated_at=strftime('%s','now') WHERE id=?`,
						a.Phone, a.JID.String(), a.ID)
					go a.syncContactsAndGroups()
				} else if evt.Event == "timeout" || evt.Event == "error" {
					a.setStatus(StatusDisconnected)
					// 触发重连
					select {
					case a.reconnectCh <- struct{}{}:
					default:
					}
				}
			case <-a.ctx.Done():
				return
			}
		}
	}()
}

func (a *Account) eventLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.reconnectCh:
			if a.cfg.WhatsApp.AutoReconnect {
				time.Sleep(time.Duration(a.cfg.WhatsApp.ReconnectInterval) * time.Second)
				a.Start()
			}
		}
	}
}

func (a *Account) handleEvent(evt interface{}) {
	// TODO: 实现事件处理
	// whatsmeow 事件需要通过事件总线分发
}

func (a *Account) syncContactsAndGroups() {
	if !a.cfg.WhatsApp.SyncContacts {
		return
	}
	// 同步联系人
	contacts, err := a.Client.Store.Contacts.GetAllContacts(a.ctx)
	if err == nil {
		for jid, contact := range contacts {
			name := contact.FullName
			if name == "" {
				name = contact.PushName
			}
			database.Exec(`
				INSERT OR REPLACE INTO contacts(jid, name, phone, avatar, updated_at)
				VALUES(?, ?, ?, ?, strftime('%s','now'))
			`, jid.String(), name, jid.User, "")
		}
	}

	// 同步群组
	groups, err := a.Client.GetJoinedGroups(a.ctx)
	if err == nil {
		for _, g := range groups {
			database.Exec(`
				INSERT OR REPLACE INTO groups(jid, name, owner_jid, participants_count, is_announce_only, updated_at)
				VALUES(?, ?, ?, ?, ?, strftime('%s','now'))
			`, g.JID.String(), g.Name, "", len(g.Participants), g.IsAnnounce)
			// 同步成员
			for _, p := range g.Participants {
				database.Exec(`
					INSERT OR REPLACE INTO group_members(group_jid, user_jid, name, phone, is_admin, is_super_admin, joined_at)
					VALUES(?, ?, ?, ?, ?, ?, ?)
				`, g.JID.String(), p.JID.String(), "", p.JID.User, p.IsAdmin, p.IsSuperAdmin, time.Now().Unix())
			}
		}
	}
}

func (a *Account) Stop() {
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	if a.Client != nil {
		a.Client.Disconnect()
	}
	a.setStatus(StatusDisconnected)
}

func (a *Account) RefreshQR() error {
	if a.Client == nil {
		return fmt.Errorf("client not initialized")
	}
	if a.getStatus() != StatusQRReady && a.getStatus() != StatusConnecting {
		// 只有在等待二维码或连接中时才能刷新
		return fmt.Errorf("account not in QR state")
	}
	// 重新启动二维码刷新 - 这会获取新的 QR channel
	a.startQRRefresh()
	return nil
}

func (a *Account) SendMessage(jidStr string, msg *waE2E.Message) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}
	resp, err := a.Client.SendMessage(a.ctx, jid, msg)
	return resp.ID, err
}

func (a *Account) SendText(jidStr, text string, mentionedJIDs []string) (string, error) {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	if len(mentionedJIDs) > 0 {
		// TODO: 根据 whatsmeow API 实现提及功能
		// 新版本 API 可能有变化
	}
	return a.SendMessage(jidStr, msg)
}

// SendSimpleText 发送简单文本消息（便捷方法）
func (a *Account) SendSimpleText(jidStr, text string) (string, error) {
	return a.SendText(jidStr, text, nil)
}

func (a *Account) SetGroupPrivacy(jidStr string, announceOnly bool) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	return a.Client.SetGroupAnnounce(a.ctx, jid, announceOnly)
}

func (a *Account) GetGroupMembers(jidStr string) ([]*types.GroupParticipant, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return nil, err
	}
	info, err := a.Client.GetGroupInfo(a.ctx, jid)
	if err != nil {
		return nil, err
	}
	// 转换 []types.GroupParticipant -> []*types.GroupParticipant
	participants := make([]*types.GroupParticipant, len(info.Participants))
	for i := range info.Participants {
		participants[i] = &info.Participants[i]
	}
	return participants, nil
}

func (a *Account) RemoveGroupParticipant(groupJID, userJID string) error {
	gjid, _ := types.ParseJID(groupJID)
	ujid, _ := types.ParseJID(userJID)
	_, err := a.Client.UpdateGroupParticipants(a.ctx, gjid, []types.JID{ujid}, whatsmeow.ParticipantChangeRemove)
	return err
}

func (a *Account) PromoteGroupParticipant(groupJID, userJID string) error {
	gjid, _ := types.ParseJID(groupJID)
	ujid, _ := types.ParseJID(userJID)
	_, err := a.Client.UpdateGroupParticipants(a.ctx, gjid, []types.JID{ujid}, whatsmeow.ParticipantChangePromote)
	return err
}

func (a *Account) DemoteGroupParticipant(groupJID, userJID string) error {
	gjid, _ := types.ParseJID(groupJID)
	ujid, _ := types.ParseJID(userJID)
	_, err := a.Client.UpdateGroupParticipants(a.ctx, gjid, []types.JID{ujid}, whatsmeow.ParticipantChangeDemote)
	return err
}
