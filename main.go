package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Commands:
// mfd list (list available deployments)
// mfd activate (activate a deployment)
// mfd help (show help message)

// Other commands:
// mfd clean (remove all deployments except the most recent N)
// mfd remove (remove a deployment)
// mfd rollback (rollback to the previous deployment)
// mfd fetch (fetch a deployment)
// mfd build (build a deployment)
// mfd deploy (fetch, build, and activate a deployment)

// Should this integrate with systemd? Requires sudo.
// Should this handle fetching and building the deployment?

const activeDeploymentSymlinkName = "current"

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
	fmt.Println("  fetch       Fetch a deployment")
	fmt.Println("  build       Build a deployment")
	fmt.Println("  deploy      Fetch, build, and activate a deployment")
	fmt.Println("  activate    Activate a deployment")
	fmt.Println("  help        Show this help message")
	return nil
}

func list() error {
	files, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	active, err := os.Readlink(activeDeploymentSymlinkName)
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
	link, err := os.Lstat(activeDeploymentSymlinkName)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		err = os.Remove(link.Name())
		if err != nil {
			return err
		}
	}

	return os.Symlink(deployment, activeDeploymentSymlinkName)
}

// TODO: Maybe the repo can be part of the configuration file?
func fetch(repo, commit string) error {
	r, err := git.PlainClone(commit, false, &git.CloneOptions{
		URL:      repo,
		Progress: os.Stdout,
	})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return nil
		}
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	err = w.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(commit),
	})
	if err != nil {
		return err
	}

	return nil
}

// TODO: How can a user configure the build command(s)?
func build(deployment string) error {
	cmd := exec.Command("npm", "install")
	cmd.Dir = deployment
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("npm", "run", "build")
	cmd.Dir = deployment
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func deploy(repo, commit string) error {
	err := fetch(repo, commit)
	if err != nil {
		return err
	}

	err = build(commit)
	if err != nil {
		return err
	}

	err = activate(commit)
	if err != nil {
		return err
	}

	return nil
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
	case "fetch":
		if len(args) < 3 {
			return usage()
		}
		repo, commit := args[1], args[2]
		return fetch(repo, commit)
	case "build":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return build(deployment)
	case "deploy":
		if len(args) < 3 {
			return usage()
		}
		repo, commit := args[1], args[2]
		return deploy(repo, commit)
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
