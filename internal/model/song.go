package model

type Song struct {
	ID          string   `json:"spotifyId"`
	Name        string   `json:"name"`
	ArtistNames []string `json:"artistNames"`
}