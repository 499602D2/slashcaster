package main

import (
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"slashcaster/api"
	"slashcaster/bots"
	"slashcaster/config"
	"slashcaster/queue"
	"slashcaster/spam"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func setupSignalHandler(conf *config.Config) {
	// Listens for incoming interrupt signals, dumps config if detected
	channel := make(chan os.Signal)
	signal.Notify(channel, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-channel
		go log.Info().Msg("🚦 Received interrupt signal: dumping config...")
		config.DumpConfig(conf)
		os.Exit(0)
	}()
}

func setupLogFile(logFolder string) *os.File {
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
		log.Error().Err(err)
	}

	return logf
}

func main() {
	// Create session
	session := config.Session{}

	// Load (or create) config, set version number
	session.Config = config.LoadConfig("")
	session.Config.Version = "1.4.0"

	// Setup anti-spam
	session.Spam = &spam.AntiSpam{}
	session.Spam.ChatBannedUntilTimestamp = make(map[int64]int64)
	session.Spam.ChatLogs = make(map[int64]spam.ChatLog)
	session.Spam.ChatBanned = make(map[int64]bool)
	session.Spam.Rules = make(map[string]int64)

	// Add rules
	session.Spam.Rules["TimeBetweenCommands"] = 1

	// Command line arguments
	flag.BoolVar(&session.Config.Debug, "debug", false, "Specify to enable debug mode")
	flag.BoolVar(&session.Config.NoStream, "no-stream", false, "Specify to disable slot streaming")
	flag.Parse()

	// Set-up logging
	if !session.Config.Debug {
		// If not debugging, log to file
		logf := setupLogFile(session.Config.LogPath)
		defer logf.Close()

		//log.Logger = zerolog.New(logf).With().Logger()
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: logf, NoColor: true, TimeFormat: time.RFC822Z})
	} else {
		// If debugging, output to console
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC822Z})
	}

	// Handle signals
	setupSignalHandler(session.Config)

	// Create send queue, start MessageSender in a goroutine
	sendQueue := queue.SendQueue{MessagesPerSecond: session.Config.RateLimit}
	go queue.MessageSender(&sendQueue, &session)

	// Set-up Telegram bot
	bots.SetupTelegramBot(&session, &sendQueue)

	// Set-up Discord bot
	bots.SetupDiscordBot(&session, &sendQueue)

	// Start slotStreamer in a goroutine, unless explicitly disabled
	if !session.Config.NoStream {
		go api.SlotStreamer(&sendQueue, session.Config)
	} else {
		log.Print("⛔️ Slot-streaming explicitly disabled!")
	}

	// Log start
	log.Printf("🔪 SlashCaster %s started at %s", session.Config.Version, time.Now())

	// Start Telegram bot
	/*
		TODO: run in a go-thread, start in bots package?
		Depends on how Discord bot is started
	*/
	session.Telegram.Start()
}
