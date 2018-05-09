package backend

import (
	"strings"
	"time"

	"github.com/jfk9w-go/dvach"
	"github.com/jfk9w-go/hikkabot/feed"
	"github.com/jfk9w-go/telegram"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
)

type T struct {
	bot   Bot
	ff    FeedFactory
	state cmap.ConcurrentMap
}

func (back *T) gc() {
	for {
		time.Sleep(10 * time.Second)
		keys := back.state.Keys()
		log.Debugf("Running GC, %d total keys", len(keys))
		for _, key := range keys {
			entry, ok := back.state.Get(key)
			if !ok {
				continue
			}

			chat := fromKey(key)
			feed := entry.(Feed)
			empty, errs := feed.CollectErrors()
			if len(errs) == 0 {
				if empty {
					back.state.Remove(key)
					back.bot.DeleteRoute(chat)
					log.Debugf("Garbage collected %s", key)
				}

				continue
			}

			log.Debugf("Collected %d errors from %s", len(errs), key)

			admins, err := back.bot.GetAdmins(chat)
			if err != nil {
				log.Errorf("Failed to get admin list for %s: %s", key, err)
				back.state.Upsert(key, nil,
					func(exists bool, old interface{}, new interface{}) interface{} {
						if exists {
							old.(Feed).Close()
						}

						return nil
					})

				continue
			}

			text := &strings.Builder{}
			text.WriteString("#info\n")
			for _, err := range errs {
				text.WriteString(err.Error())
				text.WriteRune('\n')
			}

			back.bot.NotifyAll(admins, text.String())

			if empty {
				back.state.Remove(key)
				back.bot.DeleteRoute(chat)
				log.Debugf("Garbage collected %s", key)
			}
		}
	}
}

func (back *T) Subscribe(chat telegram.ChatRef, thread dvach.ID, hash string, offset int) error {
	var err error
	back.state.Upsert(toKey(chat), nil,
		func(exists bool, old interface{}, new interface{}) interface{} {
			var feed Feed
			if exists {
				feed = old.(Feed)
			} else {
				feed = back.ff.CreateFeed(chat)
			}

			if !feed.Subscribe(thread, hash, offset) {
				err = errors.Errorf("exists")
			}

			return feed
		})

	return err
}

func (back *T) Unsubscribe(chat telegram.ChatRef, thread dvach.ID) error {
	if entry, ok := back.state.Get(toKey(chat)); ok {
		entry.(Feed).Unsubscribe(thread)
		return nil
	}

	return errors.New("not subscribed")
}

func (back *T) UnsubscribeAll(chat telegram.ChatRef) error {
	var err error
	back.state.Upsert(toKey(chat), nil,
		func(exists bool, old interface{}, new interface{}) interface{} {
			if !exists {
				err = errors.New("not subscribed")
			}

			old.(Feed).Close()
			return nil
		})

	return err
}

func (back *T) Dump(chat telegram.ChatRef) (feed.State, error) {
	if entry, ok := back.state.Get(toKey(chat)); ok {
		return entry.(Feed).Running(), nil
	}

	return nil, errors.New("not subscribed")
}
