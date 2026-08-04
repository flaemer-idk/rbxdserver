package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"rbxdserver/internal/api"
	"rbxdserver/internal/config"
	"rbxdserver/internal/placesindex"
	"rbxdserver/internal/session"
	"rbxdserver/internal/supervisor"
)

func main() {
	cfg := config.ParseFlags()

	if cfg.Token == "" {
		log.Println("WARNING: --token is not set. API is UNPROTECTED and open to anyone!")
	}

	// Инициализация компонентов
	index := placesindex.New(cfg.PlacesDir)
	sup := supervisor.New(cfg)
	sessMgr := session.NewManager(func() {
		log.Println("No active sessions. Triggering auto-shutdown.")
		sup.StopCurrentPlace()
	})

	// Запуск HTTP/WS сервера
	router := api.NewRouter(cfg, sup, sessMgr, index)
	
	go func() {
		log.Printf("Starting rbxdserver on :%d", cfg.Port)
		if err := router.Start(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Ожидание сигнала завершения
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	<-stopChan

	log.Println("Shutting down rbxdserver...")
	sup.StopCurrentPlace()
}