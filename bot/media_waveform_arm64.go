//go:build arm64

package main

//go:noescape
func sumAbsPCM16(pcm []byte) uint64
