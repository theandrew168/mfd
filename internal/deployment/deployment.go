package deployment

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("deployment not found")
	ErrInvalid  = errors.New("invalid deployment")
)

type Deployment struct {
	CreatedAt  time.Time
	CommitHash string
}

func New(createdAt time.Time, commitHash string) Deployment {
	return Deployment{
		CreatedAt:  createdAt,
		CommitHash: commitHash,
	}
}

func Parse(s string) (Deployment, error) {
	parts := strings.Split(s, "_")
	if len(parts) != 3 {
		return Deployment{}, ErrInvalid
	}

	if parts[0] != "mfd" {
		return Deployment{}, ErrInvalid
	}

	t, err := strconv.Atoi(parts[1])
	if err != nil {
		return Deployment{}, ErrInvalid
	}

	// Validate that the hash is a 40-character SHA1 hash.
	if len(parts[2]) != 40 {
		return Deployment{}, ErrInvalid
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

func SortNewestToOldest(deployments []Deployment) []Deployment {
	sorted := slices.Clone(deployments)
	slices.SortFunc(sorted, func(a, b Deployment) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return sorted
}

func FromFiles(files []os.DirEntry) []Deployment {
	var deployments []Deployment

	for _, file := range files {
		name := file.Name()

		// Ignore anythhing that is not a directory.
		if !file.IsDir() {
			continue
		}

		deployment, err := Parse(name)
		if err != nil {
			continue
		}

		deployments = append(deployments, deployment)
	}

	return deployments
}

func List() ([]Deployment, error) {
	files, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	deployments := FromFiles(files)
	return SortNewestToOldest(deployments), nil
}

func FindByCommitHash(deployments []Deployment, commitHash string) (Deployment, error) {
	for _, deployment := range deployments {
		if deployment.CommitHash == commitHash {
			return deployment, nil
		}
	}

	return Deployment{}, ErrNotFound
}
