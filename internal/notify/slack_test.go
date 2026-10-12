package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMrkdwn(t *testing.T) {
	for in, want := range map[string]string{
		"🆕 <b>Anthropic</b> listed: <b>a&lt;b &amp; c</b>":      "🆕 *Anthropic* listed: *a&lt;b &amp; c*",
		"ID: <code>claude-x</code>\n<i>it&#39;s</i> <s>old</s>": "ID: `claude-x`\n_it's_ ~old~",
		`<a href="https://x.dev/a?b=1&amp;c=2">Docs</a>`:        "<https://x.dev/a?b=1&c=2|Docs>",
		"<u>u</u><blockquote>q</blockquote><pre>p</pre>":        "uq```p```",
	} {
		if got := mrkdwn(in); got != want {
			t.Errorf("mrkdwn(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestSlackSend(t *testing.T) {
	var got map[string]any
	reply := `{"ok":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" || r.Header.Get("Authorization") != "Bearer xoxb-T" {
			t.Errorf("path %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(reply))
	}))
	defer srv.Close()
	s := &Slack{Token: "xoxb-T", Channel: "C1", BaseURL: srv.URL}
	if err := s.Send(context.Background(), "<b>hi</b>"); err != nil {
		t.Fatal(err)
	}
	if got["channel"] != "C1" || got["text"] != "*hi*" {
		t.Fatalf("bad body %v", got)
	}
	reply = `{"ok":false,"error":"not_in_channel"}`
	if err := s.Send(context.Background(), "x"); err == nil || err.Error() != "slack send: not_in_channel" {
		t.Fatalf("want not_in_channel, got %v", err)
	}
}
