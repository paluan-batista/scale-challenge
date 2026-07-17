package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"scale-challenge/internal/simulator"
)

func main() {
	baseURL := flag.String("base-url", "", "API base URL (required for future HTTP delivery)")
	scenarioPath := flag.String("scenario", "", "path to a versioned scenario (required)")
	seed := flag.Int64("seed", 42, "deterministic scenario seed")
	frequency := flag.Int("frequency-ms", -1, "override scenario frequency in milliseconds")
	flag.Parse()

	if strings.TrimSpace(*baseURL) == "" || strings.TrimSpace(*scenarioPath) == "" || *seed < 0 {
		log.Print("base-url, scenario, and a non-negative seed are required")
		os.Exit(2)
	}

	scenario, err := simulator.Load(*scenarioPath)
	if err != nil {
		log.Printf("load simulator scenario: %v", err)
		os.Exit(2)
	}
	if *frequency >= 0 {
		scenario.FrequencyMS = *frequency
	}
	events, err := simulator.Sequence(scenario, *seed)
	if err != nil {
		log.Printf("validate simulator configuration: %v", err)
		os.Exit(2)
	}

	encoder := json.NewEncoder(os.Stdout)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			log.Printf("write deterministic event: %v", err)
			os.Exit(1)
		}
	}
	log.Printf("simulator emitted %d deterministic events for %s at %dms; HTTP delivery starts in T03", len(events), *baseURL, scenario.FrequencyMS)
}
