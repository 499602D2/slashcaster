package bots

import (
	"fmt"
	"log"
	"slashcaster/config"
	"slashcaster/queue"
	"slashcaster/spam"
	"time"

	"github.com/dustin/go-humanize"
	tb "gopkg.in/tucnak/telebot.v2"
)

func SetupTelegramBot(session *config.Session, sendQueue *queue.SendQueue) {
	var err error
	session.Telegram, err = tb.NewBot(tb.Settings{
		Token:  session.Config.Tokens.Telegram,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})

	if err != nil {
		log.Fatal("Error creating Telegram bot:", err)
	}

	// Start command handler
	session.Telegram.Handle("/start", func(message *tb.Message) {
		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return
		}

		text := "🔪 Welcome to Eth2 slasher! " +
			"This bot broadcasts slashing events occurring on the Ethereum beacon chain.\n\n" +
			"To subscribe to slashing messages, use the channel @ethslashings."

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
	})

	// Output statistics
	session.Telegram.Handle("/stats", func(message *tb.Message) {
		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return
		}

		ago := time.Now().Unix() - session.Config.Stats.BlockTime
		slot := humanize.Comma(int64(session.Config.Stats.CurrentSlot))
		startedAgo := humanize.RelTime(
			time.Now(), time.Unix(session.Config.Stats.StartTime, 0), "ago", "ago")

		text := "🔪 *SlashCaster statistics*\n" +
			fmt.Sprintf("Current slot: %s\n", slot) +
			fmt.Sprintf("Blocks parsed: %d\n", session.Config.Stats.BlocksParsed) +
			fmt.Sprintf("Last block %d seconds ago\n\n", ago) +
			fmt.Sprintf("_Bot started %s_", startedAgo)

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
	})

	// Subscribe command handler
	session.Telegram.Handle("/subscribe", func(message *tb.Message) {
		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return
		}

		// Subscribe
		success := config.AddSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully subscribed! You will now be notified of slashings."
		} else {
			text = "You are already subscribed to notifications!"
		}

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
	})

	// Unsubscribe command handler
	session.Telegram.Handle("/unsubscribe", func(message *tb.Message) {
		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return
		}

		// Unsubscribe
		success := config.RemoveSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully unsubscribed! No notifications will be sent to you."
		} else {
			text = "Nothing to do, you will not receive notifications!"
		}

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
	})
}
