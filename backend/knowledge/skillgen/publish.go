package skillgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layout of the published skill store (under the app data dir):
//
//	<base>/versions/<version>/<skill-name>/SKILL.md ...
//	<base>/current -> versions/<version>/<skill-name>   (symlink, swapped atomically)
//
// Agent-facing targets (e.g. ~/.claude/skills/<skill-name>) are symlinks to
// <base>/current, so they always resolve to the latest valid version without
// being rewritten.

// PublishResult describes what Publish did.
type PublishResult struct {
	VersionDir string   `json:"version_dir"`
	Current    string   `json:"current"`
	Changed    bool     `json:"changed"` // false = this version was already current
	Targets    []string `json:"targets"` // agent-facing links in place
	Warnings   []string `json:"warnings"`
	Pruned     []string `json:"pruned"`
}

// CurrentVersion returns the version that <base>/current points to ("" if none).
func CurrentVersion(base string) string {
	b, err := os.ReadFile(filepath.Join(base, "current", "metadata.json"))
	if err != nil {
		return ""
	}
	var m metaDoc
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.Version
}

// CurrentDir returns the resolved directory of the current version.
func CurrentDir(base string) string {
	p, err := filepath.EvalSymlinks(filepath.Join(base, "current"))
	if err != nil {
		return ""
	}
	return p
}

// Publish writes out (already validated) as a new immutable version and
// atomically points <base>/current at it. If anything fails before the swap,
// the previous version stays current.
func Publish(base, skillName string, out Output, targets []string, keep int) (PublishResult, error) {
	res := PublishResult{Current: filepath.Join(base, "current")}
	if errs := Validate(out); len(errs) > 0 {
		return res, fmt.Errorf("refusing to publish invalid skill: %s", strings.Join(errs, "; "))
	}
	versions := filepath.Join(base, "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return res, err
	}
	finalVersion := filepath.Join(versions, out.Version)
	res.VersionDir = filepath.Join(finalVersion, skillName)

	if !sameContent(res.VersionDir, out) {
		tmp, err := os.MkdirTemp(versions, ".tmp-"+out.Version+"-")
		if err != nil {
			return res, err
		}
		cleanup := true
		defer func() {
			if cleanup {
				os.RemoveAll(tmp)
			}
		}()
		for _, f := range out.Files {
			if err := writeFileSync(filepath.Join(tmp, skillName, filepath.FromSlash(f.Path)), f.Content); err != nil {
				return res, err
			}
		}
		// A stale/corrupt dir with the same version name is replaced.
		if err := os.RemoveAll(finalVersion); err != nil {
			return res, err
		}
		if err := os.Rename(tmp, finalVersion); err != nil {
			return res, err
		}
		cleanup = false
		syncDir(versions)
	}

	rel := filepath.Join("versions", out.Version, skillName)
	if old, _ := os.Readlink(res.Current); old != rel {
		if err := atomicSymlink(rel, res.Current); err != nil {
			return res, fmt.Errorf("switch current version: %w", err)
		}
		res.Changed = true
	}

	for _, t := range targets {
		link, warn, err := linkTarget(t, skillName, res.Current)
		if err != nil {
			res.Warnings = append(res.Warnings, err.Error())
			continue
		}
		if warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
		if link != "" {
			res.Targets = append(res.Targets, link)
		}
	}
	res.Pruned = prune(versions, out.Version, keep)
	return res, nil
}

func sameContent(dir string, out Output) bool {
	for _, f := range out.Files {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil || string(b) != string(f.Content) {
			return false
		}
	}
	return true
}

func writeFileSync(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// atomicSymlink creates/replaces link -> target via rename (atomic on POSIX).
func atomicSymlink(target, link string) error {
	tmp := link + ".tmp"
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(filepath.Dir(link))
	return nil
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// linkTarget makes <parent>/<skillName> a symlink to current. A real
// directory or file already at that path is never overwritten.
func linkTarget(parent, skillName, current string) (string, string, error) {
	parent = ExpandHome(strings.TrimSpace(parent))
	if parent == "" {
		return "", "", nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", fmt.Errorf("publish target %s: %w", parent, err)
	}
	link := filepath.Join(parent, skillName)
	absCurrent, _ := filepath.Abs(current)
	fi, err := os.Lstat(link)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.Symlink(absCurrent, link); err != nil {
			return "", "", fmt.Errorf("publish target %s: %w", link, err)
		}
		return link, "", nil
	case err != nil:
		return "", "", fmt.Errorf("publish target %s: %w", link, err)
	case fi.Mode()&os.ModeSymlink != 0:
		if dst, _ := os.Readlink(link); dst != absCurrent {
			if err := atomicSymlink(absCurrent, link); err != nil {
				return "", "", fmt.Errorf("publish target %s: %w", link, err)
			}
		}
		return link, "", nil
	default:
		return "", fmt.Sprintf("publish target %s exists and is not a symlink managed by review-dock; left untouched (remove it to let review-dock manage it)", link), nil
	}
}

// prune keeps the newest `keep` version directories (by mtime) and always
// keeps the current one.
func prune(versions, current string, keep int) []string {
	if keep < 2 {
		keep = 2
	}
	entries, err := os.ReadDir(versions)
	if err != nil {
		return nil
	}
	type v struct {
		name string
		mod  int64
	}
	var all []v
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, v{e.Name(), info.ModTime().UnixNano()})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].mod != all[j].mod {
			return all[i].mod > all[j].mod
		}
		return all[i].name < all[j].name
	})
	var pruned []string
	kept := 0
	for _, x := range all {
		if x.name == current || kept < keep {
			kept++
			continue
		}
		if os.RemoveAll(filepath.Join(versions, x.name)) == nil {
			pruned = append(pruned, x.name)
		}
	}
	return pruned
}
