package infra

import (
	"fmt"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// The forms' multi-line text fields, read into domain values.

// parseEnv reads "NAME=value" lines, skipping blank ones. Spaces around
// the name and the value are dropped, so "FOO = bar" sets FOO to "bar".
func parseEnv(text string) (domain.Env, error) {
	vars := make(map[string]string)
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return domain.Env{}, fmt.Errorf("%q: want NAME=value", line)
		}
		vars[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return domain.NewEnv(vars)
}

// parseCheckouts reads "dir = remote" lines, skipping blank ones.
func parseCheckouts(text string) ([]domain.Checkout, error) {
	var checkouts []domain.Checkout
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir, remote, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%q: want directory = remote", line)
		}
		checkout, err := domain.NewCheckout(strings.TrimSpace(dir), strings.TrimSpace(remote))
		if err != nil {
			return nil, err
		}
		checkouts = append(checkouts, checkout)
	}
	return checkouts, nil
}
