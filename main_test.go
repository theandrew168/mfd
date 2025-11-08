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

		[build]
		commands = [
			["%s"],
		]

		[auth]
		token = "%s"
	`, url, strings.Join(command, `", "`), token)

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
	if config.Auth.Token != token {
		t.Errorf("Expected auth token %s, got %s", token, config.Auth.Token)
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

		[build]
		commands = [
			["true"],
		]

		[auth]
		token = "mytoken"
		password = "mypassword"
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

		[build]
		commands = [
			["true"],
		]

		[auth]
		password = "mypassword"
	`, url)

	_, err := readConfig(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !errors.Is(err, ErrMissingUsername) {
		t.Errorf("Expected error to be ErrMissingUsername, got '%v'", err)
	}
}

func TestFilterRelevantDirectories(t *testing.T) {
	t.Parallel()

	now := time.Now()
	files := []os.DirEntry{
		// Not a directory, should be ignored.
		NewFakeDirEntry("foo.txt", false, now),
		// Not a directory, should be ignored.
		NewFakeDirEntry("README.md", false, now),
		// Should be included.
		NewFakeDirEntry("src/", true, now),
		// Should be included.
		NewFakeDirEntry("testdata/", true, now),
		// Hidden directory, should be ignored.
		NewFakeDirEntry(".github/", true, now),
	}

	relevant := filterRelevantDirectories(files)
	if len(relevant) != 2 {
		t.Fatalf("Expected 2 relevant directories, got %d", len(relevant))
	}

	expected := []string{"src/", "testdata/"}
	for _, entry := range relevant {
		if !slices.Contains(expected, entry.Name()) {
			t.Errorf("Unexpected directory: %s", entry.Name())
		}
	}
}

func TestSortFilesNewestToOldest(t *testing.T) {
	t.Parallel()

	now := time.Now()
	files := []os.DirEntry{
		// Current (should be second).
		NewFakeDirEntry("file1.txt", false, now),
		// Newer (should be first).
		NewFakeDirEntry("file2.txt", false, now.AddDate(0, 0, 1)),
		// Oldest (should be third).
		NewFakeDirEntry("file3.txt", false, now.AddDate(0, 0, -1)),
	}

	sorted := sortFilesNewestToOldest(files)
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(sorted))
	}

	if sorted[0].Name() != "file2.txt" {
		t.Errorf("First file is incorrect: got %s, want %s", sorted[0].Name(), "file2.txt")
	}
	if sorted[1].Name() != "file1.txt" {
		t.Errorf("Second file is incorrect: got %s, want %s", sorted[1].Name(), "file1.txt")
	}
	if sorted[2].Name() != "file3.txt" {
		t.Errorf("Third file is incorrect: got %s, want %s", sorted[2].Name(), "file3.txt")
	}
}
