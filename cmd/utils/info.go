package cliutils

import (
	"sync"
	"time"
)

const (
	AliveTemplateKey  = "alive_template"
	AliveMediaKey     = "alive_media"
	AliveMediaTypeKey = "alive_media_type"
	AliveMediaMimeKey = "alive_media_mime"
	AliveMediaFileKey = "alive_media_file"

	DefaultAliveTpl      = "{bot} IS ALIVE\n\nOwner: {owner}\n⏱️ Uptime: {uptime}\nLatency: {latency}\nRAM: {ram}\nGoroutines: {goroutines}\nVersion: {version}\n\nType {prefix}menu to display available commands."
	DefaultAliveTemplate = DefaultAliveTpl
)

var (
	StartTime = time.Now()

	AliveOnce sync.Once
	BootTime  time.Time

	MenuThumbPromptsMu      sync.RWMutex
	PendingMenuThumbPrompts = make(map[string]time.Time)
)

func InitBootTime() {
	AliveOnce.Do(func() {
		BootTime = time.Now()
	})
}
