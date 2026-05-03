package handlers

import (
	"fmt"

	"github.com/Hattorius/wakkerdam-bot/internal/config"
	"github.com/bwmarrin/discordgo"
)

func AddPlayer(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User) {
	member, err := s.GuildMember(i.GuildID, user.ID)
	displayName := user.Username
	if err == nil && member.Nick != "" {
		displayName = member.Nick
	} else if user.GlobalName != "" {
		displayName = user.GlobalName
	}

	player := config.Player{
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: displayName,
	}

	config.Get().AddPlayer(player)
	config.Get().Save()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Speler %s (%s) toegevoegd!", displayName, user.Username),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func RemovePlayer(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User) {
	removed := config.Get().RemovePlayer(user.ID)
	config.Get().Save()

	msg := fmt.Sprintf("Speler %s verwijderd!", user.Username)
	if !removed {
		msg = fmt.Sprintf("Speler %s zat niet in het spel.", user.Username)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
