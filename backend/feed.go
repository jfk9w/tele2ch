package backend

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/jfk9w-go/aconvert"
	"github.com/jfk9w-go/dvach"
	"github.com/jfk9w-go/logrus"
	"github.com/jfk9w-go/telegram"
	"github.com/jfk9w-go/unit"
)

const (
	maxQueueSize = 100
	maxDelSize   = 50
)

type FeedEntry struct {
	Thread dvach.ID
	Offset int
	Admins []telegram.ChatRef
}

func (e FeedEntry) WithOffset(offset int) FeedEntry {
	e.Offset = offset
	return e
}

type Feed struct {
	io.Closer

	chat  telegram.ChatRef
	queue chan FeedEntry
	del   chan dvach.ID

	queueSize *int32
	delSize   *int32
}

func NewFeed(bot Bot, dvch dvach.Api, webm aconvert.CacheService, chat telegram.ChatRef) *Feed {
	aux := unit.NewAux()
	feed := &Feed{
		aux,
		chat,
		make(chan FeedEntry, 100),
		make(chan dvach.ID, 50),
		new(int32),
		new(int32),
	}

	*feed.queueSize = 0
	*feed.delSize = 0

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Panicf("%s %s", feed.chat, err)
			}
		}()

		main := time.NewTicker(1 * time.Minute)
		defer main.Stop()
		log.Infof("Started %s feed", feed.chat)

	main:
		for {
			select {
			case <-aux.C:
				log.Infof("Stopped %s feed", feed.chat)
				return

			case first := <-feed.del:
				feed.gc(first)

			case <-main.C:
				feed.gc()

				select {
				case entry := <-feed.queue:
					thread, err := dvch.Thread(entry.Thread, entry.Offset)
					if err != nil {
						switch e := err.(type) {
						case *dvach.Error:
							log.Warningf("Failed to update thread %s: %s", entry.Thread.URL(), e)
							if feed.Unsubscribe(entry.Thread) {
								go bot.NotifyAll(entry.Admins,
									"#info\nAn error has occured. Subscription deleted.\nChat: %s\nThread: %s\nCode: %d\nError: %s",
									feed.chat, entry.Thread.URL(), e.Code, e.Err)

								continue main
							}
						}
					}

					log.Debugf("%d new posts for %s %s", len(thread), feed.chat, entry.Thread.URL())

					for _, post := range thread {
						for _, file := range post.Files {
							if file.Type == dvach.Webm {
								go webm.Convert(file.URL(), nil)
							}
						}
					}

					offset := entry.Offset
					for _, post := range thread {
						err := bot.SendPost(chat, post.WithBoard(entry.Thread.Board))
						if err != nil {
							log.WithFields(logrus.Fields{
								"Post": post,
							}).Errorf("Failed to send post to %s", feed.chat)

							if feed.Unsubscribe(entry.Thread) {
								go bot.NotifyAll(entry.Admins,
									"#info\nFailed to send post. Subscription deleted.\nChat: %s\nThread: %s\nPost: %s\nError: %s",
									feed.chat, entry.Thread.URL(), post.Num, err)

								continue main
							}
						}

						offset = post.NumInt() + 1
					}

					feed.queue <- entry.WithOffset(offset)

				default:
				}
			}
		}
	}()

	return feed
}

func (feed *Feed) gc(items ...dvach.ID) {
	del := make([]dvach.ID, len(items))
	copy(del, items)
	if atomic.AddInt32(feed.delSize, int32(-len(del))) > 0 {
		for {
			del = append(del, <-feed.del)
			if atomic.AddInt32(feed.delSize, -1) == 0 {
				break
			}
		}
	}

	size := int(atomic.LoadInt32(feed.queueSize))
	unique := make(map[dvach.ID]unit.T)
	deleted := 0

outer:
	for i := 0; i < size; i++ {
		entry := <-feed.queue
		if _, ok := unique[entry.Thread]; ok {
			atomic.AddInt32(feed.queueSize, -1)
			continue outer
		}

		unique[entry.Thread] = unit.Value

		for _, d := range del {
			if d.URL() == entry.Thread.URL() {
				deleted++
				atomic.AddInt32(feed.queueSize, -1)
				continue outer
			}
		}

		feed.queue <- entry
	}

	log.Infof("%s garbage collected %d entries (%d unique total)", feed.chat, deleted, len(unique))
}

func (feed *Feed) Submit(thread dvach.ID, offset int, admins []telegram.ChatRef) bool {
	for {
		queueSize := atomic.LoadInt32(feed.queueSize)
		if queueSize >= maxQueueSize {
			log.Errorf("%s queue size reached limit %d", feed.chat, maxQueueSize)
			return false
		}

		if atomic.CompareAndSwapInt32(feed.queueSize, queueSize, 1) {
			feed.queue <- FeedEntry{thread, offset, admins}
			break
		}
	}

	log.Infof("Subscribed %s to %s", feed.chat, thread.URL())
	return true
}

func (feed *Feed) Unsubscribe(thread dvach.ID) bool {
	for {
		delSize := atomic.LoadInt32(feed.queueSize)
		if delSize >= maxDelSize {
			log.Errorf("%s deleted queue reached limit %d", feed.chat, maxDelSize)
			return false
		}

		if atomic.CompareAndSwapInt32(feed.delSize, delSize, 1) {
			feed.del <- thread
			break
		}
	}

	log.Infof("Unsubscribed %s from %s", feed.chat, thread.URL())
	return true
}

func (feed *Feed) IsEmpty() bool {
	return atomic.LoadInt32(feed.queueSize) == 0
}
