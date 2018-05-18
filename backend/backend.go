package backend

import (
	"io"

	"github.com/jfk9w-go/dvach"
	"github.com/jfk9w-go/hikkabot/feed"
	"github.com/jfk9w-go/hikkabot/html"
	"github.com/jfk9w-go/logrus"
	"github.com/jfk9w-go/telegram"
	"github.com/orcaman/concurrent-map"
)

type (
	Feed interface {
		io.Closer
		Subscribe(dvach.Ref, string, int) bool
		Unsubscribe(dvach.Ref)
		Running() feed.State
		CollectErrors() (bool, []error)
	}

	FeedFactory interface {
		CreateFeed(telegram.ChatRef) Feed
	}

	Bot interface {
		DeleteRoute(telegram.ChatRef)
		SendPost(telegram.ChatRef, html.Post, bool) error
		GetAdmins(telegram.ChatRef) ([]telegram.ChatRef, error)
		NotifyAll([]telegram.ChatRef, string, ...interface{})
	}

	Dvach interface {
		Thread(dvach.Ref, int) ([]dvach.Post, error)
		Post(dvach.Ref) (*dvach.Post, error)
	}
)

var log = logrus.GetLogger("backend")

func Run(bot Bot, ff FeedFactory) *T {
	back := &T{
		bot:   bot,
		ff:    ff,
		state: cmap.New(),
	}

	go back.gc()
	return back
}

func NewFeedFactory(bot feed.Bot, dvch feed.Dvach, conv feed.Converter) FeedFactory {
	return &DefaultFeedFactory{bot, dvch, conv}
}

type DefaultFeedFactory struct {
	bot  feed.Bot
	dvch feed.Dvach
	conv feed.Converter
}

func (df *DefaultFeedFactory) CreateFeed(chat telegram.ChatRef) Feed {
	return feed.Run(df.bot, df.dvch, df.conv, chat)
}
