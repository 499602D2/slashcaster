package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"slashcaster/config"
	"slashcaster/queue"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

func bumpStats(conf *config.Config, currSlot int64, blockTime int64) {
	conf.Mutex.Lock()
	conf.Stats.BlocksParsed++
	conf.Stats.CurrentSlot = currSlot
	conf.Stats.BlockTime = blockTime
	conf.Mutex.Unlock()
}

func doGetRequest(client *resty.Client, url string) (BlockData, error) {
	// Perform GET-requests
	resp, err := client.R().Get(url)

	// Block
	var block BlockData

	if err != nil {
		log.Error().Err(err).Msg("Error performing GET request")
		return block, err
	}

	// Unmarshal into our block object
	err = json.Unmarshal(resp.Body(), &block)

	if err != nil {
		log.Trace().Err(err).Msg("Error unmarshaling data to block")
		return block, err
	}

	return block, err
}

func getHead(client *resty.Client, config *config.Config) (string, error) {
	// Endpoint
	url := fmt.Sprintf("%s/eth/v1/node/syncing", config.Tokens.Infura)

	// Perform GET-requests
	resp, err := client.R().Get(url)

	if resp.IsError() {
		err = errors.New(fmt.Sprintf("Request failed with status code = %d", resp.StatusCode()))
		log.Error().Err(resp.Request.Context().Err()).Msg(err.Error())

		return "", err
	}

	// Chain head
	var headData HeadData

	if err != nil {
		log.Error().Err(err).Msg("Error getting chain head! Error")
		return "", err
	}

	// Unmarshal into our block object
	err = json.Unmarshal(resp.Body(), &headData)

	if err != nil {
		log.Trace().Err(err).Msg("Error unmarshaling head data to block")
		return "", err
	}

	return headData.HeadData.HeadSlot, nil
}

func getSlot(client *resty.Client, config *config.Config, slot string) (BlockData, error) {
	// If slot is pre-Altair, use eth/v1 endpoint
	altairSlot := 74240 * 32
	currSlot, _ := strconv.Atoi(slot)

	var blockEndpoint string
	if currSlot >= altairSlot {
		blockEndpoint = "eth/v2/beacon/blocks"
	} else {
		blockEndpoint = "eth/v1/beacon/blocks"
	}

	// Get block at slot
	slotUrl := fmt.Sprintf("%s/%s/%s", config.Tokens.Infura, blockEndpoint, slot)
	block, err := doGetRequest(client, slotUrl)

	if err != nil {
		log.Warn().Msgf("Getting slot=%s failed: sleeping for 60 seconds...", slot)
		time.Sleep(time.Second * 60)

		return getSlot(client, config, slot)
	}

	return block, err
}

func SlotStreamer(squeue *queue.SendQueue, conf *config.Config) {
	// HTTP client
	client := resty.New()
	client.SetTimeout(time.Duration(30 * time.Second))

	// Get chain head
	headSlot, err := getHead(client, conf)

	if err != nil {
		log.Fatal().Err(err).Msg("Error starting slotStreamer!")
	} else {
		log.Info().Msgf("[slotStreamer] Got chain head, slot=%s", headSlot)
	}

	// Covert head to an i64 for maths
	currSlot, _ := strconv.ParseInt(headSlot, 10, 64)

	// Check how far behind we are
	if conf.Stats.CurrentSlot != 0 {
		delta := currSlot - conf.Stats.CurrentSlot

		if delta > 0 {
			currSlot = conf.Stats.CurrentSlot + 1
			log.Debug().Msgf("[slotStreamer] %d slot(s) behind: starting sync from slot=%d",
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
			log.Debug().Msgf("Streaming slot %d", currSlot)
		}

		// Set blocktime to the last "next" block
		currentBlockTime := nextBlockTime

		// Stringify slot
		slot := strconv.FormatInt(int64(currSlot), 10)

		// Get block
		block, err := getSlot(client, conf, slot)

		if err != nil {
			log.Error().Err(err).Msgf("Error getting block %s", slot)

			// If we error out due to e.g. network conditions, sleep and retry
			time.Sleep(time.Second * time.Duration(5))
			continue
		}

		if conf.Debug {
			log.Debug().Msg("↳ Got block")
		}

		// Parse block for slashings
		foundSlashings := findSlashings(block, slot)

		if len(foundSlashings.Slashings) > 0 {
			// Log slashing event
			log.Info().Msgf("[slotStreamer] Found %d slashing(s) in slot=%s", len(foundSlashings.Slashings), slot)

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
			log.Debug().Msgf("↳ Next slot in %d seconds", nextBlockIn)
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
