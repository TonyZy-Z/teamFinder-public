package matchmaking

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// ErrCorruptMatch means stored match data violates our expected rules.
var ErrCorruptMatch = errors.New("corrupt match data")

var removeObsoleteDeadlineScript = redis.NewScript(`
-- Remove the schedule entry if the match no longer exists
if redis.call("EXISTS", KEYS[1]) == 0 then
	return redis.call("ZREM", KEYS[2], ARGV[1])
end

local status = redis.call("HGET", KEYS[1], "status")

-- Finished matches no longer need pending-timeout processing
if status == "ready" or status == "expired" then
	return redis.call("ZREM", KEYS[2], ARGV[1])
end

-- Keep pending or unexpected state for further processing
return 0
`)

var expireMatchScript = redis.NewScript(`
local matchID = ARGV[4]

if redis.call("EXISTS", KEYS[6]) == 0 then 
	return "missing_match"
end

-- Read the match details in the order we need them
local fields = redis.call("HMGET", KEYS[6],
	"game_id", "user_1", "user_2", "status", "deadline",
	"user_1_joined", "user_2_joined"
	)
-- Make sure the game and users still match what Go read
if fields[1] ~= ARGV[1] or
   fields[2] ~= ARGV[2] or
   fields[3] ~= ARGV[3] then
    return "match_changed"
end
-- Only pending matches can expire
if fields[4] ~= "pending" then
	return "not_pending"
end

local deadline = tonumber(fields[5])
if not deadline or deadline <= 0 then
	return "invalid_match"
end
-- Get Redis time in milliseconds to compare with the deadline
local now = redis.call("TIME")
local nowMilliseconds = tonumber(now[1]) * 1000 +
						math.floor(tonumber(now[2]) / 1000)

if nowMilliseconds < deadline then
	return "not_due"
end

local joined1 = fields[6]
local joined2 = fields[7]
-- Confirmation flags must be either 1 or 0
if (joined1 ~= "0" and joined1 ~= "1") or
	(joined2 ~= "0" and joined2 ~= "1") then
	return "invalid_match"
end
-- Both confirmed should mean ready, not pending
if joined1 == "1" and joined2 == "1" then
	return "invalid_match"
end
-- Check whether each user still belongs to this match
local ownsUser1 = redis.call("GET", KEYS[4]) == matchID
local ownsUser2 = redis.call("GET", KEYS[5]) == matchID

local search1 = redis.call("GET", KEYS[2])
local search2 = redis.call("GET", KEYS[3])
-- Requeue only a confirming user without another search record
local requeue1 = joined1 == "1" and ownsUser1 and not search1
local requeue2 = joined2 == "1" and ownsUser2 and not search2
-- Remove old match assignments
if ownsUser1 then
	redis.call("DEL", KEYS[4])
end

if ownsUser2 then
	redis.call("DEL", KEYS[5])
end
-- Remove leftover entires, then put the confirming user at the front
if requeue1 then
	redis.call("LREM", KEYS[1], 0, ARGV[2])
	redis.call("LPUSH", KEYS[1], ARGV[2])
	redis.call("SET", KEYS[2], ARGV[1])
end

if requeue2 then
	redis.call("LREM", KEYS[1], 0, ARGV[3])
	redis.call("LPUSH", KEYS[1], ARGV[3])
	redis.call("SET", KEYS[3], ARGV[1])
end
-- Mark the match expired and remove it from the deadline schedule
redis.call("HSET", KEYS[6], "status", "expired")
redis.call("ZREM", KEYS[7], matchID)
return "expired"
`)

// readDueMatchIDs finds matches whose scheduled deadlines have passed
func readDueMatchIDs(
	ctx context.Context,
	client *redis.Client,
	limit int64,
) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("timeout batch limit must be positive")
	}

	// Use Redis's clock, matching the clock used when creating deadlines.
	now, err := client.Time(ctx).Result()
	if err != nil {
		return nil, err
	}

	// Read the earliest due entries, up to the requested limit.
	return client.ZRangeByScore(
		ctx,
		"matchmaking:pending_deadlines",
		&redis.ZRangeBy{
			Min:   "-inf",
			Max:   strconv.FormatInt(now.UnixMilli(), 10),
			Count: limit,
		},
	).Result()
}

