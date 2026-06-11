package db

import (
	"github.com/pocketbase/pocketbase/core"
)

func InitCollections(app core.App) error {
	channels, err := app.FindCollectionByNameOrId("channels")
	if err != nil {
		channels = core.NewBaseCollection("channels")
		channels.Fields.Add(
			&core.TextField{Name: "username", Required: true, Max: 255},
			&core.TextField{Name: "title", Max: 500},
			&core.TextField{Name: "description", Max: 5000},
			&core.TextField{Name: "avatar_url", Max: 2000},
			&core.TextField{Name: "subscriber_count", Max: 100},
			&core.NumberField{Name: "last_parsed_post_id", OnlyInt: true},
			&core.BoolField{Name: "enabled"},
		)
		channels.AddIndex("idx_username", true, "username", "")
		if err := app.Save(channels); err != nil {
			return err
		}
		app.Logger().Info("created collection", "name", "channels", "id", channels.Id)
	}

	channels, _ = app.FindCollectionByNameOrId("channels")

	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		messages = core.NewBaseCollection("messages")
		messages.Fields.Add(
			&core.RelationField{Name: "channel", CollectionId: channels.Id, CascadeDelete: true, Required: true, MaxSelect: 1},
			&core.NumberField{Name: "post_id", OnlyInt: true, Required: true},
			&core.TextField{Name: "text", Max: 100000},
			&core.JSONField{Name: "links"},
			&core.JSONField{Name: "media"},
			&core.TextField{Name: "views", Max: 100},
			&core.TextField{Name: "datetime", Max: 100},
			&core.BoolField{Name: "edited"},
			&core.BoolField{Name: "deleted"},
		)
		messages.AddIndex("idx_channel_post", true, "channel, post_id", "")
		if err := app.Save(messages); err != nil {
			return err
		}
		app.Logger().Info("created collection", "name", "messages", "id", messages.Id)
	} else if messages.Fields.GetByName("deleted") == nil {
		messages.Fields.Add(&core.BoolField{Name: "deleted"})
		if err := app.Save(messages); err != nil {
			return err
		}
		app.Logger().Info("migrated messages collection: added deleted field")
	}

	channels, _ = app.FindCollectionByNameOrId("channels")
	messages, _ = app.FindCollectionByNameOrId("messages")

	if err := createWebhookTargets(app); err != nil {
		return err
	}

	if err := createChannelWebhooks(app, channels); err != nil {
		return err
	}

	if err := createDeliveredMessages(app); err != nil {
		return err
	}

	return nil
}

func createWebhookTargets(app core.App) error {
	_, err := app.FindCollectionByNameOrId("webhook_targets")
	if err == nil {
		return nil
	}

	col := core.NewBaseCollection("webhook_targets")
	col.Fields.Add(
		&core.TextField{Name: "name", Max: 255},
		&core.SelectField{Name: "type", Values: []string{"discord"}, Required: true},
		&core.TextField{Name: "url", Required: true, Max: 2000},
	)
	if err := app.Save(col); err != nil {
		return err
	}
	app.Logger().Info("created collection", "name", "webhook_targets", "id", col.Id)
	return nil
}

func createChannelWebhooks(app core.App, channels *core.Collection) error {
	_, err := app.FindCollectionByNameOrId("channel_webhooks")
	if err == nil {
		return nil
	}

	targets, err := app.FindCollectionByNameOrId("webhook_targets")
	if err != nil {
		return err
	}

	col := core.NewBaseCollection("channel_webhooks")
	col.Fields.Add(
		&core.RelationField{Name: "channel", CollectionId: channels.Id, CascadeDelete: true, Required: true, MaxSelect: 1},
		&core.RelationField{Name: "webhook", CollectionId: targets.Id, CascadeDelete: true, Required: true, MaxSelect: 1},
		&core.TextField{Name: "footer_template", Max: 500},
		&core.NumberField{Name: "last_sent_post_id", OnlyInt: true},
		&core.BoolField{Name: "enabled"},
	)
	if err := app.Save(col); err != nil {
		return err
	}
	app.Logger().Info("created collection", "name", "channel_webhooks", "id", col.Id)
	return nil
}

func createDeliveredMessages(app core.App) error {
	_, err := app.FindCollectionByNameOrId("delivered_messages")
	if err == nil {
		return nil
	}

	cwh, err := app.FindCollectionByNameOrId("channel_webhooks")
	if err != nil {
		return err
	}

	msgs, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}

	col := core.NewBaseCollection("delivered_messages")
	col.Fields.Add(
		&core.RelationField{Name: "channel_webhook", CollectionId: cwh.Id, CascadeDelete: true, Required: true, MaxSelect: 1},
		&core.RelationField{Name: "message", CollectionId: msgs.Id, CascadeDelete: true, Required: true, MaxSelect: 1},
		&core.TextField{Name: "external_id", Required: true, Max: 500},
	)
	col.AddIndex("idx_cwh_msg", true, "channel_webhook, message", "")
	if err := app.Save(col); err != nil {
		return err
	}
	app.Logger().Info("created collection", "name", "delivered_messages", "id", col.Id)
	return nil
}
