package discord

import (
	"strings"
	"testing"

	"tg-channel-parser/internal/tme-parser"
)

func TestBuildContentSkipsTitlelessDuplicateOfTitledVideo(t *testing.T) {
	links := []tme_parser.EmbedInfo{
		{URL: "https://www.youtube.com/watch?v=RW_iEfB5DNM"},
		{URL: "https://youtu.be/RW_iEfB5DNM", Provider: "YouTube", Title: "I Made Street Fighter Babes"},
	}

	got := BuildContent("", "Больше тут %", "GameDevVideos", links)

	want := "# [I Made Street Fighter Babes](https://youtu.be/RW_iEfB5DNM)\n\nБольше тут [@GameDevVideos](<https://t.me/GameDevVideos>)"
	if got != want {
		t.Errorf("BuildContent mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestBuildContentBareURLWithoutTitle(t *testing.T) {
	links := []tme_parser.EmbedInfo{
		{URL: "https://www.youtube.com/watch?v=RW_iEfB5DNM"},
	}

	got := BuildContent("", "", "GameDevVideos", links)

	want := "# https://www.youtube.com/watch?v=RW_iEfB5DNM"
	if got != want {
		t.Errorf("BuildContent mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestBuildContentTextAndTitledLinks(t *testing.T) {
	links := []tme_parser.EmbedInfo{
		{URL: "https://youtu.be/abc123defgh", Provider: "YouTube", Title: "Title"},
	}

	got := BuildContent("News", "Больше тут %", "GameDevVideos", links)

	if !strings.HasPrefix(got, "## News\n# [Title](https://youtu.be/abc123defgh)\n\nБольше тут") {
		t.Errorf("unexpected content: %q", got)
	}
}
