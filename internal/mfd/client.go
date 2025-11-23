package mfd

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
	activeDeploymentSymlinkName = "active"
	keepDeploymentsCount        = 3
)

var (
	ErrNoPreviousDeployment = errors.New("no previous deployment found")
)

func getActiveDeployment() (deployment.Deployment, error) {
	link, err := os.Readlink(activeDeploymentSymlinkName)
	if err != nil {
		return deployment.Deployment{}, err
	}

	dep, err := deployment.Parse(link)
	if err != nil {
		return deployment.Deployment{}, err
	}

	return dep, nil
}

type Client struct {
	cfg config.Config
}

func NewClient(cfg config.Config) Client {
	client := Client{
		cfg: cfg,
	}
	return client
}

func (c *Client) List() error {
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

func (c *Client) Deploy(commitHash string) error {
	deps, err := deployment.List()
	if err != nil {
		return err
	}

	dep, err := deployment.FindByCommitHash(deps, commitHash)
	if err == nil {
		// Deployment already exists, just activate and restart.
		err = c.activate(dep)
		if err != nil {
			return err
		}

		err = c.restart()
		if err != nil {
			return err
		}

		return nil
	}

	if !errors.Is(err, deployment.ErrNotFound) {
		return err
	}

	dep = deployment.New(time.Now(), commitHash)

	err = c.fetch(dep)
	if err != nil {
		return err
	}

	err = c.build(dep)
	if err != nil {
		return err
	}

	err = c.activate(dep)
	if err != nil {
		return err
	}

	err = c.restart()
	if err != nil {
		return err
	}

	err = c.clean()
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Resolve(revision string) (string, error) {
	// Perform an in-memory clone to find the expected commit / tag.
	repo, err := git.Clone(memory.NewStorage(), nil, c.cfg.CloneOptions())
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

func (c *Client) Rollback() error {
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
		return deployment.ErrNotFound
	}

	prevIndex := activeIndex + 1
	if prevIndex >= len(deps) {
		return ErrNoPreviousDeployment
	}

	prevDep := deps[prevIndex]
	fmt.Printf("Rolling back to %s\n", prevDep.CommitHash)

	err = c.activate(prevDep)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) fetch(dep deployment.Deployment) error {
	repo, err := git.PlainClone(dep.String(), false, c.cfg.CloneOptions())
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

func (c *Client) build(dep deployment.Deployment) error {
	for _, command := range c.cfg.Build.Commands {
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

func (c *Client) activate(dep deployment.Deployment) error {
	link, err := os.Lstat(activeDeploymentSymlinkName)
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
	return os.Symlink(dep.String(), activeDeploymentSymlinkName)
}

func (c *Client) restart() error {
	// If no systemd unit is configured, do nothing.
	if c.cfg.Systemd.Unit == "" {
		return nil
	}

	cmd := exec.Command("systemctl", "restart", c.cfg.Systemd.Unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Restarting: %s\n", c.cfg.Systemd.Unit)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error restarting systemd unit %s: %w", c.cfg.Systemd.Unit, err)
	}

	return nil

}

func (c *Client) clean() error {
	activeDeployment, err := getActiveDeployment()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	deps, err := deployment.List()
	if err != nil {
		return err
	}

	if len(deps) <= keepDeploymentsCount {
		return nil
	}

	deploymentsToRemove := deps[keepDeploymentsCount:]
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
