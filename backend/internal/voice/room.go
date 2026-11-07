package voice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/twitchtv/twirp"
)

// It tells LiveKit to close the voice room for this match and disconnect everyone inside it
func CloseRoom(
	ctx context.Context,
	client *lksdk.RoomServiceClient,
	matchID string,
) error {
	log.Printf("Requesting LiveKit room deletion: match-%s", matchID)
	_, err := client.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: "match-" + matchID,
	})

	if err != nil {
		var apiErr twirp.Error
		if errors.As(err, &apiErr) && apiErr.Code() == twirp.NotFound {
			log.Printf("LiveKit reports room not found: match-%s", matchID)
			return nil
		}
		return err
	}
	log.Printf("LiveKit confirmed room deletion: match-%s", matchID)
	return nil
}

// Revoke existing voice tokens and remove these participants from the room
func RevokeRoomAccess(
	ctx context.Context,
	client *lksdk.RoomServiceClient,
	matchID string,
	userIDs []string,
) error {
	// Include tokens issued during the current second
	cutoff := time.Now().Unix() + 1

	for _, userID := range userIDs {
		_, err := client.RemoveParticipant(
			ctx,
			&livekit.RoomParticipantIdentity{
				Room:          "match-" + matchID,
				Identity:      userID,
				RevokeTokenTs: cutoff,
			},
		)
		if err != nil {
			return fmt.Errorf(
				"revoke voice access for user %s: %w",
				userID,
				err,
			)
		}
	}
	return nil
}
