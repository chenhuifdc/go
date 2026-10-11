package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"whatsapp-ops/internal/config"
	"whatsapp-ops/internal/database"
	"whatsapp-ops/internal/events"
	"whatsapp-ops/internal/risk"
	"whatsapp-ops/internal/static"
	"whatsapp-ops/internal/translation"
	"whatsapp-ops/internal/whatsapp"
	"go.uber.org/zap"
)

var (
	waMgr       *whatsapp.Manager
	transClient *translation.Client
)

func NewRouter(waMgr *whatsapp.Manager) http.Handler {
	// 初始化翻译客户端
	cfg := config.Get()
	if cfg.Translation.Enabled && cfg.Translation.BaseURL != "" {
		transClient = translation.NewClient(&cfg.Translation, zap.L())
	}

	r := chi.NewRouter()

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 静态文件服务（前端 - 内嵌模式）
	r.Get("/*", static.ServeStatic)

	// WebSocket 实时事件
	r.Get("/ws", events.HandleWebSocket)

	// API 路由组
	r.Route("/api", func(r chi.Router) {
		// 账号管理
			r.Route("/accounts", func(r chi.Router) {
				r.Get("/", GetAccounts)
				r.Post("/", CreateAccount)
				r.Get("/{id}", GetAccount)
				r.Put("/{id}", UpdateAccount)
				r.Delete("/{id}", DeleteAccount)
				r.Post("/{id}/connect", ConnectAccount)
				r.Post("/{id}/disconnect", DisconnectAccount)
				r.Post("/{id}/qr/refresh", RefreshAccountQR)
				r.Get("/{id}/qr", GetAccountQR)
				r.Get("/{id}/status", GetAccountStatus)
			})

		// CRM
		r.Route("/crm", func(r chi.Router) {
			r.Route("/contacts", func(r chi.Router) {
				r.Get("/", GetContacts)
				r.Post("/", CreateContact)
				r.Get("/{jid}", GetContact)
				r.Put("/{jid}", UpdateContact)
				r.Delete("/{jid}", DeleteContact)
				r.Get("/search", SearchContacts)
				r.Post("/import", ImportContacts)
				r.Get("/export", ExportContacts)
			})
			r.Route("/tags", func(r chi.Router) {
				r.Get("/", GetTags)
				r.Post("/", CreateTag)
				r.Put("/{id}", UpdateTag)
				r.Delete("/{id}", DeleteTag)
			})
			r.Route("/followups", func(r chi.Router) {
				r.Get("/", GetFollowups)
				r.Post("/", CreateFollowup)
				r.Delete("/{id}", DeleteFollowup)
			})
		})

		// 群组管理
		r.Route("/groups", func(r chi.Router) {
			r.Get("/", GetGroups)
			r.Get("/{jid}", GetGroup)
			r.Get("/{jid}/members", GetGroupMembers)
			r.Post("/{jid}/toggle", ToggleGroupPrivacy)
			r.Post("/{jid}/export", ExportGroupMembers)
			r.Post("/{jid}/kick", KickGroupMember)
			r.Post("/{jid}/promote", PromoteGroupMember)
			r.Post("/{jid}/demote", DemoteGroupMember)
		})

		// 群发
		r.Route("/broadcast", func(r chi.Router) {
			r.Post("/", CreateBroadcastTask)
			r.Get("/", GetBroadcastTasks)
			r.Get("/{id}", GetBroadcastTask)
			r.Put("/{id}", UpdateBroadcastTask)
			r.Delete("/{id}", DeleteBroadcastTask)
			r.Post("/{id}/start", StartBroadcastTask)
			r.Post("/{id}/pause", PauseBroadcastTask)
			r.Post("/{id}/cancel", CancelBroadcastTask)
			r.Get("/{id}/progress", GetBroadcastProgress)
			r.Get("/{id}/logs", GetBroadcastTaskLogs)
		})

		// 风控
		r.Route("/risk", func(r chi.Router) {
			r.Get("/events", GetRiskEvents)
			r.Get("/rules", GetRiskRules)
			r.Post("/rules", CreateRiskRule)
			r.Put("/{id}", UpdateRiskRule)
			r.Delete("/{id}", DeleteRiskRule)
			r.Post("/test", TestRiskRule)
			r.Get("/words", GetRiskWords)
			r.Post("/words", UploadRiskWords)
			r.Delete("/words", DeleteRiskWords)
		})

		// 翻译
		r.Route("/translate", func(r chi.Router) {
			r.Post("/", TranslateText)
			r.Post("/batch", BatchTranslate)
			r.Post("/detect", DetectLanguage)
			r.Get("/cache", GetTranslationCache)
			r.Delete("/cache", ClearTranslationCache)
			r.Get("/settings", GetTranslationSettings)
			r.Put("/settings", UpdateTranslationSettings)
		})

		// 统计/监控
		r.Route("/stats", func(r chi.Router) {
			r.Get("/overview", GetStatsOverview)
			r.Get("/accounts", GetAccountStats)
			r.Get("/messages", GetMessageStats)
			r.Get("/risk", GetRiskStats)
		})

		// 配置
		r.Route("/config", func(r chi.Router) {
			r.Get("/", GetConfig)
			r.Put("/", UpdateConfig)
			r.Get("/example", GetConfigExample)
		})

		// 健康检查
		r.Get("/health", HealthCheck)
	})

	return r
}

