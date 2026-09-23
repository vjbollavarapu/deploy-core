package managedfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrAccessDenied is returned when a path is outside the allowed roots.
	ErrAccessDenied = errors.New("filesystem access denied: path outside allowed roots")
	// ErrPathTraversal is returned when path traversal sequences or null bytes are detected.
	ErrPathTraversal = errors.New("filesystem access denied: path traversal detected")
	// ErrSymlinkEscape is returned when a symlink resolves to a target outside the allowed root.
	ErrSymlinkEscape = errors.New("filesystem access denied: symlink escapes allowed root")
)

// AllowedRoots defines the canonical host directories the agent is permitted to touch.
type AllowedRoots struct {
	DataDir        string
	WorkspacesDir  string
	BackupsDir     string
	CredentialsDir string
	ConfigDir      string
}

// Slice returns all non-empty configured roots as clean absolute paths.
func (r AllowedRoots) Slice() []string {
	var roots []string
	add := func(p string) {
		if p != "" {
			roots = append(roots, filepath.Clean(p))
		}
	}
	add(r.DataDir)
	add(r.WorkspacesDir)
	add(r.BackupsDir)
	add(r.CredentialsDir)
	add(r.ConfigDir)
	return roots
}

// SafePathResolver provides strict path resolution within authorized roots.
type SafePathResolver struct {
	roots []string
}

// NewResolver constructs a SafePathResolver for the given allowed roots.
func NewResolver(roots AllowedRoots) *SafePathResolver {
	return &SafePathResolver{roots: roots.Slice()}
}

// NewResolverWithRoots constructs a SafePathResolver from a slice of allowed roots.
func NewResolverWithRoots(roots ...string) *SafePathResolver {
	var cleaned []string
	for _, r := range roots {
		if r != "" {
			cleaned = append(cleaned, filepath.Clean(r))
		}
	}
	return &SafePathResolver{roots: cleaned}
}

// Resolve validates and resolves userRelativePath strictly under the specified root.
func Resolve(root string, userRelativePath string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%w: root cannot be empty", ErrAccessDenied)
	}
	// Normalize any backslashes to standard forward slashes to prevent cross-platform traversal
	normalized := strings.ReplaceAll(userRelativePath, "\\", "/")
	if strings.ContainsRune(normalized, 0) {
		return "", ErrPathTraversal
	}

	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(normalized)

	var target string
	if filepath.IsAbs(cleanPath) {
		// If absolute, it must start with cleanRoot
		target = cleanPath
	} else {
		target = filepath.Join(cleanRoot, cleanPath)
	}

	// Double clean
	target = filepath.Clean(target)

	// Boundary check
	if !isSubpath(cleanRoot, target) {
		return "", fmt.Errorf("%w: %s is outside root %s", ErrPathTraversal, target, cleanRoot)
	}

	// Symlink escape check: evaluate symlinks for existing components
	if err := checkSymlinkEscape(cleanRoot, target); err != nil {
		return "", err
	}

	return target, nil
}

// ResolveUnderAny validates and resolves a path against any of the resolver's allowed roots.
func (r *SafePathResolver) ResolveUnderAny(targetPath string) (string, error) {
	if len(r.roots) == 0 {
		return "", fmt.Errorf("%w: no allowed roots configured", ErrAccessDenied)
	}
	if strings.ContainsRune(targetPath, 0) {
		return "", ErrPathTraversal
	}

	cleanTarget := filepath.Clean(strings.ReplaceAll(targetPath, "\\", "/"))

	for _, root := range r.roots {
		var candidate string
		if filepath.IsAbs(cleanTarget) {
			candidate = cleanTarget
		} else {
			candidate = filepath.Join(root, cleanTarget)
		}
		candidate = filepath.Clean(candidate)

		if isSubpath(root, candidate) {
			if err := checkSymlinkEscape(root, candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%w: path %s is not under any allowed root", ErrAccessDenied, targetPath)
}

// isSubpath checks if target is equal to or inside root.
func isSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// checkSymlinkEscape verifies that if any existing part of the target path is a symlink,
// it does not resolve to an external location outside root.
func checkSymlinkEscape(root, target string) error {
	// Find the deepest ancestor that exists
	curr := target
	for {
		fi, err := os.Lstat(curr)
		if err == nil {
			// Found existing path
			if fi.Mode()&os.ModeSymlink != 0 {
				eval, evalErr := filepath.EvalSymlinks(curr)
				if evalErr != nil {
					return fmt.Errorf("%w: %v", ErrSymlinkEscape, evalErr)
				}
				if !isSubpath(root, eval) {
					return fmt.Errorf("%w: symlink at %s points to %s outside %s", ErrSymlinkEscape, curr, eval, root)
				}
			}
			break
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return nil
}
