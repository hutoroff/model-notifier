package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Telegram sends messages via the Bot API sendMessage method.
type Telegram struct {
	Token, ChatID, BaseURL string
}

var tgClient = &http.Client{Timeout: 15 * time.Second}

func (t *Telegram) Send(ctx context.Context, msg string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":                  t.ChatID,
		"text":                     msg,
		"parse_mode":               "HTML",
		"disable_web_page_preview": false,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/bot"+t.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram: build request failed") // error would contain the token
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := tgClient.Do(req)
	if err != nil {
		// *url.Error embeds the URL, which contains the bot token; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram send: %s: %s", resp.Status, b)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}
