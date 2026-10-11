package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"whatsapp-ops/internal/config"
	"go.uber.org/zap"
)

// Client 翻译客户端（OpenAI 兼容）
type Client struct {
	httpClient *http.Client
	cfg        *config.TranslationConfig
	logger     *zap.Logger
	cache      *sync.Map // key -> translated text
	cacheOrder []string  // LRU 顺序
	cacheMu    sync.Mutex
	maxCache   int
}

// OpenAI 兼容请求/响应结构
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// NewClient 创建翻译客户端
func NewClient(cfg *config.TranslationConfig, logger *zap.Logger) *Client {
	if cfg == nil {
		cfg = &config.Get().Translation
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		cfg:      cfg,
		logger:   logger,
		cache:    &sync.Map{},
		cacheOrder: make([]string, 0),
		maxCache: 1000, // 最多缓存 1000 条
	}
}

// Translate 单条翻译
func (c *Client) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	// 检查缓存
	cacheKey := c.cacheKey(text, targetLang)
	if cached, ok := c.cache.Load(cacheKey); ok {
		c.moveToFront(cacheKey)
		return cached.(string), nil
	}

	// 构建 prompt
	prompt := c.buildPrompt(text, targetLang)

	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are a professional translator."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from translation API")
	}

	translated := strings.TrimSpace(chatResp.Choices[0].Message.Content)

	// 写入缓存
	c.setCache(cacheKey, translated)

	return translated, nil
}

// BatchTranslate 批量翻译（并发控制）
func (c *Client) BatchTranslate(ctx context.Context, texts []string, targetLang string, concurrency int) ([]string, error) {
	if concurrency <= 0 {
		concurrency = 5
	}

	results := make([]string, len(texts))
	errs := make([]error, len(texts))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, text := range texts {
		if strings.TrimSpace(text) == "" {
			results[i] = ""
			continue
		}

		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			translated, err := c.Translate(ctx, t, targetLang)
			if err != nil {
				errs[idx] = err
				results[idx] = t // 失败时回退原文
			} else {
				results[idx] = translated
			}
		}(i, text)
	}

	wg.Wait()

	// 检查是否有错误
	for i, err := range errs {
		if err != nil {
			c.logger.Warn("翻译失败", zap.Int("index", i), zap.Error(err))
			// 不返回 error，保持结果数组长度一致
		}
	}

	return results, nil
}

// DetectLanguage 语言检测（简单实现：调用翻译 API 让其检测）
func (c *Client) DetectLanguage(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "unknown", nil
	}

	prompt := fmt.Sprintf(`Detect the language of the following text. Respond with ONLY the language code (e.g., "zh", "en", "ja", "ko", "es", "fr", "de", "ru", "ar", "pt", "it", "nl", "pl", "tr", "vi", "th", "id", "ms", "hi", "bn", "ta", "te", "mr", "gu", "kn", "ml", "or", "pa", "as", "ur", "fa", "he", "sw", "zu", "af", "sq", "am", "hy", "az", "be", "bg", "bs", "ca", "ceb", "co", "cs", "cy", "da", "eo", "et", "eu", "fa", "fi", "fr", "fy", "ga", "gd", "gl", "gu", "ha", "haw", "hi", "hmn", "hr", "ht", "hu", "hy", "id", "ig", "is", "it", "iw", "ja", "jw", "ka", "kk", "km", "kn", "ko", "ku", "ky", "la", "lb", "lo", "lt", "lv", "mg", "mi", "mk", "ml", "mn", "mr", "ms", "mt", "my", "ne", "nl", "no", "ny", "or", "pa", "pl", "ps", "pt", "ro", "ru", "rw", "sd", "si", "sk", "sl", "sm", "sn", "so", "sq", "sr", "st", "su", "sv", "sw", "ta", "te", "tg", "th", "tk", "tl", "tr", "tt", "ug", "uk", "ur", "uz", "vi", "xh", "yi", "yo", "zh", "zu"):
%s`, text)

	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   10,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "auto", err
	}
	defer resp.Body.Close()

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "auto", err
	}

	if len(chatResp.Choices) > 0 {
		lang := strings.TrimSpace(strings.ToLower(chatResp.Choices[0].Message.Content))
		// 简单验证：只取前 2-3 字符
		if len(lang) >= 2 {
			return lang[:2], nil
		}
	}
	return "auto", nil
}

// buildPrompt 构建翻译提示词
func (c *Client) buildPrompt(text, targetLang string) string {
	template := c.cfg.PromptTemplate
	if template == "" {
		template = "将以下内容翻译成 {{.TargetLang}}，只返回翻译结果，不要解释：\n\n{{.Text}}"
	}

	// 简单的模板替换
	prompt := strings.ReplaceAll(template, "{{.TargetLang}}", targetLang)
	prompt = strings.ReplaceAll(prompt, "{{.Text}}", text)
	prompt = strings.ReplaceAll(prompt, "{{.SourceLang}}", "auto")

	return prompt
}

// cacheKey 生成缓存键
func (c *Client) cacheKey(text, targetLang string) string {
	return fmt.Sprintf("%s|%s", targetLang, text)
}

// setCache 设置缓存（LRU）
func (c *Client) setCache(key, value string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// 如果已存在，移到前面
	for i, k := range c.cacheOrder {
		if k == key {
			c.cacheOrder = append(c.cacheOrder[:i], c.cacheOrder[i+1:]...)
			break
		}
	}

	// 添加到前面
	c.cacheOrder = append([]string{key}, c.cacheOrder...)
	c.cache.Store(key, value)

	// 超过最大值，删除最后
	if len(c.cacheOrder) > c.maxCache {
		oldest := c.cacheOrder[len(c.cacheOrder)-1]
		c.cache.Delete(oldest)
		c.cacheOrder = c.cacheOrder[:len(c.cacheOrder)-1]
	}
}

// moveToFront 移到前面（LRU 访问更新）
func (c *Client) moveToFront(key string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	for i, k := range c.cacheOrder {
		if k == key {
			c.cacheOrder = append(c.cacheOrder[:i], c.cacheOrder[i+1:]...)
			c.cacheOrder = append([]string{key}, c.cacheOrder...)
			break
		}
	}
}

// GetCacheStats 获取缓存统计
func (c *Client) GetCacheStats() map[string]interface{} {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	return map[string]interface{}{
		"size":      len(c.cacheOrder),
		"max_size":  c.maxCache,
	}
}

// ClearCache 清空缓存
func (c *Client) ClearCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache = &sync.Map{}
	c.cacheOrder = make([]string, 0)
}