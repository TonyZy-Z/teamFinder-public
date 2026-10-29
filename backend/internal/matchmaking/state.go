package matchmaking

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type State struct {
	Status   string `json:"status"`
	GameID   string `json:"game_id,omitempty"`
	MatchID  string `json:"match_id,omitempty"`
	Deadline int64  `json:"deadline,omitempty"`
}

func GetState(
	ctx context.Context,
	client *redis.Client,
	userID int64,
) (State, error) {
	userIDText := strconv.FormatInt(userID, 10)
	activeKey := "matchmaking:active_match:" + userIDText
	searchKey := "matchmaking:user:" + userIDText

	for attempt := 0; attempt < 5; attempt++ {
		membership, err := readMembershipScript.Run(
			ctx, client,
			[]string{activeKey, searchKey},
		).StringSlice()
		if err != nil {
			return State{}, err
		}

		if len(membership) != 2 {
			return State{}, fmt.Errorf("unexpected membership response")
		}

		switch membership[0] {
		case "idle":
			return State{Status: "idle"}, nil

		case "searching":
			return State{
				Status: "searching",
				GameID: membership[1],
			}, nil

		case "match":
			matchID := membership[1]

			details, err := readPendingMatchScript.Run(
				ctx, client,
				[]string{activeKey, "matchmaking:match:" + matchID},
				matchID,
				userIDText,
			).StringSlice()
			if err != nil {
				return State{}, err
			}

			if len(details) == 1 && details[0] == "retry" {
				continue
			}

			if len(details) != 4 || details[0] != "pending" {
				return State{}, fmt.Errorf("inconsistent pending-match state")
			}

			deadline, err := strconv.ParseInt(details[3], 10, 64)
			if err != nil || deadline <= 0 {
				return State{}, fmt.Errorf("invalid match deadline")
			}

			return State{
				Status:   "pending",
				GameID:   details[1],
				MatchID:  details[2],
				Deadline: deadline,
			}, nil

		default:
			return State{}, fmt.Errorf("unknown membership state")

		}
	}
	return State{}, fmt.Errorf("state lookup attempt limit reached")
}

var readMembershipScript = redis.NewScript(`
local matchID = redis.call("GET", KEYS[1])

if matchID then
	return {"match", matchID}
end

local gameID = redis.call("GET", KEYS[2])

if gameID then
	return {"searching", gameID}
end

return {"idle", ""}
`)

var readPendingMatchScript = redis.NewScript(`
local expectedMatchID = ARGV[1]
local userID = ARGV[2]

if redis.call("GET", KEYS[1]) ~= expectedMatchID then
	return {"retry"}
end

if redis.call("EXISTS", KEYS[2]) == 0 then 
	return {"missing_match"}
end

local fields = redis.call("HMGET", KEYS[2],
"status", "game_id", "deadline", "user_1", "user_2"
)

if fields[4] ~= userID and fields[5] ~= userID then
	return {"invalid_membership"}
end

if fields[1] ~= "pending" or not fields[2] or not fields[3] then
	return {"invalid_match"}
end

return {"pending", fields[2], expectedMatchID, fields[3]}

`)
