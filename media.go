package whatsrook

import (
	"whatsrook/media"
)

// AudioPTTMeta contains converted Opus OGG data, duration in seconds, and 64-bin amplitude waveform bytes.
type AudioPTTMeta = media.AudioPTTMeta

var (
	// EnsureJPEG decodes input image bytes (JPEG, PNG, GIF, WebP, etc.) and re-encodes to valid JPEG bytes.
	EnsureJPEG = media.JPEGConvert

	// EnsureOpusPTT converts audio bytes to WhatsApp-compatible Opus OGG format.
	EnsureOpusPTT = media.OpusPTTConvert

	// OpusPTT is an alias for EnsureOpusPTT for concise caller ergonomics.
	OpusPTT = media.OpusPTT

	// ExtractWaveformForTest exposes waveform extraction for testing.
	ExtractWaveformForTest = media.ExtractWaveformForTest

	// TranscodeToMP3 converts input audio to MP3 via ffmpeg CLI.
	TranscodeToMP3 = media.TranscodeToMP3

	// PrepareCallVideo converts video to audio (.mp3) and Annex-B H.264 video stream (.h264) via ffmpeg CLI.
	PrepareCallVideo = media.PrepareCallVideo

	// AudioDuration calculates MP3 duration in pure Go by reading frame headers.
	AudioDuration = media.AudioDuration

	// SplitAnnexBAccessUnits parses Annex-B H.264 stream into individual access units.
	SplitAnnexBAccessUnits = media.SplitAnnexBAccessUnits

	// AnnexBHasIDR reports whether the given Annex-B packet contains an IDR keyframe.
	AnnexBHasIDR = media.AnnexBHasIDR

	// ExtensionFor maps a MIME type string to its corresponding file extension.
	ExtensionFor = media.ExtensionFor
)
