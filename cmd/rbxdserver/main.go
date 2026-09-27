package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rbxdserver/internal/api"
	"rbxdserver/internal/cdnweb"
	"rbxdserver/internal/config"
	"rbxdserver/internal/placesindex"
	"rbxdserver/internal/session"
	"rbxdserver/internal/supervisor"
)

func main() {
	cfg, err := config.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatalf("config: %v", err)
	}

	log.Printf("rbxdserver: API :%d, CDN :%d (rbxd 'webserver'), places %q, skins %q, web cooldown %s",
		cfg.Port, cfg.CDNPort, cfg.PlacesDir, cfg.SkinsDir, cfg.WebCooldown)
	log.Printf("trust model: no auth — LAN-only, single user (see DESIGN.md)")

	index := placesindex.New(cfg.PlacesDir)

	cdn := cdnweb.New(cfg)

	var sup *supervisor.Supervisor
	sessMgr := session.NewManager(func() {
		log.Println("Nobody in game. Triggering auto-shutdown.")
		sup.StopCurrentPlace()
	}, cfg.EmptyTimeout)
	sup = supervisor.New(cfg, sessMgr)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           api.NewRouter(cfg, sup, sessMgr, index).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       180 * time.Second, // /start ждёт готовности RCC — до ~2.5 мин
		IdleTimeout:       120 * time.Second,
		// WriteTimeout не ставим: он рвёт долгоживущие WebSocket-соединения.
	}

	go func() {
		log.Printf("Starting rbxdserver API on :%d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	// CDN-веб поднимаем после API, чтобы порт уже слушался, когда панель откроют.
	cdn.Start()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	<-stopChan

	log.Println("Shutting down rbxdserver...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("API shutdown: %v", err)
	}

	sup.Shutdown() // сессию — без кулдауна, всё убить
	cdn.Stop()     // CDN-веб остановить
	log.Println("Goodbye.")
}
