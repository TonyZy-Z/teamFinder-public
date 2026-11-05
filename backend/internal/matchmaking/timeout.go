package matchmaking

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// This ends a pending match only after its deadline and applies the confiramtion rules
var expireMatchScript = redis.NewScript(`
-- KEYS[1]: Game queue
-- KEYS[2] and KEYS[3]: Users' search records
-- KEYS[4] and KEYS[5]: Users' active-match records
-- KEYS[6]: Match hash
-- KEYS[7]: Pending-deadlines sorted set
-- ARGV[1]: Game ID
-- ARGV[2] and ARGV[3]: User IDs
-- ARGV[4]: Match ID

local matchID = ARGV[4]

-- Remove an obsolete schedule entry if the match is gone
if redis.call("EXISTS", KEYS[6]) == 0 then
	redis.call("ZREM", KEYS[7], matchID)
	return "missing"
end

local fields = redis.call("HMGET", KEYS[6],
	"game_id", "user_1", "user_2", "status", "deadline",
	"user_1_joined", "user_2_joined"	
)

-- Verify the details Go used to construct our keys
-- End this script if some details are out-dated
if fields[1] ~= ARGV[1] or
   fields[2] ~= ARGV[2] or 
   fields[3] ~= ARGV[3] then
	return "changed"
end

-- Ready or expired matches no longer need pending-timeout checks
if fields[4] == "ready" or fields[4] == "expired" then
	redis.call("ZREM", KEYS[7], matchID)
	return "finished"
end

local deadline = tonumber(fields[5])
local joined1 = fields[6]
local joined2 = fields[7]

-- Only expire pending matches with a valid deadline
if fields[4] ~= "pending" or not deadline or deadline <= 0 then
	return "invalid_state"
end

local now = redis.call("TIME")
local nowMilliseconds = tonumber(now[1]) * 1000 +
						math.floor(tonumber(now[2]) / 1000)
if nowMilliseconds < deadline then
	return "not_due"
end

-- Release each user only if they still belong to this match
for i = 1, 2 do
	local activeKey = KEYS[i + 3]
	local searchKey = KEYS[i + 1]
	local userID = ARGV[i + 1]
	local joined = fields[i + 5]

	if redis.call("GET", activeKey) == matchID then
		redis.call("DEL", activeKey)

		-- Return a confirming user to the front without overwriting a search
		if joined == "1" and redis.call("EXISTS", searchKey) == 0 then
			redis.call("LREM", KEYS[1], 0, userID)
			redis.call("LPUSH", KEYS[1], userID)
			redis.call("SET", searchKey, ARGV[1])
		end
	end
end

redis.call("HSET", KEYS[6], "status", "expired")
redis.call("ZREM", KEYS[7], matchID)

return "expired"
`)

// It checks whether one pending match should expire.
func ExpireMatch(
	ctx context.Context,
	client *redis.Client,
	matchID string,
) (string, error) {
	// Read the game and users needed to construct the script's keys
	fields, err := client.HGetAll(
		ctx,
		"matchmaking:match:"+matchID,
	).Result()
	if err != nil {
		return "", err
	}

	if len(fields) == 0 {
		return "missing", nil
	}

	gameID := fields["game_id"]
	user1 := fields["user_1"]
	user2 := fields["user_2"]

	// Do not construct user keys from missing or duplicate participants
	if gameID == "" || user1 == "" || user2 == "" || user1 == user2 {
		return "", fmt.Errorf("match %s has invalid participant details", matchID)
	}

	// Keep this order aligned with KEYS in Lua script
	keys := []string{
		"matchmaking:queue:" + gameID,
		"matchmaking:user:" + user1,
		"matchmaking:user:" + user2,
		"matchmaking:active_match:" + user1,
		"matchmaking:active_match:" + user2,
		"matchmaking:match:" + matchID,
		"matchmaking:pending_deadlines",
	}

	// Calling Lua script to mark this match expire if it is overdue
	return expireMatchScript.Run(
		ctx,
		client,
		keys,
		gameID,
		user1,
		user2,
		matchID,
	).Text()
}

// It checks up to 100 overdue matches
func ProcessTimeoutBatch(
	ctx context.Context,
	client *redis.Client,
	onExpired func(context.Context, string),
) error {
	// Use the same Redis clock used to create match deadline
	now, err := client.Time(ctx).Result()
	if err != nil {
		return err
	}

	matchIDs, err := client.ZRangeByScore(
		ctx,
		"matchmaking:pending_deadlines",
		&redis.ZRangeBy{
			Min:   "-inf",
			Max:   strconv.FormatInt(now.UnixMilli(), 10),
			Count: 100,
		},
	).Result()
	if err != nil {
		return err
	}

	for _, matchID := range matchIDs {
		if err := ctx.Err(); err != nil {
			return err
		}

		result, err := ExpireMatch(ctx, client, matchID)
		if err != nil {
			// Report this match's failure and continue with others
			log.Printf("Timeout processing failed for %s: %v", matchID, err)
			continue
		}

		switch result {
		case "expired":
			// Redis updates are complete; notify the affected participants
			if onExpired != nil {
				onExpired(ctx, matchID)
			}
		case "finished":
			// No new expiration occurred, so no notification is needed
		case "missing":
			// Match IDs are never reused; remove this obsolete entry
			if err := client.ZRem(
				ctx, "matchmaking:pending_deadlines", matchID,
			).Err(); err != nil {
				log.Printf("Deadline cleanup failed for %s: %v", matchID, err)
			}
		default:
			// Keep unexpected state visible without adding recovery logic
			log.Printf("Timeout processing skipped %s: %s", matchID, result)
		}
	}
	return nil
}

// It checks pending mathc deadlines every second
// It runs until the caller cancels ctx
func RunTimeoutWorker(
	ctx context.Context,
	client *redis.Client,
	onExpired func(context.Context, string),
) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// The backend is stopping
			return
		case <-ticker.C:
			//Limit how long one processing round may take
			batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := ProcessTimeoutBatch(batchCtx, client, onExpired)
			cancel()

			if err != nil && ctx.Err() == nil {
				log.Printf("Match timeout processing failed:%v", err)
			}
		}
	}
}
