package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

var _ os.DirEntry = (*fakeDirEntry)(nil)
var _ os.FileInfo = (*fakeDirEntry)(nil)

type fakeDirEntry struct {
	name    string
	isDir   bool
	modTime time.Time
}

func NewFakeDirEntry(name string, isDir bool, modTime time.Time) *fakeDirEntry {
	fde := fakeDirEntry{
		name:    name,
		isDir:   isDir,
		modTime: modTime,
	}
	return &fde
}

func (fde *fakeDirEntry) Name() string {
	return fde.name
}

func (fde *fakeDirEntry) Size() int64 {
	return 0
}

func (fde *fakeDirEntry) IsDir() bool {
	return fde.isDir
}

func (fde *fakeDirEntry) Type() os.FileMode {
	return 0
}

func (fde *fakeDirEntry) Mode() os.FileMode {
	return 0
}

func (fde *fakeDirEntry) ModTime() time.Time {
	return fde.modTime
}

func (fde *fakeDirEntry) Info() (os.FileInfo, error) {
	return fde, nil
}

func (fde *fakeDirEntry) Sys() any {
	return nil
}

func TestReadConfig(t *testing.T) {
	t.Parallel()

	url := "https://example.com/repo.git"
	command := strings.Split("go build -o mfd .", " ")
	token := "mytoken"
	toml := fmt.Sprintf(`
		[repo]
		url = "%s"
		token = "%s"

		[build]
		commands = [
			["%s"],
		]
	`, url, token, strings.Join(command, `", "`))

	config, err := readConfig(toml)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	if config.Repo.URL != url {
		t.Errorf("Expected repo URL %s, got %s", url, config.Repo.URL)
	}
	if len(config.Build.Commands) != 1 {
		t.Fatalf("Expected 1 build command, got %d", len(config.Build.Commands))
	}
	if !slices.Equal(config.Build.Commands[0], command) {
		t.Errorf("Expected build command %s, got %s", command, config.Build.Commands[0])
	}
	if config.Repo.Token != token {
		t.Errorf("Expected auth token %s, got %s", token, config.Repo.Token)
	}
}

func TestReadConfigRequired(t *testing.T) {
	t.Parallel()

	url := "https://example.com/repo.git"
	data := fmt.Sprintf(`
		[repo]
		url = "%s"
	`, url)

	_, err := readConfig(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !strings.Contains(err.Error(), "missing config values") {
		t.Errorf("Expected error to mention 'missing config values', got '%s'", err.Error())
	}

	if !strings.Contains(err.Error(), "build.commands") {
		t.Errorf("Expected error to mention 'build.commands', got '%s'", err.Error())
	}
}

func TestReadConfigTokenAndPassword(t *testing.T) {
	t.Parallel()

	url := "https://example.com/repo.git"
	data := fmt.Sprintf(`
		[repo]
		url = "%s"
		token = "mytoken"
		password = "mypassword"

		[build]
		commands = [
			["true"],
		]
	`, url)

	_, err := readConfig(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !errors.Is(err, ErrTokenAndPassword) {
		t.Errorf("Expected error to be ErrTokenAndPassword, got '%v'", err)
	}
}

func TestReadConfigMissingUsername(t *testing.T) {
	t.Parallel()

	url := "https://example.com/repo.git"
	data := fmt.Sprintf(`
		[repo]
		url = "%s"
		password = "mypassword"

		[build]
		commands = [
			["true"],
		]
	`, url)

	_, err := readConfig(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !errors.Is(err, ErrMissingUsername) {
		t.Errorf("Expected error to be ErrMissingUsername, got '%v'", err)
	}
}

func TestFilesToDeployments(t *testing.T) {
	t.Parallel()

	now := time.Now()
	files := []os.DirEntry{
		// Not a directory, should be ignored.
		NewFakeDirEntry("mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", false, now),
		// Invalid deployment directory, should be ignored.
		NewFakeDirEntry("testdata", true, now),
		// Valid deployment directory, should be included.
		NewFakeDirEntry("mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", true, now),
	}

	deployments := filesToDeployments(files)
	if len(deployments) != 1 {
		t.Fatalf("Expected 1 relevant directory, got %d", len(deployments))
	}

	if deployments[0].CommitHash != "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3" {
		t.Errorf("Unexpected commit hash: %s", deployments[0].CommitHash)
	}
}

func TestDeploymentString(t *testing.T) {
	t.Parallel()

	deployment := NewDeployment(
		time.Unix(1625079600, 0),
		"a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
	)

	expected := "mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	if deployment.String() != expected {
		t.Errorf("Expected deployment string '%s', got '%s'", expected, deployment.String())
	}
}

func TestParseDeployment(t *testing.T) {
	t.Parallel()

	dirName := "mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	deployment, err := ParseDeployment(dirName)
	if err != nil {
		t.Fatalf("Failed to parse deployment: %v", err)
	}

	expectedCreatedAt := time.Unix(1625079600, 0)
	if !deployment.CreatedAt.Equal(expectedCreatedAt) {
		t.Errorf("Expected created at %v, got %v", expectedCreatedAt, deployment.CreatedAt)
	}

	expectedCommitHash := "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	if deployment.CommitHash != expectedCommitHash {
		t.Errorf("Expected commit hash %s, got %s", expectedCommitHash, deployment.CommitHash)
	}
}

func TestParseDeploymentInvalid(t *testing.T) {
	t.Parallel()

	invalidDirNames := []string{
		// Missing commit hash
		"mfd_1625079600",
		// Invalid timestamp
		"mfd_invalidtimestamp_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		// Invalid commit hash
		"mfd_1625079600_invalidhash",
		// Invalid prefix
		"invalidprefix_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
	}

	for _, dirName := range invalidDirNames {
		_, err := ParseDeployment(dirName)
		if !errors.Is(err, ErrInvalidDeployment) {
			t.Errorf("Expected error parsing invalid directory name '%s', got nil", dirName)
		}
	}
}

func TestSortDeploymentsNewestToOldest(t *testing.T) {
	t.Parallel()

	deployments := []Deployment{
		{CreatedAt: time.Unix(2, 0)},
		{CreatedAt: time.Unix(1, 0)},
		{CreatedAt: time.Unix(3, 0)},
	}

	sorted := sortDeploymentsNewestToOldest(deployments)

	expectedOrder := []time.Time{
		time.Unix(3, 0),
		time.Unix(2, 0),
		time.Unix(1, 0),
	}

	for i, deployment := range sorted {
		if !deployment.CreatedAt.Equal(expectedOrder[i]) {
			t.Errorf("At index %d, expected time %v, got %v", i, expectedOrder[i], deployment.CreatedAt)
		}
	}
}
