package tme_parser

type ChannelPage struct {
	Channel  ChannelInfo `json:"channel"`
	Messages []Message   `json:"messages"`
}

type ChannelInfo struct {
	Title       string `json:"title"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	Description string `json:"description"`
	Subscribers string `json:"subscribers"`
}

type Message struct {
	PostID    string      `json:"post_id"`
	Text      string      `json:"text"`
	Links     []EmbedInfo `json:"links"`
	Media     []MediaItem `json:"media"`
	Views     string      `json:"views"`
	DateTime  string      `json:"datetime"`
	TimeLabel string      `json:"time_label"`
	IsEdited  bool        `json:"is_edited"`
}

type MediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type EmbedInfo struct {
	URL         string `json:"url,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}
