package api

import (
	"encoding/json"
	"log"
	"net/http"

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

	// Возвращаем динамические порты обратно клиенту в JSON
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
		"place":    place,
		"state":    state,
		"players":  rt.sess.Count(),
		"rcc_port": rccPort,
		"web_port": webPort,
	})
}

func (rt *Router) handlePlaces(w http.ResponseWriter, r *http.Request) {
	places := rt.idx.Scan()
	json.NewEncoder(w).Encode(places)
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