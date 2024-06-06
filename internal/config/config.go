package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type ModelConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Config []ModelConfig

func LoadConfig(filePath string) (Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open config file: %v", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %v", err)
	}

	var config Config
	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config JSON: %v", err)
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %v", err)
	}

	return config, nil
}

func validateConfig(config Config) error {
	if len(config) == 0 {
		return errors.New("models cannot be empty")
	}
	for _, model := range config {
		if model.Name == "" {
			return errors.New("model name cannot be empty")
		}
		if model.Description == "" {
			return errors.New("model description cannot be empty")
		}
	}
	return nil
}
