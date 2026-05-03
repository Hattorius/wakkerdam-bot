package config

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	c    *Config
	lock sync.RWMutex
)

type Player struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type Config struct {
	dataFile     string
	Channel      *string  `json:"c,omitempty"`
	StoryChannel *string  `json:"story_channel,omitempty"`
	Players      []Player `json:"players,omitempty"`
}

func init() {
	lock.Lock()
	defer lock.Unlock()

	dataFolder := filepath.Join(".", "data")
	err := os.MkdirAll(dataFolder, os.ModePerm)

	if err != nil {
		slog.Error("Failed creating the data folder at ./data", "error", err)
		os.Exit(1)
	}

	dataFile := filepath.Join(dataFolder, "config.json")
	_, err = os.Stat(dataFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("Failed checking if config file exists", "error", err)
		os.Exit(1)
	}

	if errors.Is(err, os.ErrNotExist) {
		c = newConfig()

		f, err := os.Create(dataFile)
		if err != nil {
			slog.Error("Failed creating data file", "error", err)
			os.Exit(1)
		}
		defer f.Close()

		d, err := json.Marshal(c)
		if err != nil {
			slog.Error("Failed marshalling json", "error", err)
			os.Exit(1)
		}

		f.Write(d)
	} else {
		c = newConfig()

		dat, err := os.ReadFile(dataFile)
		if err != nil {
			slog.Error("Failed reading file", "error", err)
			os.Exit(1)
		}

		err = json.Unmarshal(dat, c)
		if err != nil {
			slog.Error("Failed unmarhsalling config file", "error", err)
			os.Exit(1)
		}
	}

	c.dataFile = dataFile

	initMessages()
}

func newConfig() *Config {
	return &Config{}
}

func (conf Config) Save() {
	lock.Lock()
	defer lock.Unlock()

	f, err := os.Create(conf.dataFile)
	if err != nil {
		slog.Error("Failed creating data file", "error", err)
		os.Exit(1)
	}
	defer f.Close()

	d, err := json.Marshal(conf)
	if err != nil {
		slog.Error("Failed marshalling json", "error", err)
		os.Exit(1)
	}

	f.Write(d)
}

func Get() *Config {
	return c
}

func (conf *Config) IsPlayer(userID string) bool {
	for _, p := range conf.Players {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

func (conf *Config) AddPlayer(player Player) {
	lock.Lock()
	defer lock.Unlock()
	for _, p := range conf.Players {
		if p.UserID == player.UserID {
			return
		}
	}
	conf.Players = append(conf.Players, player)
}

func (conf *Config) RemovePlayer(userID string) bool {
	lock.Lock()
	defer lock.Unlock()
	for i, p := range conf.Players {
		if p.UserID == userID {
			conf.Players = append(conf.Players[:i], conf.Players[i+1:]...)
			return true
		}
	}
	return false
}