// ====== 账号相关 ======
func GetAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := []map[string]interface{}{}
	for _, acc := range waMgr.ListAccounts() {
		jidStr := ""
		if acc.JID != nil {
			jidStr = acc.JID.String()
		}
		accounts = append(accounts, map[string]interface{}{
			"id":         acc.ID,
			"name":       acc.Name,
			"phone":      acc.Phone,
			"jid":        jidStr,
			"proxy":      acc.ProxyURL,
			"status":     string(acc.getStatus()),
			"last_seen":  acc.LastSeen.Unix(),
			"has_qr":     acc.QRCode != "",
		})
	}
	json.NewEncoder(w).Encode(accounts)
}

func CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ProxyURL string `json:"proxy_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// 生成账号ID（时间戳+随机）
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	acc, err := waMgr.CreateAccount(id, req.Name, req.ProxyURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": acc.ID,
	})
}

func GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	jidStr := ""
	if acc.JID != nil {
		jidStr = acc.JID.String()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        acc.ID,
		"name":      acc.Name,
		"phone":     acc.Phone,
		"jid":       jidStr,
		"proxy":     acc.ProxyURL,
		"status":    string(acc.getStatus()),
		"last_seen": acc.LastSeen.Unix(),
		"qr_code":   acc.QRCode,
	})
}

func UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name     string `json:"name"`
		ProxyURL string `json:"proxy_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := database.Exec(`
		UPDATE accounts SET name=?, proxy_url=?, updated_at=strftime('%s','now') WHERE id=?`,
		req.Name, req.ProxyURL, id,
	); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := waMgr.DeleteAccount(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func StartAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	if err := acc.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func StopAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	acc.Stop()
	w.WriteHeader(http.StatusNoContent)
}

func GetAccountQR(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	if acc.QRCode == "" {
		http.Error(w, "QR code not available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"qr_code": acc.QRCode,
		"expires": strconv.FormatInt(acc.QRExpiresAt, 10),
	})
}

func GetAccountStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": string(acc.getStatus()),
	})
}

func ConnectAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	if err := acc.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "connecting"})
}

func DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	acc.Stop()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "disconnected"})
}

func RefreshAccountQR(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	acc, ok := waMgr.GetAccount(id)
	if !ok {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	// 触发二维码刷新 - 通过 manager 的 refresh 方法
	if err := acc.RefreshQR(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "qr refresh triggered"})
}

// ====== CRM 相关 ======
func CreateContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JID    string `json:"jid"`
		Name   string `json:"name"`
		Phone  string `json:"phone"`
		Avatar string `json:"avatar"`
		Tags   string `json:"tags"` // JSON array string
		Notes  string `json:"notes"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.JID == "" {
		http.Error(w, "jid is required", http.StatusBadRequest)
		return
	}
	if req.Tags == "" {
		req.Tags = "[]"
	}

	_, err := database.Exec(`
		INSERT OR REPLACE INTO contacts(jid, name, phone, avatar, tags, notes, source, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, strftime('%s','now'))
	`, req.JID, req.Name, req.Phone, req.Avatar, req.Tags, req.Notes, req.Source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"jid": req.JID})
}

