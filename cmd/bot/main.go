package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/Hattorius/wakkerdam-bot/internal/config"
	"github.com/Hattorius/wakkerdam-bot/internal/handlers"
	"github.com/Hattorius/wakkerdam-bot/internal/summary"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var s *discordgo.Session

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "samenvattingen",
		Description: "Zet hiermee het kanaal waar de bot naar moet kijken voor samenvattingen",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "kanaal",
				Description: "Kanaal waar de bot naar moet luisteren",
				ChannelTypes: []discordgo.ChannelType{
					discordgo.ChannelTypeGuildText,
				},
				Required: true,
			},
		},
	},
	{
		Name:        "verhaallijn",
		Description: "Zet het kanaal voor de verhaallijn van de spelleider",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "kanaal",
				Description: "Verhaallijn-kanaal",
				ChannelTypes: []discordgo.ChannelType{
					discordgo.ChannelTypeGuildText,
				},
				Required: true,
			},
		},
	},
	{
		Name:        "add-speler",
		Description: "Voeg een speler toe aan het spel",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "speler",
				Description: "De speler om toe te voegen",
				Required:    true,
			},
		},
	},
	{
		Name:        "remove-speler",
		Description: "Verwijder een speler uit het spel",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "speler",
				Description: "De speler om te verwijderen",
				Required:    true,
			},
		},
	},
	{
		Name:        "vat",
		Description: "Genereer een samenvatting opnieuw voor een specifieke datum",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "datum",
				Description: "Datum (YYYY-MM-DD)",
				Required:    true,
			},
		},
	},
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"samenvattingen": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		opt := i.ApplicationCommandData().GetOption("kanaal")
		channel := opt.ChannelValue(s)
		handlers.SetSummary(s, i, channel)
	},
	"verhaallijn": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		opt := i.ApplicationCommandData().GetOption("kanaal")
		channel := opt.ChannelValue(s)
		handlers.SetStoryline(s, i, channel)
	},
	"add-speler": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		opt := i.ApplicationCommandData().GetOption("speler")
		user := opt.UserValue(s)
		handlers.AddPlayer(s, i, user)
	},
	"remove-speler": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		opt := i.ApplicationCommandData().GetOption("speler")
		user := opt.UserValue(s)
		handlers.RemovePlayer(s, i, user)
	},
	"vat": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Member.User.ID != "721670943440896050" {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Je hebt geen toestemming om dit commando te gebruiken.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		dateStr := i.ApplicationCommandData().GetOption("datum").StringValue()
		_, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Ongeldige datum. Gebruik het formaat YYYY-MM-DD.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Samenvatting voor %s wordt gegenereerd...", dateStr),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		go func() {
			msgs := config.GetMessagesByDate(dateStr)
			storyMsgs := config.GetStoryMessagesByDate(dateStr)
			prevSummaries := config.GetSummariesBefore(dateStr)

			summaryText := summary.GenerateSummary(msgs, storyMsgs, prevSummaries)
			config.SaveSummaryForDate(dateStr, summaryText)

			slog.Info("Re-generated summary", "date", dateStr)

			s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("Samenvatting voor %s is opnieuw gegenereerd en opgeslagen.", dateStr),
				Flags:   discordgo.MessageFlagsEphemeral,
			})
		}()
	},
}

func main() {
	_ = godotenv.Load()
	var err error

	discordBotToken := os.Getenv("DISCORD_BOT_TOKEN")

	s, err = discordgo.New("Bot " + discordBotToken)
	if err != nil {
		slog.Error("Failed setting up Discord client", "error", err)
		os.Exit(1)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		h, ok := commandHandlers[i.ApplicationCommandData().Name]
		if ok {
			h(s, i)
		}
	})

	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		conf := config.Get()

		if conf.StoryChannel != nil && m.ChannelID == *conf.StoryChannel {
			config.AddStoryMessage(m.Author.Username, m.Content, m.Timestamp)
			return
		}

		if conf.Channel == nil || m.ChannelID != *conf.Channel {
			return
		}

		isPlayer := conf.IsPlayer(m.Author.ID)
		config.AddMessage(m.Author.Username, m.Content, isPlayer, m.Timestamp)

		if m.Content == "geef me samenvatting papi" {
			go handleOnDemandSummary(s, m)
		}
	})

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("Logged in", "username", fmt.Sprintf("%v#%v", s.State.User.Username, s.State.User.Discriminator))
		go catchUpMessages(s)
	})

	err = s.Open()
	if err != nil {
		slog.Error("Cannot open the session", "error", err)
		os.Exit(1)
	}

	log.Println("Adding commands...")
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))

	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "1401530718223335496", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	go scheduleDailySummary(s)

	defer s.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	slog.Info("Gracefully shutting down")
	config.StopFlusher()
	config.FlushMessages()
}

