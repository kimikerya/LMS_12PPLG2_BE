// Package backup stores database dumps and immutable learning uploads together.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version  int       `json:"version"`
	Database string    `json:"database"`
	Created  time.Time `json:"created_at"`
	Files    []File    `json:"files"`
}

func digest(path string) (File, error) {
	file, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	return File{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, err
}

// CopyTree refuses links and special files. Destination must be a new directory.
func CopyTree(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("sumber harus direktori biasa")
	}
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	targetPath, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(sourcePath, targetPath)
	if err != nil {
		return err
	}
	if relative == "." || filepath.IsLocal(relative) {
		return fmt.Errorf("tujuan salinan tidak boleh berada di dalam sumber")
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if !filepath.IsLocal(relative) || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("file tautan tidak didukung: %s", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bukan file biasa: %s", relative)
		}
		return copyFile(path, target)
	})
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func Seal(directory, database string) error {
	manifest := Manifest{Version: 1, Database: database, Created: time.Now().UTC()}
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("file backup tidak valid")
		}
		file, err := digest(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		file.Path = filepath.ToSlash(relative)
		manifest.Files = append(manifest.Files, file)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "manifest.json"), raw, 0600)
}

// Verify rejects incomplete backups, traversal paths and changed file contents.
// A checksum detects corruption; only restore backups from a trusted source.
func Verify(directory string) (Manifest, error) {
	var manifest Manifest
	root, err := os.OpenRoot(directory)
	if err != nil {
		return manifest, err
	}
	defer root.Close()
	raw, err := root.ReadFile("manifest.json")
	if err != nil {
		return manifest, err
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != 1 || len(manifest.Files) == 0 {
		return manifest, fmt.Errorf("manifest backup tidak valid")
	}
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if !filepath.IsLocal(file.Path) || seen[file.Path] || (file.Path != "database.sql" && !startsWithUploads(file.Path)) {
			return manifest, fmt.Errorf("path backup tidak valid")
		}
		seen[file.Path] = true
		input, err := root.Open(file.Path)
		if err != nil {
			return manifest, err
		}
		info, err := input.Stat()
		if err != nil || !info.Mode().IsRegular() {
			input.Close()
			return manifest, fmt.Errorf("file backup tidak valid")
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, input)
		input.Close()
		if readErr != nil {
			return manifest, readErr
		}
		if size != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
			return manifest, fmt.Errorf("checksum tidak sesuai: %s", file.Path)
		}
	}
	if !seen["database.sql"] {
		return manifest, fmt.Errorf("dump database tidak ditemukan")
	}
	// Extra files are rejected as well; restore must not copy unverified uploads.
	err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("tautan tidak diizinkan dalam backup")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		if key != "manifest.json" && !seen[key] {
			return fmt.Errorf("file tidak terdaftar dalam manifest: %s", key)
		}
		return nil
	})
	return manifest, err
}

func startsWithUploads(path string) bool {
	return len(path) > len("uploads/") && path[:len("uploads/")] == "uploads/"
}
