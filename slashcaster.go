package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slashcaster/api"
	"slashcaster/bots"
	"slashcaster/config"
	"slashcaster/queue"
	"syscall"
	"time"
)

func setupSignalHandler(conf *config.Config) {
	// Listens for incoming interrupt signals, dumps config if detected
	channel := make(chan os.Signal)
	signal.Notify(channel, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-channel
		go log.Println("🚦 Received interrupt signal: dumping config...")
		config.DumpConfig(conf)
		os.Exit(0)
	}()
}

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

	// Load (or create) config, set version number
	session.Config = config.LoadConfig()
	session.Config.Version = "1.1.0"

	// Command line arguments
	flag.BoolVar(&session.Config.Debug, "debug", false, "Specify to enable debug mode")
	flag.Parse()

	// Set-up logging
	logf := setupLogs(session.Config.LogPath)
	defer logf.Close()
	log.SetOutput(logf)

	// Handle signals
	setupSignalHandler(session.Config)

	// Create send queue, start MessageSender in a goroutine
	sendQueue := queue.SendQueue{MessagesPerSecond: session.Config.RateLimit}
	go queue.MessageSender(&sendQueue, &session)

	// Set-up Telegram bot
	bots.SetupTelegramBot(&session, &sendQueue)

	// Set-up Discord bot
	bots.SetupDiscordBot(&session, &sendQueue)

	// Start slotStreamer in a goroutine
	go api.SlotStreamer(&sendQueue, session.Config)

	// Log start
	log.Printf("🔪 SlashCaster %s started at %s", session.Config.Version, time.Now())

	// Start Telegram bot
	/*
		TODO: run in a go-thread, start in bots package?
		Depends on how Discord bot is started
	*/
	session.Telegram.Start()
}
