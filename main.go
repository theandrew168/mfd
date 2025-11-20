package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

const (
	ActiveDeploymentSymlinkName = "active"
	KeepDeploymentsCount        = 3
)

var (
	ErrDeploymentNotFound = errors.New("deployment not found")
	ErrInvalidDeployment  = errors.New("invalid deployment")
	ErrMissingUsername    = errors.New("username must be specified when using password authentication")
	ErrTokenAndPassword   = errors.New("cannot specify both password and token for authentication")
)

type Command []string

func (c Command) String() string {
	return strings.Join(c, " ")
}

type Deployment struct {
	CreatedAt  time.Time
	CommitHash string
}

func NewDeployment(createdAt time.Time, commitHash string) Deployment {
	return Deployment{
		CreatedAt:  createdAt,
		CommitHash: commitHash,
	}
}

func ParseDeployment(s string) (Deployment, error) {
	parts := strings.Split(s, "_")
	if len(parts) != 3 {
		return Deployment{}, ErrInvalidDeployment
	}

	if parts[0] != "mfd" {
		return Deployment{}, ErrInvalidDeployment
	}

	t, err := strconv.Atoi(parts[1])
	if err != nil {
		return Deployment{}, ErrInvalidDeployment
	}

	// Validate that the hash is a 40-character SHA1 hash.
	if len(parts[2]) != 40 {
		return Deployment{}, ErrInvalidDeployment
	}

	d := Deployment{
		CreatedAt:  time.Unix(int64(t), 0),
		CommitHash: parts[2],
	}
	return d, nil
}

func (d Deployment) String() string {
	return fmt.Sprintf("mfd_%d_%s", d.CreatedAt.Unix(), d.CommitHash)
}

func sortDeploymentsNewestToOldest(deployments []Deployment) []Deployment {
	sortedDeployments := slices.Clone(deployments)
	slices.SortFunc(sortedDeployments, func(a, b Deployment) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return sortedDeployments
}

type Config struct {
	Repo struct {
		URL      string `toml:"url"`
		Username string `toml:"username"`
		Password string `toml:"password"`
		Token    string `toml:"token"`
	} `toml:"repo"`
	Build struct {
		Commands []Command `toml:"commands"`
	} `toml:"build"`
	Systemd struct {
		Unit string `toml:"unit"`
	} `toml:"systemd"`
}

func (c Config) CloneOptions() *git.CloneOptions {
	opts := git.CloneOptions{
		URL: c.Repo.URL,
	}

	if c.Repo.Token != "" {
		opts.Auth = &http.BasicAuth{
			Username: "mfd",
			Password: c.Repo.Token,
		}
	} else if c.Repo.Password != "" {
		opts.Auth = &http.BasicAuth{
			Username: c.Repo.Username,
			Password: c.Repo.Password,
		}
	}

	return &opts
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

	if conf.Repo.Password != "" && conf.Repo.Token != "" {
		return Config{}, ErrTokenAndPassword
	}
	if conf.Repo.Password != "" && conf.Repo.Username == "" {
		return Config{}, ErrMissingUsername
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
	fmt.Println("  rollback    Rollback to the previous deployment")
	fmt.Println("  clean       Remove old, non-active deployments")
	fmt.Println("  help        Show this help message")
	return nil
}

func filesToDeployments(files []os.DirEntry) []Deployment {
	var deployments []Deployment

	for _, file := range files {
		name := file.Name()

		// Ignore anythhing that is not a directory.
		if !file.IsDir() {
			continue
		}

		deployment, err := ParseDeployment(name)
		if err != nil {
			continue
		}

		deployments = append(deployments, deployment)
	}

	return deployments
}

func listDeployments() ([]Deployment, error) {
	files, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	deployments := filesToDeployments(files)
	deployments = sortDeploymentsNewestToOldest(deployments)
	return deployments, nil
}

func findDeploymentByCommitHash(deployments []Deployment, commitHash string) (Deployment, error) {
	for _, deployment := range deployments {
		if deployment.CommitHash == commitHash {
			return deployment, nil
		}
	}

	return Deployment{}, ErrDeploymentNotFound
}

func getActiveDeployment() (Deployment, error) {
	link, err := os.Readlink(ActiveDeploymentSymlinkName)
	if err != nil {
		return Deployment{}, err
	}

	deployment, err := ParseDeployment(link)
	if err != nil {
		return Deployment{}, err
	}

	return deployment, nil
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
	deployments, err := listDeployments()
	if err != nil {
		return err
	}

	activeDeployment, err := getActiveDeployment()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for _, deployment := range deployments {
		if deployment.String() == activeDeployment.String() {
			fmt.Printf("%s (active)\n", deployment.CommitHash)
		} else {
			fmt.Println(deployment.CommitHash)
		}
	}

	return nil
}

func (mfd *MFD) Activate(deployment Deployment) error {
	link, err := os.Lstat(ActiveDeploymentSymlinkName)
	if err != nil {
		// If the symlink does not exist, we'll soon create it.
		// This code only returns other, non-not-exist errors.
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

	return os.Symlink(deployment.String(), ActiveDeploymentSymlinkName)
}

func (mfd *MFD) Fetch(deployment Deployment) error {
	repo, err := git.PlainClone(deployment.String(), false, mfd.conf.CloneOptions())
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
		Hash: plumbing.NewHash(deployment.CommitHash),
	})
	if err != nil {
		return fmt.Errorf("error checking out commit %s: %w", deployment.CommitHash, err)
	}

	return nil
}

func (mfd *MFD) Build(deployment Deployment) error {
	for _, command := range mfd.conf.Build.Commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = deployment.String()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Println(command)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("error running build command %s: %w", command.String(), err)
		}
	}

	return nil
}

