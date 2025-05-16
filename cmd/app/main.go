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

func main() {    
    rabbitMQConnectionString := os.Getenv("RABBITMQ_CONNECTION_STRING")
    conn, ch, qSendSong, err := messaging.SetUpMessaging(rabbitMQConnectionString)
	if err != nil {
		slog.Error("Failed to set up messaging", "error", err)
		return
	}
	defer conn.Close()
	defer ch.Close()

	slog.Info("Messaging set up successfully")
	retryInterval, err := strconv.Atoi(os.Getenv("RETRY_INTERVAL"))
	if err != nil {
		slog.Error("Invalid RETRY_INTERVAL value", "error", err)
		return
	}
	slog.Info(fmt.Sprintf("Retry interval: %d seconds", retryInterval))

	searchTerm := "a"
	limit := 50
	query := searchTerm + "&type=track&limit=" + strconv.Itoa(limit) + "&offset="
	slog.Info(fmt.Sprintf("Query: %s", query))

	retryTicker := time.NewTicker(time.Duration(retryInterval) * time.Second)
	defer retryTicker.Stop()

	slog.Info("Starting to request the Spotify API")
	DoSpotifyRequests(query, limit, ch, qSendSong)
	slog.Info("Halting requests to the Spotify API")

	for range retryTicker.C {
		slog.Info("Starting to request the Spotify API")
		DoSpotifyRequests(query, limit, ch, qSendSong)
		slog.Info("Halting requests to the Spotify API")
	}
}

func DoSpotifyRequests(query string, limit int, ch *amqp.Channel, qSendSong amqp.Queue) {
	requestInterval, err := strconv.Atoi(os.Getenv("REQUEST_INTERVAL"))
	if err != nil {
		slog.Error("Invalid REQUEST_INTERVAL value", "error", err)
		return
	}
	slog.Info(fmt.Sprintf("Request interval: %d seconds", requestInterval))

	requestTicker := time.NewTicker(time.Duration(requestInterval) * time.Second)
	defer requestTicker.Stop()

	offset := 0
	token := ""
	tokenExpiration := time.Now()
	slog.Info(fmt.Sprintf("Requesting the Spotify API every %d seconds", requestInterval))

	for range requestTicker.C {
		token, tokenExpiration, err = CheckAccessToken(token, tokenExpiration)
		if err != nil {
			slog.Error("Failed to check access token", "error", err)
			continue
		}
		songs, err := api.SearchTracks(query + strconv.Itoa(offset), token)
		if err != nil {
			slog.Error("Failed to search tracks", "error", err)
			continue
		}
		if len(*songs) == 0 {
			slog.Info("No songs found")
			continue
		}
		slog.Info(fmt.Sprintf("Found %d songs", len(*songs)))
		for _, song := range *songs {
			messaging.SendJsonMessage(ch, qSendSong, song)
		}
		offset += limit
		if offset >= 1000 {
			slog.Info("Reached the maximum offset of 1000, halting requests")
			break
		}
	}
}

func CheckAccessToken(token string, tokenExpiration time.Time) (string, time.Time, error) {
	if (time.Now().Unix() < tokenExpiration.Unix()) {
		return token, tokenExpiration, nil
	}

	slog.Info("Access token expired, requesting a new one")
	token, err := api.GetAccessToken(
		os.Getenv("SPOTIFY_CLIENT_ID"),
		os.Getenv("SPOTIFY_CLIENT_SECRET"),
	)
	if err != nil {
		slog.Error("Failed to get access token", "error", err)
		return "", tokenExpiration, err
	}
	tokenExpiration = time.Now().Add(time.Hour)
	return token, tokenExpiration, nil
}