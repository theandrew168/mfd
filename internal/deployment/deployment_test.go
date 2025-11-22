package deployment_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/theandrew168/mfd/internal/deployment"
)

var _ os.DirEntry = (*fakeDirEntry)(nil)
var _ os.FileInfo = (*fakeDirEntry)(nil)

type fakeDirEntry struct {
	name    string
	isDir   bool
	modTime time.Time
}

func newFakeDirEntry(name string, isDir bool, modTime time.Time) *fakeDirEntry {
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
func TestFromFiles(t *testing.T) {
	t.Parallel()

	now := time.Now()
	files := []os.DirEntry{
		// Not a directory, should be ignored.
		newFakeDirEntry("mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", false, now),
		// Invalid deployment directory, should be ignored.
		newFakeDirEntry("testdata", true, now),
		// Valid deployment directory, should be included.
		newFakeDirEntry("mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3", true, now),
	}

	deps := deployment.FromFiles(files)
	if len(deps) != 1 {
		t.Fatalf("Expected 1 relevant directory, got %d", len(deps))
	}

	if deps[0].CommitHash != "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3" {
		t.Errorf("Unexpected commit hash: %s", deps[0].CommitHash)
	}
}

func TestString(t *testing.T) {
	t.Parallel()

	dep := deployment.New(
		time.Unix(1625079600, 0),
		"a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
	)

	expected := "mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	if dep.String() != expected {
		t.Errorf("Expected deployment string '%s', got '%s'", expected, dep.String())
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	dirName := "mfd_1625079600_a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	dep, err := deployment.Parse(dirName)
	if err != nil {
		t.Fatalf("Failed to parse deployment: %v", err)
	}

	expectedCreatedAt := time.Unix(1625079600, 0)
	if !dep.CreatedAt.Equal(expectedCreatedAt) {
		t.Errorf("Expected created at %v, got %v", expectedCreatedAt, dep.CreatedAt)
	}

	expectedCommitHash := "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
	if dep.CommitHash != expectedCommitHash {
		t.Errorf("Expected commit hash %s, got %s", expectedCommitHash, dep.CommitHash)
	}
}

func TestParseInvalid(t *testing.T) {
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
		_, err := deployment.Parse(dirName)
		if !errors.Is(err, deployment.ErrInvalid) {
			t.Errorf("Expected error parsing invalid directory name '%s', got nil", dirName)
		}
	}
}

func TestSortNewestToOldest(t *testing.T) {
	t.Parallel()

	deps := []deployment.Deployment{
		{CreatedAt: time.Unix(2, 0)},
		{CreatedAt: time.Unix(1, 0)},
		{CreatedAt: time.Unix(3, 0)},
	}

	sorted := deployment.SortNewestToOldest(deps)

	expectedOrder := []time.Time{
		time.Unix(3, 0),
		time.Unix(2, 0),
		time.Unix(1, 0),
	}

	for i, dep := range sorted {
		if !dep.CreatedAt.Equal(expectedOrder[i]) {
			t.Errorf("At index %d, expected time %v, got %v", i, expectedOrder[i], dep.CreatedAt)
		}
	}
}
