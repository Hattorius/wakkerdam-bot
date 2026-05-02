package main

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		slog.Error("Failed loading environmental variables from .env file", "error", err)
		os.Exit(1)
	}

	slog.Info("Selam")
}