func GetContact(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	row := database.QueryRow(`
		SELECT jid, name, phone, avatar, tags, notes, source, created_at, updated_at
		FROM contacts WHERE jid=?
	`, jid)
	var c struct {
		JID, Name, Phone, Avatar, Tags, Notes, Source string
		CreatedAt, UpdatedAt                          int64
	}
	if err := row.Scan(&c.JID, &c.Name, &c.Phone, &c.Avatar, &c.Tags, &c.Notes, &c.Source, &c.CreatedAt, &c.UpdatedAt); err != nil {
		http.Error(w, "contact not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(c)
}

func UpdateContact(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	var req struct {
		Name   string `json:"name"`
		Phone  string `json:"phone"`
		Avatar string `json:"avatar"`
		Tags   string `json:"tags"`
		Notes  string `json:"notes"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := database.Exec(`
		UPDATE contacts SET name=?, phone=?, avatar=?, tags=?, notes=?, source=?, updated_at=strftime('%s','now')
		WHERE jid=?
	`, req.Name, req.Phone, req.Avatar, req.Tags, req.Notes, req.Source, jid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DeleteContact(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	_, err := database.Exec(`DELETE FROM contacts WHERE jid=?`, jid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func SearchContacts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' required", http.StatusBadRequest)
		return
	}
	rows, err := database.Query(`
		SELECT jid, name, phone, avatar, tags, notes, source, created_at, updated_at
		FROM contacts
		WHERE name LIKE ? OR phone LIKE ? OR jid LIKE ?
		ORDER BY name ASC LIMIT 50
	`, "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var contacts []map[string]interface{}
	for rows.Next() {
		var jid, name, phone, avatar, tags, notes, source string
		var createdAt, updatedAt int64
		if err := rows.Scan(&jid, &name, &phone, &avatar, &tags, &notes, &source, &createdAt, &updatedAt); err != nil {
			continue
		}
		contacts = append(contacts, map[string]interface{}{
			"jid": jid, "name": name, "phone": phone, "avatar": avatar,
			"tags": tags, "notes": notes, "source": source,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	json.NewEncoder(w).Encode(contacts)
}

func ImportContacts(w http.ResponseWriter, r *http.Request) {
	// TODO: 支持 CSV/JSON 文件上传
	http.Error(w, "not implemented yet", http.StatusNotImplemented)
}

func ExportContacts(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT jid, name, phone, avatar, tags, notes, source, created_at, updated_at
		FROM contacts ORDER BY name ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// 输出 CSV
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=contacts.csv")
	fmt.Fprintln(w, "JID,Name,Phone,Avatar,Tags,Notes,Source,CreatedAt,UpdatedAt")
	for rows.Next() {
		var jid, name, phone, avatar, tags, notes, source string
		var createdAt, updatedAt int64
		if err := rows.Scan(&jid, &name, &phone, &avatar, &tags, &notes, &source, &createdAt, &updatedAt); err != nil {
			continue
		}
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%d,%d\n",
			jid, name, phone, avatar, tags, notes, source, createdAt, updatedAt)
	}
}

// ====== 标签管理 ======
func GetTags(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`SELECT id, name, color FROM tags ORDER BY name`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []map[string]interface{}
	for rows.Next() {
		var id int
		var name, color string
		if err := rows.Scan(&id, &name, &color); err != nil {
			continue
		}
		tags = append(tags, map[string]interface{}{
			"id":   id,
			"name": name,
			"color": color,
		})
	}
	json.NewEncoder(w).Encode(tags)
}

func CreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Color == "" {
		req.Color = "#409EFF"
	}

	result, err := database.Exec(`INSERT INTO tags(name, color) VALUES(?, ?)`, req.Name, req.Color)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, _ := result.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func UpdateTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := database.Exec(`UPDATE tags SET name=?, color=? WHERE id=?`, req.Name, req.Color, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DeleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`DELETE FROM tags WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ====== 跟进记录 ======
func GetFollowups(w http.ResponseWriter, r *http.Request) {
	contactJID := r.URL.Query().Get("contact_jid")
	if contactJID == "" {
		http.Error(w, "contact_jid required", http.StatusBadRequest)
		return
	}
	rows, err := database.Query(`
		SELECT id, contact_jid, content, operator, created_at
		FROM followups WHERE contact_jid=? ORDER BY created_at DESC
	`, contactJID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var followups []map[string]interface{}
	for rows.Next() {
		var id int
		var contactJID, content, operator string
		var createdAt int64
		if err := rows.Scan(&id, &contactJID, &content, &operator, &createdAt); err != nil {
			continue
		}
		followups = append(followups, map[string]interface{}{
			"id": id, "contact_jid": contactJID, "content": content,
			"operator": operator, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(followups)
}

func CreateFollowup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContactJID string `json:"contact_jid"`
		Content    string `json:"content"`
		Operator   string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ContactJID == "" || req.Content == "" {
		http.Error(w, "contact_jid and content required", http.StatusBadRequest)
		return
	}
	res, err := database.Exec(`
		INSERT INTO followups(contact_jid, content, operator) VALUES(?, ?, ?)
	`, req.ContactJID, req.Content, req.Operator)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func DeleteFollowup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`DELETE FROM followups WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ====== 群组管理 ======
func GetGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT jid, name, owner_jid, participants_count, is_announce_only, created_at, updated_at
		FROM groups ORDER BY name ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var jid, name, ownerJID string
		var participantsCount int
		var isAnnounceOnly bool
		var createdAt, updatedAt int64
		if err := rows.Scan(&jid, &name, &ownerJID, &participantsCount, &isAnnounceOnly, &createdAt, &updatedAt); err != nil {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"jid":              jid,
			"name":             name,
			"owner_jid":        ownerJID,
			"participants_count": participantsCount,
			"is_announce_only": isAnnounceOnly,
			"created_at":       createdAt,
			"updated_at":       updatedAt,
		})
	}
	json.NewEncoder(w).Encode(groups)
}

func GetGroup(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	row := database.QueryRow(`
		SELECT jid, name, owner_jid, participants_count, is_announce_only, created_at, updated_at
		FROM groups WHERE jid=?
	`, jid)
	var g struct {
		JID              string
		Name             string
		OwnerJID         string
		ParticipantsCount int
		IsAnnounceOnly   bool
		CreatedAt, UpdatedAt int64
	}
	if err := row.Scan(&g.JID, &g.Name, &g.OwnerJID, &g.ParticipantsCount, &g.IsAnnounceOnly, &g.CreatedAt, &g.UpdatedAt); err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(g)
}

func GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	rows, err := database.Query(`
		SELECT jid, name, is_admin, is_super_admin, joined_at
		FROM group_members WHERE group_jid=? ORDER BY is_super_admin DESC, is_admin DESC
	`, jid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var jid, name string
		var isAdmin, isSuperAdmin bool
		var joinedAt int64
		if err := rows.Scan(&jid, &name, &isAdmin, &isSuperAdmin, &joinedAt); err != nil {
			continue
		}
		members = append(members, map[string]interface{}{
			"jid": jid, "name": name, "is_admin": isAdmin,
			"is_super_admin": isSuperAdmin, "joined_at": joinedAt,
		})
	}
	json.NewEncoder(w).Encode(members)
}

func ToggleGroupPrivacy(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	var req struct {
		AnnounceOnly bool `json:"announce_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 获取第一个可用账号执行操作
	accounts := waMgr.ListAccounts()
	if len(accounts) == 0 {
		http.Error(w, "no accounts available", http.StatusServiceUnavailable)
		return
	}
	acc := accounts[0]

	// 调用 whatsmeow 设置群公告模式
		if err := acc.SetGroupPrivacy(jid, req.AnnounceOnly); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 更新本地数据库
	_, err := database.Exec(`UPDATE groups SET is_announce_only=?, updated_at=strftime('%s','now') WHERE jid=?`, req.AnnounceOnly, jid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ExportGroupMembers(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	rows, err := database.Query(`
		SELECT jid, name, is_admin, is_super_admin, joined_at
		FROM group_members WHERE group_jid=? ORDER BY is_super_admin DESC, is_admin DESC
	`, jid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=group_members_%s.csv", jid))
	fmt.Fprintln(w, "JID,Name,IsAdmin,IsSuperAdmin,JoinedAt")
	for rows.Next() {
		var jid, name string
		var isAdmin, isSuperAdmin bool
		var joinedAt int64
		if err := rows.Scan(&jid, &name, &isAdmin, &isSuperAdmin, &joinedAt); err != nil {
			continue
		}
		fmt.Fprintf(w, "%s,%s,%v,%v,%d\n", jid, name, isAdmin, isSuperAdmin, joinedAt)
	}
}

func KickGroupMember(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	var req struct {
		TargetJID string `json:"target_jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TargetJID == "" {
		http.Error(w, "target_jid required", http.StatusBadRequest)
		return
	}

	accounts := waMgr.ListAccounts()
	if len(accounts) == 0 {
		http.Error(w, "no accounts available", http.StatusServiceUnavailable)
		return
	}
	acc := accounts[0]

	if err := acc.RemoveGroupParticipant(jid, req.TargetJID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func PromoteGroupMember(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	var req struct {
		TargetJID string `json:"target_jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TargetJID == "" {
		http.Error(w, "target_jid required", http.StatusBadRequest)
		return
	}

	accounts := waMgr.ListAccounts()
	if len(accounts) == 0 {
		http.Error(w, "no accounts available", http.StatusServiceUnavailable)
		return
	}
	acc := accounts[0]

	if err := acc.PromoteGroupParticipant(jid, req.TargetJID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DemoteGroupMember(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	var req struct {
		TargetJID string `json:"target_jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TargetJID == "" {
		http.Error(w, "target_jid required", http.StatusBadRequest)
		return
	}

	accounts := waMgr.ListAccounts()
	if len(accounts) == 0 {
		http.Error(w, "no accounts available", http.StatusServiceUnavailable)
		return
	}
	acc := accounts[0]

	if err := acc.DemoteGroupParticipant(jid, req.TargetJID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ====== 群发 ======
func CreateBroadcastTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		AccountID   string   `json:"account_id"`
		Content     string   `json:"content"`
		MediaURL    string   `json:"media_url,omitempty"`
		TargetType  string   `json:"target_type"` // "contacts" or "groups"
		TargetJIDs  []string `json:"target_jids"`
		IntervalSec int      `json:"interval_sec"`
		AtAll       bool     `json:"at_all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.AccountID == "" || req.Content == "" || len(req.TargetJIDs) == 0 {
		http.Error(w, "name, account_id, content, target_jids required", http.StatusBadRequest)
		return
	}
	if req.IntervalSec == 0 {
		req.IntervalSec = 3
	}

	targetJIDs, _ := json.Marshal(req.TargetJIDs)
	res, err := database.Exec(`
		INSERT INTO broadcast_tasks(name, account_id, content, media_url, target_type, target_jids, interval_sec, at_all, status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'pending', strftime('%s','now'), strftime('%s','now'))
	`, req.Name, req.AccountID, req.Content, req.MediaURL, req.TargetType, string(targetJIDs), req.IntervalSec, req.AtAll)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func GetBroadcastTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT id, name, account_id, content, media_url, target_type, target_jids, interval_sec, at_all, status, progress_total, progress_sent, progress_failed, created_at, started_at, finished_at, updated_at
		FROM broadcast_tasks ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tasks []map[string]interface{}
	for rows.Next() {
		var id int
		var name, accountID, content, mediaURL, targetType, targetJIDs string
		var intervalSec int
		var atAll bool
		var status string
		var progressTotal, progressSent, progressFailed int
		var createdAt, startedAt, finishedAt, updatedAt int64
		if err := rows.Scan(&id, &name, &accountID, &content, &mediaURL, &targetType, &targetJIDs, &intervalSec, &atAll, &status, &progressTotal, &progressSent, &progressFailed, &createdAt, &startedAt, &finishedAt, &updatedAt); err != nil {
			continue
		}
		tasks = append(tasks, map[string]interface{}{
			"id": id, "name": name, "account_id": accountID, "content": content,
			"media_url": mediaURL, "target_type": targetType, "target_jids": targetJIDs,
			"interval_sec": intervalSec, "at_all": atAll, "status": status,
			"progress_total": progressTotal, "progress_sent": progressSent, "progress_failed": progressFailed,
			"created_at": createdAt, "started_at": startedAt, "finished_at": finishedAt, "updated_at": updatedAt,
		})
	}
	json.NewEncoder(w).Encode(tasks)
}

func GetBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row := database.QueryRow(`
		SELECT id, name, account_id, content, media_url, target_type, target_jids, interval_sec, at_all, status, progress_total, progress_sent, progress_failed, created_at, started_at, finished_at, updated_at
		FROM broadcast_tasks WHERE id=?
	`, id)
	var task struct {
		ID             int
		Name           string
		AccountID      string
		Content        string
		MediaURL       string
		TargetType     string
		TargetJIDs     string
		IntervalSec    int
		AtAll          bool
		Status         string
		ProgressTotal  int
		ProgressSent   int
		ProgressFailed int
		CreatedAt      int64
		StartedAt      int64
		FinishedAt     int64
		UpdatedAt      int64
	}
	if err := row.Scan(&task.ID, &task.Name, &task.AccountID, &task.Content, &task.MediaURL, &task.TargetType, &task.TargetJIDs, &task.IntervalSec, &task.AtAll, &task.Status, &task.ProgressTotal, &task.ProgressSent, &task.ProgressFailed, &task.CreatedAt, &task.StartedAt, &task.FinishedAt, &task.UpdatedAt); err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(task)
}

func UpdateBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string   `json:"name"`
		Content     string   `json:"content"`
		MediaURL    string   `json:"media_url"`
		TargetType  string   `json:"target_type"`
		TargetJIDs  []string `json:"target_jids"`
		IntervalSec int      `json:"interval_sec"`
		AtAll       bool     `json:"at_all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	targetJIDs, _ := json.Marshal(req.TargetJIDs)
	_, err := database.Exec(`
		UPDATE broadcast_tasks SET name=?, content=?, media_url=?, target_type=?, target_jids=?, interval_sec=?, at_all=?, updated_at=strftime('%s','now') WHERE id=?
	`, req.Name, req.Content, req.MediaURL, req.TargetType, string(targetJIDs), req.IntervalSec, req.AtAll, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DeleteBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`DELETE FROM broadcast_tasks WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func StartBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// TODO: 实际启动群发任务（由后台调度器处理）
	_, err := database.Exec(`UPDATE broadcast_tasks SET status='running', started_at=strftime('%s','now') WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func PauseBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`UPDATE broadcast_tasks SET status='paused' WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func CancelBroadcastTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`UPDATE broadcast_tasks SET status='cancelled', finished_at=strftime('%s','now') WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func GetBroadcastTaskLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := database.Query(`
		SELECT id, task_id, target_jid, status, error_msg, sent_at
		FROM broadcast_logs WHERE task_id=? ORDER BY sent_at DESC
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var logID, taskID int
		var targetJID, status, errorMsg string
		var sentAt int64
		if err := rows.Scan(&logID, &taskID, &targetJID, &status, &errorMsg, &sentAt); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"id": logID, "task_id": taskID, "target_jid": targetJID,
			"status": status, "error_msg": errorMsg, "sent_at": sentAt,
		})
	}
	json.NewEncoder(w).Encode(logs)
}

// ====== 风控 ======
func GetRiskEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT id, account_id, chat_jid, sender_jid, level, rule_id, action, message, created_at
		FROM risk_events ORDER BY created_at DESC LIMIT 200
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id int
		var accountID, chatJID, senderJID, level, ruleID, action, message string
		var createdAt int64
		if err := rows.Scan(&id, &accountID, &chatJID, &senderJID, &level, &ruleID, &action, &message, &createdAt); err != nil {
			continue
		}
		events = append(events, map[string]interface{}{
			"id": id, "account_id": accountID, "chat_jid": chatJID, "sender_jid": senderJID,
			"level": level, "rule_id": ruleID, "action": action, "message": message, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(events)
}

func GetRiskRules(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT id, name, level, keywords, regex, action, enabled, created_at, updated_at
		FROM risk_rules ORDER BY level ASC, id ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rules []map[string]interface{}
	for rows.Next() {
		var id int
		var name, level, keywords, regex, action string
		var enabled bool
		var createdAt, updatedAt int64
		if err := rows.Scan(&id, &name, &level, &keywords, &regex, &action, &enabled, &createdAt, &updatedAt); err != nil {
			continue
		}
		rules = append(rules, map[string]interface{}{
			"id": id, "name": name, "level": level, "keywords": keywords,
			"regex": regex, "action": action, "enabled": enabled,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	json.NewEncoder(w).Encode(rules)
}

func CreateRiskRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Level    string `json:"level"`     // "L1", "L2", "L3"
		Keywords string `json:"keywords"`  // 逗号分隔
		Regex    string `json:"regex"`     // 正则
		Action   string `json:"action"`    // "delete", "warn", "kick"
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Level == "" || req.Action == "" {
		http.Error(w, "name, level, action required", http.StatusBadRequest)
		return
	}
	res, err := database.Exec(`
		INSERT INTO risk_rules(name, level, keywords, regex, action, enabled, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))
	`, req.Name, req.Level, req.Keywords, req.Regex, req.Action, req.Enabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	// 触发风控引擎热重载
	// TODO: 调用 riskEngine.ReloadRules()
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func UpdateRiskRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name     string `json:"name"`
		Level    string `json:"level"`
		Keywords string `json:"keywords"`
		Regex    string `json:"regex"`
		Action   string `json:"action"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := database.Exec(`
		UPDATE risk_rules SET name=?, level=?, keywords=?, regex=?, action=?, enabled=?, updated_at=strftime('%s','now') WHERE id=?
	`, req.Name, req.Level, req.Keywords, req.Regex, req.Action, req.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func DeleteRiskRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := database.Exec(`DELETE FROM risk_rules WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestRiskRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text   string `json:"text"`
		Level  string `json:"level"`
		RuleID int    `json:"rule_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: 实际测试逻辑
	json.NewEncoder(w).Encode(map[string]interface{}{
		"matched": false,
		"level":   req.Level,
		"rule_id": req.RuleID,
	})
}

func GetRiskWords(w http.ResponseWriter, r *http.Request) {
	// 从文件读取风控词库
	// TODO: 实现
	json.NewEncoder(w).Encode(map[string]string{"words": ""})
}

func UploadRiskWords(w http.ResponseWriter, r *http.Request) {
	// TODO: 文件上传解析
	w.WriteHeader(http.StatusNotImplemented)
}

func DeleteRiskWords(w http.ResponseWriter, r *http.Request) {
	// TODO: 清空词库
	w.WriteHeader(http.StatusNotImplemented)
}

// ====== 翻译 ======
func TranslateText(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text      string `json:"text"`
		TargetLang string `json:"target_lang,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}
	if req.TargetLang == "" {
		cfg := config.Get()
		req.TargetLang = cfg.Translation.TargetLang
	}

	if transClient == nil {
		renderError(w, http.StatusServiceUnavailable, "翻译服务未配置")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	translated, err := transClient.Translate(ctx, req.Text, req.TargetLang)
	if err != nil {
		zap.L().Error("翻译失败", zap.Error(err))
		renderError(w, http.StatusInternalServerError, "翻译失败: "+err.Error())
		return
	}

	resp := map[string]string{
		"original":    req.Text,
		"translated":  translated,
		"source_lang": "auto",
		"target_lang": req.TargetLang,
	}
	json.NewEncoder(w).Encode(resp)
}

func DetectLanguage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if transClient == nil {
		renderError(w, http.StatusServiceUnavailable, "翻译服务未配置")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	lang, err := transClient.DetectLanguage(ctx, req.Text)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"text":     req.Text,
		"language": lang,
	})
}

func GetTranslationCache(w http.ResponseWriter, r *http.Request) {
	if transClient == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"size":      0,
			"max_size":  1000,
			"enabled":   false,
		})
		return
	}
	stats := transClient.GetCacheStats()
	stats["enabled"] = true
	json.NewEncoder(w).Encode(stats)
}

func ClearTranslationCache(w http.ResponseWriter, r *http.Request) {
	if transClient != nil {
		transClient.ClearCache()
	}
	w.WriteHeader(http.StatusNoContent)
}

func GetTranslationSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":        cfg.Translation.Enabled,
		"provider":       cfg.Translation.Provider,
		"base_url":       cfg.Translation.BaseURL,
		"model":          cfg.Translation.Model,
		"target_lang":    cfg.Translation.TargetLang,
		"prompt_template": cfg.Translation.PromptTemplate,
	})
}

func UpdateTranslationSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider       string `json:"provider"`
		BaseURL        string `json:"base_url"`
		APIKey         string `json:"api_key"`
		Model          string `json:"model"`
		TargetLang     string `json:"target_lang"`
		PromptTemplate string `json:"prompt_template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cfg := config.Get()
	changed := false
	if req.Provider != "" {
		cfg.Translation.Provider = req.Provider
		changed = true
	}
	if req.BaseURL != "" {
		cfg.Translation.BaseURL = req.BaseURL
		changed = true
	}
	if req.APIKey != "" {
		cfg.Translation.APIKey = req.APIKey
		changed = true
	}
	if req.Model != "" {
		cfg.Translation.Model = req.Model
		changed = true
	}
	if req.TargetLang != "" {
		cfg.Translation.TargetLang = req.TargetLang
		changed = true
	}
	if req.PromptTemplate != "" {
		cfg.Translation.PromptTemplate = req.PromptTemplate
		changed = true
	}

	// 持久化到配置文件
	if changed {
		if err := config.Save(cfg); err != nil {
			zap.L().Error("保存翻译配置失败", zap.Error(err))
			renderError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}

		// 重新初始化翻译客户端
		if cfg.Translation.Enabled && cfg.Translation.BaseURL != "" {
			transClient = translation.NewClient(&cfg.Translation, zap.L())
		} else {
			transClient = nil
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ====== 统计 ======
func GetStatsOverview(w http.ResponseWriter, r *http.Request) {
	// 账号数
	accRow := database.QueryRow(`SELECT COUNT(*) FROM accounts`)
	var totalAccounts int
	accRow.Scan(&totalAccounts)

	// 联系人数
	contactRow := database.QueryRow(`SELECT COUNT(*) FROM contacts`)
	var totalContacts int
	contactRow.Scan(&totalContacts)

	// 群组数
	groupRow := database.QueryRow(`SELECT COUNT(*) FROM groups`)
	var totalGroups int
	groupRow.Scan(&totalGroups)

	// 今日消息数
	msgRow := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE datetime(created_at/1000, 'unixepoch') >= date('now')`)
	var todayMessages int
	msgRow.Scan(&todayMessages)

	// 待发送群发
	bcRow := database.QueryRow(`SELECT COUNT(*) FROM broadcast_tasks WHERE status='pending'`)
	var pendingBroadcasts int
	bcRow.Scan(&pendingBroadcasts)

	// 风控事件 (最近7天)
	riskRow := database.QueryRow(`SELECT COUNT(*) FROM risk_events WHERE created_at >= strftime('%s','now','-7 days')`)
	var recentRiskEvents int
	riskRow.Scan(&recentRiskEvents)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_accounts":        totalAccounts,
		"total_contacts":        totalContacts,
		"total_groups":          totalGroups,
		"today_messages":        todayMessages,
		"pending_broadcasts":    pendingBroadcasts,
		"recent_risk_events":    recentRiskEvents,
	})
}

func GetAccountStats(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query(`
		SELECT id, name, jid, proxy, status, last_seen, created_at
		FROM accounts ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var accounts []map[string]interface{}
	for rows.Next() {
		var id, name, jid, proxy, status string
		var lastSeen, createdAt int64
		if err := rows.Scan(&id, &name, &jid, &proxy, &status, &lastSeen, &createdAt); err != nil {
			continue
		}
		// 获取该账号的消息统计
		msgRow := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE account_id=?`, id)
		var msgCount int
		msgRow.Scan(&msgCount)

		accounts = append(accounts, map[string]interface{}{
			"id": id, "name": name, "jid": jid, "proxy": proxy,
			"status": status, "last_seen": lastSeen, "created_at": createdAt,
			"message_count": msgCount,
		})
	}
	json.NewEncoder(w).Encode(accounts)
}

