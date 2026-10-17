package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := strings.ToLower(os.Args[1])
	subArgs := os.Args[2:]

	switch subcommand {
	case "bump":
		if err := runBump(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running bump: %v\n", err)
			os.Exit(1)
		}
	case "proto":
		if err := runProto(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running proto: %v\n", err)
			os.Exit(1)
		}
	case "res":
		if err := runRes(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running res: %v\n", err)
			os.Exit(1)
		}
	case "icon":
		if err := runIcon(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running icon: %v\n", err)
			os.Exit(1)
		}
	case "sign":
		if err := runSign(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running sign: %v\n", err)
			os.Exit(1)
		}
	case "page":
		if err := runPage(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error running page: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q.\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`WhatsRook Development Scripts CLI

Usage:
  go run ./scripts <command> [arguments...]

Available Commands:
  bump   [version]        Bump monthly release version (YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO) and update resources
  proto  [--sync] [filter] Compile and update lib protobuf definitions (optional --sync pulls latest WAProto)
  res    [version]        Generate Windows binary resources & metadata with app icon from bot/assets/logo.png
  icon   [binary]         Apply and deterministically verify OS binary icon (Linux GNOME, Windows PE, macOS Mach-O)
  sign   <binary>         Digitally sign Windows PE binary with SHA-256 Authenticode signature
  page   [output_dir]     Generate static GitHub Pages site using htmlbuilder
  help                    Display this help message

Examples:
  go run ./scripts bump
  go run ./scripts bump 21.8.26
  go run ./scripts proto
  go run ./scripts proto waE2E
  go run ./scripts proto --sync
  go run ./scripts icon bin/whatsrook
  go run ./scripts sign whatsrook.exe`)
}
