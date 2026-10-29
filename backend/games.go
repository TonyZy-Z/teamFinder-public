package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type game struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var supportedGames = []game{
	{ID: "civilization_vi", Name: "Civilization VI"},
	{ID: "battlefield_6", Name: "Battlefield 6"},
	{ID: "ready_or_not", Name: "Ready or Not"},
}

func isSupportedGame(gameID string) bool {
	for _, game := range supportedGames {
		if game.ID == gameID {
			return true
		}
	}
	return false
}

func gamesHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(supportedGames); err != nil {
		log.Print("Games response failed: ", err)
	}
}
