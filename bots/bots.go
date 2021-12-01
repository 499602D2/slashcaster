package bots

import (
	"log"
	"slashcaster/config"
	"slashcaster/queue"
	"time"

	dg "github.com/bwmarrin/discordgo"
	tb "gopkg.in/tucnak/telebot.v2"
)

func SetupTelegramBot(session *config.Session) {
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
		text := "🔪 Welcome to Eth2 slasher! " +
			"This bot broadcasts slashing events occurring on the Ethereum beacon chain.\n\n" +
			"To subscribe to slashing messages, use the channel @ethslashings."

		msg := queue.Message{
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(&session.Queue, &msg)
	})

	// Subscribe command handler
	session.Telegram.Handle("/subscribe", func(message *tb.Message) {
		success := config.AddSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully subscribed! You will now be notified of slashings."
		} else {
			text = "You are already subscribed to notifications!"
		}

		msg := queue.Message{
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(&session.Queue, &msg)
	})

	// Unsubscribe command handler
	session.Telegram.Handle("/unsubscribe", func(message *tb.Message) {
		success := config.RemoveSubscriber(session.Config, message.Sender.ID)

		var text string
		if success {
			text = "✅ Successfully unsubscribed! No notifications will be sent to you."
		} else {
			text = "Nothing to do, you will not receive notifications!"
		}

		msg := queue.Message{
			Recipient: message.Sender.ID,
			Message:   text,
			Sopts:     tb.SendOptions{ParseMode: "Markdown"},
		}

		queue.AddToQueue(&session.Queue, &msg)
	})
}

func SetupDiscordBot(session *config.Session) {
	// If bot is not configured, return
	if session.Config.Tokens.Discord == "" {
		return
	}

	var err error
	session.Discord, err = dg.New("Bot " + session.Config.Tokens.Discord)

	if err != nil {
		log.Fatal("Error creating Discord bot:", err)
		return
	}
}
