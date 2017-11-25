package service

import (
	"errors"
	"fmt"
	"sync"

	dv "github.com/jfk9w/hikkabot/dvach"
	"github.com/jfk9w/hikkabot/html2md"
	"github.com/jfk9w/hikkabot/telegram"
	"github.com/jfk9w/hikkabot/util"
	"github.com/phemmer/sawmill"
)

const (
	maxPostsPerAlert       = 20
	maxGetThreadAttempts   = 10
	maxSendMessageAttempts = 2
)

var (
	errGetThread   = errors.New("GetThread failed")
	errSendMessage = errors.New("SendMessage failed")

	_mutex   = new(sync.RWMutex)
	_subs    = make(map[SubscriberKey]*Subscriber)
	_runtime serviceRT
)

// Init service instance
func Init(bot *telegram.BotAPI, dvach *dv.API, filename string) {
	var persister *persister
	if len(filename) > 0 {
		persister = newPersister(filename)
		persister.init()
	}

	_runtime = serviceRT{
		bot:                 bot,
		dvach:               dvach,
		persister:           persister,
		attemptsGetThread:   make(map[ThreadKey]int),
		attemptsSendMessage: make(map[SubscriberKey]int),
		mutex:               new(sync.Mutex),
	}
}

// Start service instance
func Start() {
	if _runtime.persister != nil {
		_runtime.persister.start()
	}

	for key, sub := range _subs {
		chat := ParseSubscriberKey(key)
		sub.start(func(board string, threadID string, offset int) (int, error) {
			newOffset, err := onEvent(chat, board, threadID, offset)
			if err != nil {
				go onAlertAdministrators(chat,
					"An error has occured. Subscription suspended.\nChat: %s\nThread: %s\nError: %s",
					chat.Key(), dv.FormatThreadURL(board, threadID), err.Error())

				return 0, err
			}

			return newOffset, nil
		})
	}

	sawmill.Notice("service started")
}

// Stop service instance
func Stop() {
	p := _runtime.persister
	if p != nil {
		p.stop()
	}

	for _, sub := range _subs {
		sub.stop()
	}

	sawmill.Notice("service stopped")

	if p != nil {
		p.persist()
	}
}

// Subscribe to a thread
func Subscribe(chat telegram.ChatRef, board string, threadID string) {
	sub := subscriber(chat)
	err := sub.newActiveThread(board, threadID)
	if err != nil {
		go onAlertAdministrators(chat,
			"Subscription failed.\nChat: %s\nThread: %s\nError: %s",
			chat.Key(), dv.FormatThreadURL(board, threadID), err.Error())

		return
	}

	go onAlertAdministrators(chat,
		"Subscription OK.\nChat: %s\nThread: %s",
		chat.Key(), dv.FormatThreadURL(board, threadID))
}

// Unsubscribe chat from all threads
func Unsubscribe(chat telegram.ChatRef) {
	key := FormatSubscriberKey(chat)

	_mutex.Lock()
	defer _mutex.Unlock()

	if sub, ok := _subs[key]; ok {
		sub.deleteAllActiveThreads()
		go onAlertAdministrators(chat,
			"Subscriptions cleared.\nChat: %s",
			chat.Key())
	}
}

func subscriber(chat telegram.ChatRef) *Subscriber {
	key := FormatSubscriberKey(chat)

	_mutex.Lock()
	defer _mutex.Unlock()

	if sub, ok := _subs[key]; !ok {
		sub = newSubscriber()
		sub.init()
		sub.start(func(board string, threadID string, offset int) (int, error) {
			newOffset, err := onEvent(chat, board, threadID, offset)
			if err != nil {
				go onAlertAdministrators(chat,
					"An error has occured. Subscription suspended.\nChat: %s\nThread: %s\nError: %s",
					chat.Key(), dv.FormatThreadURL(board, threadID), err.Error())

				return 0, err
			}

			return newOffset, nil
		})

		_subs[key] = sub
	}

	return _subs[key]
}

