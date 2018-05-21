# hikkabot

A Telegram subscription service for [2ch.hk](https://2ch.hk).

## Warning

This is the deprecated version. The better and newer version is located at https://github.com/jfk9w-go/hikkabot.

## Features

* Subscribe to any thread.
* Manage group and channel subscriptions. Caller must be either a `creator` or an `administrator` who `can_post_messages`. The bot will try to alert all chat administrators about subscription changes.
* Images and other post attachments are sent as links (marked as `[A]`) to leverage Telegram link previews.
* Reply navigation using hashtags.

### Available commands

| Command | Description |
|---------|-------------|
| /subscribe [thread\_link] | Subscribe this chat to a thread. If a `thread_link` is not provided, it will be requested. |
| /subscribe thread\_link channel\_name | Subscribe a channel to a thread. A `channel_name` must start with a `@`. |
| /unsubscribe | Unsubscribe this chat from all threads. |
| /unsubscribe channel\_name | Unsubscribe a channel from all threads. A `channel_name` must start with a `@`. |
| /status | Check if the bot is alive. |

For `/subscribe` and `/unsubscribe` commands shortcuts are available: `/sub` and `/unsub` respectively.

### Navigation and hashtags

Navigation is built entirely upon hashtags. Every detected post number will be replaced by a similar hashtag. You can click on any post hashtag and use standard Telegram search features.

At the beginning of each thread a message containing a hashtag `#thread`, a thread summary and the URL will be sent to the subscribed chat.

All service messages begin with a hashtag `#info`.


## Installation and execution

Install using Go package manager:

```bash
$ go get -u github.com/jfk9w/hikkabot
$ go install github.com/jfk9w/hikkabot
$ hikkabot -config=YOUR_CONFIG_FILE
```

You can use [this skeleton](https://github.com/jfk9w/hikkabot/blob/master/config.json) to build the configuration upon.

### <span>bot.sh</span>

This is a [utility script](https://github.com/jfk9w/hikkabot/blob/master/bot.sh) provided for bot control.

```bash
# Starts an instance of Hikkabot.
# If LOG_FILE is specified, the instance will
# be run in background printing its output to
# LOG_FILE.
$ ./bot.sh start YOUR_CONFIG_FILE [LOG_FILE]

# Stops the background instance of Hikkabot.
$ ./bot.sh stop
```
