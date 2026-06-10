package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"tg-channel-parser/internal/tme-parser"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type ParserService struct {
	app    core.App
	client *tme_parser.Client
}

func NewParserService(app core.App) *ParserService {
	return &ParserService{
		app:    app,
		client: tme_parser.NewClient(),
	}
}

func (s *ParserService) RunParse() {
	start := time.Now()
	s.app.Logger().Info("cron run started", "source", "parser")

	records, err := s.app.FindAllRecords("channels")
	if err != nil {
		s.app.Logger().Error("failed to fetch channels", "source", "parser", "error", err)
		return
	}

	if len(records) == 0 {
		s.app.Logger().Info("no channels found — add them via Admin UI", "source", "parser")
		return
	}

	var processed int
	for _, ch := range records {
		username := ch.GetString("username")
		enabled := ch.GetBool("enabled")

		if !enabled {
			s.app.Logger().Info("channel skipped (disabled)", "source", "parser", "channel", username)
			continue
		}

		if err := s.parseChannel(ch); err != nil {
			s.app.Logger().Error("channel parse error", "source", "parser", "channel", username, "error", err)
			continue
		}
		processed++
	}

	s.app.Logger().Info("cron run done",
		"source", "parser",
		"processed", processed,
		"duration", time.Since(start).String(),
	)
}

func (s *ParserService) parseChannel(ch *core.Record) error {
	username := ch.GetString("username")
	lastPostId := int64(ch.GetFloat("last_parsed_post_id"))

	if lastPostId == 0 {
		s.app.Logger().Info("channel mode=seed", "source", "parser", "channel", username, "last_parsed", 0)
		return s.seedChannel(ch)
	}

	s.app.Logger().Info("channel mode=delta", "source", "parser", "channel", username, "last_parsed", lastPostId)
	return s.deltaChannel(ch, lastPostId)
}

func (s *ParserService) seedChannel(ch *core.Record) error {
	username := ch.GetString("username")

	page, err := s.client.Get(username, tme_parser.GetConfig{})
	if err != nil {
		return fmt.Errorf("initial fetch: %w", err)
	}

	if len(page.Messages) == 0 {
		s.app.Logger().Info("no messages found", "source", "parser", "channel", username)
		return nil
	}

	if err := s.updateChannelInfo(ch, &page.Channel); err != nil {
		return fmt.Errorf("update channel info: %w", err)
	}

	saved := 0
	var maxPostId int64 = 0
	var minPostId int64 = math.MaxInt64

	for _, msg := range page.Messages {
		pid := parsePostID(msg.PostID)
		if pid <= 0 {
			continue
		}
		if pid > maxPostId {
			maxPostId = pid
		}
		if pid < minPostId {
			minPostId = pid
		}
		result, err := s.saveMessage(ch.Id, &msg)
		if err != nil {
			s.app.Logger().Error("save message failed", "source", "parser", "channel", username, "post_id", pid, "error", err)
		} else if result == "created" || result == "updated" {
			saved++
		}
	}

	s.app.Logger().Info("seed page received",
		"source", "parser",
		"channel", username,
		"page", 1,
		"posts_min", minPostId,
		"posts_max", maxPostId,
		"saved", saved,
	)

	ch.Set("last_parsed_post_id", float64(maxPostId))
	if err := s.save(ch); err != nil {
		return fmt.Errorf("update last_parsed: %w", err)
	}

	pageNum := 2
	for {
		before := minPostId
		page, err := s.client.Get(username, tme_parser.GetConfig{BeforeId: &before})
		if err != nil {
			s.app.Logger().Error("seed page fetch error", "source", "parser", "channel", username, "page", pageNum, "error", err)
			break
		}

		if len(page.Messages) == 0 {
			s.app.Logger().Info("seed done (empty page)", "source", "parser", "channel", username, "before", before)
			break
		}

		pageSaved := 0
		visibleIDs := make(map[int64]bool)
		var pageMin int64 = math.MaxInt64
		var pageMax int64 = 0

		for _, msg := range page.Messages {
			pid := parsePostID(msg.PostID)
			if pid <= 0 {
				continue
			}
			visibleIDs[pid] = true
			if pid > pageMax {
				pageMax = pid
			}
			if pid < pageMin {
				pageMin = pid
			}
			result, err := s.saveMessage(ch.Id, &msg)
			if err != nil {
				s.app.Logger().Error("save message failed", "source", "parser", "channel", username, "post_id", pid, "error", err)
			} else if result == "created" || result == "updated" {
				pageSaved++
			}
		}

		if err := s.checkDeleted(ch.Id, visibleIDs); err != nil {
			s.app.Logger().Error("deletion check failed", "source", "parser", "channel", username, "error", err)
		}

		saved += pageSaved
		s.app.Logger().Info("seed page received",
			"source", "parser",
			"channel", username,
			"page", pageNum,
			"posts_min", pageMin,
			"posts_max", pageMax,
			"saved", pageSaved,
			"total", saved,
		)

		if pageMin == minPostId || pageMin <= 1 {
			break
		}
		minPostId = pageMin
		pageNum++
	}

	s.app.Logger().Info("seed complete",
		"source", "parser",
		"channel", username,
		"total_saved", saved,
		"max_post_id", maxPostId,
	)
	return nil
}

