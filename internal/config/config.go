package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken     string
	SuperAdminID int64
	DatabasePath string
	LogLevel     string

	// AI-assisted spam analysis, optional [FEAT-020]
	AIProvider string // gemini | openai | claude; empty disables AI
	AIAPIKey   string
	AIBaseURL  string
	AIModel    string
}

func Load() *Config {
	_ = godotenv.Load()

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	superAdminStr := os.Getenv("SUPERADMIN_ID")
	superAdminID, err := strconv.ParseInt(superAdminStr, 10, 64)
	if err != nil {
		log.Fatalf("SUPERADMIN_ID must be a numeric Telegram user ID, got: %q", superAdminStr)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/bot.db"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	aiProvider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	aiKey := os.Getenv("AI_API_KEY")
	if aiProvider != "" && aiKey == "" {
		log.Printf("AI_PROVIDER=%s is set but AI_API_KEY is empty — AI analysis disabled", aiProvider)
		aiProvider = ""
	}

	return &Config{
		BotToken:     token,
		SuperAdminID: superAdminID,
		DatabasePath: dbPath,
		LogLevel:     logLevel,
		AIProvider:   aiProvider,
		AIAPIKey:     aiKey,
		AIBaseURL:    strings.TrimRight(os.Getenv("AI_BASE_URL"), "/"),
		AIModel:      os.Getenv("AI_MODEL"),
	}
}