func handleOnDemandSummary(s *discordgo.Session, m *discordgo.MessageCreate) {
	ch, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		slog.Error("Failed creating DM channel", "error", err)
		s.ChannelMessageSendReply(m.ChannelID, "dan moet je wel je dms openen, aap", m.Reference())
		return
	}

	testMsg, err := s.ChannelMessageSend(ch.ID, "Even kijken of ik je kan bereiken... 👀")
	if err != nil {
		slog.Error("Failed sending test DM", "error", err)
		s.ChannelMessageSendReply(m.ChannelID, "dan moet je wel je dms openen, aap", m.Reference())
		return
	}
	s.ChannelMessageDelete(ch.ID, testMsg.ID)

	msgs := config.GetMessages()
	storyMsgs := config.GetStoryMessages()
	recentSummaries := config.GetRecentSummaries(3)

	summaryText := summary.GenerateSummary(msgs, storyMsgs, recentSummaries)

	_, err = s.ChannelMessageSend(ch.ID, summaryText)
	if err != nil {
		slog.Error("Failed sending summary DM", "error", err)
	}
}

func catchUpMessages(s *discordgo.Session) {
	conf := config.Get()

	if conf.Channel != nil {
		catchUpChannel(s, *conf.Channel, false, config.GetLastMessageTime())
	}

	if conf.StoryChannel != nil {
		catchUpChannel(s, *conf.StoryChannel, true, config.GetLastStoryMessageTime())
	}

	slog.Info("Finished catching up messages")
}

func catchUpChannel(s *discordgo.Session, channelID string, isStory bool, lastKnown *time.Time) {
	var allMessages []*discordgo.Message
	var beforeID string
	for {
		msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			slog.Error("Failed fetching channel messages", "channel", channelID, "error", err)
			return
		}

		if len(msgs) == 0 {
			break
		}

		reachedExisting := false
		for _, m := range msgs {
			if lastKnown != nil && !m.Timestamp.After(*lastKnown) {
				reachedExisting = true
				break
			}
			allMessages = append(allMessages, m)
		}

		if reachedExisting {
			break
		}

		beforeID = msgs[len(msgs)-1].ID

		if len(msgs) < 100 {
			break
		}
	}

	for i := len(allMessages) - 1; i >= 0; i-- {
		m := allMessages[i]
		if m.Author.ID == s.State.User.ID {
			continue
		}

		if isStory {
			config.AddStoryMessage(m.Author.Username, m.Content, m.Timestamp)
		} else {
			isPlayer := config.Get().IsPlayer(m.Author.ID)
			config.AddMessage(m.Author.Username, m.Content, isPlayer, m.Timestamp)
		}
	}
}

func scheduleDailySummary(s *discordgo.Session) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())

		timer := time.NewTimer(time.Until(next))
		<-timer.C

		channelID := config.Get().Channel
		if channelID == nil {
			continue
		}

		msgs := config.GetMessages()
		if msgs == "" {
			continue
		}

		storyMsgs := config.GetStoryMessages()
		recentSummaries := config.GetRecentSummaries(3)

		summaryText := summary.GenerateSummary(msgs, storyMsgs, recentSummaries)

		config.SaveSummary(summaryText)

		msg, err := s.ChannelMessageSend(*channelID, summaryText)
		if err != nil {
			slog.Error("Failed sending daily summary", "error", err)
			continue
		}

		err = s.ChannelMessagePin(*channelID, msg.ID)
		if err != nil {
			slog.Error("Failed pinning daily summary", "error", err)
		}

		config.ClearMessages()
		config.ClearStoryMessages()
		config.FlushMessages()
	}
}
