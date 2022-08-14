package bots

import (
	"fmt"
	"log"
	"slashcaster/config"
	"slashcaster/queue"
	"slashcaster/spam"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hako/durafmt"
	tb "gopkg.in/telebot.v3"
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
	session.Telegram.Handle("/start", func(c tb.Context) error {
		// Extract message
		message := *c.Message()

		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return nil
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
		return nil
	})

	// Output statistics
	session.Telegram.Handle("/stats", func(c tb.Context) error {
		// Extract message
		message := *c.Message()

		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return nil
		}

		ago := time.Now().Unix() - session.Config.Stats.BlockTime
		slot := humanize.Comma(int64(session.Config.Stats.CurrentSlot))
		blocksParsed := humanize.Comma(int64(session.Config.Stats.BlocksParsed))

		startedAgo := durafmt.Parse(
			time.Since(time.Unix(session.Config.Stats.StartTime, 0)),
		).LimitFirstN(2).String()

		text := "🔪 *SlashCaster statistics*\n" +
			fmt.Sprintf("Current slot: %s\n", slot) +
			fmt.Sprintf("Blocks parsed: %s\n", blocksParsed) +
			fmt.Sprintf("Last block %d seconds ago\n\n", ago) +
			fmt.Sprintf("_Bot started %s ago_", startedAgo)

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
		return nil
	})

	// Subscribe command handler
	session.Telegram.Handle("/subscribe", func(c tb.Context) error {
		// Extract message
		message := *c.Message()

		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return nil
		}

		// Subscribe
		success := config.AddSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully subscribed! You will now be notified of slashings."
		} else {
			text = "ℹ️ You are already subscribed to notifications!\n\n" +
				"_To unsubscribe, use /unsubscribe._"
		}

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
		return nil
	})

	// Unsubscribe command handler
	session.Telegram.Handle("/unsubscribe", func(c tb.Context) error {
		// Extract message
		message := *c.Message()

		// Throttle requests
		if !spam.CommandPreHandler(session.Spam, message.Sender.ID, message.Unixtime) {
			return nil
		}

		// Unsubscribe
		success := config.RemoveSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully unsubscribed! No notifications will be sent to you."
		} else {
			text = "ℹ️Nothing to do, you will not receive notifications!\n\n" +
				"_To receive notifications, use /subscribe._"
		}

		msg := queue.Message{
			Type:      "telegram",
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(sendQueue, &msg)
		return nil
	})
}
