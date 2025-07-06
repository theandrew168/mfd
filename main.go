package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

const (
	ActiveDeploymentSymlinkName = "active"
	KeepDeploymentsCount        = 3
)

type Command []string

func (c Command) String() string {
	return strings.Join(c, " ")
}

type Config struct {
	Repo struct {
		URL string `toml:"url"`
	} `toml:"repo"`
	Build struct {
		Commands []Command `toml:"commands"`
	} `toml:"build"`
}

// This function reads the configuration from a TOML string and returns a Config struct.
// It checks for required fields and returns an error if any are missing.
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
	fmt.Println("  deploy      Resolve, fetch, build, and activate a revision")
	fmt.Println("  resolve     Resolve a revision to a deployment")
	fmt.Println("  activate    Activate a deployment")
	fmt.Println("  rollback    Rollback to the previous deployment")
	fmt.Println("  remove      Remove a deployment")
	fmt.Println("  clean       Remove old deployments")
	fmt.Println("  help        Show this help message")
	return nil
}

// This function filters out directories that are not relevant for deployments.
// It ignores hidden directories (those starting with a dot) and non-directory entries.
func filterRelevantDirectories(files []os.DirEntry) []os.DirEntry {
	var filteredFiles []os.DirEntry

	for _, file := range files {
		name := file.Name()

		// Ignore anythhing that is not a directory.
		if !file.IsDir() {
			continue
		}

		// Ignore hidden directories.
		if strings.HasPrefix(name, ".") {
			continue
		}

		filteredFiles = append(filteredFiles, file)
	}

	return filteredFiles
}

// This function sorts the files by modification time, newest first.
// It uses the `slices.Clone` function to avoid modifying the original slice.
func sortFilesNewestToOldest(files []os.DirEntry) []os.DirEntry {
	sortedFiles := slices.Clone(files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		infoI, _ := files[i].Info()
		infoJ, _ := files[j].Info()
		// Since this function is checking for "less" and we want newest first,
		// we need to check if infoI was modified after infoJ.
		return infoI.ModTime().After(infoJ.ModTime())
	})
	return sortedFiles
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

	files = filterRelevantDirectories(files)

	active, err := os.Readlink(ActiveDeploymentSymlinkName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	for _, file := range files {
		name := file.Name()
		if name == active {
			fmt.Printf("%s (active)\n", name)
		} else {
			fmt.Println(name)
		}
	}

	return nil
}

func (mfd *MFD) Activate(deployment string) error {
	link, err := os.Lstat(ActiveDeploymentSymlinkName)
	if err != nil {
		// If the symlink does not exist, we'll soon create it.
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		// If the symlink already exists, remove it.
		// NOTE: There is technically a small race condition here between
		// removing the current symlink and creating the new one.
		err = os.Remove(link.Name())
		if err != nil {
			return err
		}
	}

	return os.Symlink(deployment, ActiveDeploymentSymlinkName)
}

func (mfd *MFD) Fetch(deployment string) error {
	repo, err := git.PlainClone(deployment, false, &git.CloneOptions{
		URL: mfd.conf.Repo.URL,
	})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return nil
		}
		return fmt.Errorf("error cloning repository: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("error getting worktree: %w", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(deployment),
	})
	if err != nil {
		return fmt.Errorf("error checking out commit %s: %w", deployment, err)
	}

	return nil
}

func (mfd *MFD) Build(deployment string) error {
	for _, command := range mfd.conf.Build.Commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = deployment
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Println(command)
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

func (mfd *MFD) Resolve(revision string) (string, error) {
	// Perform an in-memory clone to find the expected commit / tag.
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: mfd.conf.Repo.URL,
	})
	if err != nil {
		return "", fmt.Errorf("error performing in-memory clone: %w", err)
	}

	// Resolve the revision to a commit hash.
	commit, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return "", fmt.Errorf("error resolving revision: %w", err)
	}

	return commit.String(), nil
}

func (mfd *MFD) Remove(deployment string) error {
	active, err := os.Readlink(ActiveDeploymentSymlinkName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if active == deployment || deployment == ActiveDeploymentSymlinkName {
		return errors.New("cannot remove active deployment")
	}

	err = os.RemoveAll(deployment)
	if err != nil {
		return err
	}

	return nil
}

func (mfd *MFD) Clean() error {
	files, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	files = filterRelevantDirectories(files)
	files = sortFilesNewestToOldest(files)

	// Remove all but the most recent N deployments.
	for _, file := range files[KeepDeploymentsCount:] {
		if file.Name() == ActiveDeploymentSymlinkName {
			fmt.Println("Skipping removal of active deployment")
			continue
		}

		fmt.Printf("Removing deployment %s\n", file.Name())
		err = os.RemoveAll(file.Name())
		if err != nil {
			return err
		}
	}

	return nil
}

func (mfd *MFD) Rollback() error {
	files, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	files = filterRelevantDirectories(files)
	files = sortFilesNewestToOldest(files)

	// Read the active deployment symlink.
	active, err := os.Readlink(ActiveDeploymentSymlinkName)
	if err != nil {
		return errors.New("active deployment not found")
	}

	// Find the index of the active deployment.
	activeIndex := slices.IndexFunc(files, func(file os.DirEntry) bool {
		return file.Name() == active
	})

	if activeIndex == -1 {
		return errors.New("active deployment not found")
	}

	prevIndex := activeIndex + 1
	if prevIndex >= len(files) {
		return errors.New("no previous deployment found")
	}

	prevDeployment := files[prevIndex].Name()
	fmt.Printf("Rolling back to %s\n", prevDeployment)

	err = mfd.Activate(prevDeployment)
	if err != nil {
		return err
	}

	return nil
}

func run() error {
	data, err := os.ReadFile("mfd.toml")
	if err != nil {
		return fmt.Errorf("error reading configuration: %w", err)
	}

	conf, err := readConfig(string(data))
	if err != nil {
		return fmt.Errorf("error reading configuration: %w", err)
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
	case "ls":
		fallthrough
	case "list":
		return mfd.List()
	case "deploy":
		revision := "HEAD"
		if len(args) > 1 {
			revision = args[1]
		}

		deployment, err := mfd.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, deployment)
		return mfd.Deploy(deployment)
	case "resolve":
		revision := "HEAD"
		if len(args) > 1 {
			revision = args[1]
		}

		deployment, err := mfd.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, deployment)
		return nil
	case "activate":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Activate(deployment)
	case "rollback":
		return mfd.Rollback()
	case "rm":
		fallthrough
	case "delete":
		fallthrough
	case "remove":
		if len(args) < 2 {
			return usage()
		}
		deployment := args[1]
		return mfd.Remove(deployment)
	case "clean":
		return mfd.Clean()
	default:
		return usage()
	}
}
