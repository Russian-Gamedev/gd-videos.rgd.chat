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

	return nil
}
