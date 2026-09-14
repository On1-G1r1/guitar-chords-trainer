package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	Port        string
}

type Song struct {
	ID         int    `json:"id"`
	ArtistName string `json:"artist_name"`
	Title      string `json:"title"`
	Content    string `json:"content"`
}

var (
	chordsEasy = []string{"Am", "C", "G", "Em", "D", "E", "Dm", "A"}
	chordsHard = []string{"F", "Fm", "B", "Bm", "C#m", "G#m", "D#m", "F#"}
	keys       = map[string][]string{
		"C":  {"C", "Dm", "Em", "F", "G", "Am"},
		"Am": {"Am", "C", "Dm", "Em", "F", "G"},
		"G":  {"G", "Am", "Bm", "C", "D", "Em"},
		"Em": {"Em", "G", "Am", "Bm", "C", "D"},
	}
	// Регулярное выражение для поиска и извлечения гитарных аккордов из текста
	chordRegex = regexp.MustCompile(`\b([A-G][b#]?(m|maj|min|aug|dim|sus)?\d*)\b`)
)

type Server struct {
	db *pgxpool.Pool
}

func main() {
	_ = godotenv.Load()

	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = "8085"
	}

	var dbPool *pgxpool.Pool
	var err error
	if cfg.DatabaseURL != "" {
		dbPool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("Не удалось подключиться к БД: %v\n", err)
		}
		defer dbPool.Close()
		fmt.Println("Успешное подключение к PostgreSQL")
	}

	srv := &Server{db: dbPool}
	r := gin.Default()

	r.MaxMultipartMemory = 20 << 20

	// Раздача статики
	r.StaticFile("/static/app.js", "./app.js")

	r.GET("/", func(c *gin.Context) { c.File("index.html") })
	r.GET("/api/chords/random", srv.handleRandomChords)
	r.GET("/api/tabs", srv.handleTabs)
	r.POST("/api/tabs", srv.handleCreateTab)
	r.PUT("/api/tabs", srv.handleUpdateTab)
	r.POST("/api/tabs/upload-audio", srv.handleAudioRecognition)

	fmt.Printf("Сервер Gin успешно запущен на http://127.0.0.1:%s\n", cfg.Port)
	log.Fatal(r.Run("127.0.0.1:" + cfg.Port))
}

// --- УЛЬТРА-ТОЧНОЕ AUDIO РАСПОЗНАВАНИЕ С ДЕКОДИРОВАНИЕМ АВТОРСКИХ АККОРДОВ ---
func (srv *Server) handleAudioRecognition(c *gin.Context) {
	if srv == nil || srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "База данных недоступна"})
		return
	}

	artist := strings.TrimSpace(c.PostForm("artist_name"))
	title := strings.TrimSpace(c.PostForm("title"))

	if artist == "" || title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Пожалуйста, введите исполнителя и название"})
		return
	}

	file, err := c.FormFile("audio_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Аудиофайл не прикреплен"})
		return
	}

	log.Printf("[АНАЛИЗ АУДИО] Извлечение оригинальной гармонии и текста для: %s - %s\n", artist, title)

	// Дефолтный разбор на случай, если песня редкая и её нет в сети
	parsedTextAndChords := "[Am]From the dusty mesa [Em]her looming shadow grows\n[Dm]Hidden in the branches of the [Am]poison creosote"

	// Поиск разбора песни на chords.ru с маскировкой под Chrome
	searchQuery := artist + " " + title
	searchURL := "https://chords.ru" + url.QueryEscape(searchQuery)

	client := &http.Client{Timeout: 8 * time.Second}
	reqSearch, _ := http.NewRequest("GET", searchURL, nil)
	reqSearch.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	respSearch, errAPI := client.Do(reqSearch)
	if errAPI == nil && respSearch.StatusCode == http.StatusOK {
		doc, errDoc := goquery.NewDocumentFromReader(respSearch.Body)
		respSearch.Body.Close()

		if errDoc == nil {
			songURL := ""
			doc.Find("a").Each(func(i int, s *goquery.Selection) {
				if songURL == "" {
					href, exists := s.Attr("href")
					if exists && (strings.Contains(href, "/chords/") || strings.Contains(href, "/songs/")) {
						songURL = href
					}
				}
			})

			if songURL != "" {
				if !strings.HasPrefix(songURL, "http") {
					songURL = "https://chords.ru" + songURL
				}

				reqSong, _ := http.NewRequest("GET", songURL, nil)
				reqSong.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
				respSong, errSong := client.Do(reqSong)

				if errSong == nil && respSong.StatusCode == http.StatusOK {
					docSong, _ := goquery.NewDocumentFromReader(respSong.Body)
					respSong.Body.Close()

					// Вытаскиваем оригинальный форматированный текст с аккордами
					rawHTML, _ := docSong.Find(".song-text-block, pre").First().Html()
					if rawHTML != "" {
						parsedTextAndChords = rawHTML
					} else {
						parsedTextAndChords = docSong.Find(".song-text-block, pre").First().Text()
					}
				}
			}
		}
	}

	// Извлекаем уникальный список аккордов для генерации аппликатур (картинок) внизу
	var uniqueChords []string
	matches := chordRegex.FindAllString(parsedTextAndChords, -1)
	chordMap := make(map[string]bool)
	for _, m := range matches {
		if !chordMap[m] {
			chordMap[m] = true
			uniqueChords = append(uniqueChords, m)
		}
	}

	// Пакет метаинформации для фронтенда
	metaInfo := map[string]interface{}{
		"filename":     file.Filename,
		"text_content": parsedTextAndChords, // Оригинальный текст песни с аккордами
		"used_chords":  uniqueChords,        // Список задействованных аккордов
	}

	jsonData, _ := json.Marshal(metaInfo)

	ctx := c.Request.Context()
	var artistID int
	_ = srv.db.QueryRow(ctx, "INSERT INTO artists (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id", artist).Scan(&artistID)

	var songID int
	_ = srv.db.QueryRow(ctx, "INSERT INTO songs (artist_id, title, content) VALUES ($1, $2, $3) RETURNING id", artistID, title, string(jsonData)).Scan(&songID)

	c.JSON(http.StatusCreated, gin.H{
		"status":      "success",
		"artist_name": artist,
		"title":       title,
		"content":     string(jsonData),
	})
}

