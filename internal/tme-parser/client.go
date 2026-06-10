package tme_parser

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/PuerkitoBio/goquery"
)

type Client struct {
	client *http.Client
}

func NewClient() *Client {
	return &Client{
		client: &http.Client{},
	}
}

type GetConfig struct {
	BeforeId *int64
	AfterId  *int64
}

func (c *Client) Get(channelName string, config GetConfig) (*ChannelPage, error) {
	query := url.Values{}
	if config.AfterId != nil {
		query.Add("after", strconv.FormatInt(*config.AfterId, 10))
	}

	if config.BeforeId != nil {
		query.Add("before", strconv.FormatInt(*config.BeforeId, 10))
	}

	path := fmt.Sprintf("https://t.me/s/%s?%s", channelName, query.Encode())

	resp, err := c.client.Get(path)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return parsePage(doc), nil
}
