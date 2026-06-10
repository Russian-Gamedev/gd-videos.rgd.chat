package api

import (
		"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func RegisterRoutes(root *router.Router[*core.RequestEvent]) {
	root.GET("/channels/{channel}/messages", func(e *core.RequestEvent) error {
		channelParam := e.Request.PathValue("channel")

		channel, err := e.App.FindFirstRecordByData("channels", "username", channelParam)
		if err != nil {
			channel, err = e.App.FindRecordById("channels", channelParam)
			if err != nil {
				return e.NotFoundError(
					"channel not found — add it via Admin UI first",
					map[string]any{"channel": channelParam},
				)
			}
		}

		page, _ := strconv.Atoi(e.Request.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		perPage, _ := strconv.Atoi(e.Request.URL.Query().Get("perPage"))
		if perPage < 1 || perPage > 200 {
			perPage = 50
		}

		var total int64
		var row struct {
			Count int64 `db:"count"`
		}
		qerr := e.App.DB().NewQuery(
			"SELECT COUNT(*) as count FROM messages WHERE channel={:channel} AND deleted=0",
		).Bind(dbx.Params{"channel": channel.Id}).One(&row)
		if qerr != nil {
			return e.InternalServerError("failed to count messages", map[string]any{"error": qerr.Error()})
		}
		total = row.Count

		offset := (page - 1) * perPage
		totalPages := int(math.Ceil(float64(total) / float64(perPage)))

		records, err := e.App.FindRecordsByFilter(
			"messages",
			"channel={:channel} && deleted=false",
			"+post_id",
			perPage,
			offset,
			dbx.Params{"channel": channel.Id},
		)
		if err != nil {
			return e.InternalServerError("failed to fetch messages", nil)
		}

		type messageItem struct {
			Id       int64             `json:"id"`
			Text     *string           `json:"text,omitempty"`
			Links    []string          `json:"links"`
			Media    []json.RawMessage `json:"media"`
			Views    int64             `json:"views"`
			Datetime string            `json:"datetime"`
			Edited   bool              `json:"edited"`
		}

		items := make([]messageItem, 0, len(records))
		for _, r := range records {
			var links []string
			if s := r.GetString("links"); s != "" && s != "[]" {
				json.Unmarshal([]byte(s), &links)
			}
			var media []json.RawMessage
			if s := r.GetString("media"); s != "" && s != "[]" {
				json.Unmarshal([]byte(s), &media)
			}

			var textPtr *string
			if t := r.GetString("text"); t != "" {
				textPtr = &t
			}

			views, _ := strconv.ParseInt(r.GetString("views"), 10, 64)

			items = append(items, messageItem{
				Id:       int64(r.GetFloat("post_id")),
				Text:     textPtr,
				Links:    links,
				Media:    media,
				Views:    views,
				Datetime: r.GetString("datetime"),
				Edited:   r.GetBool("edited"),
			})
		}

		return e.JSON(http.StatusOK, map[string]any{
			"items":      items,
			"page":       page,
			"perPage":    perPage,
			"total":      total,
			"totalPages": totalPages,
		})
	})
}
