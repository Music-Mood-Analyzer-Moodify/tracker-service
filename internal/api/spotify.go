package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"tracker-service/internal/model"
)

type spotifyTracksResponse struct {
	Tracks struct {
		Items []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		}
	} `json:"tracks"`
}

func GetAccessToken(
	clientID string,
	clientSecret string,
) (string, error) {
	slog.Info("Getting access token from Spotify API")

	url := "https://accounts.spotify.com/api/token"
	body := strings.NewReader("grant_type=client_credentials&client_id=" + clientID + "&client_secret=" + clientSecret)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		slog.Error("Error making request to Spotify API", "error", err)
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		slog.Error("Error response from Spotify API", "status_code", res.StatusCode)
		return "", fmt.Errorf("error response from Spotify API: %s", res.Status)
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tokenResponse); err != nil {
		slog.Error("Error decoding Spotify API response", "error", err)
		return "", err
	}

	return tokenResponse.AccessToken, nil
}

func SearchTracks(
	query string,
	accessToken string,
) (*[]model.Song, error) {
	slog.Info("Searching for tracks", "query", query)

	url := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s", query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer " + accessToken)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		slog.Error("Error making request to Spotify API", "error", err)
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		slog.Error("Error response from Spotify API", "status_code", res.StatusCode)
		return nil, fmt.Errorf("error response from Spotify API: %s", res.Status)
	}

    var spotifyResp spotifyTracksResponse
    if err := json.NewDecoder(res.Body).Decode(&spotifyResp); err != nil {
        slog.Error("Error decoding Spotify API response", "error", err)
        return nil, err
    }

	songs := make([]model.Song, len(spotifyResp.Tracks.Items))

	for i, item := range spotifyResp.Tracks.Items {
		artistNames := make([]string, len(item.Artists))
		for j, artist := range item.Artists {
			artistNames[j] = artist.Name
		}
		
		songs[i] = model.Song{
			ID:          item.ID,
			Name:        item.Name,
			ArtistNames: artistNames,
		}
	}

    return &songs, nil
}