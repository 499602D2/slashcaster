package api

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"slashcaster/config"
	"slashcaster/queue"

	"github.com/rs/zerolog/log"
)

func bumpStats(conf *config.Config, currSlot int64, blockTime int64) {
	conf.Mutex.Lock()
	conf.Stats.BlocksParsed++
	conf.Stats.CurrentSlot = currSlot
	conf.Stats.BlockTime = blockTime
	conf.Mutex.Unlock()
}

func prettyPrintJson(body []byte) {
	// Pretty-print JSON
	var prettyJSON bytes.Buffer
	error := json.Indent(&prettyJSON, body, "", "  ")

	if error != nil {
		log.Error().Err(error).Msg("JSON parse error")
		return
	}

	log.Printf("Block data: %s", prettyJSON.String())
}

func doGetRequest(client *http.Client, url string) (BlockData, error) {
	// Perform GET-requests
	resp, err := client.Get(url)

	// Block
	var block BlockData

	if err != nil {
		log.Error().Err(err).Msg("Error performing GET request")
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

func getHead(client *http.Client, config *config.Config) (string, error) {
	// Endpoint
	url := config.Tokens.Infura + "/eth/v1/node/syncing"

	// Perform GET-requests
	resp, err := client.Get(url)

	// Chain head
	var headData HeadData

	if err != nil {
		log.Error().Err(err).Msg("Error getting chain head! Error")
		return "", err
	}

	// Read bytes from returned data
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	// Unmarshal into our block object
	json.Unmarshal(bodyBytes, &headData)

	return headData.HeadData.HeadSlot, nil
}

func getSlot(client *http.Client, config *config.Config, slot string) (BlockData, error) {
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
	return doGetRequest(client, config.Tokens.Infura+blockEndpoint+slot)
}

func SlotStreamer(squeue *queue.SendQueue, conf *config.Config) {
	// HTTP client
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	// Get chain head
	headSlot, err := getHead(&client, conf)

	if err != nil {
		log.Fatal().Err(err).Msg("Error starting slotStreamer!")
	} else {
		log.Printf("[slotStreamer] Got chain head, slot=%s", headSlot)
	}

	// Covert head to an i64 for maths
	currSlot, _ := strconv.ParseInt(headSlot, 10, 64)

	// Check how far behind we are
	if conf.Stats.CurrentSlot != 0 {
		delta := currSlot - conf.Stats.CurrentSlot

		if delta > 0 {
			currSlot = conf.Stats.CurrentSlot + 1
			log.Printf(
				"[slotStreamer] %d slot(s) behind: starting sync from slot=%d",
				delta, conf.Stats.CurrentSlot)
		}
	}

	// Altair activation time + slot
	altairStart := int64(1635332183)
	altairSlot := int64(74240 * 32)

	// Get current block time
	slotSinceAltair := currSlot - altairSlot
	nextBlockTime := altairStart + slotSinceAltair*12

	// Start streaming from headSlot
	for {
		if conf.Debug {
			log.Printf("Streaming slot %d", currSlot)
		}

		// Set blocktime to the last "next" block
		currentBlockTime := nextBlockTime

		// Stringify slot
		slot := strconv.FormatInt(int64(currSlot), 10)

		// Get block
		block, err := getSlot(&client, conf, slot)

		if err != nil {
			log.Error().Err(err).Msgf("Error getting block %s", slot)

			// If we error out due to e.g. network conditions, sleep and retry
			time.Sleep(time.Second * time.Duration(5))
			continue
		}

		if conf.Debug {
			log.Print("↳ Got block")
		}

		// Parse block for slashings
		foundSlashings := findSlashings(block, slot)

		if len(foundSlashings.Slashings) > 0 {
			// Log slashing event
			log.Printf("[slotStreamer] Found %d slashing(s) in slot=%s", len(foundSlashings.Slashings), slot)

			// Produce a message from found slashings
			slashStr := slashingString(foundSlashings, conf)

			// Broadcast the slashing
			broadcastSlashing(squeue, conf, slashStr)

			// Save slashing in statistics
			config.SlashingObserved(conf, foundSlashings.AttSlashings, foundSlashings.PropSlashings, currentBlockTime)
		}

		// Calculate when next slot arrives
		nextBlockTime = currentBlockTime + 12

		// Calculate how long to sleep for
		currTime := time.Now().Unix()
		nextBlockIn := nextBlockTime - currTime

		if conf.Debug {
			log.Printf("↳ Next slot in %d seconds", nextBlockIn)
		}

		// Set stats in goroutine
		go bumpStats(conf, currSlot, currentBlockTime)

		if nextBlockIn >= 0 {
			// Sleep until next block, add 3 seconds for some propagation time
			time.Sleep(time.Second * time.Duration(nextBlockIn+3))
		} else {
			// Sleep >200 ms (Infura's rate-limit)
			time.Sleep(time.Millisecond * time.Duration(300))
		}

		// Loop over to the next slot if no errors during request
		if err == nil {
			// Next slot
			currSlot++
		}
	}
}
