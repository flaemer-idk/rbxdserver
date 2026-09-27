package placesindex

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reRobloxVersion = regexp.MustCompile(`(?i)roblox_version\s*=\s*['"]?([^'"\r\n#\s]+)['"]?`)

type Place struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Creator       string `json:"creator,omitempty"`
	Created       string `json:"created,omitempty"`
	RobloxVersion string `json:"roblox_version,omitempty"`
}

type Index struct {
	dir string
}

func New(dir string) *Index {
	return &Index{dir: dir}
}

func parseRobloxVersionFromTOML(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	matches := reRobloxVersion.FindStringSubmatch(string(data))
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func (idx *Index) GetRobloxVersion(slug string) string {
	if slug == "" {
		return ""
	}
	placeDir := filepath.Join(idx.dir, slug)
	tomlPath := filepath.Join(placeDir, "GameConfig.toml")
	return parseRobloxVersionFromTOML(tomlPath)
}

func (idx *Index) Scan() []Place {
	places := make([]Place, 0) // никогда не null в JSON

	entries, err := os.ReadDir(idx.dir)
	if err != nil {
		log.Printf("Failed to read places dir: %v", err)
		return places
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		slug := entry.Name()
		placeDir := filepath.Join(idx.dir, slug)
		tomlPath := filepath.Join(placeDir, "GameConfig.toml")

		if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
			continue
		}

		place := Place{
			Slug:          slug,
			Name:          slug,
			RobloxVersion: parseRobloxVersionFromTOML(tomlPath),
		}

		infoPath := filepath.Join(placeDir, "info.json")
		if data, err := os.ReadFile(infoPath); err == nil {
			var info struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Creator     string `json:"creator"`
				Created     string `json:"created"`
			}
			if err := json.Unmarshal(data, &info); err == nil {
				if info.Name != "" {
					place.Name = info.Name
				}
				place.Description = info.Description
				place.Creator = info.Creator
				place.Created = info.Created
			}
		}

		places = append(places, place)
	}

	return places
}
