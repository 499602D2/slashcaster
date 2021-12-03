package bots

import (
	"log"
	"slashcaster/config"
	"slashcaster/queue"

	dg "github.com/bwmarrin/discordgo"
)

func SetupDiscordBot(session *config.Session, sendQueue *queue.SendQueue) {
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
