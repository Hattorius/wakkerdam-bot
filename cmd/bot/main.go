package main

import (
	"log/slog"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		slog.Error("Failed loading environmental variables from .env file", "error", err)
		os.Exit(1)
	}

	discordBotToken := os.Getenv("DISCORD_BOT_TOKEN")

	discord, err := discordgo.New("Bot " + discordBotToken)
	if err != nil {
		slog.Error("Failed setting up Discord client", "error", err)
		os.Exit(1)
	}
}
