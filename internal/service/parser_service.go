package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"tg-channel-parser/internal/discord"
	"tg-channel-parser/internal/tme-parser"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type ParserService struct {
	app            core.App
	client         *tme_parser.Client
	discordClient *discord.Client
}

func NewParserService(app core.App, discordClient *discord.Client) *ParserService {
	return &ParserService{
		app:            app,
		client:         tme_parser.NewClient(),
		discordClient: discordClient,
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

	var editedIDs, deletedIDs []int64
	if lastPostId == 0 {
		s.app.Logger().Info("channel mode=seed", "source", "parser", "channel", username, "last_parsed", 0)
		if err := s.seedChannel(ch); err != nil {
			return err
		}
	} else {
		s.app.Logger().Info("channel mode=delta", "source", "parser", "channel", username, "last_parsed", lastPostId)
		var err error
		editedIDs, deletedIDs, err = s.deltaChannel(ch, lastPostId)
		if err != nil {
			return err
		}
	}

	s.forwardMessagesForChannel(ch, editedIDs, deletedIDs)
	return nil
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

		if _, err := s.checkDeleted(ch.Id, visibleIDs); err != nil {
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

func (s *ParserService) deltaChannel(ch *core.Record, lastPostId int64) (editedIDs, deletedIDs []int64, err error) {
	username := ch.GetString("username")

	after := lastPostId
	page, err := s.client.Get(username, tme_parser.GetConfig{AfterId: &after})
	if err != nil {
		return nil, nil, fmt.Errorf("delta fetch: %w", err)
	}

	if len(page.Messages) == 0 {
		s.app.Logger().Info("no new messages", "source", "parser", "channel", username)
		return nil, nil, nil
	}

	if err := s.updateChannelInfo(ch, &page.Channel); err != nil {
		return nil, nil, fmt.Errorf("update channel info: %w", err)
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
	editedIDs, err = s.checkEditsOnLatestPage(ch.Id, username)
	if err != nil {
		s.app.Logger().Error("edit check on latest page failed", "source", "parser", "channel", username, "error", err)
	}
	deletedIDs, err = s.checkDeleted(ch.Id, visibleIDs)
	if err != nil {
		s.app.Logger().Error("deletion check failed", "source", "parser", "channel", username, "error", err)
	}

	ch.Set("last_parsed_post_id", float64(maxPostId))
	if err := s.save(ch); err != nil {
		return nil, nil, fmt.Errorf("update last_parsed: %w", err)
	}

	s.app.Logger().Info("delta complete",
		"source", "parser",
		"channel", username,
		"saved", saved,
		"new_max", maxPostId,
	)
	return editedIDs, deletedIDs, nil
}

func (s *ParserService) checkEditsOnLatestPage(channelId, username string) ([]int64, error) {
	latest, err := s.client.Get(username, tme_parser.GetConfig{})
	if err != nil {
		return nil, err
	}

	var editedIDs []int64
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
				editedIDs = append(editedIDs, pid)
			}
		}
	}
	return editedIDs, nil
}

func (s *ParserService) checkDeleted(channelId string, visibleIDs map[int64]bool) ([]int64, error) {
	if len(visibleIDs) == 0 {
		return nil, nil
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
		return nil, err
	}

	var deletedIDs []int64
	for _, rec := range records {
		pid := rec.GetFloat("post_id")
		if !visibleIDs[int64(pid)] {
			rec.Set("deleted", true)
			if err := s.save(rec); err != nil {
				return nil, err
			}
			deletedIDs = append(deletedIDs, int64(pid))
		}
	}
	return deletedIDs, nil
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
	if msg.DateTime != "" {
		record.Set("datetime", msg.DateTime)
	}
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
	if msg.DateTime != "" {
		record.Set("datetime", msg.DateTime)
	}
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

func (s *ParserService) forwardMessagesForChannel(ch *core.Record, editedIDs, deletedIDs []int64) {
	if s.discordClient == nil {
		return
	}

	s.forwardNewMessages(ch)

	if len(editedIDs) > 0 {
		s.forwardEditedMessages(ch.Id, editedIDs, ch.GetString("username"))
	}

	if len(deletedIDs) > 0 {
		s.forwardDeletedMessages(ch.Id, deletedIDs)
	}
}

func (s *ParserService) forwardNewMessages(ch *core.Record) {
	channelId := ch.Id
	username := ch.GetString("username")

	cwhRecords, err := s.app.FindRecordsByFilter(
		"channel_webhooks",
		"channel={:channel} && enabled=true",
		"", 0, 0,
		dbx.Params{"channel": channelId},
	)
	if err != nil || len(cwhRecords) == 0 {
		return
	}

	for _, cwh := range cwhRecords {
		lastSent := int64(cwh.GetFloat("last_sent_post_id"))

		if lastSent == 0 {
			lastParsed := int64(ch.GetFloat("last_parsed_post_id"))
			if lastParsed > 0 {
				cwh.Set("last_sent_post_id", float64(lastParsed))
				if err := s.save(cwh); err != nil {
					s.app.Logger().Error("failed to init last_sent_post_id", "error", err)
				}
			}
			continue
		}

		webhookId := cwh.GetString("webhook")
		target, err := s.app.FindRecordById("webhook_targets", webhookId)
		if err != nil || target == nil {
			s.app.Logger().Error("webhook target not found", "id", webhookId)
			continue
		}

		if target.GetString("type") != "discord" {
			continue
		}
		webhookURL := target.GetString("url")
		footerTemplate := cwh.GetString("footer_template")
		if footerTemplate == "" {
			footerTemplate = "Больше тут %"
		}

		messages, err := s.app.FindRecordsByFilter(
			"messages",
			"channel={:channel} && post_id>{:last_sent} && deleted=0",
			"post_id",
			0, 0,
			dbx.Params{"channel": channelId, "last_sent": float64(lastSent)},
		)
		if err != nil {
			s.app.Logger().Error("failed to query undelivered messages", "error", err)
			continue
		}

		for _, msg := range messages {
			pid := int64(msg.GetFloat("post_id"))
			text := msg.GetString("text")
			content := discord.BuildContent(text, footerTemplate, username)

			extID, sendErr := s.discordClient.Send(webhookURL, content)
			if sendErr != nil {
				s.app.Logger().Error("discord send failed",
					"channel", username,
					"post_id", pid,
					"error", sendErr,
				)
				continue
			}

			if err := s.saveDeliveredMessage(cwh.Id, msg.Id, extID); err != nil {
				s.app.Logger().Error("failed to save delivered_message", "error", err)
			}

			cwh.Set("last_sent_post_id", float64(pid))
			if err := s.save(cwh); err != nil {
				s.app.Logger().Error("failed to update last_sent_post_id", "error", err)
			}
		}
	}
}

func (s *ParserService) forwardEditedMessages(channelId string, editedIDs []int64, username string) {
	for _, pid := range editedIDs {
		deliveries, err := s.app.FindRecordsByFilter(
			"delivered_messages",
			"message.channel={:channel} && message.post_id={:post_id}",
			"", 0, 0,
			dbx.Params{"channel": channelId, "post_id": float64(pid)},
		)
		if err != nil || len(deliveries) == 0 {
			continue
		}

		msgRecord, err := s.app.FindFirstRecordByFilter(
			"messages",
			"channel={:channel} && post_id={:post_id}",
			dbx.Params{"channel": channelId, "post_id": float64(pid)},
		)
		if err != nil || msgRecord == nil {
			continue
		}
		text := msgRecord.GetString("text")

		for _, d := range deliveries {
			extID := d.GetString("external_id")
			webhookURL, footerTemplate := s.webhookForDelivery(d.GetString("channel_webhook"))
			if webhookURL == "" {
				continue
			}

			content := discord.BuildContent(text, footerTemplate, username)
			if err := s.discordClient.Edit(webhookURL, extID, content); err != nil {
				s.app.Logger().Error("discord edit failed",
					"channel", username,
					"post_id", pid,
					"error", err,
				)
			}
		}
	}
}

func (s *ParserService) forwardDeletedMessages(channelId string, deletedIDs []int64) {
	for _, pid := range deletedIDs {
		deliveries, err := s.app.FindRecordsByFilter(
			"delivered_messages",
			"message.channel={:channel} && message.post_id={:post_id}",
			"", 0, 0,
			dbx.Params{"channel": channelId, "post_id": float64(pid)},
		)
		if err != nil {
			continue
		}

		for _, d := range deliveries {
			extID := d.GetString("external_id")
			webhookURL, _ := s.webhookForDelivery(d.GetString("channel_webhook"))
			if webhookURL == "" {
				continue
			}

			if err := s.discordClient.Delete(webhookURL, extID); err != nil {
				s.app.Logger().Error("discord delete failed",
					"post_id", pid,
					"error", err,
				)
			}
		}
	}
}

func (s *ParserService) webhookForDelivery(cwhId string) (webhookURL, footerTemplate string) {
	cwh, err := s.app.FindRecordById("channel_webhooks", cwhId)
	if err != nil || cwh == nil {
		return "", ""
	}

	webhookId := cwh.GetString("webhook")
	target, err := s.app.FindRecordById("webhook_targets", webhookId)
	if err != nil || target == nil {
		return "", ""
	}

	if target.GetString("type") != "discord" {
		return "", ""
	}

	footerTemplate = cwh.GetString("footer_template")
	if footerTemplate == "" {
		footerTemplate = "Больше тут %"
	}

	return target.GetString("url"), footerTemplate
}

func (s *ParserService) saveDeliveredMessage(cwhId, msgId, extID string) error {
	col, err := s.app.FindCollectionByNameOrId("delivered_messages")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("channel_webhook", cwhId)
	rec.Set("message", msgId)
	rec.Set("external_id", extID)
	return s.save(rec)
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
