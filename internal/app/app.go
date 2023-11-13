package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stickpro/google-calendar-bot/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func Run() {
	cfg, err := config.Init()
	if err != nil {
		panic(err)
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = true

	db, err := sql.Open("sqlite3", "user_info.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			chat_id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Goroutine 1: Handle incoming messages
	go func() {
		for update := range updates {
			if update.Message == nil { // Ignore any non-Message updates
				continue
			}

			if update.Message.IsCommand() {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				switch update.Message.Command() {
				case "start":
					// Save user info to the database
					_, err := db.Exec("INSERT INTO users (chat_id, username, first_name, last_name) VALUES (?, ?, ?, ?)",
						update.Message.Chat.ID, update.Message.Chat.UserName, update.Message.Chat.FirstName, update.Message.Chat.LastName)
					if err != nil {
						log.Println("Error saving user info:", err)
					}

					msg.Text = "Welcome! Your information has been saved."
				}

				_, err := bot.Send(msg)
				if err != nil {
					log.Println("Error sending message:", err)
				}
			}
		}
	}()

	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	gConfig, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	client := getClient(gConfig)

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}
	sentEvents := make(map[string]bool)
	go func() {

		for {
			now := time.Now()
			nextHour := now.Add(time.Hour)
			events, err := srv.Events.List("primary").TimeMin(now.Format(time.RFC3339)).TimeMax(nextHour.Format(time.RFC3339)).MaxResults(5).Do()
			if err != nil {
				log.Printf("Error while retrieving events: %v", err)
			}

			for _, event := range events.Items {
				eventID := event.Id
				if sentEvents[eventID] {
					continue
				}

				username := extractUsernameFromDescription(event.Description)
				if username != "" {
					chatID, err := getChatIDByUsername(db, username)
					if err != nil {
						log.Printf("Error getting chat_id: %v", err)
						continue
					}
					msg := tgbotapi.NewMessage(int64(chatID), fmt.Sprintf("Тема: %s - %s", event.Summary, removeFirstWord(event.Description)))
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Telegram notification failed: %v", err)
					}

					sentEvents[eventID] = true
				}
			}
			time.Sleep(1 * time.Minute)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	<-quit

	const timeout = 5 * time.Second

	ctx, shutdown := context.WithTimeout(context.Background(), timeout)
	defer shutdown()
}

func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getChatIDByUsername(db *sql.DB, username string) (int, error) {
	var chatID int
	query := "SELECT chat_id FROM users WHERE username = ?"
	err := db.QueryRow(query, username).Scan(&chatID)
	return chatID, err
}

func extractUsernameFromDescription(description string) string {
	words := strings.Fields(description)
	if len(words) > 0 {
		return words[0]
	}
	return ""
}

func removeFirstWord(input string) string {
	index := strings.Index(input, " ")
	if index != -1 {
		return input[index+1:]
	}
	return ""
}
