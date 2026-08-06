package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

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

	_, _, rccPort, webPort := rt.sup.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "OK",
		"rcc_port": rccPort,
		"web_port": webPort,
	})
}

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

func (rt *Router) handleStatus(w http.ResponseWriter, r *http.Request) {
	place, state, rccPort, webPort := rt.sup.GetStatus()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"place":       place,
		"state":       state,
		"players":     rt.sess.Count(),
		"player_list": rt.sess.Players(),
		"rcc_port":    rccPort,
		"web_port":    webPort,
	})
}

func (rt *Router) handlePlaces(w http.ResponseWriter, r *http.Request) {
	places := rt.idx.Scan()
	json.NewEncoder(w).Encode(places)
}

func (rt *Router) handlePlaceFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	slug := parts[1]
	fileName := parts[2]

	if fileName != "icon.png" && fileName != "icon.jpg" && fileName != "banner.png" && fileName != "banner.jpg" {
		http.Error(w, "Forbidden asset type", http.StatusForbidden)
		return
	}

	safePath := filepath.Clean(filepath.Join(rt.cfg.PlacesDir, slug, fileName))
	if !strings.HasPrefix(safePath, filepath.Clean(rt.cfg.PlacesDir)) {
		http.Error(w, "Forbidden path traversal attempt", http.StatusForbidden)
		return
	}

	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, safePath)
}

func (rt *Router) handleGetFavorites(w http.ResponseWriter, r *http.Request) {
	favsPath := filepath.Join(rt.cfg.StateDir, "favorites.json")
	if _, err := os.Stat(favsPath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	data, err := os.ReadFile(favsPath)
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

	favsPath := filepath.Join(rt.cfg.StateDir, "favorites.json")
	var favs []string
	if _, err := os.Stat(favsPath); err == nil {
		data, _ := os.ReadFile(favsPath)
		json.Unmarshal(data, &favs)
	}

	found := false
	var newFavs []string
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "OK",
		"favorite": !found,
	})
}

func (rt *Router) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs := rt.sup.GetLogs()
	json.NewEncoder(w).Encode(logs)
}

func (rt *Router) handleSession(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	place := r.URL.Query().Get("place")

	if user == "" || place == "" {
		http.Error(w, "Missing user or place", http.StatusBadRequest)
		return
	}

	currPlace, state, _, _ := rt.sup.GetStatus()
	if state != "Running" || currPlace != place {
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

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}