package main

import (
	"log"
	"os"
	"path/filepath"
	"slashcaster/api"
	"slashcaster/bots"
	"slashcaster/config"
	"slashcaster/queue"
	"time"
)

func setupLogs(logFolder string) *os.File {
	// Log file
	wd, _ := os.Getwd()
	logPath := filepath.Join(wd, logFolder)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		_ = os.Mkdir(logPath, os.ModePerm)
	}

	// Set-up logging
	logFilePath := filepath.Join(logPath, "bot.log")
	logf, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Println(err)
	}

	return logf
}

func main() {
	// Create session
	session := config.Session{}

	// Load (or create) config
	session.Config = config.LoadConfig()

	// Set-up logging
	logf := setupLogs(session.Config.LogPath)
	defer logf.Close()
	log.SetOutput(logf)

	// Set-up Telegram bot
	bots.SetupTelegramBot(&session)

	// Set-up Discord bot
	bots.SetupDiscordBot(&session)

	// Create send queue, start MessageSender in a goroutine
	session.Queue = queue.SendQueue{MessagesPerSecond: session.Config.RateLimit}
	go queue.MessageSender(&session.Queue, session.Telegram)

	// Start slotStreamer in a goroutine
	go api.SlotStreamer(&session.Queue, session.Config)

	// Log start
	log.Printf("🔪 SlashCaster started at %s", time.Now())

	// Start Telegram bot
	/*
		TODO: run in a go-thread, start in bots package?
		Depends on how Discord bot is started
	*/
	session.Telegram.Start()
}
