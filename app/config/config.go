package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type config struct {
	Debug              bool
	DatabasePath       string
	HomeserverName     string
	PrimaryBotUsername string
	PrimaryBotPassword string
}

func New(envPath string) (*config, error) {
	if err := godotenv.Load(envPath); err != nil {
		return nil, err
	}

	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))

	config := &config{
		Debug:              debug,
		DatabasePath:       os.Getenv("DB_FILE"),
		HomeserverName:     os.Getenv("MATRIX_SERVER_NAME"),
		PrimaryBotUsername: os.Getenv("MATRIX_USERNAME"),
		PrimaryBotPassword: os.Getenv("MATRIX_PASSWORD"),
	}

	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c config) validate() error {
	if c.DatabasePath == "" {
		return errors.New("DB_FILE environment variable must be set and not empty")
	}

	if c.HomeserverName == "" {
		return errors.New("MATRIX_SERVER_NAME environment variable must be set and not empty")
	}

	if c.PrimaryBotUsername == "" {
		return errors.New("MATRIX_USERNAME environment variable must be set and not empty")
	}

	if c.PrimaryBotPassword == "" {
		return errors.New("MATRIX_PASSWORD environment variable must be set and not empty")
	}

	return nil
}
