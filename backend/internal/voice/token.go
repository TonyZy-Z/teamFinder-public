package voice

import (
	"strconv"
	"time"

	"github.com/livekit/protocol/auth"
)

// Create a token for one user to join one match's voice room
func CreateJoinToken(
	apiKey string,
	apiSecret string,
	matchID string,
	userID int64,
) (string, error) {
	// Both matched users use the same room name
	roomName := "match-" + matchID

	grant := &auth.VideoGrant{
		RoomJoin:          true,
		Room:              roomName,
		CanPublishSources: []string{"microphone"},
	}
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(false)

	// Identify the participant using their TeamFinder userID
	token := auth.NewAccessToken(apiKey, apiSecret)
	token.SetIdentity(strconv.FormatInt(userID, 10))
	token.SetVideoGrant(grant)
	token.SetValidFor(5 * time.Minute)

	return token.ToJWT()
}
