package config

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port      int
	RfdDir    string
	PlacesDir string
	StateDir  string
	Token     string
	Test      bool
}

func ParseFlags() *Config {
	port := flag.Int("port", 8080, "HTTP/WS API port")
	rfdDir := flag.String("rfd", "", "Path to the unified rfd-fork directory (containing Source/ and Roblox/)")
	placesDir := flag.String("places", "", "Path to Places directory")
	stateDir := flag.String("state-dir", "", "Directory for state/logs (deprecated)")
	dataDir := flag.String("data-dir", "", "Directory for daemon state, logs, and sessions")
	token := flag.String("token", "", "Shared secret for API auth")
	tokenFile := flag.String("token-file", "", "File containing shared secret for API auth (NixOS mode)")
	test := flag.Bool("test", false, "Enable test mode (disables headless cage rendering for server)")

	flag.Parse()

	if *rfdDir == "" || *placesDir == "" {
		log.Fatal("--rfd and --places flags are required")
	}

	finalDataDir := *dataDir
	if finalDataDir == "" {
		finalDataDir = *stateDir
	}

	if finalDataDir == "" {
		log.Fatal("ERROR: --data-dir flag is required. Please specify the data directory!")
	}

	absRfd, err := filepath.Abs(*rfdDir)
	if err != nil {
		log.Fatalf("Failed to resolve absolute path for --rfd: %v", err)
	}

	mainPy := filepath.Join(absRfd, "Source", "_main.py")
	if _, err := os.Stat(mainPy); os.IsNotExist(err) {
		log.Fatalf("RFD main script not found at %s. Please check your --rfd path!", mainPy)
	}

	absPlaces, err := filepath.Abs(*placesDir)
	if err != nil {
		log.Fatalf("Failed to resolve absolute path for --places: %v", err)
	}

	absStateDir, err := filepath.Abs(finalDataDir)
	if err != nil {
		log.Fatalf("Failed to resolve absolute path for --data-dir: %v", err)
	}

	finalToken := *token
	if *tokenFile != "" {
		data, err := os.ReadFile(*tokenFile)
		if err != nil {
			log.Fatalf("Failed to read token file: %v", err)
		}
		finalToken = strings.TrimSpace(string(data))
	}

	if err := os.MkdirAll(absStateDir, 0755); err != nil {
		log.Fatalf("Failed to create state/data dir: %v", err)
	}

	return &Config{
		Port:      *port,
		RfdDir:    absRfd,
		PlacesDir: absPlaces,
		StateDir:  absStateDir,
		Token:     finalToken,
		Test:      *test,
	}
}