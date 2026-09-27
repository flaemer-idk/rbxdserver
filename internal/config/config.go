package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Port         int           // HTTP/WS API порт
	CDNPort      int           // фиксированный порт постоянного CDN-веба
	RfdDir       string        // корень rbxd (Source/, data/)
	PlacesDir    string        // каталог плейсов (slug/GameConfig.toml)
	DataDir      string        // состояние демона (favorites, wine-префикс)
	SkinsDir     string        // каталог скинов rbxd (skins/*.json)
	WebCooldown  time.Duration // сколько живёт веб сессии после конца сессии
	EmptyTimeout time.Duration // никого в игре столько — автостоп сессии
	Test         bool          // RFD_NO_CAGE=1 для дочерних RCC
}

func ParseFlags(argv []string) (*Config, error) {
	fs := flag.NewFlagSet("rbxdserver", flag.ContinueOnError)

	port := fs.Int("port", 8080, "HTTP/WS API port")
	cdnPort := fs.Int("cdn-port", 8090, "Fixed port of the always-on CDN webserver (rbxd 'webserver' mode)")
	rfdDir := fs.String("rfd", "", "Path to the rbxd directory (containing Source/ and data/)")
	placesDir := fs.String("places", "", "Path to places directory (<slug>/GameConfig.toml)")
	dataDir := fs.String("data-dir", "", "Directory for daemon state (favorites, wine prefix)")
	skinsDir := fs.String("skins-dir", "", "Path to rbxd skins directory (defaults to <rfd>/data/skins)")
	webCooldown := fs.Duration("web-cooldown", 4*time.Minute, "How long the session webserver outlives the session")
	emptyTimeout := fs.Duration("empty-timeout", 5*time.Minute, "Auto-stop the session when nobody is in game for this long (presence from rbxd)")
	test := fs.Bool("test", false, "Enable test mode (sets RFD_NO_CAGE=1 for child RCC)")

	if err := fs.Parse(argv); err != nil {
		return nil, err
	}

	if *rfdDir == "" || *placesDir == "" {
		return nil, errors.New("--rfd and --places flags are required")
	}
	if *dataDir == "" {
		return nil, errors.New("--data-dir flag is required")
	}
	if *cdnPort == *port {
		return nil, errors.New("--cdn-port must differ from --port")
	}

	absRfd, err := filepath.Abs(*rfdDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --rfd: %w", err)
	}
	mainPy := filepath.Join(absRfd, "Source", "_main.py")
	if _, err := os.Stat(mainPy); err != nil {
		return nil, fmt.Errorf("rbxd main script not found at %s: %w", mainPy, err)
	}

	absPlaces, err := filepath.Abs(*placesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --places: %w", err)
	}
	if _, err := os.Stat(absPlaces); err != nil {
		return nil, fmt.Errorf("places dir not found at %s: %w", absPlaces, err)
	}

	absData, err := filepath.Abs(*dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --data-dir: %w", err)
	}
	if err := os.MkdirAll(absData, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	skins := *skinsDir
	if skins == "" {
		skins = filepath.Join(absRfd, "data", "skins")
	}
	absSkins, err := filepath.Abs(skins)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --skins-dir: %w", err)
	}
	if err := os.MkdirAll(absSkins, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skins dir: %w", err)
	}

	return &Config{
		Port:         *port,
		CDNPort:      *cdnPort,
		RfdDir:       absRfd,
		PlacesDir:    absPlaces,
		DataDir:      absData,
		SkinsDir:     absSkins,
		WebCooldown:  *webCooldown,
		EmptyTimeout: *emptyTimeout,
		Test:         *test,
	}, nil
}
