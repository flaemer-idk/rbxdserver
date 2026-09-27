package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"rbxdserver/internal/supervisor"
)

var upgrader = websocket.Upgrader{
	// Панель/клиент могут заходить с любого origin: сервис для доверенной LAN.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleStart — POST /start?place=<slug>.
// Блокируется до полной готовности сессии (веб + RCC READY), может занять
// до ~2.5 минут под Wine. Ответ: порты для подключения игрока.
func (rt *Router) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	place := r.URL.Query().Get("place")
	if place == "" {
		http.Error(w, "Missing 'place' parameter", http.StatusBadRequest)
		return
	}

	if err := rt.sup.StartPlace(place); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	st := rt.sup.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "OK",
		"rcc_port":       st.RccPort,
		"web_port":       st.WebPort,
		"roblox_version": rt.idx.GetRobloxVersion(place),
	})
}

// handleStop — POST /stop. RCC умирает сразу, веб сессии живёт ещё
// --web-cooldown (тёплый веб переиспользуется при быстром перезапуске).
func (rt *Router) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := rt.sup.StopCurrentPlace(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleKill — POST /kill. Жёстко: убивает и RCC, и веб сессии немедленно,
// без кулдауна. Для случая «всё зависло и /stop не помогает».
func (rt *Router) handleKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := rt.sup.KillCurrentPlace(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (rt *Router) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := rt.sup.GetStatus()
	robloxVersion := ""
	if st.Place != "" {
		robloxVersion = rt.idx.GetRobloxVersion(st.Place)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"place": st.Place,
		"state": st.State,
		// Список игроков — из presence rbxd (кто реально в игре).
		"players":        rt.sess.InGame(),
		"player_list":    rt.sess.Players(),
		"players_detail": rt.sess.PlayersDetail(),
		// Легаси: открытые WS-соединения старого клиента (на списки не влияет).
		"connections":    rt.sess.Connections(),
		"rcc_port":       st.RccPort,
		"web_port":       st.WebPort,
		"cdn_port":       rt.cfg.CDNPort,
		"roblox_version": robloxVersion,
	})
}

func (rt *Router) handlePlaces(w http.ResponseWriter, r *http.Request) {
	places := rt.idx.Scan()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(places)
}

// handlePlaceFile — отдаёт обложки плейса (icon/banner) для каталога клиента.
// Защита от path traversal: чистый slug по регэкспу + проверка резолва с
// trailing-сепаратором + ResolveSymlinks (симлинки не выпускаем наружу).
func (rt *Router) handlePlaceFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	slug, fileName := parts[1], parts[2]

	allowed := map[string]bool{
		"icon.png": true, "icon.jpg": true,
		"banner.png": true, "banner.jpg": true,
		"place-icon.png": true, "place-thumbnail.png": true,
	}
	if !allowed[fileName] {
		http.Error(w, "Forbidden asset type", http.StatusForbidden)
		return
	}
	if filepath.Base(slug) != slug || slug == "." || slug == ".." {
		http.Error(w, "Forbidden path", http.StatusForbidden)
		return
	}

	placesRoot, err := filepath.Abs(rt.cfg.PlacesDir)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	safePath := filepath.Join(placesRoot, slug, fileName)
	resolved, err := filepath.EvalSymlinks(safePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if resolved != filepath.Join(placesRoot, slug, fileName) &&
		!strings.HasPrefix(resolved, placesRoot+string(filepath.Separator)) {
		http.Error(w, "Forbidden path traversal attempt", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, resolved)
}

func (rt *Router) handleGetFavorites(w http.ResponseWriter, r *http.Request) {
	data, err := rt.readFavorites()
	if err != nil {
		http.Error(w, "Failed to read favorites", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (rt *Router) handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("place")
	if slug == "" {
		http.Error(w, "Missing 'place' parameter", http.StatusBadRequest)
		return
	}

	rt.favMu.Lock()
	defer rt.favMu.Unlock()

	favsPath := filepath.Join(rt.cfg.DataDir, "favorites.json")
	var favs []string
	if data, err := os.ReadFile(favsPath); err == nil {
		json.Unmarshal(data, &favs)
	}

	found := false
	newFavs := make([]string, 0, len(favs)+1)
	for _, f := range favs {
		if f == slug {
			found = true
		} else {
			newFavs = append(newFavs, f)
		}
	}
	if !found {
		newFavs = append(newFavs, slug)
	}

	newData, err := json.Marshal(newFavs)
	if err != nil {
		http.Error(w, "Failed to marshal favorites", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(favsPath, newData, 0644); err != nil {
		http.Error(w, "Failed to save favorites", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "OK",
		"favorite": !found,
	})
}

func (rt *Router) readFavorites() ([]byte, error) {
	rt.favMu.Lock()
	defer rt.favMu.Unlock()

	favsPath := filepath.Join(rt.cfg.DataDir, "favorites.json")
	data, err := os.ReadFile(favsPath)
	if err != nil {
		return []byte("[]"), nil
	}
	return data, nil
}

func (rt *Router) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs := rt.sup.GetLogs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (rt *Router) handleSession(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	place := r.URL.Query().Get("place")

	if user == "" || place == "" {
		http.Error(w, "Missing user or place", http.StatusBadRequest)
		return
	}

	st := rt.sup.GetStatus()
	if st.State != supervisor.StateRunning || st.Place != place {
		http.Error(w, "Requested place is not running", http.StatusConflict)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	rt.sess.Join(user)
	defer rt.sess.Leave(user)

	// Read deadline: отсутствие пинга в течение 60 с = клиент отвалился.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
