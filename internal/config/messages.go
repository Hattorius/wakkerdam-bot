package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type StoredSummary struct {
	Date    string
	Content string
}

var (
	messages     []string
	messagesFile string
	messagesLock sync.RWMutex

	storyMessages     []string
	storyMessagesFile string
	storyMessagesLock sync.RWMutex

	flushTicker *time.Ticker
	stopFlusher chan struct{}

	summaries     []StoredSummary
	summariesLock sync.RWMutex
	summariesDir  string
)

func initMessages() {
	messagesLock.Lock()
	defer messagesLock.Unlock()

	dataFolder := filepath.Join(".", "data")
	messagesFile = filepath.Join(dataFolder, "messages.txt")
	storyMessagesFile = filepath.Join(dataFolder, "story.txt")

	messages = loadOrCreateFile(messagesFile)
	storyMessages = loadOrCreateFile(storyMessagesFile)

	summariesDir = filepath.Join(dataFolder, "summaries")
	err := os.MkdirAll(summariesDir, os.ModePerm)
	if err != nil {
		slog.Error("Failed creating summaries folder", "error", err)
		os.Exit(1)
	}
	loadSummaries()

	stopFlusher = make(chan struct{})
	flushTicker = time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-flushTicker.C:
				FlushMessages()
			case <-stopFlusher:
				return
			}
		}
	}()
}

func loadOrCreateFile(path string) []string {
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("Failed checking if file exists", "path", path, "error", err)
		os.Exit(1)
	}

	if errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(path)
		if err != nil {
			slog.Error("Failed creating file", "path", path, "error", err)
			os.Exit(1)
		}
		f.Close()
		return []string{}
	}

	dat, err := os.ReadFile(path)
	if err != nil {
		slog.Error("Failed reading file", "path", path, "error", err)
		os.Exit(1)
	}
	content := strings.TrimSpace(string(dat))
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}

func AddMessage(user, message string, isPlayer bool, t time.Time) {
	messagesLock.Lock()
	defer messagesLock.Unlock()

	role := "SPELLEIDER"
	if isPlayer {
		role = "SPELER"
	}

	entry := fmt.Sprintf("[%s] [%s] %s: %s", t.Format("2006-01-02 15:04:05"), role, user, message)
	messages = append(messages, entry)
}

func AddStoryMessage(user, message string, t time.Time) {
	storyMessagesLock.Lock()
	defer storyMessagesLock.Unlock()

	entry := fmt.Sprintf("[%s] %s: %s", t.Format("2006-01-02 15:04:05"), user, message)
	storyMessages = append(storyMessages, entry)
}

func GetMessages() string {
	messagesLock.RLock()
	defer messagesLock.RUnlock()

	return strings.Join(messages, "\n")
}

func GetStoryMessages() string {
	storyMessagesLock.RLock()
	defer storyMessagesLock.RUnlock()

	return strings.Join(storyMessages, "\n")
}

func lastTimestampFromLines(lines []string) *time.Time {
	if len(lines) == 0 {
		return nil
	}
	last := lines[len(lines)-1]
	if len(last) < 21 || last[0] != '[' {
		return nil
	}
	dateStr := last[1:20]
	t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, time.Now().Location())
	if err != nil {
		return nil
	}
	return &t
}

func GetLastMessageTime() *time.Time {
	messagesLock.RLock()
	defer messagesLock.RUnlock()
	return lastTimestampFromLines(messages)
}

func GetLastStoryMessageTime() *time.Time {
	storyMessagesLock.RLock()
	defer storyMessagesLock.RUnlock()
	return lastTimestampFromLines(storyMessages)
}

