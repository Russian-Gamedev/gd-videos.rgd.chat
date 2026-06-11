package tme_parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIndexHTML(t *testing.T) {
	path := filepath.Join("testdata", "channel.html")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("index.html not found:", err)
	}
	defer f.Close()

	b := make([]byte, 1<<20)
	n, _ := f.Read(b)
	html := string(b[:n])

	page, err := Parse(html)
	if err != nil {
		t.Fatal("Parse error:", err)
	}

	if page.Channel.Title != "GDV" {
		t.Errorf("expected title 'GDV', got %q", page.Channel.Title)
	}
	if page.Channel.Username != "GameDevVideos" {
		t.Errorf("expected username 'GameDevVideos', got %q", page.Channel.Username)
	}
	if page.Channel.Subscribers != "13" {
		t.Errorf("expected subscribers '13', got %q", page.Channel.Subscribers)
	}
	if len(page.Messages) == 0 {
		t.Fatal("expected at least 1 message, got 0")
	}

	first := page.Messages[0]
	if first.PostID != "GameDevVideos/187" {
		t.Errorf("expected PostID 'GameDevVideos/187', got %q", first.PostID)
	}
	if first.Views != "11" {
		t.Errorf("expected views '11', got %q", first.Views)
	}
	if first.DateTime != "2026-05-31T05:36:28+00:00" {
		t.Errorf("expected datetime '2026-05-31T05:36:28+00:00', got %q", first.DateTime)
	}
	if len(first.Links) == 0 {
		t.Error("expected at least 1 link in first message")
	}
	if first.IsEdited {
		t.Error("expected first message not to be edited")
	}
	if first.Media != nil && len(first.Media) != 0 {
		t.Error("expected no media in first message")
	}

	var embed *EmbedInfo
	for _, l := range first.Links {
		if l.Provider != "" {
			embed = &l
			break
		}
	}
	if embed == nil {
		t.Fatal("expected embed in first message links")
	}
	if embed.URL != "https://youtu.be/Qje-PSZeMHI" {
		t.Errorf("expected embed URL 'https://youtu.be/Qje-PSZeMHI', got %q", embed.URL)
	}
	if embed.Provider != "YouTube" {
		t.Errorf("expected embed provider 'YouTube', got %q", embed.Provider)
	}
	if embed.Title != "Игры заставляют нас копить мусор" {
		t.Errorf("expected embed title, got %q", embed.Title)
	}
	if embed.Description == "" {
		t.Error("expected embed description")
	}
	if embed.Thumbnail == "" {
		t.Error("expected embed thumbnail")
	}

	last := page.Messages[len(page.Messages)-1]
	if last.IsEdited {
		t.Error("expected last message not to be edited")
	}
}
