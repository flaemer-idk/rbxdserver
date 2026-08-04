package api

import (
	"fmt"
	"net/http"

	"rbxdserver/internal/config"
	"rbxdserver/internal/placesindex"
	"rbxdserver/internal/session"
	"rbxdserver/internal/supervisor"
)

type Router struct {
	cfg  *config.Config
	sup  *supervisor.Supervisor
	sess *session.Manager
	idx  *placesindex.Index
}

func NewRouter(cfg *config.Config, sup *supervisor.Supervisor, sess *session.Manager, idx *placesindex.Index) *Router {
	return &Router{cfg: cfg, sup: sup, sess: sess, idx: idx}
}

func (rt *Router) Start() error {
	mux := http.NewServeMux()

	// Мутирующие эндпоинты (защищены токеном)
	mux.HandleFunc("/start", AuthMiddleware(rt.cfg.Token, rt.handleStart))
	mux.HandleFunc("/stop", AuthMiddleware(rt.cfg.Token, rt.handleStop))
	
	// Чтение статуса (защищено токеном)
	mux.HandleFunc("/status", AuthMiddleware(rt.cfg.Token, rt.handleStatus))
	mux.HandleFunc("/logs", AuthMiddleware(rt.cfg.Token, rt.handleLogs))
	
	// Публичные / LAN эндпоинты
	mux.HandleFunc("/places", rt.handlePlaces)
	
	// WebSocket presence (защищен токеном, WS-клиент должен передавать заголовок)
	mux.HandleFunc("/session", AuthMiddleware(rt.cfg.Token, rt.handleSession))

	return http.ListenAndServe(fmt.Sprintf(":%d", rt.cfg.Port), mux)
}