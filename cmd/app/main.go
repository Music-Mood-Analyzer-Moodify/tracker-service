package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
	"tracker-service/internal/api"
	"tracker-service/internal/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
)

type config struct {
    RabbitMQConnectionString string
    RetryInterval           time.Duration
    RequestInterval         time.Duration
    SpotifyClientID         string
    SpotifyClientSecret     string
    SearchTerm              string
    Limit                   int
    MaxOffset               int
}

func readConfig() (config, error) {
    var cfg config
    cfg.RabbitMQConnectionString = os.Getenv("RABBITMQ_CONNECTION_STRING")

    if rabbitErr := checkEmpty(cfg.RabbitMQConnectionString, "RABBITMQ_CONNECTION_STRING"); rabbitErr != nil {
        return cfg, rabbitErr
    }

    // Retry interval (for the outer loop)
    retry, err := strconv.Atoi(os.Getenv("RETRY_INTERVAL"))
    if err != nil {
        return cfg, fmt.Errorf("invalid RETRY_INTERVAL value: %v", err)
    }
    cfg.RetryInterval = time.Duration(retry) * time.Second

    // Request interval (how frequently we poll for songs)
    req, err := strconv.Atoi(os.Getenv("REQUEST_INTERVAL"))
    if err != nil {
        return cfg, fmt.Errorf("invalid REQUEST_INTERVAL value: %v", err)
    }
    cfg.RequestInterval = time.Duration(req) * time.Second

    // Spotify credentials
    cfg.SpotifyClientID = os.Getenv("SPOTIFY_CLIENT_ID")
    cfg.SpotifyClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")

    cfg.SearchTerm = "a"
    cfg.Limit = 50
    cfg.MaxOffset = 1000

    return cfg, nil
}

func checkEmpty(value, name string) error {
    if value == "" {
        return fmt.Errorf("environment variable %s is not set or empty", name)
    }
    return nil
}

func main() {
    cfg, err := readConfig()
    if err != nil {
        slog.Error("Failed to read config", "error", err)
        return
    }
	
    conn, ch, qSendSong, err := messaging.SetUpMessaging(cfg.RabbitMQConnectionString)
    if err != nil {
        slog.Error("Failed to set up messaging", "error", err)
        return
    }
	defer conn.Close()
	defer ch.Close()

    slog.Info("Messaging set up successfully", "rabbitMQ", cfg.RabbitMQConnectionString)
    slog.Info("Retry interval", "seconds", cfg.RetryInterval.Seconds())

    query := cfg.SearchTerm + "&type=track&tag=hipster&limit=" + strconv.Itoa(cfg.Limit) + "&offset="
    slog.Info("Initial query", "query", query)
	
    retryTicker := time.NewTicker(cfg.RetryInterval)
    defer retryTicker.Stop()

    slog.Info("Starting to request the Spotify API (first run)")
    DoSpotifyRequestSet(query, cfg.Limit, cfg.RequestInterval, cfg.MaxOffset, ch, qSendSong)
    slog.Info("Halting requests to the Spotify API until next retry")

    for range retryTicker.C {
        slog.Info("Starting to request the Spotify API (retry)")
        DoSpotifyRequestSet(query, cfg.Limit, cfg.RequestInterval, cfg.MaxOffset, ch, qSendSong)
        slog.Info("Halting requests to the Spotify API until next retry")
    }
}

// DoSpotifyRequestSet runs the offset-incrementing loop for a given query
func DoSpotifyRequestSet(
    query string,
    limit int,
    requestInterval time.Duration,
    maxOffset int,
    ch *amqp.Channel,
    qSendSong amqp.Queue,
) {
    slog.Info("Request interval", "seconds", requestInterval.Seconds())
    requestTicker := time.NewTicker(requestInterval)
    defer requestTicker.Stop()

    offset := 0
    token := ""
    tokenExpiration := time.Now()

    // Log once before starting the loop
    slog.Info("Performing initial DoSpotifyRequests call")

    // First call outside of ticker
    token, tokenExpiration, err := DoSpotifyRequests(token, tokenExpiration, query, offset, ch, qSendSong)
    if err != nil {
        slog.Error("Error during Spotify requests", "error", err)
    } else {
        offset += limit
    }

    // Loop over ticker
    for range requestTicker.C {
        token, tokenExpiration, err = DoSpotifyRequests(token, tokenExpiration, query, offset, ch, qSendSong)
        if err != nil {
            slog.Error("Error during Spotify requests", "error", err)
            continue
        }
        offset += limit
        if offset >= maxOffset {
            slog.Info("Reached the maximum offset, halting requests", "offsetLimit", maxOffset)
            break
        }
    }
}

// DoSpotifyRequests checks the access token, searches tracks, sends them
func DoSpotifyRequests(
    token string,
    tokenExpiration time.Time,
    query string,
    offset int,
    ch *amqp.Channel,
    qSendSong amqp.Queue,
) (string, time.Time, error) {
    newToken, newTokenExpiration, err := CheckAccessToken(token, tokenExpiration)
    if err != nil {
        slog.Error("Failed to check access token", "error", err)
        return "", time.Time{}, err
    }

    fullQuery := query + strconv.Itoa(offset)
    songs, err := api.SearchTracks(fullQuery, newToken)
    if err != nil {
        slog.Error("Failed to search tracks", "query", fullQuery, "error", err)
        return newToken, newTokenExpiration, err
    }
    if len(*songs) == 0 {
        slog.Info("No songs found", "query", fullQuery)
        return newToken, newTokenExpiration, nil
    }
    slog.Info(fmt.Sprintf("Found %d songs", len(*songs)), "query", fullQuery)

    for _, song := range *songs {
        messaging.SendJsonMessage(ch, qSendSong, song)
    }
    return newToken, newTokenExpiration, nil
}

// CheckAccessToken refreshes the token if expired
func CheckAccessToken(token string, tokenExpiration time.Time) (string, time.Time, error) {
    if time.Now().Before(tokenExpiration) {
        // Token is still valid
        return token, tokenExpiration, nil
    }

    slog.Info("Access token expired, requesting a new one")
    newToken, err := api.GetAccessToken(
        os.Getenv("SPOTIFY_CLIENT_ID"),
        os.Getenv("SPOTIFY_CLIENT_SECRET"),
    )
    if err != nil {
        slog.Error("Failed to get access token", "error", err)
        return "", tokenExpiration, err
    }
    // Arbitrary expiration for simplicity (1 hour)
    newTokenExpiration := time.Now().Add(time.Hour)
    return newToken, newTokenExpiration, nil
}