package matchmaking

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var cancelQueueScript = redis.NewScript(`
local userKey = KEYS[1]
local userID = ARGV[1]

for i = 2, #KEYS do
    redis.call("LREM", KEYS[i], 0, userID)
end

redis.call("DEL", userKey)

return "cancelled"
`)

var joinQueueScript = redis.NewScript(`
local userKey = KEYS[1]
local queueKey = KEYS[2]
local activeMatchKey = KEYS[3]

local gameID = ARGV[1]
local userID = ARGV[2]

if redis.call("EXISTS", activeMatchKey) == 1 then
    return "active_match"
end

local currentGame = redis.call("GET", userKey)

if currentGame then
    if currentGame == gameID then
        return "already_queued"
    end

    return "different_game"
end

redis.call("RPUSH", queueKey, userID)
redis.call("SET", userKey, gameID)

return "joined"
`)

func JoinQueue(
	ctx context.Context,
	client *redis.Client,
	userID int64,
	gameID string,
) (string, error) {
	userIDText := strconv.FormatInt(userID, 10)

	keys := []string{
		"matchmaking:user:" + userIDText,
		"matchmaking:queue:" + gameID,
		"matchmaking:active_match:" + userIDText,
	}

	return joinQueueScript.Run(
		ctx,
		client,
		keys,
		gameID,
		userIDText,
	).Text()
}

func CancelQueue(
	ctx context.Context,
	client *redis.Client,
	userID int64,
	gameIDs []string,
) error {
	userIDText := strconv.FormatInt(userID, 10)

	keys := []string{
		"matchmaking:user:" + userIDText,
	}

	for _, gameID := range gameIDs {
		keys = append(keys, "matchmaking:queue:"+gameID)
	}

	return cancelQueueScript.Run(
		ctx,
		client,
		keys,
		userIDText,
	).Err()
}
