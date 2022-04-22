package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"slashcaster/spam"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	dg "github.com/bwmarrin/discordgo"
	tb "gopkg.in/telebot.v3"
)

type Session struct {
	Config   *Config
	Spam     *spam.AntiSpam
	Discord  *dg.Session
	Telegram *tb.Bot
}

type Config struct {
	Version   string     // Version number
	Debug     bool       // Is debugging enabled?
	NoStream  bool       // Skip slot streaming?
	LogPath   string     // Folder to log to
	RateLimit int        // Rate-limit, messages/second
	Tokens    Tokens     // Tokens for auth
	Stats     Stats      // Statistics
	Broadcast Broadcast  // Channels we broadcast to
	Mutex     sync.Mutex // Mutex to avoid concurrent writes
}

type Tokens struct {
	Telegram string // Telegram bot API token
	Infura   string // Infura API token
	Discord  string // Discord bot token
}

type Stats struct {
	StartTime     int64  // Unix timestamp of startup time
	CurrentSlot   int64  // Current slot
	BlockTime     int64  // Current block time
	BlocksParsed  uint64 // Count of blocks parsed
	AttSlashings  int    // Keep track of observed slashings
	PropSlashings int    // Keep track of observed slashings
	LastSlashing  int64  // Timestamp to keep track of last slashing
	MessagesSent  int    // Keep track of converted images
}

type Broadcast struct {
	TelegramOwner       int64   // Owner of the bot: skips logging
	TelegramChannel     int64   // The channel the bot broadcasts in
	TelegramSubscribers []int64 // Telegram subscribers
	DiscordGuild        string
}

func SlashingObserved(config *Config, attCount int, propCount int, time int64) {
	// Lock struct
	config.Mutex.Lock()

	// Update stats
	config.Stats.AttSlashings += attCount
	config.Stats.PropSlashings += propCount
	config.Stats.LastSlashing = time
	config.Stats.MessagesSent++

	// Unlock
	config.Mutex.Unlock()

	// Dump config now to avoid possible data loss
	DumpConfig(config)
}

func AddSubscriber(config *Config, chatId int64) bool {
	// Lock struct
	config.Mutex.Lock()

	// Update config with ID
	subs := config.Broadcast.TelegramSubscribers

	// Find index
	index := -1

	for i, id := range subs {
		if id == chatId {
			index = i
			break
		}
	}

	if index == -1 {
		// Update config with ID
		config.Broadcast.TelegramSubscribers = append(config.Broadcast.TelegramSubscribers, chatId)
	} else {
		// Unlock
		config.Mutex.Unlock()

		return false
	}

	// Unlock
	config.Mutex.Unlock()

	// Dump config now to avoid possible data loss
	DumpConfig(config)

	return true
}

func RemoveSubscriber(config *Config, chatId int64) bool {
	// Lock struct
	config.Mutex.Lock()

	// Update config with ID
	subs := config.Broadcast.TelegramSubscribers

	// Find index
	index := -1

	for i, id := range subs {
		if id == chatId {
			index = i
			break
		}
	}

	if index != -1 {
		// Index mangling
		subs[index] = subs[len(subs)-1]
		subs = subs[:len(subs)-1]

		// Update to new list
		config.Broadcast.TelegramSubscribers = subs
	} else {
		// Unlock
		config.Mutex.Unlock()

		return false
	}

	// Unlock
	config.Mutex.Unlock()

	// Dump config now to avoid possible data loss
	DumpConfig(config)

	return true
}

func DumpConfig(config *Config) {
	// Dumps config to disk
	jsonbytes, err := json.MarshalIndent(config, "", "\t")

	if err != nil {
		log.Error().Err(err).Msg("⚠️ Error marshaling json")
	}

	wd, _ := os.Getwd()
	configf := filepath.Join(wd, "config", "bot-config.json")

	file, err := os.Create(configf)
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}

	// Write, close
	_, err = file.Write(jsonbytes)
	if err != nil {
		log.Error().Err(err).Msg("Dumping config failed")
	}

	file.Close()
}

func LoadConfig(cfgPath string) *Config {
	/* Loads the config, returns a pointer to it */

	// Get log file's path relative to working dir
	var configPath string
	if cfgPath == "" {
		wd, _ := os.Getwd()
		configPath = filepath.Join(wd, "config")
	} else {
		configPath = cfgPath
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		_ = os.Mkdir(configPath, os.ModePerm)
	}

	configf := filepath.Join(configPath, "bot-config.json")
	if _, err := os.Stat(configf); os.IsNotExist(err) {
		// Config doesn't exist: create
		fmt.Print("\nEnter bot API key: ")
		reader := bufio.NewReader(os.Stdin)
		inp, _ := reader.ReadString('\n')
		tgBotToken := strings.TrimSuffix(inp, "\n")

		fmt.Print("\nEnter Infura API key: ")
		reader = bufio.NewReader(os.Stdin)
		inp, _ = reader.ReadString('\n')
		infuraToken := strings.TrimSuffix(inp, "\n")

		// Create config
		config := Config{
			LogPath:   "logs",
			RateLimit: 5,

			Tokens: Tokens{
				Telegram: tgBotToken,
				Infura:   infuraToken,
			},

			Stats: Stats{
				StartTime: time.Now().Unix(),
			},
		}

		// Dump config
		go DumpConfig(&config)

		fmt.Println("Config created! Transitioning to logging...")
		return &config
	}

	// Config exists: load
	fbytes, err := ioutil.ReadFile(configf)
	if err != nil {
		log.Error().Err(err).Msg("⚠️ Error reading config file")
		os.Exit(1)
	}

	// New config struct
	var config Config

	// Unmarshal into our config struct
	err = json.Unmarshal(fbytes, &config)
	if err != nil {
		log.Error().Err(err).Msg("⚠️ Error unmarshaling config json")
		os.Exit(1)
	}

	// Set startup time
	config.Stats.StartTime = time.Now().Unix()

	return &config
}
