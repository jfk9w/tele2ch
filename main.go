package main

import (
	"os"
	"time"

	aconvert "github.com/jfk9w-go/aconvert-api"
	"github.com/jfk9w-go/flu"
	telegram "github.com/jfk9w-go/telegram-bot-api"
	"github.com/jfk9w/hikkabot/api/dvach"
	"github.com/jfk9w/hikkabot/api/reddit"
	"github.com/jfk9w/hikkabot/media"
	"github.com/jfk9w/hikkabot/services"
	"github.com/jfk9w/hikkabot/storage"
	"github.com/jfk9w/hikkabot/subscription"
	"github.com/jfk9w/hikkabot/util"
)

func main() {
	config := new(struct {
		AdminID        telegram.ID
		Aliases        map[telegram.Username]telegram.ID
		Storage        storage.SQLConfig
		UpdateInterval string
		Telegram       struct {
			Token string
			Proxy string
		}

		Media struct {
			media.Config
			Aconvert aconvert.Config
		}

		Reddit reddit.Config
		Dvach  struct{ Usercode string }
	})

	util.ReadJSON(os.Args[1], config)
	updateInterval, err := time.ParseDuration(config.UpdateInterval)
	util.Check(err)

	bot := telegram.NewBot(flu.NewTransport().
		ResponseHeaderTimeout(2*time.Minute).
		ProxyURL(config.Telegram.Proxy).
		NewClient(), config.Telegram.Token)
	aconvertClient := aconvert.NewClient(nil, &config.Media.Aconvert)
	mediaManager := media.NewManager(config.Media.Config, aconvertClient)
	defer mediaManager.Shutdown()

	ctx := subscription.Context{
		MediaManager: mediaManager,
		DvachClient:  dvach.NewClient(nil, config.Dvach.Usercode),
		RedditClient: reddit.NewClient(nil, &config.Reddit),
	}

	storage := storage.NewSQL(config.Storage)
	handler := subscription.NewHandler(bot, ctx, storage, updateInterval, services.All, config.Aliases)
	go bot.Send(config.AdminID, &telegram.Text{Text: "⬆️"}, nil)
	bot.Listen(handler.CommandListener())
}
