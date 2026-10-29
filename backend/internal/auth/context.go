package auth

import "context"

type userIDKey struct{}

/**
Handlers use UserIDFromContext to retrieve userID verified by middleware
**/
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey{}).(int64)
	return userID, ok
}