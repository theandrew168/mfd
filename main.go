package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
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

type Config struct {
	Repo struct {
		URL string `toml:"url"`
	} `toml:"repo"`
	Build struct {
		Commands [][]string `toml:"commands"`
	} `toml:"build"`
}

func readConfig(data string) (Config, error) {
	conf := Config{}

	meta, err := toml.Decode(data, &conf)
	if err != nil {
		return Config{}, err
	}

	// Build set of present config keys.
	present := make(map[string]bool)
	for _, key := range meta.Keys() {
		present[key.String()] = true
	}

	required := []string{
		"repo.url",
		"build.commands",
	}

	// Gather any missing values.
	missing := []string{}
	for _, key := range required {
		if _, ok := present[key]; !ok {
			missing = append(missing, key)
		}
	}

	// Error upon missing values
	if len(missing) > 0 {
		msg := strings.Join(missing, ", ")
		return Config{}, fmt.Errorf("missing config values: %s", msg)
	}

	return conf, nil
}

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

type MFD struct {
	conf Config
}

func NewMFD(conf Config) MFD {
	mfd := MFD{
		conf: conf,
	}
	return mfd
}

func (mfd *MFD) List() error {
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

func (mfd *MFD) Activate(deployment string) error {
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

func (mfd *MFD) Fetch(deployment string) error {
	r, err := git.PlainClone(deployment, false, &git.CloneOptions{
		URL:      mfd.conf.Repo.URL,
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
		Hash: plumbing.NewHash(deployment),
	})
	if err != nil {
		return err
	}

	return nil
}

func (mfd *MFD) Build(deployment string) error {
	for _, command := range mfd.conf.Build.Commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = deployment
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Println(strings.Join(command, " "))
		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func (mfd *MFD) Deploy(deployment string) error {
	err := mfd.Fetch(deployment)
	if err != nil {
		return err
	}

	err = mfd.Build(deployment)
	if err != nil {
		return err
	}

	err = mfd.Activate(deployment)
	if err != nil {
		return err
	}

	return nil
}

func (mfd *MFD) Remove(deployment string) error {
	active, err := os.Readlink(activeDeploymentSymlinkName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if active == deployment || deployment == activeDeploymentSymlinkName {
		return errors.New("cannot remove active deployment")
	}

	err = os.RemoveAll(deployment)
	if err != nil {
		return err
	}

	return nil
}

func run() error {
	data, err := os.ReadFile("mfd.toml")
	if err != nil {
		return err
	}

	conf, err := readConfig(string(data))
	if err != nil {
		return err
	}

	mfd := NewMFD(conf)

	args := os.Args[1:]
	if len(args) == 0 {
		return usage()
	}

	cmd := args[0]
	switch cmd {
	case "help":
		return usage()
	case "list":
		return mfd.List()
	case "fetch":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Fetch(deployment)
	case "build":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Build(deployment)
	case "deploy":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Deploy(deployment)
	case "activate":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Activate(deployment)
	case "remove":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Remove(deployment)
	default:
		return usage()
	}
}
