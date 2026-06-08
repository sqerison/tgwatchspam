package main

import (
	"log"

	"github.com/sqerison/tgwatchspam/internal/bot"
	"github.com/sqerison/tgwatchspam/internal/config"
	"github.com/sqerison/tgwatchspam/internal/storage"
)

func main() {
	cfg := config.Load()

	st, err := storage.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}
	defer st.Close()

	bot.Run(cfg, st)
}
