// Package telegram sends messages via the Telegram Bot API's sendMessage
// endpoint (PRD section 7: "Telegram Bot API ... atau webhook manual").
//
// This deliberately does not pull in a full bot-framework library:
// sendMessage is one simple JSON POST call, and this service only ever
// sends (bot-service, not built here, is what receives updates) — so
// net/http covers it without adding a dependency this service doesn't
// otherwise need.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func NewClient(baseURL, token string, timeout time.Duration, log *slog.Logger) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
		log:        log,
	}
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

// SendError wraps a failed Telegram API call. Permanent is true when
// retrying won't help (e.g. the user blocked the bot, or the chat no
// longer exists) — callers should stop retrying and move on rather than
// leaving the message pending forever.
type SendError struct {
	StatusCode  int
	ErrorCode   int
	Description string
	Permanent   bool
}

func (e *SendError) Error() string {
	return fmt.Sprintf("telegram sendMessage failed: http=%d error_code=%d description=%q", e.StatusCode, e.ErrorCode, e.Description)
}

// SendMessage posts text to chatID via bot{token}/sendMessage.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	if c.token == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}

	body, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text})
	if err != nil {
		return fmt.Errorf("marshal sendMessage request: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		c.log.Debug("telegram sendMessage request failed", "chat_id", chatID, "elapsed_ms", elapsed.Milliseconds(), "error", err)
		return fmt.Errorf("do sendMessage request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var parsed sendMessageResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		c.log.Debug("telegram sendMessage response undecodable", "chat_id", chatID, "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("decode sendMessage response: %w", err)
	}

	c.log.Debug("telegram sendMessage response",
		"chat_id", chatID, "status", resp.StatusCode, "ok", parsed.OK, "elapsed_ms", elapsed.Milliseconds(),
	)

	if !parsed.OK {
		return &SendError{
			StatusCode:  resp.StatusCode,
			ErrorCode:   parsed.ErrorCode,
			Description: parsed.Description,
			Permanent:   isPermanent(parsed.ErrorCode, parsed.Description),
		}
	}
	return nil
}

// isPermanent classifies Telegram API errors that will never succeed on
// retry: the bot was blocked/kicked, or the chat/user no longer exists.
// Everything else (rate limits, 5xx, network errors) is treated as
// transient and left pending for XAUTOCLAIM to retry.
func isPermanent(errorCode int, description string) bool {
	if errorCode == http.StatusForbidden {
		return true
	}
	if errorCode == http.StatusBadRequest {
		d := strings.ToLower(description)
		if strings.Contains(d, "chat not found") || strings.Contains(d, "user not found") || strings.Contains(d, "user is deactivated") {
			return true
		}
	}
	return false
}
