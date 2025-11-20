package main

import (
	"fmt"
	"os"

	"github.com/theandrew168/mfd/internal/config"
	"github.com/theandrew168/mfd/internal/mfd"
)

func main() {
	code := 0

	err := run()
	if err != nil {
		fmt.Println(err.Error())
		code = 1
	}

	os.Exit(code)
}

func usage() error {
	fmt.Println("usage: mfd <command> [<args>]")
	fmt.Println("commands:")
	fmt.Println("  list        List available deployments")
	fmt.Println("  deploy      Resolve, fetch, build, and activate a revision")
	fmt.Println("  resolve     Resolve a revision to a deployment")
	fmt.Println("  rollback    Rollback to the previous deployment")
	fmt.Println("  clean       Remove old, non-active deployments")
	fmt.Println("  help        Show this help message")
	return nil
}

func run() error {
	cfg, err := config.ReadFile("mfd.toml")
	if err != nil {
		return fmt.Errorf("error reading configuration: %w", err)
	}

	client := mfd.NewClient(cfg)

	args := os.Args[1:]
	if len(args) == 0 {
		return usage()
	}

	cmd := args[0]
	switch cmd {
	case "help":
		return usage()
	case "ls":
		fallthrough
	case "list":
		return client.List()
	case "deploy":
		revision := "HEAD"
		if len(args) > 1 {
			revision = args[1]
		}

		commitHash, err := client.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, commitHash)
		return client.Deploy(commitHash)
	case "rollback":
		return client.Rollback()
	case "resolve":
		revision := "HEAD"
		if len(args) > 1 {
			revision = args[1]
		}

		commitHash, err := client.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, commitHash)
		return nil
	case "restart":
		return client.Restart()
	case "clean":
		return client.Clean()
	default:
		return usage()
	}
}
