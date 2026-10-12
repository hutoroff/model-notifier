package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Slack sends messages via the Web API chat.postMessage method.
type Slack struct {
	Token, Channel, BaseURL string
}

var slackClient = &http.Client{Timeout: 15 * time.Second}

func (s *Slack) Send(ctx context.Context, msg string) error {
	body, err := json.Marshal(map[string]any{
		"channel":      s.Channel,
		"text":         mrkdwn(msg),
		"unfurl_links": true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := slackClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("slack send: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack send: %s: %.512s", resp.Status, b)
	}
	// Slack reports API errors as 200 + {"ok":false}.
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("slack send: decode response: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("slack send: %s", r.Error)
	}
	return nil
}

var (
	htmlTag  = regexp.MustCompile(`<(/?)(\w+)([^>]*)>`)
	hrefAttr = regexp.MustCompile(`href="([^"]*)"`)
	slackEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	mdMarks  = map[string]string{"b": "*", "i": "_", "s": "~", "code": "`", "pre": "```"}
)

// mrkdwn converts the Telegram HTML subset produced by templates into Slack mrkdwn.
// ponytail: u and blockquote are dropped (no underline in Slack, quotes are per-line "> ").
func mrkdwn(s string) string {
	var b strings.Builder
	text := func(t string) { b.WriteString(slackEsc.Replace(html.UnescapeString(t))) }
	last := 0
	for _, m := range htmlTag.FindAllStringSubmatchIndex(s, -1) {
		text(s[last:m[0]])
		last = m[1]
		closing, tag, attrs := s[m[2]:m[3]] == "/", s[m[4]:m[5]], s[m[6]:m[7]]
		switch {
		case tag == "a" && closing:
			b.WriteString(">")
		case tag == "a":
			href := ""
			if h := hrefAttr.FindStringSubmatch(attrs); h != nil {
				href = html.UnescapeString(h[1])
			}
			b.WriteString("<" + href + "|")
		default:
			b.WriteString(mdMarks[tag])
		}
	}
	text(s[last:])
	return b.String()
}
