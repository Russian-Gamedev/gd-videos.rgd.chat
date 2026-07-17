package tme_parser

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var backgroundImageURLPattern = regexp.MustCompile(`url\(['"]?([^'")]+)['"]?\)`)
var youtuBeURLPattern = regexp.MustCompile(`(?:youtube\.com/(?:watch\?v=|shorts/|embed/)|youtu\.be/)([a-zA-Z0-9_-]{11})`)

func Parse(html string) (*ChannelPage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	return parsePage(doc), nil
}

func parsePage(doc *goquery.Document) *ChannelPage {
	return &ChannelPage{
		Channel:  parseChannelInfo(doc),
		Messages: parseMessages(doc),
	}
}

func parseChannelInfo(doc *goquery.Document) ChannelInfo {
	info := ChannelInfo{
		Title:       strings.TrimSpace(doc.Find(".tgme_channel_info_header_title span[dir='auto']").Text()),
		Description: strings.TrimSpace(doc.Find(".tgme_channel_info_description").Text()),
		Subscribers: strings.TrimSpace(doc.Find(".tgme_channel_info_counter").First().Find(".counter_value").Text()),
	}

	username := strings.TrimSpace(doc.Find(".tgme_channel_info_header_username a").Text())
	info.Username = strings.TrimPrefix(username, "@")

	info.AvatarURL, _ = doc.Find(".tgme_page_photo_image img").First().Attr("src")

	return info
}

func parseMessages(doc *goquery.Document) []Message {
	var messages []Message

	doc.Find(".tgme_widget_message_wrap").Each(func(_ int, s *goquery.Selection) {
		msg := Message{}

		postID, hasPost := s.Find(".js-widget_message").Attr("data-post")
		if !hasPost {
			return
		}
		msg.PostID = postID

		if s.Find(".js-widget_message").HasClass("service_message") {
			return
		}

		if s.Find(".tgme_widget_message_user").Length() == 0 {
			return
		}

		msg.Text = s.Find(".tgme_widget_message_text").Text()

		s.Find(".tgme_widget_message_text a").Each(func(_ int, a *goquery.Selection) {
			if href, exists := a.Attr("href"); exists {
				msg.Links = append(msg.Links, EmbedInfo{URL: href})
				msg.Text = strings.ReplaceAll(msg.Text, strings.TrimSpace(a.Text()), "")
			}
		})

		msg.Text = strings.TrimSpace(msg.Text)

		msg.Media = parseMedia(s)

		if embed := parseEmbed(s); embed != nil {
			for i := len(msg.Links) - 1; i >= 0; i-- {
				if msg.Links[i].URL == embed.URL && msg.Links[i].Provider == "" {
					msg.Links = append(msg.Links[:i], msg.Links[i+1:]...)
				}
			}
			msg.Links = append(msg.Links, *embed)
			if embed.Title != "" {
				msg.Text = strings.ReplaceAll(msg.Text, embed.Title, "")
			}
			msg.Text = strings.TrimSpace(msg.Text)
		}

		msg.Views = strings.TrimSpace(s.Find(".tgme_widget_message_views").Text())

		if t := s.Find(".tgme_widget_message_date time").First(); t.Length() > 0 {
			msg.DateTime, _ = t.Attr("datetime")
			msg.TimeLabel = strings.TrimSpace(t.Text())
		}

		msg.IsEdited = strings.Contains(s.Find(".tgme_widget_message_info").Text(), "edited")

		messages = append(messages, msg)
	})

	return messages
}

func parseMedia(s *goquery.Selection) []MediaItem {
	var media []MediaItem

	s.Find(".tgme_widget_message_photo_wrap img").Each(func(_ int, img *goquery.Selection) {
		if src, exists := img.Attr("src"); exists {
			media = append(media, MediaItem{Type: "photo", URL: src})
		}
	})

	s.Find(".tgme_widget_message_video_wrap video").Each(func(_ int, v *goquery.Selection) {
		if src, exists := v.Attr("src"); exists && src != "" {
			media = append(media, MediaItem{Type: "video", URL: src})
			return
		}
		v.Find("source").Each(func(_ int, source *goquery.Selection) {
			if src, exists := source.Attr("src"); exists {
				media = append(media, MediaItem{Type: "video", URL: src})
			}
		})
	})

	s.Find(".tgme_widget_message_roundvideo_wrap video").Each(func(_ int, v *goquery.Selection) {
		if src, exists := v.Attr("src"); exists && src != "" {
			media = append(media, MediaItem{Type: "gif", URL: src})
			return
		}
		v.Find("source").Each(func(_ int, source *goquery.Selection) {
			if src, exists := source.Attr("src"); exists {
				media = append(media, MediaItem{Type: "gif", URL: src})
			}
		})
	})

	s.Find(".tgme_widget_message_document_wrap a").Each(func(_ int, a *goquery.Selection) {
		if href, exists := a.Attr("href"); exists {
			media = append(media, MediaItem{Type: "document", URL: href})
		}
	})

	return media
}

func parseEmbed(s *goquery.Selection) *EmbedInfo {
	preview := s.Find(".tgme_widget_message_link_preview").First()
	if preview.Length() == 0 {
		return nil
	}

	embed := &EmbedInfo{
		Provider:    strings.TrimSpace(preview.Find(".link_preview_site_name").Text()),
		Title:       strings.TrimSpace(preview.Find(".link_preview_title").Text()),
		Description: strings.TrimSpace(preview.Find(".link_preview_description").Text()),
	}

	if href, exists := preview.Attr("href"); exists {
		embed.URL = strings.TrimSpace(href)
	}

	if style, exists := preview.Find(".link_preview_right_image").First().Attr("style"); exists {
		if match := backgroundImageURLPattern.FindStringSubmatch(style); len(match) == 2 {
			embed.Thumbnail = strings.TrimSpace(match[1])
		}
	}

	if embed.Thumbnail == "" && (strings.Contains(embed.URL, "youtube") || strings.Contains(embed.URL, "youtu.be")) {
		if match := youtuBeURLPattern.FindStringSubmatch(embed.URL); len(match) == 2 {
			embed.Thumbnail = "https://i.ytimg.com/vi/" + match[1] + "/hqdefault.jpg"
		}
	}

	if embed.URL == "" && embed.Provider == "" && embed.Title == "" && embed.Description == "" && embed.Thumbnail == "" {
		return nil
	}
	return embed
}
