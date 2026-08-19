// Instruction cache that rebuilds the RUN_COMMAND prompt block at most once
// per cacheTTL.
package cliutils

import (
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const cacheTTL = 10 * time.Minute

// instructionCache holds the built RUN_COMMAND instruction block, rebuilt
// at most once per cacheTTL, since it's expensive-ish to rebuild (string
// concatenation over the full command list) and identical for every
// request until the command set changes.
var (
	instructionMu     sync.Mutex
	cachedInstruction string
	cachedBotName     string
	cachedPrefix      string
	instructionSetAt  time.Time
)

// GetOrBuildInstruction returns the cached instruction block if it's still
// within cacheTTL, otherwise rebuilds it via buildFn and caches the result.
func GetOrBuildInstruction(buildFn func() string) string {
	return GetOrBuildInstructionWithNameAndPrefix("", "!", buildFn)
}

// GetOrBuildInstructionWithName returns the cached instruction block for botName if still valid.
func GetOrBuildInstructionWithName(botName string, buildFn func() string) string {
	return GetOrBuildInstructionWithNameAndPrefix(botName, "!", buildFn)
}

// GetOrBuildInstructionWithNameAndPrefix returns the cached instruction block for botName and prefix if still valid.
func GetOrBuildInstructionWithNameAndPrefix(botName, prefix string, buildFn func() string) string {
	instructionMu.Lock()
	defer instructionMu.Unlock()

	if time.Since(instructionSetAt) < cacheTTL && cachedInstruction != "" && cachedBotName == botName && cachedPrefix == prefix {
		return cachedInstruction
	}

	cachedInstruction = buildFn()
	cachedBotName = botName
	cachedPrefix = prefix
	instructionSetAt = time.Now()
	return cachedInstruction
}

// ClearInstructionCache invalidates the cached RUN_COMMAND prompt block.
func ClearInstructionCache() {
	instructionMu.Lock()
	defer instructionMu.Unlock()
	cachedInstruction = ""
	cachedBotName = ""
	cachedPrefix = ""
}

// groupMetaCacheEntry holds a cached GroupInfo plus when it was fetched.
type groupMetaCacheEntry struct {
	info    types.GroupInfo
	fetchAt time.Time
}

var (
	groupMetaMu    sync.Mutex
	groupMetaCache = make(map[string]groupMetaCacheEntry)
)

// GetOrFetchGroupMeta returns cached GroupInfo for chatKey if it's still
// within cacheTTL, otherwise calls fetchFn to refresh it and caches the
// result. fetchFn is only called when a refetch is actually needed, which
// keeps this from hammering WhatsApp's group-info endpoint on every
// message in an active group.
func GetOrFetchGroupMeta(chatKey string, fetchFn func() (types.GroupInfo, error)) (types.GroupInfo, error) {
	groupMetaMu.Lock()
	entry, ok := groupMetaCache[chatKey]
	groupMetaMu.Unlock()

	if ok && time.Since(entry.fetchAt) < cacheTTL {
		return entry.info, nil
	}

	info, err := fetchFn()
	if err != nil {
		if ok {
			// Fall back to stale cached data rather than failing outright.
			return entry.info, nil
		}
		return types.GroupInfo{}, err
	}

	groupMetaMu.Lock()
	groupMetaCache[chatKey] = groupMetaCacheEntry{info: info, fetchAt: time.Now()}
	groupMetaMu.Unlock()

	return info, nil
}
