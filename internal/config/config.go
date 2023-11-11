package config

import "github.com/ilyakaznacheev/cleanenv"

type (
	Config struct {
		Telegram TelegramBot `yaml:"telegram_bot"`
	}
	TelegramBot struct {
		BotToken string `yaml:"bot_token"`
	}
)

func Init() (*Config, error) {
	cfg := Config{}
	err := cleanenv.ReadConfig("./config.yml", &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
