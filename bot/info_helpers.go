package info

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

	DefaultAliveTpl      = "Hey @user, I am online and working!\n\nType {prefix}alive guide to learn how to customize this message."
	DefaultAliveTemplate = DefaultAliveTpl
)

var (
	StartTime = time.Now()

	MenuThumbPromptsMu      sync.RWMutex
	PendingMenuThumbPrompts = make(map[string]time.Time)
)