func FlushMessages() {
	messagesLock.RLock()
	content := strings.Join(messages, "\n")
	messagesLock.RUnlock()

	err := os.WriteFile(messagesFile, []byte(content), 0644)
	if err != nil {
		slog.Error("Failed flushing messages to disk", "error", err)
	}

	storyMessagesLock.RLock()
	storyContent := strings.Join(storyMessages, "\n")
	storyMessagesLock.RUnlock()

	err = os.WriteFile(storyMessagesFile, []byte(storyContent), 0644)
	if err != nil {
		slog.Error("Failed flushing story messages to disk", "error", err)
	}
}

func StopFlusher() {
	if flushTicker != nil {
		flushTicker.Stop()
	}
	if stopFlusher != nil {
		close(stopFlusher)
	}
}

func GetRecentSummaries(days int) []StoredSummary {
	summariesLock.RLock()
	defer summariesLock.RUnlock()

	if len(summaries) <= days {
		result := make([]StoredSummary, len(summaries))
		copy(result, summaries)
		return result
	}
	result := make([]StoredSummary, days)
	copy(result, summaries[len(summaries)-days:])
	return result
}

func loadSummaries() {
	summariesLock.Lock()
	defer summariesLock.Unlock()

	entries, err := os.ReadDir(summariesDir)
	if err != nil {
		slog.Error("Failed reading summaries directory", "error", err)
		return
	}

	summaries = []StoredSummary{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		dat, err := os.ReadFile(filepath.Join(summariesDir, name))
		if err != nil {
			slog.Error("Failed reading summary file", "file", name, "error", err)
			continue
		}
		date := strings.TrimSuffix(name, ".txt")
		summaries = append(summaries, StoredSummary{Date: date, Content: string(dat)})
	}
}

func SaveSummary(content string) {
	date := time.Now().Format("2006-01-02")
	filename := filepath.Join(summariesDir, date+".txt")

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		slog.Error("Failed saving summary to disk", "error", err)
		return
	}

	summariesLock.Lock()
	defer summariesLock.Unlock()
	summaries = append(summaries, StoredSummary{Date: date, Content: content})
}

func GetSummaries() []StoredSummary {
	summariesLock.RLock()
	defer summariesLock.RUnlock()

	result := make([]StoredSummary, len(summaries))
	copy(result, summaries)
	return result
}

func GetPlayersContext() string {
	conf := Get()
	if len(conf.Players) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Spelers in het spel:\n")
	for _, p := range conf.Players {
		sb.WriteString(fmt.Sprintf("- %s (weergavenaam: %s)\n", p.Username, p.DisplayName))
	}
	return sb.String()
}

func GetMessagesByDate(date string) string {
	messagesLock.RLock()
	defer messagesLock.RUnlock()

	prefix := "[" + date
	var filtered []string
	for _, m := range messages {
		if strings.HasPrefix(m, prefix) {
			filtered = append(filtered, m)
		}
	}
	return strings.Join(filtered, "\n")
}

func GetStoryMessagesByDate(date string) string {
	storyMessagesLock.RLock()
	defer storyMessagesLock.RUnlock()

	prefix := "[" + date
	var filtered []string
	for _, m := range storyMessages {
		if strings.HasPrefix(m, prefix) {
			filtered = append(filtered, m)
		}
	}
	return strings.Join(filtered, "\n")
}

func GetSummariesBefore(date string) []StoredSummary {
	summariesLock.RLock()
	defer summariesLock.RUnlock()

	var result []StoredSummary
	for _, s := range summaries {
		if s.Date < date {
			result = append(result, s)
		}
	}
	if len(result) > 3 {
		result = result[len(result)-3:]
	}
	return result
}

func SaveSummaryForDate(date string, content string) {
	filename := filepath.Join(summariesDir, date+".txt")

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		slog.Error("Failed saving summary to disk", "error", err)
		return
	}

	summariesLock.Lock()
	defer summariesLock.Unlock()

	for i, s := range summaries {
		if s.Date == date {
			summaries[i].Content = content
			return
		}
	}
	summaries = append(summaries, StoredSummary{Date: date, Content: content})
}
