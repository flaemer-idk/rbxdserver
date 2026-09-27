package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"rbxdserver/internal/config"
)

// Каталог скинов rbxd: <rfd>/data/skins/<name>.json — тот же каталог, который
// читает вебсервер rbxd на каждый запрос аватара (endpoints/avatar.py).
// rbxdclient через эти эндпойнты может смотреть и менять скины по сети,
// не имея доступа к файловой системе сервера.

var reSkinName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

type skinEntry struct {
	Name     string `json:"name"`
	Modified string `json:"modified,omitempty"`
}

func skinPath(cfg *config.Config, name string) (string, bool) {
	if !reSkinName.MatchString(name) || strings.Contains(name, "..") {
		return "", false
	}
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}
	return filepath.Join(cfg.SkinsDir, name), true
}

// handleSkinsList — GET /skins → [{"name": "default.json", "modified": ...}]
func (rt *Router) handleSkinsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := os.ReadDir(rt.cfg.SkinsDir)
	out := make([]skinEntry, 0, len(entries))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			entry := skinEntry{Name: e.Name()}
			if info, err := e.Info(); err == nil {
				entry.Modified = info.ModTime().UTC().Format(time.RFC3339)
			}
			out = append(out, entry)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// handleSkinFile — GET /skins/<name>.json (чтение) и PUT/POST (запись; тело —
// валидный JSON). Путь строится из проверенного имени, traversal исключён.
func (rt *Router) handleSkinFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/skins/")
	path, ok := skinPath(rt.cfg, name)
	if !ok {
		http.Error(w, "Invalid skin name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodPut, http.MethodPost:
		var pretty json.RawMessage
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&pretty); err != nil {
			http.Error(w, "Body is not valid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		data, _ := json.MarshalIndent(pretty, "", "  ")
		if err := os.WriteFile(path, data, 0644); err != nil {
			http.Error(w, "Failed to write skin", http.StatusInternalServerError)
			return
		}
		// Скин читается вебсервером rbxd на каждый запрос — изменения
		// подхватываются вживую, рестарт не нужен.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "OK", "name": filepath.Base(path)})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
