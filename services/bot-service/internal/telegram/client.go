// Package telegram implements just enough of the Telegram Bot API — long
// polling via getUpdates, sendMessage, answerCallbackQuery — to run
// bot-service without a bot-framework dependency (same reasoning as
// telegram-notifier-service: these are a handful of simple JSON HTTP calls,
// net/http covers it without adding a library whose features this service
// mostly wouldn't use).
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	log        *slog.Logger
}

func NewClient(baseURL, token string, httpTimeout time.Duration, log *slog.Logger) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: httpTimeout},
		log:        log,
	}
}

func (c *Client) url(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
}

// envelope is the {ok, result, error_code, description} shape every
// Telegram Bot API response shares.
type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

// GetUpdates long-polls for new updates starting after offset, blocking
// server-side for up to timeoutSec. Callers should set an HTTP client
// timeout comfortably above timeoutSec (see config.HTTPClientTimeout).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	body, _ := json.Marshal(map[string]any{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	})

	var updates []Update
	if err := c.post(ctx, "getUpdates", body, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

type sendMessageRequest struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

// SendMessage sends text to chatID, optionally attaching an
// InlineKeyboardMarkup, ReplyKeyboardMarkup, or ReplyKeyboardRemove.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, replyMarkup any) error {
	body, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text, ReplyMarkup: replyMarkup})
	if err != nil {
		return fmt.Errorf("marshal sendMessage request: %w", err)
	}
	return c.post(ctx, "sendMessage", body, nil)
}

// AnswerCallbackQuery acknowledges a button press so Telegram stops
// showing the button's loading spinner; text (optional) shows a small
// toast to the user.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error {
	body, _ := json.Marshal(map[string]any{
		"callback_query_id": callbackQueryID,
		"text":              text,
	})
	return c.post(ctx, "answerCallbackQuery", body, nil)
}

// post issues one Telegram Bot API call and, on success, unmarshals the
// `result` field into out (skipped if out is nil).
func (c *Client) post(ctx context.Context, method string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(method), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return fmt.Errorf("do %s request: %w", method, err)
	}
	defer resp.Body.Close()

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}

	c.log.Debug("telegram api call", "method", method, "status", resp.StatusCode, "ok", env.OK, "elapsed_ms", elapsed.Milliseconds())

	if !env.OK {
		return fmt.Errorf("telegram api %s failed: error_code=%d description=%q", method, env.ErrorCode, env.Description)
	}

	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}
