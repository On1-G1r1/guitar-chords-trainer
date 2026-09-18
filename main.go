package main

import (
	"context"
	"encoding/json"
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

type GeneratedSequence struct {
	ID        int       `json:"id"`
	Chords    []string  `json:"chords"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	chordsEasy = []string{"Am", "C", "G", "Em", "D", "E", "Dm", "A"}
	chordsHard = []string{"F", "Fm", "B", "Bm", "C#m", "G#m", "D#m", "F#"}

	// Музыкальные ступени для мажора и минора
	harmonyKeys = map[string]map[string]string{
		"C": {
			"I": "C", "ii": "Dm", "iii": "Em", "IV": "F", "V": "G", "vi": "Am",
			"borrowed": "Fm", // Из параллельного минора (IVm)
		},
		"Am": {
			"i": "Am", "ii": "Bdim", "III": "C", "iv": "Dm", "V": "E", "VI": "F", "VII": "G",
			"borrowed": "D", // Дорийский мажор (IV)
		},
		"G": {
			"I": "G", "ii": "Am", "iii": "Bm", "IV": "C", "V": "D", "vi": "Em",
			"borrowed": "Cm",
		},
		"Em": {
			"i": "Em", "ii": "F#dim", "III": "G", "iv": "Am", "V": "B", "VI": "C", "VII": "D",
			"borrowed": "A",
		},
	}

	// Шаблоны гармонических цепочек под разное настроение
	moodProgressions = map[string][][]string{
		"happy": {
			{"I", "V", "vi", "IV"}, // Классический поп-хит
			{"I", "IV", "V", "IV"}, // Мажорный рок/блюз
		},
		"sad": {
			{"i", "VI", "III", "VII"}, // Лирическая поп-цепочка
			{"i", "iv", "V", "i"},     // Минорная классика
		},
		"epic": {
			{"i", "VI", "VII", "i"}, // Героическая
			{"vi", "IV", "I", "V"},  // Эпическая сквозная
		},
		"jazz": {
			{"ii", "V", "I", "vi"}, // Джазовый стандарт ii-V-I
			{"I", "vi", "ii", "V"}, // Турнаунд
		},
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

	// Раздача статических файлов фронтенда
	r.StaticFile("/app.js", "./app.js")
	r.StaticFile("/style.css", "./style.css")
	r.StaticFile("/index.html", "./index.html")

	r.GET("/", func(c *gin.Context) { c.File("index.html") })

	// API Эндпоинты
	r.GET("/api/chords/random", srv.handleRandomChords)
	r.POST("/api/chords/save", srv.handleSaveSequence)
	r.GET("/api/chords/history", srv.handleGetHistory)

	fmt.Printf("Сервер запущен на http://127.0.0.1:%s\n", cfg.Port)
	log.Fatal(r.Run("127.0.0.1:" + cfg.Port))
}

// Умная генерация по законам гармонии и настроениям
func (srv *Server) handleRandomChords(c *gin.Context) {
	keyParam := strings.TrimSpace(c.Query("key"))
	difficulty := c.Query("level")
	mood := c.Query("mood")
	length, err := strconv.Atoi(c.Query("length"))
	if err != nil || length <= 0 {
		length = 4
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var result []string

	// Попытка сгенерировать правильную гармоническую последовательность
	if mood != "" && mood != "random" && keyParam != "" {
		if currentKey, ok := harmonyKeys[keyParam]; ok {
			if progressions, ok := moodProgressions[mood]; ok {
				chosenProg := progressions[rng.Intn(len(progressions))]

				for i := 0; i < length; i++ {
					step := chosenProg[i%len(chosenProg)]
					chord := currentKey[step]

					// МОДУЛЯЦИЯ / ЗАИМСТВОВАННЫЕ АККОРДЫ: Шанс 15% вставить аккорд из параллельного лада
					if rng.Float32() < 0.15 && currentKey["borrowed"] != "" && i > 0 {
						chord = currentKey["borrowed"]
					}

					// Фильтр сложности Easy (убираем баррэ)
					if difficulty == "easy" && (chord == "F" || chord == "Fm" || chord == "B" || chord == "Bm" || chord == "C#m" || chord == "G#m" || chord == "D#m" || chord == "F#") {
						chord = "Am"
					}
					result = append(result, chord)
				}
			}
		}
	}

	// Фолбек: Абсолютный хаос (случайный набор), если тональность или настроение не выбраны
	if len(result) == 0 {
		var pool []string
		pool = append(pool, chordsEasy...)
		if difficulty == "hard" {
			pool = append(pool, chordsHard...)
		}

		for i := 0; i < length; i++ {
			chord := pool[rng.Intn(len(pool))]
			// Добавляем случайную красивую модуляцию (доминантсептаккорд в конце цепочки)
			if rng.Float32() < 0.12 && i == length-1 {
				chord = "E7"
			}
			result = append(result, chord)
		}
	}

	c.JSON(http.StatusOK, gin.H{"sequence": result})
}

// Сохранение удачной цепочки аккордов в историю練習
// Сохранение удачной цепочки аккордов в историю
func (srv *Server) handleSaveSequence(c *gin.Context) {
	if srv == nil || srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "БД недоступна"})
		return
	}
	var req struct {
		Chords []string `json:"chords"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Chords) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Пустая последовательность"})
		return
	}

	chordsJSON, _ := json.Marshal(req.Chords)
	// ИСПРАВЛЕНО: убран лишний обратный слэш перед \$1
	_, err := srv.db.Exec(c.Request.Context(), "INSERT INTO chord_history (chords) VALUES (\$1)", string(chordsJSON))
	if err != nil {
		log.Printf("[БД ОШИБКА] Не удалось сохранить историю: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка записи в базу данных"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}


// Получение истории тренировок
func (srv *Server) handleGetHistory(c *gin.Context) {
	if srv.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "БД недоступна"})
		return
	}
	rows, err := srv.db.Query(c.Request.Context(), "SELECT id, chords, created_at FROM chord_history ORDER BY id DESC LIMIT 10")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения истории"})
		return
	}
	defer rows.Close()

	var history []GeneratedSequence
	for rows.Next() {
		var id int
		var chordsStr string
		var t time.Time
		if err := rows.Scan(&id, &chordsStr, &t); err == nil {
			var ch []string
			_ = json.Unmarshal([]byte(chordsStr), &ch)
			history = append(history, GeneratedSequence{ID: id, Chords: ch, CreatedAt: t})
		}
	}
	c.JSON(http.StatusOK, history)
}
