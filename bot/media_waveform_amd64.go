//go:build amd64

package main

//go:noescape
func sumAbsPCM16(pcm []byte) uint64
