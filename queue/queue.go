package queue

import (
	"slashcaster/config"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	tb "gopkg.in/tucnak/telebot.v3"
)

type Message struct {
	Type      string         // Type of the message ("telegram", "discord")
	Recipient int64          // Recipient of the message
	Message   string         // Caption for the photo
	Sopts     tb.SendOptions // Send options
}

type SendQueue struct {
	/* Enforces a rate-limiter to stay within Telegram's send-rate boundaries */
	MessagesPerSecond int        // Messages-per-second limit
	MessageQueue      []Message  // Queue of messages to send
	Mutex             sync.Mutex // Mutex to avoid concurrent writes
}

func AddToQueue(queue *SendQueue, message *Message) {
	queue.Mutex.Lock()
	queue.MessageQueue = append(queue.MessageQueue, *message)
	queue.Mutex.Unlock()
}

func handleSendError(msg Message, err error) {
	log.Printf("Error sending message: %s", err.Error())
}

func MessageSender(queue *SendQueue, session *config.Session) {
	/* Function clears the SendQueue and stays within API limits while doing so */
	for {
		// If queue is not empty, clear it
		if len(queue.MessageQueue) != 0 {
			// Lock sendQueue for parsing
			queue.Mutex.Lock()

			// Iterate over queue
			for i, msg := range queue.MessageQueue {
				// Send message
				var err error
				if msg.Type == "telegram" {
					_, err = session.Telegram.Send(tb.ChatID(int64(msg.Recipient)), msg.Message, &msg.Sopts)
				} else if msg.Type == "discord" {
					log.Warn().Msg("Discord message sender not implemented!")
				}

				if err != nil {
					go handleSendError(msg, err)
				}

				// Sleep long enough to stay within API limits: convert messagesPerSecond to ms
				if i < len(queue.MessageQueue)-1 {
					time.Sleep(time.Millisecond * time.Duration(1.0/queue.MessagesPerSecond*1000.0))
				}
			}

			// Clear queue
			queue.MessageQueue = nil

			// Batch send done, unlock sendQueue
			queue.Mutex.Unlock()
		}

		// Sleep while waiting for updates
		time.Sleep(time.Millisecond * 500)
	}
}