func GetMessageStats(w http.ResponseWriter, r *http.Request) {
	// 最近 7 天消息趋势
	rows, err := database.Query(`
		SELECT date(created_at/1000, 'unixepoch') as day, COUNT(*) as count
		FROM messages WHERE created_at >= strftime('%s','now','-7 days')
		GROUP BY day ORDER BY day ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var trend []map[string]interface{}
	for rows.Next() {
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			continue
		}
		trend = append(trend, map[string]interface{}{"day": day, "count": count})
	}

	// 消息类型分布
	typeRows, _ := database.Query(`
		SELECT type, COUNT(*) FROM messages GROUP BY type
	`)
	var typeDist []map[string]interface{}
	for typeRows.Next() {
		var t string
		var c int
		typeRows.Scan(&t, &c)
		typeDist = append(typeDist, map[string]interface{}{"type": t, "count": c})
	}
	typeRows.Close()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"trend": trend, "type_distribution": typeDist,
	})
}

func GetRiskStats(w http.ResponseWriter, r *http.Request) {
	// 风控等级分布
	rows, err := database.Query(`
		SELECT level, COUNT(*) FROM risk_events GROUP BY level
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var levelDist []map[string]interface{}
	for rows.Next() {
		var level string
		var count int
		rows.Scan(&level, &count)
		levelDist = append(levelDist, map[string]interface{}{"level": level, "count": count})
	}

	// 最近风控事件
	recentRows, _ := database.Query(`
		SELECT id, account_id, chat_jid, sender_jid, level, action, message, created_at
		FROM risk_events ORDER BY created_at DESC LIMIT 20
	`)
	var recent []map[string]interface{}
	for recentRows.Next() {
		var id int
		var accountID, chatJID, senderJID, level, action, message string
		var createdAt int64
		recentRows.Scan(&id, &accountID, &chatJID, &senderJID, &level, &action, &message, &createdAt)
		recent = append(recent, map[string]interface{}{
			"id": id, "account_id": accountID, "chat_jid": chatJID, "sender_jid": senderJID,
			"level": level, "action": action, "message": message, "created_at": createdAt,
		})
	}
	recentRows.Close()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"level_distribution": levelDist, "recent_events": recent,
	})
}

