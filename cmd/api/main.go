package main

import (
	"log"
	"tg-channel-parser/internal/api"
	"tg-channel-parser/internal/db"
	"tg-channel-parser/internal/service"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

func main() {
	app := pocketbase.New()

	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			if err := db.InitCollections(e.App); err != nil {
				log.Fatalf("failed to init collections: %v", err)
			}

			parser := service.NewParserService(e.App)

			e.App.Cron().MustAdd("parse-channels", "*/5 * * * *", parser.RunParse)

			e.App.Logger().Info("cron job registered", "id", "parse-channels", "schedule", "every 5 min")

			api.RegisterRoutes(e.Router)

			e.App.Logger().Info("api routes registered", "routes", "/api/channels/{channel}/messages")

			return e.Next()
		},
		Priority: 999,
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
