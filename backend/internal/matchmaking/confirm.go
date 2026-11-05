package matchmaking

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var confirmMatchScript = redis.NewScript(`
-- KEYS[1]: Match hash
-- KEYS[2]: Requesting user's active match record
-- KEYS[3]: Pending deadlines sorted set
-- ARGV[1]: Match ID
-- ARGV[2]: Verified user ID

local matchID = ARGV[1]
local userID = ARGV[2]

if redis.call("EXISTS", KEYS[1]) == 0 then
	return "missing"
end

local fields = redis.call("HMGET", KEYS[1],
	"user_1", "user_2", "status", "deadline"
)

-- The caller must be a participant currently assigned to this match
if (fields[1] ~= userID and fields[2] ~= userID) or
	redis.call("GET", KEYS[2]) ~= matchID then
	return "not_assigned_to_match"
end

-- A retry must not undo an already successful confirmation
if fields[3] == "ready" then
	return "ready"
end

if fields[3] == "expired" then
	return "deadline_passed"
end

local deadline = tonumber(fields[4])
if fields[3] ~= "pending" or not deadline or deadline <= 0 then
	return "invalid_state"
end

local now = redis.call("TIME")
local nowMilliseconds = tonumber(now[1]) * 1000 +
						math.floor(tonumber(now[2]) / 1000)

-- Late requests cannot make an overdue match ready
if nowMilliseconds >= deadline then
	return "deadline_passed"
end

-- Record this participant's confirmation
local confirmationField = "user_1_joined"
if fields[2] == userID then
	confirmationField = "user_2_joined"
end

redis.call("HSET", KEYS[1], confirmationField, "1")
local joined = redis.call("HMGET", KEYS[1],
	"user_1_joined", "user_2_joined"
)

-- Both confirmed in time. This match no longer needs timeout processing
if joined[1] == "1" and joined[2] == "1" then
	redis.call("HSET", KEYS[1], "status", "ready")
	redis.call("ZREM", KEYS[3], matchID)
	return "ready"
end

return "pending"
`)

func ConfirmMatch(
	ctx context.Context,
	client *redis.Client,
	matchID string,
	userID int64,
) (string, error) {
	userIDText := strconv.FormatInt(userID, 10)

	// Keep these keys aligned with the script's KEYS positions
	keys := []string{
		"matchmaking:match:" + matchID,
		"matchmaking:active_match:" + userIDText,
		"matchmaking:pending_deadlines",
	}

	return confirmMatchScript.Run(
		ctx,
		client,
		keys,
		matchID,
		userIDText,
	).Text()
}
