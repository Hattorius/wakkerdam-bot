package handlers

import (
	"github.com/Hattorius/wakkerdam-bot/internal/config"
	"github.com/bwmarrin/discordgo"
)

func SetSummary(s *discordgo.Session, i *discordgo.InteractionCreate, channel *discordgo.Channel) {
	config.Get().Channel = &channel.ID
	config.Get().Save()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Okidoki",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
