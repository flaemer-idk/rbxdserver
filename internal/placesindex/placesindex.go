package placesindex

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type Place struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Creator     string `json:"creator,omitempty"`
}

type Index struct {
	dir string
}

func New(dir string) *Index {
	return &Index{dir: dir}
}

func (idx *Index) Scan() []Place {
	var places []Place

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
		
		// Место валидно, если есть GameConfig.toml
		if _, err := os.Stat(filepath.Join(placeDir, "GameConfig.toml")); os.IsNotExist(err) {
			continue
		}

		place := Place{
			Slug: slug,
			Name: slug, // Дефолтное имя — slug
		}

		// Best-effort парсинг info.json
		infoPath := filepath.Join(placeDir, "info.json")
		if data, err := os.ReadFile(infoPath); err == nil {
			var info struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Creator     string `json:"creator"`
			}
			if err := json.Unmarshal(data, &info); err == nil {
				if info.Name != "" {
					place.Name = info.Name
				}
				place.Description = info.Description
				place.Creator = info.Creator
			}
		}

		places = append(places, place)
	}

	return places
}