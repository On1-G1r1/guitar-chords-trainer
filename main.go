package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	Port        string
}

type Song struct {
	ID         int    `json:"id" db:"id"`
	ArtistName string `json:"artist_name"`
	Title      string `json:"title" db:"title"`
	Content    string `json:"content" db:"content"`
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
		cfg.Port = "8085" // Оставим порт 8085 по умолчанию
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
	} else {
		fmt.Println("DATABASE_URL не задан. Каталог табул не будет работать.")
	}

	srv := &Server{db: dbPool}

	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.File("index.html")
	})

	//API
	r.GET("/api/chords/random", srv.handleRandomChords)
	r.GET("/api/tabs", srv.handleTabs)
	r.POST("/api/tabs", srv.handleCreateTab)
	r.PUT("/api/tabs", srv.handleUpdateTab)

	fmt.Printf("Сервер Gin успешно запущен на http://127.0.0.1:%s\n", cfg.Port)
	log.Fatal(r.Run("127.0.0.1:" + cfg.Port))
}

func (srv *Server) handleRandomChords(c *gin.Context) {
	keyParam := strings.TrimSpace(c.Query("key"))
	difficulty := c.Query("level")
	lengthStr := c.Query("length")

	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 {
		length = 4
	}

	barreChords := map[string]bool{
		"F": true, "Fm": true, "B": true, "Bm": true,
		"C#m": true, "G#m": true, "D#m": true, "F#": true,
	}

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

// Получение и поиск
func (srv *Server) handleTabs(c *gin.Context) {
	if srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "База данных недоступна"})
		return
	}

	searchQuery := c.Query("search")
	sql := `
		SELECT s.id, a.name, s.title, s.content 
		FROM songs s
		JOIN artists a ON s.artist_id = a.id
		WHERE s.title ILIKE $1 OR a.name ILIKE $1
		ORDER BY s.id DESC
	`
	searchTerm := "%" + searchQuery + "%"

	rows, err := srv.db.Query(c.Request.Context(), sql, searchTerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка базы данных"})
		return
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		if err := rows.Scan(&s.ID, &s.ArtistName, &s.Title, &s.Content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения данных"})
			return
		}
		songs = append(songs, s)
	}

	c.JSON(http.StatusOK, songs)
}

// Создание
func (srv *Server) handleCreateTab(c *gin.Context) {
	if srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "База данных недоступна"})
		return
	}

	var req Song
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат запроса"})
		return
	}

	req.ArtistName = strings.TrimSpace(req.ArtistName)
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)

	if req.ArtistName == "" || req.Title == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Все поля должны быть заполнены"})
		return
	}

	ctx := c.Request.Context()
	var artistID int
	err := srv.db.QueryRow(ctx, "INSERT INTO artists (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id", req.ArtistName).Scan(&artistID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения исполнителя"})
		return
	}

	var songID int
	err = srv.db.QueryRow(ctx, "INSERT INTO songs (artist_id, title, content) VALUES ($1, $2, $3) RETURNING id", artistID, req.Title, req.Content).Scan(&songID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения песни"})
		return
	}

	req.ID = songID
	c.JSON(http.StatusCreated, req)
}

// Обновление
func (srv *Server) handleUpdateTab(c *gin.Context) {
	if srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "База данных недоступна"})
		return
	}

	var req Song
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат запроса"})
		return
	}

	if req.ID <= 0 || req.Title == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные для обновления"})
		return
	}

	_, err := srv.db.Exec(c.Request.Context(), "UPDATE songs SET title = $1, content = $2 WHERE id = $3", req.Title, req.Content, req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении в БД"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
