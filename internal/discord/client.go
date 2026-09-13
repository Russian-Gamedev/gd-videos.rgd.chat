package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"tg-channel-parser/internal/tme-parser"
)

type Client struct {
	httpClient *http.Client
}

type webhookPayload struct {
	Content   string `json:"content"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type webhookResponse struct {
	ID string `json:"id"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

func (c *Client) Send(webhookURL, username, avatarURL, content string) (string, error) {
	payload := webhookPayload{Content: content, Username: username, AvatarURL: avatarURL}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("discord send marshal: %w", err)
	}

	url := webhookURL + "?wait=true"
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("discord send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("discord send: status %d", resp.StatusCode)
	}

	var result webhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("discord send decode: %w", err)
	}

	return result.ID, nil
}

func (c *Client) Edit(webhookURL, messageID, username, avatarURL, content string) error {
	payload := webhookPayload{Content: content, Username: username, AvatarURL: avatarURL}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord edit marshal: %w", err)
	}

	url := fmt.Sprintf("%s/messages/%s", webhookURL, messageID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord edit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord edit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord edit: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Delete(webhookURL, messageID string) error {
	url := fmt.Sprintf("%s/messages/%s", webhookURL, messageID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("discord delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord delete: status %d", resp.StatusCode)
	}
	return nil
}

func BuildContent(text, footerTemplate, username string, links []tme_parser.EmbedInfo) string {
	var content string

	if text != "" {
		content = "## " + text
	}

	titled := make(map[string]bool, len(links))
	for _, l := range links {
		if id := tme_parser.VideoID(l.URL); id != "" && l.Title != "" && l.Title != l.URL {
			titled[id] = true
		}
	}

	for _, l := range links {
		if l.URL == "" {
			continue
		}
		// Discord renders [url](url) as literal text, so a titleless link goes out as a bare URL —
		// and is skipped entirely when the same video appears with a real title.
		if l.Title == "" || l.Title == l.URL {
			if titled[tme_parser.VideoID(l.URL)] {
				continue
			}
			content = appendLine(content, "# "+l.URL)
			continue
		}
		content = appendLine(content, fmt.Sprintf("# [%s](%s)", l.Title, l.URL))
	}

	if footerTemplate == "" {
		return truncate(content)
	}

	link := fmt.Sprintf("[@%s](<https://t.me/%s>)", username, username)
	footer := strings.ReplaceAll(footerTemplate, "%", link)

	if content != "" && footer != "" {
		content += "\n\n"
	}
	content += footer

	return truncate(content)
}

func appendLine(content, line string) string {
	if content != "" {
		content += "\n"
	}
	return content + line
}

func truncate(s string) string {
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}
