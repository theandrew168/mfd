package config_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/theandrew168/mfd/internal/config"
)

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

		[systemd]
		unit = "mfd"
	`, url, token, strings.Join(command, `", "`))

	config, err := config.Read(toml)
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
	if config.Systemd.Unit != "mfd" {
		t.Errorf("Expected systemd unit 'mfd', got '%s'", config.Systemd.Unit)
	}
}

func TestReadConfigRequired(t *testing.T) {
	t.Parallel()

	url := "https://example.com/repo.git"
	data := fmt.Sprintf(`
		[repo]
		url = "%s"
	`, url)

	_, err := config.Read(data)
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

	_, err := config.Read(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !errors.Is(err, config.ErrTokenAndPassword) {
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

	_, err := config.Read(data)
	if err == nil {
		t.Fatalf("Expected error reading config, got nil")
	}

	if !errors.Is(err, config.ErrMissingUsername) {
		t.Errorf("Expected error to be ErrMissingUsername, got '%v'", err)
	}
}
