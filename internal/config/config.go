package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

var (
	ErrMissingUsername  = errors.New("username must be specified when using password authentication")
	ErrTokenAndPassword = errors.New("cannot specify both password and token for authentication")
)

type Command []string

func (c Command) String() string {
	return strings.Join(c, " ")
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
func Read(data string) (Config, error) {
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

func ReadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	return Read(string(data))
}
