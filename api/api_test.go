package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"slashcaster/config"
)

func TestSlashingExtraction(t *testing.T) {
	// Load config
	cfg := config.LoadConfig("../config")

	// Create http-client
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	// Test with expected slot -> slashing count
	slashings := map[string]int{
		"0":       0,
		"6669":    1,
		"475802":  2,
		"1856963": 1,
		"1510279": 1,
		"2638206": 1,
		"2724285": 1,
		"2755555": 1,
	}

	for slot, count := range slashings {
		block, err := getSlot(&client, cfg, slot)
		if err != nil {
			t.Log("Error getting block at slot:", err)
			t.Fail()
		}

		foundSlashings := findSlashings(block, slot)
		if len(foundSlashings.Slashings) != count {
			// Fail if wrong slashing count found
			t.Logf("Expected %d slashings at slot %s, but got %d",
				count, slot, len(foundSlashings.Slashings),
			)

			t.Fail()
		} else {
			fmt.Printf("Found expected %d slashing(s) in slot %s\n", count, slot)
		}

		// Sleep to stay within API limits
		time.Sleep(time.Duration(time.Millisecond * 300))
	}
}
