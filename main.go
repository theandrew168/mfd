package main

import (
	"errors"
	"fmt"
	"os"
)

// Commands:
// mfd list (list available deployments)
// mfd activate (activate a deployment)
// mfd help (show help message)

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
	fmt.Println("  activate    Activate a deployment")
	fmt.Println("  help        Show this help message")
	return nil
}

func list() error {
	files, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	active, err := os.Readlink("current")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	for _, file := range files {
		if file.IsDir() {
			name := file.Name()
			if name == active {
				fmt.Printf("%s (active)\n", name)
			} else {
				fmt.Println(name)
			}
		}
	}

	return nil
}

func activate(deployment string) error {
	link, err := os.Lstat("current")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		err = os.Remove(link.Name())
		if err != nil {
			return err
		}
	}

	return os.Symlink(deployment, "current")
}

func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		return usage()
	}

	cmd := args[0]
	switch cmd {
	case "help":
		return usage()
	case "list":
		return list()
	case "activate":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return activate(deployment)
	default:
		return usage()
	}
}
