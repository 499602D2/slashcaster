package api

import (
	"fmt"
	"slashcaster/config"
	"slashcaster/queue"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/dustin/go-humanize/english"

	"github.com/rs/zerolog/log"
	tb "gopkg.in/telebot.v3"
)

func slashingString(event SlashingEvent, config *config.Config) string {
	// Slot to int
	slotInt, _ := strconv.Atoi(event.Slot)
	slashCount := english.Plural(len(event.Slashings), "validator", "validators")
	hSlot := humanize.Comma(int64(slotInt))

	// Beaconcha.in validator and slot URLs
	bc := "https://beaconcha.in/validator/"
	bcS := "https://beaconcha.in/block/" + event.Slot

	// Header
	slashingStr := fmt.Sprintf("🔪 %s slashed in slot [%s](%s)\n", slashCount, hSlot, bcS)
	slashingStr += "\nValidators slashed\n"

	// Loop over all found slashings, add to slashingStr
	for _, slashing := range event.Slashings {
		// Link to the validator's page
		bcUrl := bc + slashing.ValidatorIndex

		// Assign strings according to att./prop. violation bools
		if slashing.ProposerViolation && slashing.AttestationViolation {
			// If slashed due to att. + prop. violation, do a custom string
			slashingStr += fmt.Sprintf("[%s](%s): attestator & proposer violation\n", slashing.ValidatorIndex, bcUrl)
		} else {
			// Otherwise, assign string according to violation
			if slashing.AttestationViolation {
				slashingStr += fmt.Sprintf("[%s](%s): attestor violation\n", slashing.ValidatorIndex, bcUrl)
			} else if slashing.ProposerViolation {
				slashingStr += fmt.Sprintf("[%s](%s): proposer violation\n", slashing.ValidatorIndex, bcUrl)
			}
		}
	}

	// Footer with time since last slashing before this event
	since := humanize.RelTime(time.Unix(config.Stats.LastSlashing, 0), time.Now(), "since", "since")
	footer := "\n" + fmt.Sprintf(`_%s last slashing\._`, since)
	slashingStr += footer

	return slashingStr
}

func extractAttestionViolations(att AttestationViolation) []Slashing {
	// Store indices of slashed validators
	var indices []string

	// Iterate over Attestation1 and Attestation2
	for _, index1 := range att.Attestation1.AttestingIndices {
		for _, index2 := range att.Attestation2.AttestingIndices {
			if index1 == index2 {
				indices = append(indices, index1)
			}
		}
	}

	// Iterate over indices of slashed validators
	var slashedValidators []Slashing
	for _, index := range indices {
		slashing := Slashing{
			AttestationViolation: true,
			ValidatorIndex:       index,
		}

		slashedValidators = append(slashedValidators, slashing)
	}

	return slashedValidators
}

func extractProposerViolations(prop ProposerViolation, slashed []Slashing) []Slashing {
	// Index 1
	index := prop.SignedHeader1.Message.ProposerIndex

	var exists bool
	for _, slashed := range slashed {
		if slashed.Slot == index {
			exists = true
		}
	}

	if !exists {
		validator := Slashing{
			ProposerViolation: true,
			ValidatorIndex:    index,
			Slot:              prop.SignedHeader1.Message.Slot,
		}

		slashed = append(slashed, validator)
	}

	// Index 2
	index = prop.SignedHeader2.Message.ProposerIndex

	exists = false
	for _, slashed := range slashed {
		if slashed.ValidatorIndex == index {
			exists = true
		}
	}

	if !exists {
		validator := Slashing{
			ProposerViolation: true,
			ValidatorIndex:    index,
			Slot:              prop.SignedHeader2.Message.Slot,
		}

		slashed = append(slashed, validator)
	}

	return slashed
}

func findSlashings(block BlockData, slot string) SlashingEvent {
	// Init slashings struct
	var slashings []Slashing

	// Pointers for shorter lines
	attSlashings := &block.Block.Message.Body.AttesterSlashings
	propSlashings := &block.Block.Message.Body.ProposerSlashings

	// Length check; if zero slashings, return immediately
	if len(*attSlashings) == 0 && len(*propSlashings) == 0 {
		return SlashingEvent{Slot: slot}
	}

	// Look for attestation violations
	if len(*attSlashings) != 0 {
		for _, attSlashing := range *attSlashings {
			// Extract attestation violations
			slashedValidators := extractAttestionViolations(attSlashing)

			// Merge the two slices
			slashings = append(slashings, slashedValidators...)
		}
	}

	// Look for proposal violations
	if len(*propSlashings) != 0 {
		for _, propSlashing := range *propSlashings {
			// Extract proposer violations
			slashedValidators := extractProposerViolations(propSlashing, slashings)

			// Merge the two slices
			slashings = append(slashings, slashedValidators...)
		}
	}

	// Set block correctly for each slashed validator
	for _, slashing := range slashings {
		slashing.Slot = block.Block.Message.Slot
	}

	event := SlashingEvent{
		Slashings:     slashings,
		AttSlashings:  len(*attSlashings),
		PropSlashings: len(*propSlashings),
		Slot:          slot,
	}

	return event
}

func broadcastSlashing(squeue *queue.SendQueue, config *config.Config, slashingString string) {
	/*
		Broadcasts the slashing event to all configured channels.

		1. Telegram announcement channel
		2. Discord groups configured
		3. Telegram subscribers (per-chat)
	*/

	// Send to Telegram channel
	if config.Broadcast.TelegramChannel != 0 {
		// Create message object
		message := queue.Message{
			Type:      "telegram",
			Recipient: config.Broadcast.TelegramChannel,
			Message:   slashingString,
			Sopts:     tb.SendOptions{ParseMode: "MarkdownV2", DisableWebPagePreview: true},
		}

		// Add to queue -> send
		queue.AddToQueue(squeue, &message)
		log.Debug().Msg("📢 Broadcast slashing to configured channel!")
	}

	// Sleep a while before starting the mass-send so the channel message sends
	time.Sleep(time.Second)

	// Loop over Telegram subscribers
	for _, chatId := range config.Broadcast.TelegramSubscribers {
		// Create message object
		message := queue.Message{
			Type:      "telegram",
			Recipient: chatId,
			Message:   slashingString,
			Sopts:     tb.SendOptions{ParseMode: "MarkdownV2", DisableWebPagePreview: true},
		}

		// Add to queue -> send
		queue.AddToQueue(squeue, &message)
	}

	// Log amount of sent broadcasts
	log.Debug().Msgf("📢 Broadcast slashing to %d chats", len(config.Broadcast.TelegramSubscribers))
}
