package workspace

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

var (
	// deploymentIDRE ensures deployment IDs are alphanumeric slugs/UUIDs without path characters.
	deploymentIDRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_]{0,127}$`)

	ErrInvalidDeploymentID = errors.New("invalid deployment ID")
	ErrPathTraversal       = errors.New("path traversal outside approved workspace")
	ErrSymlinkEscape       = errors.New("symlink target escapes approved workspace")
	ErrDiskSpaceLow        = errors.New("insufficient disk space for build")
	ErrContextTooLarge     = errors.New("build context exceeds maximum allowed size")
	ErrArchiveTooLarge     = errors.New("source archive exceeds maximum allowed size")
	ErrTooManyFiles        = errors.New("source archive exceeds maximum allowed file count")
	ErrUnsupportedArchive  = errors.New("unsupported archive format")
)

const (
	// DefaultMinFreeDiskBytes requires at least 500 MB of free disk space before building.
	DefaultMinFreeDiskBytes uint64 = 500 * 1024 * 1024
	// DefaultMaxContextBytes limits the build context archive to 500 MB.
	DefaultMaxContextBytes int64 = 500 * 1024 * 1024
	// DefaultMaxArchiveBytes limits compressed source archives to 200 MB.
	DefaultMaxArchiveBytes int64 = 200 * 1024 * 1024
	// DefaultMaxExtractedBytes limits total uncompressed workspace size to 500 MB.
	DefaultMaxExtractedBytes int64 = 500 * 1024 * 1024
	// DefaultMaxFileCount limits maximum number of files in an archive to 20,000.
	DefaultMaxFileCount int = 20000
	// DefaultRetentionTTL defines the default temporary debug retention period.
	DefaultRetentionTTL = 2 * time.Hour
)

// RetentionPolicy defines how long a workspace is preserved after build execution.
type RetentionPolicy string

const (
	RetentionCleanAlways    RetentionPolicy = "clean_always"
	RetentionCleanOnSuccess RetentionPolicy = "clean_on_success"
	RetentionCleanOnFailure RetentionPolicy = "clean_on_failure"
	RetentionRetain         RetentionPolicy = "retain"
)

// WorkspaceMetadata stores lifecycle and retention details for a workspace.
type WorkspaceMetadata struct {
	DeploymentID    string          `json:"deploymentId"`
	CreatedAt       time.Time       `json:"createdAt"`
	RetentionPolicy RetentionPolicy `json:"retentionPolicy"`
	ExpiresAt       time.Time       `json:"expiresAt"`
}

// ExtractOptions specifies security ceilings for source archive extraction.
type ExtractOptions struct {
	MaxArchiveBytes   int64
	MaxExtractedBytes int64
	MaxFileCount      int
	StripComponents   int
}

// FileInfo represents sanitized file metadata returned by ListFiles.
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"isDir"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

// Manager manages deployment build workspaces under an approved root directory.
type Manager struct {
	workspacesRoot string
}

// NewManager creates a new workspace manager.
func NewManager(workspacesRoot string) *Manager {
	return &Manager{workspacesRoot: filepath.Clean(workspacesRoot)}
}

// Workspace represents an isolated build workspace directory for a specific deployment.
type Workspace struct {
	DeploymentID string
	Dir          string // Absolute path: <root>/<deploymentID>
}

// Create creates an isolated workspace directory with strict 0700 permissions.
func (m *Manager) Create(deploymentID string) (*Workspace, error) {
	cleanID := strings.TrimSpace(deploymentID)
	if !deploymentIDRE.MatchString(cleanID) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDeploymentID, deploymentID)
	}

	wsDir := filepath.Join(m.workspacesRoot, cleanID)

	if err := os.MkdirAll(m.workspacesRoot, 0700); err != nil {
		return nil, fmt.Errorf("failed to create workspaces root: %w", err)
	}

	if err := os.MkdirAll(wsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create deployment workspace %q: %w", wsDir, err)
	}

	ws := &Workspace{
		DeploymentID: cleanID,
		Dir:          wsDir,
	}

	// Record initial metadata
	_ = ws.SaveMetadata(RetentionCleanAlways, DefaultRetentionTTL)
	return ws, nil
}

// Get retrieves an existing workspace if it exists.
func (m *Manager) Get(deploymentID string) (*Workspace, error) {
	cleanID := strings.TrimSpace(deploymentID)
	if !deploymentIDRE.MatchString(cleanID) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDeploymentID, deploymentID)
	}

	wsDir := filepath.Join(m.workspacesRoot, cleanID)
	stat, err := os.Stat(wsDir)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("workspace path %q is not a directory", wsDir)
	}

	return &Workspace{
		DeploymentID: cleanID,
		Dir:          wsDir,
	}, nil
}

// Prune cleans up expired or aged workspaces across the workspaces root.
func (m *Manager) Prune(maxAge time.Duration) (int, error) {
	if m == nil {
		return 0, nil
	}
	entries, err := os.ReadDir(m.workspacesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	now := time.Now().UTC()
	removed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wsDir := filepath.Join(m.workspacesRoot, entry.Name())
		metaPath := filepath.Join(wsDir, ".deploycore-workspace.json")

		shouldDelete := false
		if metaBytes, err := os.ReadFile(metaPath); err == nil {
			var meta WorkspaceMetadata
			if err := json.Unmarshal(metaBytes, &meta); err == nil {
				if meta.RetentionPolicy != RetentionRetain || (!meta.ExpiresAt.IsZero() && now.After(meta.ExpiresAt)) {
					shouldDelete = true
				}
			}
		} else if maxAge > 0 {
			if fi, err := entry.Info(); err == nil {
				if now.Sub(fi.ModTime()) > maxAge {
					shouldDelete = true
				}
			}
		}

		if shouldDelete {
			if err := os.RemoveAll(wsDir); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}

// ResolvePath resolves a path relative to the workspace, strictly preventing
// path traversal outside the approved workspace root.
func (w *Workspace) ResolvePath(subpath string) (string, error) {
	cleanedSub := filepath.Clean(filepath.FromSlash(strings.TrimSpace(subpath)))

	// Disallow absolute paths that don't match the workspace dir
	if filepath.IsAbs(cleanedSub) {
		rel, err := filepath.Rel(w.Dir, cleanedSub)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("%w: absolute path %q is outside workspace %q", ErrPathTraversal, subpath, w.Dir)
		}
		return cleanedSub, nil
	}

	// Reject leading separator or traversal indicators
	if strings.HasPrefix(cleanedSub, "..") || (cleanedSub == "." && strings.Contains(subpath, "..")) {
		return "", fmt.Errorf("%w: relative path %q traverses outside workspace", ErrPathTraversal, subpath)
	}

	target := filepath.Join(w.Dir, cleanedSub)
	rel, err := filepath.Rel(w.Dir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: path %q escapes workspace boundary", ErrPathTraversal, subpath)
	}

	return target, nil
}

// CheckDiskSpace checks whether the volume hosting the workspace has sufficient free bytes.
func (w *Workspace) CheckDiskSpace(minFree uint64) error {
	usage, err := disk.Usage(w.Dir)
	if err != nil {
		return nil
	}

	if usage.Free < minFree {
		return fmt.Errorf("%w: %d bytes free, %d bytes required", ErrDiskSpaceLow, usage.Free, minFree)
	}
	return nil
}

// MaterializeFiles writes a map of relative_path -> content into the workspace.
func (w *Workspace) MaterializeFiles(files map[string]string) error {
	for relPath, content := range files {
		absPath, err := w.ResolvePath(relPath)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
			return fmt.Errorf("failed to create directory for %q: %w", relPath, err)
		}

		if err := os.WriteFile(absPath, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to materialize file %q: %w", relPath, err)
		}
	}
	return nil
}

// ExtractArchive extracts a tar, tar.gz, or zip stream safely into the workspace.
// It verifies every path against path traversal, symlink escapes, max archive size,
// max extracted size, and maximum file count.
func (w *Workspace) ExtractArchive(r io.Reader, format string, opts ExtractOptions) error {
	maxArchive := opts.MaxArchiveBytes
	if maxArchive <= 0 {
		maxArchive = DefaultMaxArchiveBytes
	}
	maxExtracted := opts.MaxExtractedBytes
	if maxExtracted <= 0 {
		maxExtracted = DefaultMaxExtractedBytes
	}
	maxFiles := opts.MaxFileCount
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFileCount
	}

	// Read bounded amount to auto-detect or verify archive header
	br := bufio.NewReader(io.LimitReader(r, maxArchive))
	peekHeader, err := br.Peek(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read archive header: %w", err)
	}

	// Detect format
	isGzip := len(peekHeader) >= 2 && peekHeader[0] == 0x1f && peekHeader[1] == 0x8b
	isZip := len(peekHeader) >= 4 && peekHeader[0] == 0x50 && peekHeader[1] == 0x4b

	if format == "zip" || isZip {
		return w.extractZip(br, maxExtracted, maxFiles, opts.StripComponents)
	}

	var tr *tar.Reader
	if format == "tar.gz" || format == "tgz" || isGzip {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	} else {
		tr = tar.NewReader(br)
	}

	return w.extractTar(tr, maxExtracted, maxFiles, opts.StripComponents)
}

func (w *Workspace) extractTar(tr *tar.Reader, maxExtracted int64, maxFiles int, stripComponents int) error {
	var totalExtracted int64
	var fileCount int

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed reading tar entry: %w", err)
		}

		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("%w: count %d exceeds %d limit", ErrTooManyFiles, fileCount, maxFiles)
		}

		cleanRel := stripPathComponents(hdr.Name, stripComponents)
		if cleanRel == "" || cleanRel == "." {
			continue
		}

		// Strictly reject any traversal in entry path
		targetPath, err := w.ResolvePath(cleanRel)
		if err != nil {
			return fmt.Errorf("%w: tar entry %q", err, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0700); err != nil {
				return err
			}

		case tar.TypeReg, tar.TypeRegA:
			totalExtracted += hdr.Size
			if totalExtracted > maxExtracted {
				return fmt.Errorf("%w: total size %d bytes exceeds %d limit", ErrContextTooLarge, totalExtracted, maxExtracted)
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
				return err
			}

			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}

			if _, err := io.Copy(out, io.LimitReader(tr, hdr.Size)); err != nil {
				out.Close()
				return err
			}
			out.Close()

		case tar.TypeSymlink:
			// Strictly validate that the symlink target remains within the workspace
			linkTarget := hdr.Linkname
			resolvedTarget := linkTarget
			if !filepath.IsAbs(resolvedTarget) {
				resolvedTarget = filepath.Join(filepath.Dir(targetPath), linkTarget)
			}
			resolvedTarget = filepath.Clean(resolvedTarget)

			rel, err := filepath.Rel(w.Dir, resolvedTarget)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("%w: symlink %q -> %q points outside workspace", ErrSymlinkEscape, hdr.Name, linkTarget)
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
				return err
			}
			_ = os.Remove(targetPath)
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				return err
			}

		case tar.TypeLink:
			// Hard link verification
			linkTarget := hdr.Linkname
			targetLinkPath, err := w.ResolvePath(linkTarget)
			if err != nil {
				return fmt.Errorf("%w: hard link %q -> %q", ErrPathTraversal, hdr.Name, linkTarget)
			}
			_ = os.Remove(targetPath)
			if err := os.Link(targetLinkPath, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Workspace) extractZip(r io.Reader, maxExtracted int64, maxFiles int, stripComponents int) error {
	// Read entire stream into memory buffer bounded by maxExtracted
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r); err != nil {
		return fmt.Errorf("failed reading zip stream: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return fmt.Errorf("invalid zip archive: %w", err)
	}

	var totalExtracted int64
	var fileCount int

	for _, f := range zr.File {
		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("%w: count %d exceeds %d limit", ErrTooManyFiles, fileCount, maxFiles)
		}

		cleanRel := stripPathComponents(f.Name, stripComponents)
		if cleanRel == "" || cleanRel == "." {
			continue
		}

		targetPath, err := w.ResolvePath(cleanRel)
		if err != nil {
			return fmt.Errorf("%w: zip entry %q", err, f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0700); err != nil {
				return err
			}
			continue
		}

		totalExtracted += int64(f.UncompressedSize64)
		if totalExtracted > maxExtracted {
			return fmt.Errorf("%w: total size %d bytes exceeds %d limit", ErrContextTooLarge, totalExtracted, maxExtracted)
		}

		// Symlink handling in zip
		if f.Mode()&os.ModeSymlink != 0 {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			targetBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			linkTarget := string(targetBytes)
			resolvedTarget := linkTarget
			if !filepath.IsAbs(resolvedTarget) {
				resolvedTarget = filepath.Join(filepath.Dir(targetPath), linkTarget)
			}
			resolvedTarget = filepath.Clean(resolvedTarget)

			rel, err := filepath.Rel(w.Dir, resolvedTarget)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("%w: zip symlink %q -> %q points outside workspace", ErrSymlinkEscape, f.Name, linkTarget)
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
				return err
			}
			_ = os.Remove(targetPath)
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}

	return nil
}

// ListFiles returns sanitized file information inside the workspace, strictly preventing
// arbitrary filesystem browsing.
func (w *Workspace) ListFiles(subpath string) ([]FileInfo, error) {
	resolvedDir, err := w.ResolvePath(subpath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return nil, err
	}

	result := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(resolvedDir, entry.Name())
		rel, err := filepath.Rel(w.Dir, fullPath)
		if err != nil {
			continue
		}

		result = append(result, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.ToSlash(rel),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime(),
		})
	}

	return result, nil
}

// SaveMetadata writes retention metadata to the workspace directory.
func (w *Workspace) SaveMetadata(policy RetentionPolicy, ttl time.Duration) error {
	now := time.Now().UTC()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	meta := WorkspaceMetadata{
		DeploymentID:    w.DeploymentID,
		CreatedAt:       now,
		RetentionPolicy: policy,
		ExpiresAt:       expiresAt,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	metaPath := filepath.Join(w.Dir, ".deploycore-workspace.json")
	return os.WriteFile(metaPath, data, 0600)
}

// ReadMetadata reads the saved retention metadata for the workspace.
func (w *Workspace) ReadMetadata() (*WorkspaceMetadata, error) {
	metaPath := filepath.Join(w.Dir, ".deploycore-workspace.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta WorkspaceMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// PackageContext packages the specified context directory into an in-memory tar archive.
func (w *Workspace) PackageContext(contextSubpath string, maxBytes int64) (io.Reader, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxContextBytes
	}

	contextDir, err := w.ResolvePath(contextSubpath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(contextDir)
	if err != nil {
		return nil, fmt.Errorf("build context not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("build context %q is not a directory", contextSubpath)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var totalBytes int64

	err = filepath.Walk(contextDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == contextDir {
			return nil
		}

		rel, err := filepath.Rel(contextDir, path)
		if err != nil {
			return err
		}
		tarName := filepath.ToSlash(rel)

		// Guard against symlink traversal pointing outside the workspace
		if fi.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err == nil {
				resolvedTarget := linkTarget
				if !filepath.IsAbs(resolvedTarget) {
					resolvedTarget = filepath.Join(filepath.Dir(path), linkTarget)
				}
				resolvedTarget = filepath.Clean(resolvedTarget)
				if !strings.HasPrefix(resolvedTarget, w.Dir) {
					return fmt.Errorf("%w: symlink %q targets outside workspace: %s", ErrPathTraversal, rel, linkTarget)
				}
			}
		}

		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}
		header.Name = tarName

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if fi.Mode().IsRegular() {
			totalBytes += fi.Size()
			if totalBytes > maxBytes {
				return fmt.Errorf("%w: context size %d bytes exceeds %d bytes limit", ErrContextTooLarge, totalBytes, maxBytes)
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}

// Cleanup deletes the workspace directory and all contained files.
func (w *Workspace) Cleanup() error {
	if w.Dir == "" || w.Dir == "/" {
		return nil
	}
	return os.RemoveAll(w.Dir)
}

func stripPathComponents(p string, count int) string {
	if count <= 0 {
		return p
	}
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) <= count {
		return ""
	}
	return strings.Join(parts[count:], "/")
}