func (s *ParserService) deltaChannel(ch *core.Record, lastPostId int64) error {
	username := ch.GetString("username")

	after := lastPostId
	page, err := s.client.Get(username, tme_parser.GetConfig{AfterId: &after})
	if err != nil {
		return fmt.Errorf("delta fetch: %w", err)
	}

	if len(page.Messages) == 0 {
		s.app.Logger().Info("no new messages", "source", "parser", "channel", username)
		return nil
	}

	if err := s.updateChannelInfo(ch, &page.Channel); err != nil {
		return fmt.Errorf("update channel info: %w", err)
	}

	saved := 0
	var maxPostId int64 = lastPostId
	visibleIDs := make(map[int64]bool)

	for _, msg := range page.Messages {
		pid := parsePostID(msg.PostID)
		if pid <= 0 || pid <= lastPostId {
			continue
		}
		visibleIDs[pid] = true
		if pid > maxPostId {
			maxPostId = pid
		}
		result, err := s.saveMessage(ch.Id, &msg)
		if err != nil {
			s.app.Logger().Error("save message failed", "source", "parser", "channel", username, "post_id", pid, "error", err)
		} else if result == "created" || result == "updated" {
			saved++
		}
	}

	// also check latest page for edits in recent posts
	if err := s.checkEditsOnLatestPage(ch.Id, username); err != nil {
		s.app.Logger().Error("edit check on latest page failed", "source", "parser", "channel", username, "error", err)
	}
	if err := s.checkDeleted(ch.Id, visibleIDs); err != nil {
		s.app.Logger().Error("deletion check failed", "source", "parser", "channel", username, "error", err)
	}

	ch.Set("last_parsed_post_id", float64(maxPostId))
	if err := s.save(ch); err != nil {
		return fmt.Errorf("update last_parsed: %w", err)
	}

	s.app.Logger().Info("delta complete",
		"source", "parser",
		"channel", username,
		"saved", saved,
		"new_max", maxPostId,
	)
	return nil
}

func (s *ParserService) checkEditsOnLatestPage(channelId, username string) error {
	latest, err := s.client.Get(username, tme_parser.GetConfig{})
	if err != nil {
		return err
	}

	for _, msg := range latest.Messages {
		pid := parsePostID(msg.PostID)
		if pid <= 0 {
			continue
		}
		if msg.IsEdited {
			existing, err := s.app.FindFirstRecordByFilter(
				"messages",
				"channel={:channel} && post_id={:post_id}",
				dbx.Params{"channel": channelId, "post_id": pid},
			)
			if err != nil || existing == nil {
				continue
			}
			result, err := s.updateMessage(existing, &msg)
			if err != nil {
				s.app.Logger().Error("update edited message failed", "source", "parser", "channel", username, "post_id", pid, "error", err)
			} else if result == "updated" {
				s.app.Logger().Info("edited message updated", "source", "parser", "channel", username, "post_id", pid)
			}
		}
	}
	return nil
}

func (s *ParserService) checkDeleted(channelId string, visibleIDs map[int64]bool) error {
	if len(visibleIDs) == 0 {
		return nil
	}

	var minID, maxID int64 = math.MaxInt64, 0
	for pid := range visibleIDs {
		if pid < minID {
			minID = pid
		}
		if pid > maxID {
			maxID = pid
		}
	}

	records, err := s.app.FindRecordsByFilter(
		"messages",
		"channel={:channel} && post_id>={:min} && post_id<={:max} && deleted=false",
		"",
		0,
		0,
		dbx.Params{"channel": channelId, "min": minID, "max": maxID},
	)
	if err != nil {
		return err
	}

	for _, rec := range records {
		pid := rec.GetFloat("post_id")
		if !visibleIDs[int64(pid)] {
			rec.Set("deleted", true)
			if err := s.save(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ParserService) saveMessage(channelId string, msg *tme_parser.Message) (string, error) {
	pid := parsePostID(msg.PostID)
	if pid <= 0 {
		return "skipped", nil
	}

	existing, err := s.app.FindFirstRecordByFilter(
		"messages",
		"channel={:channel} && post_id={:post_id}",
		dbx.Params{
			"channel": channelId,
			"post_id": pid,
		},
	)
	if err == nil && existing != nil {
		if msg.IsEdited {
			return s.updateMessage(existing, msg)
		}
		if existing.GetBool("deleted") {
			existing.Set("deleted", false)
			s.app.Logger().Info("message restored", "source", "parser", "post_id", pid)
			return "updated", s.save(existing)
		}
		return "exists", nil
	}

	linksJSON, _ := json.Marshal(msg.Links)
	mediaJSON, _ := json.Marshal(msg.Media)

	col, _ := s.app.FindCollectionByNameOrId("messages")
	record := core.NewRecord(col)
	record.Set("channel", channelId)
	record.Set("post_id", float64(pid))
	record.Set("text", msg.Text)
	record.Set("links", string(linksJSON))
	record.Set("media", string(mediaJSON))
	record.Set("views", msg.Views)
	record.Set("datetime", msg.DateTime)
	record.Set("edited", msg.IsEdited)
	record.Set("deleted", false)

	return "created", s.save(record)
}

func (s *ParserService) updateMessage(record *core.Record, msg *tme_parser.Message) (string, error) {
	linksJSON, _ := json.Marshal(msg.Links)
	mediaJSON, _ := json.Marshal(msg.Media)

	record.Set("text", msg.Text)
	record.Set("links", string(linksJSON))
	record.Set("media", string(mediaJSON))
	record.Set("views", msg.Views)
	record.Set("datetime", msg.DateTime)
	record.Set("edited", msg.IsEdited)

	return "updated", s.save(record)
}

func (s *ParserService) updateChannelInfo(ch *core.Record, info *tme_parser.ChannelInfo) error {
	ch.Set("title", info.Title)
	ch.Set("description", info.Description)
	ch.Set("avatar_url", info.AvatarURL)
	ch.Set("subscriber_count", info.Subscribers)
	return s.save(ch)
}

func (s *ParserService) save(model core.Model) error {
	return s.app.Save(model)
}

func parsePostID(postID string) int64 {
	parts := strings.SplitN(postID, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
