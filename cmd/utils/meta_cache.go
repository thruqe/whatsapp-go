// Instruction cache and group metadata cache powered by universal cache store.
package cliutils

import (
	"context"
	"fmt"
	"time"

	"whatsrook/src/cache"

	"go.mau.fi/whatsmeow/types"
)

const cacheTTL = 10 * time.Minute

// GetOrBuildInstructionWithNameAndPrefix returns the cached instruction block for botName and prefix if still valid.
func GetOrBuildInstructionWithNameAndPrefix(botName, prefix string, buildFn func() string) string {
	key := fmt.Sprintf("instruction:%s:%s", botName, prefix)
	if val, ok, _ := cache.Get(context.Background(), key); ok && val != "" {
		return val
	}

	instruction := buildFn()
	_ = cache.Set(context.Background(), key, instruction, cacheTTL)
	return instruction
}

// ClearInstructionCache invalidates the cached RUN_COMMAND prompt block.
func ClearInstructionCache() {
	_ = cache.DeletePrefix(context.Background(), "instruction:")
}

// GetOrFetchGroupMeta returns cached GroupInfo for chatKey if it's still
// within cacheTTL, otherwise calls fetchFn to refresh it and caches the
// result. fetchFn is only called when a refetch is actually needed, which
// keeps this from hammering WhatsApp's group-info endpoint on every
// message in an active group.
func GetOrFetchGroupMeta(chatKey string, fetchFn func() (types.GroupInfo, error)) (types.GroupInfo, error) {
	cacheKey := fmt.Sprintf("group_meta:%s", chatKey)
	var cached types.GroupInfo
	if ok, _ := cache.GetJSON(context.Background(), cacheKey, &cached); ok && !cached.JID.IsEmpty() {
		return cached, nil
	}

	info, err := fetchFn()
	if err != nil {
		if !cached.JID.IsEmpty() {
			// Fall back to stale cached data rather than failing outright.
			return cached, nil
		}
		return types.GroupInfo{}, err
	}

	_ = cache.SetJSON(context.Background(), cacheKey, info, cacheTTL)
	return info, nil
}