func (mfd *MFD) Restart() error {
	if mfd.conf.Systemd.Unit == "" {
		return nil
	}

	cmd := exec.Command("systemctl", "restart", mfd.conf.Systemd.Unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("systemctl restart %s\n", mfd.conf.Systemd.Unit)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error restarting systemd unit %s: %w", mfd.conf.Systemd.Unit, err)
	}

	return nil
}

func (mfd *MFD) Deploy(commitHash string) error {
	deployments, err := listDeployments()
	if err != nil {
		return err
	}

	deployment, err := findDeploymentByCommitHash(deployments, commitHash)
	if err == nil {
		return mfd.Activate(deployment)
	}

	if !errors.Is(err, ErrDeploymentNotFound) {
		return err
	}

	deployment = NewDeployment(time.Now(), commitHash)

	err = mfd.Fetch(deployment)
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

	err = mfd.Restart()
	if err != nil {
		return err
	}

	return nil
}

func (mfd *MFD) Resolve(revision string) (string, error) {
	// Perform an in-memory clone to find the expected commit / tag.
	repo, err := git.Clone(memory.NewStorage(), nil, mfd.conf.CloneOptions())
	if err != nil {
		return "", fmt.Errorf("error performing in-memory clone: %w", err)
	}

	// Resolve the revision to a commitHash hash.
	commitHash, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return "", fmt.Errorf("error resolving revision: %w", err)
	}

	return commitHash.String(), nil
}

func (mfd *MFD) Clean() error {
	activeDeployment, err := getActiveDeployment()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	deployments, err := listDeployments()
	if err != nil {
		return err
	}

	if len(deployments) <= KeepDeploymentsCount {
		return nil
	}

	deploymentsToRemove := deployments[KeepDeploymentsCount:]
	for _, deployment := range deploymentsToRemove {
		if deployment.String() == activeDeployment.String() {
			continue
		}

		err = os.RemoveAll(deployment.String())
		if err != nil {
			return err
		}
	}

	return nil
}

func (mfd *MFD) Rollback() error {
	activeDeployment, err := getActiveDeployment()
	if err != nil {
		return err
	}

	deployments, err := listDeployments()
	if err != nil {
		return err
	}

	// Find the index of the active deployment.
	activeIndex := slices.IndexFunc(deployments, func(deployment Deployment) bool {
		return deployment.String() == activeDeployment.String()
	})
	if activeIndex == -1 {
		return ErrDeploymentNotFound
	}

	prevIndex := activeIndex + 1
	if prevIndex >= len(deployments) {
		return errors.New("no previous deployment found")
	}

	prevDeployment := deployments[prevIndex]
	fmt.Printf("Rolling back to %s\n", prevDeployment.CommitHash)

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

		commitHash, err := mfd.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, commitHash)
		return mfd.Deploy(commitHash)
	case "resolve":
		revision := "HEAD"
		if len(args) > 1 {
			revision = args[1]
		}

		commitHash, err := mfd.Resolve(revision)
		if err != nil {
			return err
		}

		fmt.Printf("Resolved %s to %s\n", revision, commitHash)
		return nil
	case "restart":
		return mfd.Restart()
	case "clean":
		return mfd.Clean()
	default:
		return usage()
	}
}
