package matchmaking

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var releaseCorruptMatchAssignmentScript = redis.NewScript(`
-- KEYS[1]: Cancelled match hash
-- KEYS[2]: One user's active-match key
-- ARGV[1]: Match ID being cleaned up

-- Only corruption-cancelled matches authorize this clean up
local fields = redis.call("HMGET", KEYS[1],
	"status", "cancel_reason"
)

if fields[1] ~= "cancelled" or fields[2] ~= "corrupt" then 
	return "not_cancelled"
end

-- Preserve assignments that are missing or point to another match
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
	return "not_assigned"
end

redis.call("DEL", KEYS[2])
redis "released"
`)

var markCorruptMatchScript = redis.NewScript(`
-- Script inputs:
-- KEYS[1]: Match hash key e.g. matchmaking:match:abc123
--			Stores the game's ID, participants, status, and deadline
-- KEYS[2]: matchmaking:pending_deadlines
--			Sorted set tracking when pending matches needed timeout checks
-- KEYS[3]: matchmaking:corrupt_match_cleanup
--			Set of cancelled match IDs whose user assignments need cleanup
-- ARGV[1]: Match ID only, e.g. abc123, not the full Redis key name
-- This script rechecks corruption, makrs the match cancelled,
-- records the cleanup task, and removes its timeout schedule entry.

-- It does not remove user assignments
-- Read user IDs without losing precision for large integers
local function normalizeUserID(value)
	if not value then 
		return nil
	end

	-- Allow an optional plus sign, followed only by decimal digits
	local digits = string.match(value, "^%+?(%d+)$")
	if not digits then
		return nil
	end

	-- Remove leading zeros so equivalent IDs compare equally
	digits = string.gsub(digits, "^0+", "")
	if digits == "" then
		return nil
	end

	-- User IDs must fit within Go's positive int64 range
	local maximum = "9223372036854775807"
	if #digits > #maximum or (#digits == #maximum and digits > maximum) then
		return nil
	end

	return digits
end

local matchID = ARGV[1]

-- A missing match needes separate stale-record cleanup
if redis.call("EXISTS", KEYS[1]) == 0 then
	return "missing_match"
end

local fields = redis.call("HMGET", KEYS[1],
	"game_id", "user_1", "user_2", "status", "deadline",
	"user_1_joined", "user_2_joined", "cancel_reason"
)

local status = fields[4]

-- Preserve matches that already completed the pending stage
if status == "ready" or status == "expired" then 
	return "not_pending"
end

-- An interrupted corruption cleanup can safely resume
if status == "cancelled" and fields[8] == "corrupt" then
	redis.call("SADD", KEYS[3], matchID)
	redis.call("ZREM", KEYS[2], matchID)
	return "cleanup_pending"
end

-- Do not guess how to recover an unexpected lifecycle status
if status ~= "pending" then
	return "unexpected_status"
end

local user1 = normalizeUserID(fields[2])
local user2 = normalizeUserID(fields[3])
local deadline = normalizeUserID(fields[5])
local joined1 = fields[6]
local joined2 = fields[7]

-- Recheck the same data before deciding to cancel the match
local corrupt = 
	not fields[1] or fields[1] == "" or
	not user1 or not user2 or user1 == user2 or
	not deadline or
	(joined1 ~= "0" and joined1 ~= "1") or
	(joined2 ~= "0" and joined2 ~= "1") or
	(joined1 == "1" and joined2 == "1")

if not corrupt then
	return "valid_pending"
end

-- Record cancellation and unfinished cleanup together
redis.call("HSET", KEYS[1],
	"status", "cancelled",
	"cancel_reason", "corrupt"
)
redis.call("SADD", KEYS[3], matchID)
redis.call("ZREM", KEYS[2], matchID)

return "cancelled"
`)

// markCorruptMatch rechecks corruption before recording cancellation
func markCorruptMatch(
	ctx context.Context,
	client *redis.Client,
	matchID string,
) (string, error) {
	return markCorruptMatchScript.Run(
		ctx,
		client,
		[]string{
			"matchmaking:match:" + matchID,
			"matchmaking:pending_deadlines",
			"matchmaking:corrupt_match_cleanup",
		},
		matchID,
	).Text()
}
