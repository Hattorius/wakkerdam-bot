package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/Hattorius/wakkerdam-bot/internal/config"
	"github.com/Hattorius/wakkerdam-bot/internal/handlers"
	"github.com/Hattorius/wakkerdam-bot/internal/summary"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var s *discordgo.Session

var (
	cachedSummary     string
	cachedSummaryTime time.Time
	summaryInFlight   chan struct{}
	cacheLock         sync.Mutex
)

var reactionEmojis = []string{
	"a:e:1494384237623902338",
	"a:e:1494384226286702602",
	"a:e:1494384384503971920",
	"a:e:1494295909683691731",
	"a:e:1494295859922468955",
	"a:e:1494384380141899880",
	"a:e:1474112014400884778",
	"a:e:1494295977148940408",
	"a:e:1494384592533324008",
	"a:e:1494295736685432842",
	"a:e:1494383628015243505",
	"a:e:1494383643261407313",
	"a:e:1482293157449171156",
	"a:e:1479075456828575906",
	"a:e:1494383990096789515",
	"a:e:1494384235279155240",
	"a:e:1474119208697729076",
	"a:e:1494384909194891455",
	"a:e:1499672802654158929",
}

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

		if m.Content == "geef me samenvatting papi" {
			emoji := reactionEmojis[rand.Intn(len(reactionEmojis))]
			s.MessageReactionAdd(m.ChannelID, m.ID, emoji)
			go handleOnDemandSummary(s, m)
			return
		}

		config.AddMessage(m.Author.Username, m.Content, isPlayer, m.Timestamp)
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

	go scheduleDailySummaries(s)

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

	_, err = s.ChannelMessageSend(ch.ID, "IK BEN ER MEE BEZIG WACHT FF")
	if err != nil {
		slog.Error("Failed sending test DM", "error", err)
		s.ChannelMessageSendReply(m.ChannelID, "dan moet je wel je dms openen, aap", m.Reference())
		return
	}

	summaryText := getOrGenerateSummary()

	for len(summaryText) > 0 {
		chunk := summaryText
		if len(chunk) > 2000 {
			chunk = summaryText[:2000]
			for i := len(chunk) - 1; i > 1500; i-- {
				if chunk[i] == '\n' {
					chunk = chunk[:i+1]
					break
				}
			}
		}
		_, err = s.ChannelMessageSend(ch.ID, chunk)
		if err != nil {
			slog.Error("Failed sending summary DM", "error", err)
			s.ChannelMessageSend(ch.ID, "bro Idk what I did wrong but you should probably tell Hattorius")
			return
		}
		summaryText = summaryText[len(chunk):]
	}
}

func getOrGenerateSummary() string {
	cacheLock.Lock()

	if cachedSummary != "" && time.Since(cachedSummaryTime) < 5*time.Minute {
		result := cachedSummary
		cacheLock.Unlock()
		return result
	}

	if summaryInFlight != nil {
		ch := summaryInFlight
		cacheLock.Unlock()
		<-ch
		cacheLock.Lock()
		result := cachedSummary
		cacheLock.Unlock()
		return result
	}

	summaryInFlight = make(chan struct{})
	cacheLock.Unlock()

	today := time.Now().Format("2006-01-02")
	msgs := config.GetMessagesByDate(today)
	storyMsgs := config.GetStoryMessagesByDate(today)
	recentSummaries := config.GetSummariesBefore(today)
	result := summary.GenerateSummary(msgs, storyMsgs, recentSummaries)

	cacheLock.Lock()
	cachedSummary = result
	cachedSummaryTime = time.Now()
	ch := summaryInFlight
	summaryInFlight = nil
	cacheLock.Unlock()

	close(ch)

	return result
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

func scheduleDailySummaries(s *discordgo.Session) {
	dutch, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		slog.Error("Failed loading Europe/Amsterdam timezone", "error", err)
		return
	}

	for {
		now := time.Now().In(dutch)

		next20 := time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, dutch)
		if !now.Before(next20) {
			next20 = next20.Add(24 * time.Hour)
		}
		next00 := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, dutch)
		if !now.Before(next00) {
			next00 = next00.Add(24 * time.Hour)
		}

		var nextTime time.Time
		var isMidnight bool
		if next20.Before(next00) {
			nextTime = next20
			isMidnight = false
		} else {
			nextTime = next00
			isMidnight = true
		}

		timer := time.NewTimer(time.Until(nextTime))
		<-timer.C

		channelID := config.Get().Channel
		if channelID == nil {
			continue
		}

		var date string
		if isMidnight {
			date = time.Now().Add(-1 * time.Minute).Format("2006-01-02")
		} else {
			date = time.Now().Format("2006-01-02")
		}

		msgs := config.GetMessagesByDate(date)
		if msgs == "" {
			continue
		}

		storyMsgs := config.GetStoryMessagesByDate(date)
		recentSummaries := config.GetSummariesBefore(date)

		summaryText := summary.GenerateSummary(msgs, storyMsgs, recentSummaries)

		if isMidnight {
			config.SaveSummary(summaryText)
		}

		var firstMsg *discordgo.Message
		remaining := summaryText
		for len(remaining) > 0 {
			chunk := remaining
			if len(chunk) > 2000 {
				chunk = remaining[:2000]
				for i := len(chunk) - 1; i > 1500; i-- {
					if chunk[i] == '\n' {
						chunk = chunk[:i+1]
						break
					}
				}
			}
			msg, err := s.ChannelMessageSend(*channelID, chunk)
			if err != nil {
				slog.Error("Failed sending daily summary", "error", err)
				break
			}
			if firstMsg == nil {
				firstMsg = msg
			}
			remaining = remaining[len(chunk):]
		}

		if firstMsg != nil {
			err := s.ChannelMessagePin(*channelID, firstMsg.ID)
			if err != nil {
				slog.Error("Failed pinning daily summary", "error", err)
			}
		}

	}
}
