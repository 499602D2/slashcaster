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

	"slashcaster/config"
	"slashcaster/queue"
)

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
		log.Println("Error performing att. slashing GET request:", err)
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

func getHead(config *config.Config) (string, error) {
	// Endpoint
	url := config.Tokens.Infura + "/eth/v1/node/syncing"

	// Perform GET-requests
	resp, err := http.Get(url)

	// Chain head
	var headData HeadData

	if err != nil {
		log.Println("Error getting chain head! Error:", err)
		return "", err
	}

	// Read bytes from returned data
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	// Unmarshal into our block object
	json.Unmarshal(bodyBytes, &headData)

	return headData.HeadData.HeadSlot, nil
}

func getSlot(config *config.Config, slot string) (BlockData, error) {
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
	/*
		For testing with older slots, specify e.g.:
			var err error
			headSlot := "2624391"
	*/

	// Get chain head
	headSlot, err := getHead(conf)

	if err != nil {
		log.Fatalln("Error starting slotStreamer!")
	} else {
		log.Printf("Got chain head, slot=%s\n", headSlot)
	}

	// Covert head to an integer for maths
	currSlot, _ := strconv.Atoi(headSlot)

	// Check how far behind we are
	if conf.Stats.CurrentSlot != 0 {
		delta := currSlot - conf.Stats.CurrentSlot

		if delta > 0 {
			currSlot = conf.Stats.CurrentSlot - 1
			log.Printf(
				"[slotStreamer] %d slots behind: starting sync from slot=%d",
				delta, conf.Stats.CurrentSlot)
		}
	}

	// Altair activation time + slot
	altairStart := 1635332183
	altairSlot := 74240 * 32

	// Get current block time
	slotSinceAltair := currSlot - altairSlot
	nextBlockTime := int64(altairStart + slotSinceAltair*12)

	// Start streaming from headSlot
	for {
		if conf.Debug {
			fmt.Println("Streaming slot", currSlot)
		}

		// Set blocktime to the last "next" block
		currentBlockTime := nextBlockTime

		// Stringify slot
		slot := strconv.FormatInt(int64(currSlot), 10)

		// Get block
		block, err := getSlot(conf, slot)

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

		if conf.Debug {
			fmt.Printf("↳ Next slot in %d seconds\n\n", nextBlockIn)
		}

		if nextBlockIn >= 0 {
			// Sleep until next block, add 3 seconds for some propagation time
			time.Sleep(time.Second * time.Duration(nextBlockIn+3))
		} else {
			// Sleep _at minimum_ 0.2 seconds, which is Infura's rate-limit
			time.Sleep(time.Second * time.Duration(1))
		}

		// Loop over to the next slot if no errors during request
		if err == nil {
			// Next slot
			currSlot++

			// Set stats
			conf.Stats.CurrentSlot = currSlot
			conf.Stats.BlockTime = currentBlockTime
		}
	}
}
