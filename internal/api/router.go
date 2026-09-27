package api

import (
	"net/http"
	"sync"

	"rbxdserver/internal/config"
	"rbxdserver/internal/placesindex"
	"rbxdserver/internal/session"
	"rbxdserver/internal/supervisor"
)

// Router — HTTP/WS API. Без аутентификации: сервис рассчитан на доверенную
// LAN и одного пользователя (решение зафиксировано в DESIGN.md).
type Router struct {
	cfg   *config.Config
	sup   *supervisor.Supervisor
	sess  *session.Manager
	idx   *placesindex.Index
	favMu sync.Mutex // read-modify-write favorites.json без гонок
}

func NewRouter(cfg *config.Config, sup *supervisor.Supervisor, sess *session.Manager, idx *placesindex.Index) *Router {
	return &Router{cfg: cfg, sup: sup, sess: sess, idx: idx}
}

func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()

	// Тестовая HTML-панель (статус, кнопки, join-команды).
	mux.HandleFunc("/", rt.handleIndex)

	mux.HandleFunc("/start", rt.handleStart)
	mux.HandleFunc("/stop", rt.handleStop)
	mux.HandleFunc("/kill", rt.handleKill)
	mux.HandleFunc("/status", rt.handleStatus)
	mux.HandleFunc("/logs", rt.handleLogs)

	mux.HandleFunc("/places", rt.handlePlaces)
	mux.HandleFunc("/places/", rt.handlePlaceFile)

	mux.HandleFunc("/favorites", rt.handleGetFavorites)
	mux.HandleFunc("/favorites/toggle", rt.handleToggleFavorite)

	// Каталог скинов rbxd (skins/*.json) — чтение и запись для rbxdclient.
	mux.HandleFunc("/skins", rt.handleSkinsList)
	mux.HandleFunc("/skins/", rt.handleSkinFile)

	// WebSocket presence.
	mux.HandleFunc("/session", rt.handleSession)

	return mux
}
