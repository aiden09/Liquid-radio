package main

import (
	"encoding/json"
	"embed"
	"hash/fnv"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"
)

//go:embed web/*
var webFS embed.FS

var musicDir = "./music"

type Track struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	HasCover bool   `json:"hasCover"`
	CoverURL string `json:"coverUrl,omitempty"`
}

type coverData struct {
	Data []byte
	MIME string
}

var (
	tracksMu   sync.RWMutex
	tracks     []Track
	coverCache = map[int]coverData{}
	lastSig    string
)

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".ogg", ".wav", ".flac", ".m4a", ".aac", ".opus", ".webm":
		return true
	default:
		return false
	}
}

func stableID(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	return int(h.Sum32() & 0x7fffffff)
}

func extractMeta(path string) (title, artist string, cover *coverData) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", "", nil
	}

	title = strings.TrimSpace(m.Title())
	artist = strings.TrimSpace(m.Artist())

	if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
		mime := pic.MIMEType
		if mime == "" {
			mime = "image/jpeg"
		}
		cover = &coverData{Data: pic.Data, MIME: mime}
	}
	return title, artist, cover
}

// catalogSignature — быстрый отпечаток каталога (имя+размер+mtime)
func catalogSignature() string {
	entries, err := os.ReadDir(musicDir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !isAudioFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		b.WriteString(e.Name())
		b.WriteByte('|')
		b.WriteString(strconv.FormatInt(info.Size(), 10))
		b.WriteByte('|')
		b.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
		b.WriteByte(';')
	}
	return b.String()
}

func loadTracks() (changed bool) {
	sig := catalogSignature()
	tracksMu.RLock()
	same := sig == lastSig && lastSig != ""
	tracksMu.RUnlock()
	if same {
		return false
	}

	entries, err := os.ReadDir(musicDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Папка %s не найдена — создайте её и положите туда треки", musicDir)
		} else {
			log.Printf("Ошибка чтения каталога: %v", err)
		}
		tracksMu.Lock()
		tracks = nil
		coverCache = map[int]coverData{}
		lastSig = sig
		tracksMu.Unlock()
		return true
	}

	newTracks := make([]Track, 0)
	newCovers := map[int]coverData{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isAudioFile(name) {
			continue
		}

		path := filepath.Join(musicDir, name)
		title, artist, cover := extractMeta(path)

		if title == "" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
			title = strings.ReplaceAll(title, "_", " ")
			title = strings.ReplaceAll(title, "-", " ")
		}
		if artist == "" {
			artist = "Unknown Artist"
		}

		id := stableID(name)
		t := Track{
			ID:       id,
			Title:    title,
			Artist:   artist,
			Filename: name,
			URL:      "/stream/" + name,
			HasCover: cover != nil,
		}
		if cover != nil {
			newCovers[id] = *cover
			t.CoverURL = "/cover/" + strconv.Itoa(id)
		}
		newTracks = append(newTracks, t)
	}

	tracksMu.Lock()
	prev := len(tracks)
	tracks = newTracks
	coverCache = newCovers
	lastSig = sig
	tracksMu.Unlock()

	log.Printf("Каталог обновлён: %d → %d треков (обложек: %d)", prev, len(newTracks), len(newCovers))
	return true
}

func tracksSnapshot() []Track {
	tracksMu.RLock()
	defer tracksMu.RUnlock()
	out := make([]Track, len(tracks))
	copy(out, tracks)
	return out
}

func tracksHandler(w http.ResponseWriter, r *http.Request) {
	// Мягкое обновление при каждом запросе списка
	loadTracks()
	list := tracksSnapshot()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/stream/")
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		http.NotFound(w, r)
		return
	}
	if !isAudioFile(name) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(musicDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func coverHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/cover/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tracksMu.RLock()
	c, ok := coverCache[id]
	tracksMu.RUnlock()
	if !ok || len(c.Data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", c.MIME)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(c.Data)
}

func watchCatalog(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if loadTracks() {
			// already logged inside loadTracks
		}
	}
}

func main() {
	if d := os.Getenv("MUSIC_DIR"); d != "" {
		musicDir = d
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	scanEvery := 5 * time.Second
	if s := os.Getenv("SCAN_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d >= time.Second {
			scanEvery = d
		}
	}

	loadTracks()
	go watchCatalog(scanEvery)

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tracks", tracksHandler)
	mux.HandleFunc("/stream/", streamHandler)
	mux.HandleFunc("/cover/", coverHandler)
	mux.Handle("/", http.FileServer(http.FS(static)))

	log.Printf("🎧 Liquid Glass Radio → http://0.0.0.0%s", addr)
	log.Printf("📁 Каталог: %s · автоскан каждые %s", musicDir, scanEvery)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