// ====== 配置 ======
func GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	// 隐藏敏感信息
	cfg.Translation.APIKey = "********"
	json.NewEncoder(w).Encode(cfg)
}

func UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cfg := config.Get()

	// 更新应用配置
	if app, ok := req["app"].(map[string]interface{}); ok {
		if v, ok := app["host"].(string); ok && v != "" {
			cfg.App.Host = v
		}
		if v, ok := app["port"].(float64); ok {
			cfg.App.Port = int(v)
		}
		if v, ok := app["data_dir"].(string); ok && v != "" {
			cfg.App.DataDir = v
		}
		if v, ok := app["log_level"].(string); ok && v != "" {
			cfg.App.LogLevel = v
		}
	}

	// 更新数据库配置
	if db, ok := req["database"].(map[string]interface{}); ok {
		if v, ok := db["path"].(string); ok && v != "" {
			cfg.Database.Path = v
		}
		if v, ok := db["wal_mode"].(bool); ok {
			cfg.Database.WALMode = v
		}
	}

	// 更新翻译配置 (不包含 APIKey)
	if trans, ok := req["translation"].(map[string]interface{}); ok {
		if v, ok := trans["enabled"].(bool); ok {
			cfg.Translation.Enabled = v
		}
		if v, ok := trans["provider"].(string); ok && v != "" {
			cfg.Translation.Provider = v
		}
		if v, ok := trans["base_url"].(string); ok && v != "" {
			cfg.Translation.BaseURL = v
		}
		if v, ok := trans["model"].(string); ok && v != "" {
			cfg.Translation.Model = v
		}
		if v, ok := trans["target_lang"].(string); ok && v != "" {
			cfg.Translation.TargetLang = v
		}
		if v, ok := trans["prompt_template"].(string); ok && v != "" {
			cfg.Translation.PromptTemplate = v
		}
	}

	// 更新风控配置
	if risk, ok := req["risk"].(map[string]interface{}); ok {
		if v, ok := risk["enabled"].(bool); ok {
			cfg.Risk.Enabled = v
		}
		if v, ok := risk["l1_action"].(string); ok && v != "" {
			cfg.Risk.L1Action = v
		}
		if v, ok := risk["l2_action"].(string); ok && v != "" {
			cfg.Risk.L2Action = v
		}
		if v, ok := risk["l3_action"].(string); ok && v != "" {
			cfg.Risk.L3Action = v
		}
		if v, ok := risk["wordlist_path"].(string); ok && v != "" {
			cfg.Risk.WordlistPath = v
		}
		if v, ok := risk["auto_reload"].(bool); ok {
			cfg.Risk.AutoReload = v
		}
	}

	// 更新群发配置
	if bc, ok := req["broadcast"].(map[string]interface{}); ok {
		if v, ok := bc["default_interval_sec"].(float64); ok {
			cfg.Broadcast.DefaultIntervalSec = int(v)
		}
		if v, ok := bc["max_retry"].(float64); ok {
			cfg.Broadcast.MaxRetry = int(v)
		}
		if v, ok := bc["batch_size"].(float64); ok {
			cfg.Broadcast.BatchSize = int(v)
		}
		if v, ok := bc["at_all_tag"].(string); ok && v != "" {
			cfg.Broadcast.AtAllTag = v
		}
	}

	// TODO: 持久化到数据库/配置文件
	w.WriteHeader(http.StatusNoContent)
}