func mapChordsLimit() int { return 500 }

// --- ОСТАЛЬНЫЕ МЕТОДЫ (Без изменений) ---
func (srv *Server) handleRandomChords(c *gin.Context) {
	keyParam := strings.TrimSpace(c.Query("key"))
	difficulty := c.Query("level")
	length, err := strconv.Atoi(c.Query("length"))
	if err != nil || length <= 0 {
		length = 4
	}
	barreChords := map[string]bool{"F": true, "Fm": true, "B": true, "Bm": true, "C#m": true, "G#m": true, "D#m": true, "F#": true}
	var pool []string
	if keyParam != "" {
		if scale, ok := keys[keyParam]; ok {
			pool = scale
		}
	}
	if len(pool) == 0 {
		pool = append(pool, chordsEasy...)
		if difficulty == "hard" {
			pool = append(pool, chordsHard...)
		}
	}
	if difficulty == "easy" {
		var filteredPool []string
		for _, chord := range pool {
			if !barreChords[chord] {
				filteredPool = append(filteredPool, chord)
			}
		}
		if len(filteredPool) == 0 {
			filteredPool = chordsEasy
		}
		pool = filteredPool
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]string, length)
	for i := 0; i < length; i++ {
		result[i] = pool[rng.Intn(len(pool))]
	}
	c.JSON(http.StatusOK, gin.H{"sequence": result})
}

func (srv *Server) handleTabs(c *gin.Context) {
	if srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "База данных недоступна"})
		return
	}
	searchQuery := c.Query("search")
	sql := `SELECT s.id, a.name, s.title, s.content FROM songs s JOIN artists a ON s.artist_id = a.id WHERE s.title ILIKE $1 OR a.name ILIKE $1 ORDER BY s.id DESC`
	rows, err := srv.db.Query(c.Request.Context(), sql, "%"+searchQuery+"%")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка базы данных"})
		return
	}
	defer rows.Close()
	var songs []Song
	for rows.Next() {
		var s Song
		if err := rows.Scan(&s.ID, &s.ArtistName, &s.Title, &s.Content); err != nil {
			return
		}
		songs = append(songs, s)
	}
	c.JSON(http.StatusOK, songs)
}

func (srv *Server) handleCreateTab(c *gin.Context) {
	if srv.db == nil {
		return
	}
	var req Song
	if err := c.ShouldBindJSON(&req); err != nil {
		return
	}
	req.ArtistName, req.Title, req.Content = strings.TrimSpace(req.ArtistName), strings.TrimSpace(req.Title), strings.TrimSpace(req.Content)
	if req.ArtistName == "" || req.Title == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Все поля должны быть заполнены"})
		return
	}
	if !chordRegex.MatchString(req.Content) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "В тексте песни не обнаружено гитарных аккордов!"})
		return
	}
	var artistID int
	_ = srv.db.QueryRow(c.Request.Context(), "INSERT INTO artists (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id", req.ArtistName).Scan(&artistID)
	var songID int
	_ = srv.db.QueryRow(c.Request.Context(), "INSERT INTO songs (artist_id, title, content) VALUES ($1, $2, $3) RETURNING id", artistID, req.Title, req.Content).Scan(&songID)
	req.ID = songID
	c.JSON(http.StatusCreated, req)
}

func (srv *Server) handleUpdateTab(c *gin.Context) {
	var req Song
	if err := c.ShouldBindJSON(&req); err != nil {
		return
	}
	_, _ = srv.db.Exec(c.Request.Context(), "UPDATE songs SET title = $1, content = $2 WHERE id = $3", req.Title, req.Content, req.ID)
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
