package matchmaking

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// KEYS[1]: match hash
// KEYS[2]: user 1's active-match record
// KEYS[3]: user 2's active-match record
// ARGV[1]: match ID
// ARGV[2]: requesting user's ID
// ARGV[3]: user 1's ID
// ARGV[4]: user 2's ID

var leaveMatchScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
	return "missing"
end

local fields = redis.call("HMGET", KEYS[1],
	"status", "user_1", "user_2"
)

-- Check that the supplied participant keys describe this match
if fields[2] ~= ARGV[3] or fields[3] ~= ARGV[4] then
	return "invalid_state"
end

-- Only participants may end this match
if ARGV[2] ~= fields[2] and ARGV[2] ~= fields[3] then
	return "not_assigned"
end

-- Repeating a completed leave does not change anything
if fields[1] == "ended" then
	return "ended"
end

if fields[1] ~= "ready" then
	return "not_ready"
end

-- Preserve any newer assignment that points to another match
for i = 2, 3 do
	if redis.call("GET", KEYS[i]) == ARGV[1] then
		redis.call("DEL", KEYS[i])
	end
end

redis.call("HSET", KEYS[1], "status", "ended")
return "ended"
`)

func LeaveMatch(
	ctx context.Context,
	client *redis.Client,
	matchID string,
	userID int64,
) (string, error) {
	matchKey := "matchmaking:match:" + matchID

	// Read the participants so we can construct their assignment keys
	fields, err := client.HGetAll(ctx, matchKey).Result()
	if err != nil {
		return "", err
	}

	if len(fields) == 0 {
		return "missing", nil
	}

	user1 := fields["user_1"]
	user2 := fields["user_2"]

	// Do not construct assignment keys from missing participants
	if user1 == "" || user2 == "" || user1 == user2 {
		return "", fmt.Errorf("match %s has invalid participant details", matchID)
	}

	// Lua rechecks the match before changing it
	return leaveMatchScript.Run(
		ctx,
		client,
		[]string{
			matchKey,
			"matchmaking:active_match:" + user1,
			"matchmaking:active_match:" + user2,
		},
		matchID,
		strconv.FormatInt(userID, 10),
		user1,
		user2,
	).Text()
}
