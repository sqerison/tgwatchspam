package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken     string
	SuperAdminID int64
	DatabasePath string
	LogLevel     string
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

	return &Config{
		BotToken:     token,
		SuperAdminID: superAdminID,
		DatabasePath: dbPath,
		LogLevel:     logLevel,
	}
}