func GetConfigExample(w http.ResponseWriter, r *http.Request) {
	path := "./configs/config.yaml.example"
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(data)
}

// GetBroadcastProgress 获取群发任务进度
func GetBroadcastProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		renderError(w, http.StatusBadRequest, "缺少任务 ID")
		return
	}

	db := database.GetDB()
	var progress struct {
		Total     int    `json:"total"`
		Sent      int    `json:"sent"`
		Failed    int    `json:"failed"`
		Pending   int    `json:"pending"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
	}

	err := db.QueryRow(`
		SELECT total, sent, failed, pending, status, updated_at
		FROM broadcast_tasks WHERE id = ?
	`, id).Scan(&progress.Total, &progress.Sent, &progress.Failed, &progress.Pending, &progress.Status, &progress.UpdatedAt)

	if err != nil {
		renderError(w, http.StatusNotFound, "任务不存在")
		return
	}

	json.NewEncoder(w).Encode(progress)
}

// BatchTranslate 批量翻译
func BatchTranslate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Texts      []string `json:"texts"`
		TargetLang string   `json:"target_lang"`
		SourceLang string   `json:"source_lang"`
		AccountID  string   `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Texts) == 0 {
		renderError(w, http.StatusBadRequest, "texts 不能为空")
		return
	}

	if transClient == nil {
		renderError(w, http.StatusServiceUnavailable, "翻译服务未配置")
		return
	}

	if req.TargetLang == "" {
		cfg := config.Get()
		req.TargetLang = cfg.Translation.TargetLang
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	results, err := transClient.BatchTranslate(ctx, req.Texts, req.TargetLang, 5)
	if err != nil {
		zap.L().Error("批量翻译失败", zap.Error(err))
		renderError(w, http.StatusInternalServerError, "批量翻译失败: "+err.Error())
		return
	}

	resp := make([]map[string]string, len(req.Texts))
	for i, text := range req.Texts {
		translated := results[i]
		if strings.TrimSpace(text) == "" {
			translated = ""
		}
		resp[i] = map[string]string{
			"original":    text,
			"translated":  translated,
			"source_lang": "auto",
			"target_lang": req.TargetLang,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": resp,
		"count":   len(resp),
	})
}

// ====== 健康检查 ======
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// 辅助函数
func renderError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}