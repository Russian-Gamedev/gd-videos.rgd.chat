package tme_parser

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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

		msg.Text = strings.TrimSpace(s.Find(".tgme_widget_message_text").Text())

		s.Find(".tgme_widget_message_text a").Each(func(_ int, a *goquery.Selection) {
			if href, exists := a.Attr("href"); exists {
				msg.Links = append(msg.Links, href)
			}
		})

		if msg.Text != "" && len(msg.Links) == 1 && msg.Text == msg.Links[0] {
			msg.Text = ""
		}

		msg.Media = parseMedia(s)

		msg.Views = strings.TrimSpace(s.Find(".tgme_widget_message_views").Text())

		s.Find("time").Each(func(i int, t *goquery.Selection) {
			if i == 0 {
				msg.DateTime, _ = t.Attr("datetime")
				msg.TimeLabel = strings.TrimSpace(t.Text())
			}
		})

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
