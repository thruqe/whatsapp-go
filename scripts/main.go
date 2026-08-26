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
  bump   [version]  Bump release version to current date (D.M.YY) or specified version across metadata files
  proto  [filter]   Compile and update all wa-core protobuf definitions using protoc
  res               Generate Windows binary resources & metadata with app icon from assets/logo.png
  help              Display this help message

Examples:
  go run ./scripts bump
  go run ./scripts bump 21.8.26
  go run ./scripts proto
  go run ./scripts proto waE2E`)
}
