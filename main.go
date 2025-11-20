package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/theandrew168/mfd/internal/config"
	"github.com/theandrew168/mfd/internal/deployment"
)

const (
	ActiveDeploymentSymlinkName = "active"
	KeepDeploymentsCount        = 3
)

var (
	ErrDeploymentNotFound   = errors.New("deployment not found")
	ErrNoPreviousDeployment = errors.New("no previous deployment found")
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

func getActiveDeployment() (deployment.Deployment, error) {
	link, err := os.Readlink(ActiveDeploymentSymlinkName)
	if err != nil {
		return deployment.Deployment{}, err
	}

	dep, err := deployment.Parse(link)
	if err != nil {
		return deployment.Deployment{}, err
	}

	return dep, nil
}

type MFD struct {
	conf config.Config
}

func NewMFD(conf config.Config) MFD {
	mfd := MFD{
		conf: conf,
	}
	return mfd
}

func (mfd *MFD) List() error {
	deps, err := deployment.List()
	if err != nil {
		return err
	}

	activeDep, err := getActiveDeployment()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for _, dep := range deps {
		if dep.String() == activeDep.String() {
			fmt.Printf("%s (active)\n", dep.CommitHash)
		} else {
			fmt.Println(dep.CommitHash)
		}
	}

	return nil
}

func (mfd *MFD) Activate(dep deployment.Deployment) error {
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

	fmt.Printf("Activating deployment: %s\n", dep.CommitHash)
	return os.Symlink(dep.String(), ActiveDeploymentSymlinkName)
}

func (mfd *MFD) Fetch(dep deployment.Deployment) error {
	repo, err := git.PlainClone(dep.String(), false, mfd.conf.CloneOptions())
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
		Hash: plumbing.NewHash(dep.CommitHash),
	})
	if err != nil {
		return fmt.Errorf("error checking out commit %s: %w", dep.CommitHash, err)
	}

	return nil
}

func (mfd *MFD) Build(dep deployment.Deployment) error {
	for _, command := range mfd.conf.Build.Commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = dep.String()
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
	deps, err := deployment.List()
	if err != nil {
		return err
	}

	dep, err := deployment.FindByCommitHash(deps, commitHash)
	if err == nil {
		// Deployment already exists, just activate it and return.
		return mfd.Activate(dep)
	}

	if !errors.Is(err, ErrDeploymentNotFound) {
		return err
	}

	dep = deployment.New(time.Now(), commitHash)

	err = mfd.Fetch(dep)
	if err != nil {
		return err
	}

	err = mfd.Build(dep)
	if err != nil {
		return err
	}

	err = mfd.Activate(dep)
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

	deps, err := deployment.List()
	if err != nil {
		return err
	}

	if len(deps) <= KeepDeploymentsCount {
		return nil
	}

	deploymentsToRemove := deps[KeepDeploymentsCount:]
	for _, deployment := range deploymentsToRemove {
		if deployment.String() == activeDeployment.String() {
			continue
		}

		fmt.Printf("Removing deployment: %s\n", deployment.CommitHash)
		err = os.RemoveAll(deployment.String())
		if err != nil {
			return err
		}
	}

	return nil
}

func (mfd *MFD) Rollback() error {
	activeDep, err := getActiveDeployment()
	if err != nil {
		return err
	}

	deps, err := deployment.List()
	if err != nil {
		return err
	}

	// Find the index of the active deployment.
	activeIndex := slices.IndexFunc(deps, func(dep deployment.Deployment) bool {
		return dep.String() == activeDep.String()
	})
	if activeIndex == -1 {
		return ErrDeploymentNotFound
	}

	prevIndex := activeIndex + 1
	if prevIndex >= len(deps) {
		return ErrNoPreviousDeployment
	}

	prevDep := deps[prevIndex]
	fmt.Printf("Rolling back to %s\n", prevDep.CommitHash)

	err = mfd.Activate(prevDep)
	if err != nil {
		return err
	}

	return nil
}

func run() error {
	conf, err := config.ReadFile("mfd.toml")
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
	case "rollback":
		return mfd.Rollback()
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
