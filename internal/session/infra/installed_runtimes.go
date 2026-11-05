package infra

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// phpBinary matches the side-by-side PHP binaries Debian and Ubuntu
	// install, e.g. php8.4, but not php-config8.4 or phpize8.4.
	phpBinary = regexp.MustCompile(`^php(\d+\.\d+)$`)
	// nvmVersion matches nvm's install directories, e.g. v20.20.2.
	nvmVersion = regexp.MustCompile(`^v(\d+\.\d+\.\d+)$`)
)

// InstalledRuntimes finds pinned toolchain versions on this machine: PHP
// installed side by side by the distribution (/usr/bin/php8.4), and
// Node.js installed by nvm (~/.nvm/versions/node/v20.20.2).
type InstalledRuntimes struct {
	phpDir  string
	nvmDir  string
	shimDir string
}

// NewInstalledRuntimes looks for phpX.Y binaries in phpDir and for nvm's
// Node.js installs in nvmDir. PHP has no per-version directory to put on
// PATH, so weaver links `php` to the pinned binary under shimDir.
func NewInstalledRuntimes(phpDir, nvmDir, shimDir string) InstalledRuntimes {
	return InstalledRuntimes{phpDir: phpDir, nvmDir: nvmDir, shimDir: shimDir}
}

// Resolve implements domain.RuntimeResolver: it returns the directory to
// put first on PATH for runtime.
func (r InstalledRuntimes) Resolve(runtime domain.Runtime) (string, error) {
	switch runtime.Kind() {
	case domain.PHP:
		return r.resolvePHP(runtime)
	case domain.Node:
		return r.resolveNode(runtime)
	}
	return "", fmt.Errorf("%w: %s", domain.ErrUnknownRuntime, runtime.Kind())
}

func (r InstalledRuntimes) resolvePHP(runtime domain.Runtime) (string, error) {
	versions, err := installedVersions(r.phpDir, phpBinary)
	if err != nil {
		return "", err
	}
	version, ok := newestMatching(versions, runtime.Version())
	if !ok {
		return "", notInstalled(runtime, versions)
	}
	dir := filepath.Join(r.shimDir, version)
	if err := link(filepath.Join(r.phpDir, "php"+version), filepath.Join(dir, "php")); err != nil {
		return "", fmt.Errorf("linking php %s: %w", version, err)
	}
	return dir, nil
}

func (r InstalledRuntimes) resolveNode(runtime domain.Runtime) (string, error) {
	installs := filepath.Join(r.nvmDir, "versions", "node")
	versions, err := installedVersions(installs, nvmVersion)
	if err != nil {
		return "", err
	}
	version, ok := newestMatching(versions, runtime.Version())
	if !ok {
		return "", notInstalled(runtime, versions)
	}
	bin := filepath.Join(installs, "v"+version, "bin")
	if _, err := os.Stat(filepath.Join(bin, "node")); err != nil {
		return "", fmt.Errorf("%w: %s (%v)", domain.ErrRuntimeNotInstalled, runtime, err)
	}
	return bin, nil
}

// installedVersions lists the versions captured by pattern from the
// entries of dir, oldest first. A missing dir has none.
func installedVersions(dir string, pattern *regexp.Regexp) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, entry := range entries {
		if match := pattern.FindStringSubmatch(entry.Name()); match != nil {
			versions = append(versions, match[1])
		}
	}
	slices.SortFunc(versions, compareVersions)
	return versions, nil
}

// newestMatching is the newest of versions starting with want at a dot
// boundary: "20" matches 20.20.2 but neither 2.0.0 nor 200.1.0.
func newestMatching(versions []string, want string) (string, bool) {
	for _, version := range slices.Backward(versions) {
		if version == want || strings.HasPrefix(version, want+".") {
			return version, true
		}
	}
	return "", false
}

// compareVersions orders dot-separated numeric versions.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(as), len(bs)) {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

func notInstalled(runtime domain.Runtime, versions []string) error {
	found := "none installed"
	if len(versions) > 0 {
		found = "installed: " + strings.Join(versions, ", ")
	}
	return fmt.Errorf("%w: %s (%s)", domain.ErrRuntimeNotInstalled, runtime, found)
}

// link makes path a symlink to target, replacing an older link
// atomically so a concurrent shell never sees it missing.
func link(target, path string) error {
	if current, err := os.Readlink(path); err == nil && current == target {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Available implements domain.RuntimeCatalog: PHP as installed (8.4),
// Node.js by major (20), which keeps a pin valid when nvm upgrades it.
func (r InstalledRuntimes) Available() ([]domain.Runtime, error) {
	nodes, err := installedVersions(filepath.Join(r.nvmDir, "versions", "node"), nvmVersion)
	if err != nil {
		return nil, err
	}
	var majors []string
	for _, version := range nodes {
		major, _, _ := strings.Cut(version, ".")
		if !slices.Contains(majors, major) {
			majors = append(majors, major)
		}
	}
	phps, err := installedVersions(r.phpDir, phpBinary)
	if err != nil {
		return nil, err
	}
	nodeRuntimes, err := runtimesOf(domain.Node, majors)
	if err != nil {
		return nil, err
	}
	phpRuntimes, err := runtimesOf(domain.PHP, phps)
	if err != nil {
		return nil, err
	}
	return append(nodeRuntimes, phpRuntimes...), nil
}

// runtimesOf pins each of versions of kind.
func runtimesOf(kind domain.RuntimeKind, versions []string) ([]domain.Runtime, error) {
	runtimes := make([]domain.Runtime, 0, len(versions))
	for _, version := range versions {
		runtime, err := domain.NewRuntime(string(kind), version)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, runtime)
	}
	return runtimes, nil
}
