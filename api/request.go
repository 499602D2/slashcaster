package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/dustin/go-humanize/english"

	"slashcaster/config"
	"slashcaster/queue"

	tb "gopkg.in/tucnak/telebot.v2"
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

	// Length check
	if len(*attSlashings) == 0 && len(*propSlashings) == 0 {
		return SlashingEvent{}
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

	// Set block correctly for each slashed validator
	for _, slashing := range slashings {
		slashing.Slot = block.Block.Message.Slot
	}

	// Look for proposal violations
	if len(*propSlashings) != 0 {
		for _, propSlashing := range *propSlashings {
			// Extract attestation violations
			slashings = extractProposerViolations(propSlashing, slashings)
		}
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
			Recipient: int(config.Broadcast.TelegramChannel),
			Message:   slashingString,
			Sopts:     tb.SendOptions{ParseMode: "MarkdownV2", DisableWebPagePreview: true},
		}

		// Add to queue -> send
		queue.AddToQueue(squeue, &message)
		log.Println("📢 Broadcast slashing to configured channel!")
	}

	// Sleep a while before starting the mass-send so the channel message sends
	time.Sleep(time.Second)

	// Loop over subscribers
	for _, chatId := range config.Broadcast.TelegramSubscribers {
		// Create message object
		message := queue.Message{
			Recipient: chatId,
			Message:   slashingString,
			Sopts:     tb.SendOptions{ParseMode: "MarkdownV2", DisableWebPagePreview: true},
		}

		// Add to queue -> send
		queue.AddToQueue(squeue, &message)
	}

	// Log amount of sent broadcasts
	log.Println("📢 Broadcast slashing to", len(config.Broadcast.TelegramSubscribers), "chats!")
}

func prettyPrintJson(body []byte) {
	// Pretty-print JSON
	var prettyJSON bytes.Buffer
	error := json.Indent(&prettyJSON, body, "", "  ")

	if error != nil {
		log.Println("JSON parse error: ", error)
		return
	}

	log.Println("Block data", prettyJSON.String())
}

func doGetRequest(url string) (BlockData, error) {
	// Perform GET-requests
	resp, err := http.Get(url)

	// Block
	var block BlockData

	if err != nil {
		fmt.Println("Error performing att. slashing GET request:", err)
		return block, err
	}

	// Read bytes from returned data
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	// Pretty-print for debug
	// prettyPrintJson(bodyBytes)

	// Unmarshal into our block object
	json.Unmarshal(bodyBytes, &block)

	return block, err
}

func GetHead(config *config.Config) (string, error) {
	// Endpoint
	url := config.Tokens.Infura + "/eth/v1/node/syncing"

	// Perform GET-requests
	resp, err := http.Get(url)

	// Chain head
	var headData HeadData

	if err != nil {
		fmt.Println("Error getting chain head! Error:", err)
		return "", err
	}

	// Read bytes from returned data
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	// Unmarshal into our block object
	json.Unmarshal(bodyBytes, &headData)

	return headData.HeadData.HeadSlot, nil
}

func GetSlot(config *config.Config, slot string) (BlockData, error) {
	// If slot is pre-Altair, use eth/v1 endpoint
	altairSlot := 74240 * 32
	currSlot, _ := strconv.Atoi(slot)

	var blockEndpoint string
	if currSlot >= altairSlot {
		blockEndpoint = "/eth/v2/beacon/blocks/"
	} else {
		blockEndpoint = "/eth/v1/beacon/blocks/"
	}

	// Get block at slot
	return doGetRequest(config.Tokens.Infura + blockEndpoint + slot)
}

func SlotStreamer(squeue *queue.SendQueue, conf *config.Config) {
	// Get chain head
	headSlot, err := GetHead(conf)

	if err != nil {
		log.Fatalln("Error starting slotStreamer!")
	} else {
		log.Printf("Got chain head, slot=%s\n", headSlot)
	}

	// Covert head to an integer for maths
	currSlot, _ := strconv.Atoi(headSlot)

	// Altair activation time + slot
	altairStart := 1635332183
	altairSlot := 74240 * 32

	// Get current block time
	slotSinceAltair := currSlot - altairSlot
	nextBlockTime := int64(altairStart + slotSinceAltair*12)

	// Start streaming from headSlot
	for {
		// Set blocktime to the last "next" block
		currentBlockTime := nextBlockTime

		// Stringify slot
		slot := strconv.FormatInt(int64(currSlot), 10)

		// Get block
		block, err := GetSlot(conf, slot)

		if err != nil {
			log.Println("Error getting block ", slot, ", error:", err.Error())
		}

		// Parse block for slashings
		foundSlashings := findSlashings(block, slot)

		if len(foundSlashings.Slashings) != 0 {
			// Log slashing event
			log.Printf("[slotStreamer] Found %d slashing(s) in slot=%s", len(foundSlashings.Slashings), slot)

			// Produce a message from found slashings
			slashStr := slashingString(foundSlashings, conf)

			// Broadcast the slashing
			go broadcastSlashing(squeue, conf, slashStr)

			// Save slashing in statistics
			config.SlashingObserved(conf, foundSlashings.AttSlashings, foundSlashings.PropSlashings, currentBlockTime)
		}

		// Calculate when next slot arrives
		nextBlockTime = currentBlockTime + 12

		// Calculate how long to sleep for
		currTime := time.Now().Unix()
		nextBlockIn := nextBlockTime - currTime

		if nextBlockIn >= 0 {
			// Sleep until next block, add 3 seconds for some propagation time
			time.Sleep(time.Second * time.Duration(nextBlockIn+3))
		} else {
			// Sleep _at minimum_ 0.2 seconds, which is Infura's rate-limit
			time.Sleep(time.Second * time.Duration(5))
		}

		// Loop over to the next slot if no errors during request
		if err == nil {
			currSlot++
		}
	}
}