func removeObsoleteDeadline(
	ctx context.Context,
	client *redis.Client,
	matchID string,
) (bool, error) {
	removed, err := removeObsoleteDeadlineScript.Run(
		ctx,
		client,
		[]string{
			"matchmaking:match:" + matchID,
			"matchmaking:pending_deadlines",
		},
		matchID,
	).Int64()
	if err != nil {
		return false, err
	}
	return removed == 1, nil
}

// readMatchForTimeout reads the game and users needed to build the Redis keys
func readMatchForTimeout(
	ctx context.Context,
	client *redis.Client,
	matchID string,
) (*PendingMatch, error) {
	fields, err := client.HGetAll(
		ctx,
		"matchmaking:match:"+matchID,
	).Result()
	if err != nil {
		return nil, err
	}
	// An empty result mean the match record does not exist
	if len(fields) == 0 {
		return nil, nil
	}

	gameID := fields["game_id"]
	if gameID == "" {
		return nil, fmt.Errorf("%w: missing gameID", ErrCorruptMatch)
	}
	// Convert stored user IDs into numbers and check they are valid
	user1, err := strconv.ParseInt(fields["user_1"], 10, 64)
	if err != nil || user1 <= 0 {
		return nil, fmt.Errorf("%w: invalid first participant", ErrCorruptMatch)
	}

	user2, err := strconv.ParseInt(fields["user_2"], 10, 64)
	if err != nil || user2 <= 0 || user1 == user2 {
		return nil, fmt.Errorf("%w: invalid second participant", ErrCorruptMatch)
	}

	return &PendingMatch{
		ID:     matchID,
		GameID: gameID,
		User1:  user1,
		User2:  user2,
	}, nil
}

// ProcessTimeoutBatch checks a limited batch of overdue matches
func ProcessTimeoutBatch(
	ctx context.Context,
	client *redis.Client,
	limit int64,
) error {
	matchIDs, err := readDueMatchIDs(ctx, client, limit)
	if err != nil {
		return err
	}

	var failures []error

	for _, matchID := range matchIDs {
		// Stop starting new work if the caller cancels or times out
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}

		result, err := ExpireMatch(ctx, client, matchID)
		if err != nil {
			failures = append(failures, fmt.Errorf("expire match %s: %w", matchID, err))
			continue
		}

		switch result {
		case "expired":
			// The expiration script already removed the schedule entry
		case "missing_match", "not_pending":
			// Recheck atomically before removing an obsolete entry
			if _, err := removeObsoleteDeadline(ctx, client, matchID); err != nil {
				failures = append(failures, fmt.Errorf("clean deadline for %s: %w", matchID, err))
			}
		case "not_due":
			// Keep the entry; the match's current deadline has not arrived
		case "match_changed":
			failures = append(failures, fmt.Errorf("match %s changed during timeout processing", matchID))
		default:
			failures = append(failures, fmt.Errorf("unexpected timeout result for %s: %q", matchID, result))
		}
	}
	// Return all collected failures, or nil if non occurred
	return errors.Join(failures...)
}

// ExpireMatch asks Redis to check and process one match's timeout
func ExpireMatch(
	ctx context.Context,
	client *redis.Client,
	matchID string,
) (string, error) {
	match, err := readMatchForTimeout(ctx, client, matchID)
	if err != nil {
		return "", err
	}

	if match == nil {
		return "missing_match", nil
	}
	// Convert user IDs back to text for Redis keys and script arguments
	user1Text := strconv.FormatInt(match.User1, 10)
	user2Text := strconv.FormatInt(match.User2, 10)
	// Keep this key order aligned with KEYS in the Lua script
	keys := []string{
		"matchmaking:queue:" + match.GameID,
		"matchmaking:user:" + user1Text,
		"matchmaking:user:" + user2Text,
		"matchmaking:active_match:" + user1Text,
		"matchmaking:active_match:" + user2Text,
		"matchmaking:match:" + matchID,
		"matchmaking:pending_deadlines",
	}
	// The script rechecks the match before changing its state
	result, err := expireMatchScript.Run(
		ctx,
		client,
		keys,
		match.GameID,
		user1Text,
		user2Text,
		matchID,
	).Text()
	if err != nil {
		return "", err
	}

	// The script ran, but found invalid data in the match
	if result == "invalid_match" {
		return "", fmt.Errorf("%w: invalid timeout state", ErrCorruptMatch)
	}
	return result, nil

}
