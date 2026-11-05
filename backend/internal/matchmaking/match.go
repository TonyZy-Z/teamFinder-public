package matchmaking

import (
	"context"

	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/redis/go-redis/v9"
	"strconv"
)

func TryPair(
	ctx context.Context,
	client *redis.Client,
	gameID string,
) (*PendingMatch, error) {
	for attempt := 0; attempt < 5; attempt++ {
		candidates, err := readPairCandidates(ctx, client, gameID)
		if err != nil {
			return nil, err
		}

		if len(candidates) < 2 {
			return nil, nil
		}

		user1, err := strconv.ParseInt(candidates[0], 10, 64)
		if err != nil || user1 <= 0 {
			return nil, fmt.Errorf("invalid first queue user ID")
		}

		user2, err := strconv.ParseInt(candidates[1], 10, 64)
		if err != nil || user2 <= 0 {
			return nil, fmt.Errorf("invalid second queue user ID")
		}

		// Generates a random match ID
		randomBytes := make([]byte, 16)
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, err
		}
		matchID := hex.EncodeToString(randomBytes)

		keys := []string{
			"matchmaking:queue:" + gameID,
			"matchmaking:user:" + candidates[0],
			"matchmaking:user:" + candidates[1],
			"matchmaking:active_match:" + candidates[0],
			"matchmaking:active_match:" + candidates[1],
			"matchmaking:match:" + matchID,
			"matchmaking:pending_deadlines",
		}

		result, err := pairScript.Run(
			ctx, client, keys,
			gameID, candidates[0], candidates[1], matchID,
		).Text()
		if err != nil {
			return nil, err
		}

		switch result {
		case "matched":
			return &PendingMatch{
				ID:     matchID,
				GameID: gameID,
				User1:  user1,
				User2:  user2,
			}, nil

		case "retry", "match_id_conflict":
			continue

		case "ineligible":
			err := cleanPairCandidatesScript.Run(
				ctx, client, keys[:5],
				gameID, candidates[0], candidates[1],
			).Err()
			if err != nil {
				return nil, err
			}
			continue

		default:
			return nil, fmt.Errorf("unexpected pairing result: %q", result)
		}
	}
	return nil, fmt.Errorf("pairing attempt limit reached")
}

func readPairCandidates(
	ctx context.Context,
	client *redis.Client,
	gameID string,
) ([]string, error) {
	queueKey := "matchmaking:queue:" + gameID

	return client.LRange(ctx, queueKey, 0, 1).Result()
}

type PendingMatch struct {
	ID     string
	GameID string
	User1  int64
	User2  int64
}

var cleanPairCandidatesScript = redis.NewScript(`
local queueKey = KEYS[1]
local gameID = ARGV[1]
local user1 = ARGV[2]
local user2 = ARGV[3]

-- Only act if these are still the first two entries
local candidates = redis.call("LRANGE", queueKey, 0, 1)
if candidates[1] ~= user1 or candidates[2] ~= user2 then
	return "retry"
end

local user1Eligible = 
	redis.call("GET", KEYS[2]) == gameID and 
	redis.call("EXISTS", KEYS[4]) == 0

local user2Eligible = 
	redis.call("GET", KEYS[3]) == gameID and 
	redis.call("EXISTS", KEYS[5]) == 0

if not user1Eligible then
	redis.call("LREM", queueKey, 0, user1)
end

if not user2Eligible then
	redis.call("LREM", queueKey, 0, user2)
end

-- If the same eligible user occupies both positions, 
-- keep one entry at their original position: the front
if user1 == user2 and user1Eligible then
	redis.call("LREM", queueKey, 0, user1)
	redis.call("LPUSH", queueKey, user1)
end

return "checked"
`)

var pairScript = redis.NewScript(`
local queueKey = KEYS[1]
local gameID = ARGV[1]
local user1 = ARGV[2]
local user2 = ARGV[3]
local matchID = ARGV[4]

-- Check the candidates are still first and second.
local candidates = redis.call("LRANGE", queueKey, 0, 1)
if candidates[1] ~= user1 or candidates[2] ~= user2 then
    return "retry"
end

-- A match requires two different users.
if user1 == user2 then
    return "ineligible"
end

-- Both users must still be searching for this game.
if redis.call("GET", KEYS[2]) ~= gameID or
   redis.call("GET", KEYS[3]) ~= gameID then
    return "ineligible"
end

-- Neither user may already belong to a match.
if redis.call("EXISTS", KEYS[4], KEYS[5]) > 0 then
    return "ineligible"
end

-- Never overwrite an existing match.
if redis.call("EXISTS", KEYS[6]) == 1 then
    return "match_id_conflict"
end

-- Calculate the confirmation deadline.
local now = redis.call("TIME")
local nowMilliseconds =
    tonumber(now[1]) * 1000 +
    math.floor(tonumber(now[2]) / 1000)

local deadline = nowMilliseconds + 30000

-- Create the pending match.
redis.call("HSET", KEYS[6],
    "game_id", gameID,
    "user_1", user1,
    "user_2", user2,
    "status", "pending",
    "user_1_joined", "0",
    "user_2_joined", "0",
    "deadline", tostring(deadline)
)

-- Track when this pending match needs a timeout check
redis.call("ZADD", KEYS[7], deadline, matchID)

-- Assign both users to it.
redis.call("SET", KEYS[4], matchID)
redis.call("SET", KEYS[5], matchID)

-- Remove their waiting state.
redis.call("LPOP", queueKey)
redis.call("LPOP", queueKey)
redis.call("DEL", KEYS[2], KEYS[3])

return "matched"
`)