func onEvent(chat telegram.ChatRef, board string, threadID string, offset int) (int, error) {
	key := FormatThreadKey(board, threadID)
	posts, err := _runtime.dvach.GetThread(board, threadID, offset)
	if err != nil {
		err = registerGetThreadAttempt(key)
		if err != nil {
			return 0, err
		}

		return offset, nil
	}

	resetGetThreadAttempts(key)

	newOffset := offset
	limit := util.MinInt(maxPostsPerAlert, len(posts))
	for i := 0; i < limit; i++ {
		post := posts[i]
		msgs, err := html2md.Parse(post)
		if err != nil {
			go onAlertAdministrators(chat, "Parsing post failed, skipping.\n%s", err.Error())
			newOffset = post.NumInt() + 1
			continue
		}

		for _, msg := range msgs {
			done := util.NewHook()
			var err error
			_runtime.bot.SendMessage(telegram.SendMessageRequest{
				Chat:      chat,
				Text:      msg,
				ParseMode: telegram.Markdown,
			}, func(resp *telegram.Response, err0 error) {
				if err0 != nil {
					err = fmt.Errorf("unable to send message (%s)", err0.Error())
				} else if !resp.Ok {
					err = fmt.Errorf("unable to send message (%d, %s)", resp.ErrorCode, resp.Description)
				}

				done.Send()
			}, false)

			done.Wait()

			key := FormatSubscriberKey(chat)
			if err != nil {
				err = registerSendMessageAttempt(key)
				if err != nil {
					return 0, err
				}

				return offset, nil
			}

			resetSendMessageAttempts(key)
		}

		newOffset = post.NumInt() + 1
	}

	return newOffset, nil
}

func onAlertAdministrators(chat telegram.ChatRef, pattern string, args ...interface{}) {
	text := fmt.Sprintf(pattern, args...)

	admins, err0 := _runtime.bot.GetChatAdministrators(chat)
	if err0 != nil {
		sawmill.Error("GetChatAdministrators: "+err0.Error(), sawmill.Fields{
			"chat": chat.Key(),
		})

		return
	}

	if admins == nil {
		notify(chat, text)
		return
	}

	for _, admin := range admins {
		notify(telegram.ChatRef{
			ID: telegram.ChatID(admin.ID),
		}, text)
	}
}

func notify(chat telegram.ChatRef, text string) {
	go _runtime.bot.SendMessage(telegram.SendMessageRequest{
		Chat: chat,
		Text: text,
	}, func(resp *telegram.Response, err error) {
		if err != nil {
			sawmill.Error("notify failed: "+err.Error(), sawmill.Fields{
				"user": chat.Key(),
			})

			return
		}

		if !resp.Ok {
			sawmill.Error("notify failed", sawmill.Fields{
				"user":        chat.Key(),
				"errorCode":   resp.ErrorCode,
				"description": resp.Description,
			})
		}
	}, true)
}

func registerGetThreadAttempt(key ThreadKey) error {
	_runtime.mutex.Lock()
	defer _runtime.mutex.Unlock()

	attempts := _runtime.attemptsGetThread[key]
	attempts++
	if attempts > maxGetThreadAttempts {
		delete(_runtime.attemptsGetThread, key)
		return errGetThread
	}

	_runtime.attemptsGetThread[key] = attempts

	return nil
}

func resetGetThreadAttempts(key ThreadKey) {
	_runtime.mutex.Lock()
	defer _runtime.mutex.Unlock()

	delete(_runtime.attemptsGetThread, key)
}

func registerSendMessageAttempt(key SubscriberKey) error {
	_runtime.mutex.Lock()
	defer _runtime.mutex.Unlock()

	attempts := _runtime.attemptsSendMessage[key]
	attempts++
	if attempts > maxGetThreadAttempts {
		delete(_runtime.attemptsSendMessage, key)
		return errSendMessage
	}

	_runtime.attemptsSendMessage[key] = attempts

	return nil
}

func resetSendMessageAttempts(key SubscriberKey) {
	_runtime.mutex.Lock()
	defer _runtime.mutex.Unlock()

	delete(_runtime.attemptsSendMessage, key)
}

type serviceRT struct {
	bot       *telegram.BotAPI
	dvach     *dv.API
	persister *persister

	attemptsGetThread   map[ThreadKey]int
	attemptsSendMessage map[SubscriberKey]int
	mutex               *sync.Mutex
}