package spam

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type AntiSpam struct {
	/* In-memory struct keeping track of banned chats and per-chat activity */
	ChatBanned               map[int]bool     // Simple "if ChatBanned[chat] { do }" checks
	ChatBannedUntilTimestamp map[int]int      // How long banned chats are banned for
	ChatLogs                 map[int]ChatLog  // Map chat ID to a ChatLog struct
	Rules                    map[string]int64 // Arbitrary rules for code flexibility
	Mutex                    sync.Mutex       // Mutex to avoid concurrent map writes
}

type ChatLog struct {
	/* Per-chat struct keeping track of activity for spam management */
	NextAllowedCommandTimestamp int64 // Next time the chat is allowed to call a command
	CommandSpamOffenses         int   // Count of spam offences (not used yet)
}

func CommandPreHandler(spam *AntiSpam, chat int, sentAt int64) bool {
	/* When user sends a command, verify the chat is eligible for a command parse. */
	spam.Mutex.Lock()
	chatLog := spam.ChatLogs[chat]

	if chatLog.NextAllowedCommandTimestamp > sentAt {
		chatLog.CommandSpamOffenses++
		spam.ChatLogs[chat] = chatLog
		spam.Mutex.Unlock()

		log.Printf("Chat %d now has %d spam offenses", chat, chatLog.CommandSpamOffenses)
		return false
	}

	// No spam, update chat's ConversionLog
	chatLog.NextAllowedCommandTimestamp = time.Now().Unix() + spam.Rules["TimeBetweenCommands"]
	spam.ChatLogs[chat] = chatLog
	spam.Mutex.Unlock()
	return true
}
