package fileconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrBackupDirectory   = errors.New("backup directory must be a private, non-symlink directory")
	ErrInvalidBackupName = errors.New("invalid backup name")
	ErrBackupNotRegular  = errors.New("backup must be a regular file")
)

type Backup struct {
	Name       string
	Size       int64
	ModifiedAt time.Time
}

func ResolvePath() string {
	if path := os.Getenv("PI_SWITCH_CONFIG"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch-config.json"
	}
	return filepath.Join(home, ".pi-switch", "config.json")
}

func ValidateBackupDirectory(path string) error {
	info, err := os.Lstat(path + ".backups")
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrBackupDirectory
	}
	return nil
}

func CreateConfigBackup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path+".backups", 0700); err != nil {
		return "", err
	}
	if err := ValidateBackupDirectory(path); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(path+".backups", "config-*.json")
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(file.Name())
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	complete = true
	return filepath.Base(file.Name()), nil
}

func ListConfigBackups(path string) ([]Backup, error) {
	if err := ValidateBackupDirectory(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Backup{}, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(path + ".backups")
	if errors.Is(err, os.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]Backup, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "config-") && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			backups = append(backups, Backup{Name: entry.Name(), Size: info.Size(), ModifiedAt: info.ModTime()})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		if !backups[i].ModifiedAt.Equal(backups[j].ModifiedAt) {
			return backups[i].ModifiedAt.After(backups[j].ModifiedAt)
		}
		return backups[i].Name < backups[j].Name
	})
	return backups, nil
}

func ReadConfigBackup(path, name string) ([]byte, error) {
	if name != filepath.Base(name) || !strings.HasPrefix(name, "config-") || !strings.HasSuffix(name, ".json") {
		return nil, ErrInvalidBackupName
	}
	if err := ValidateBackupDirectory(path); err != nil {
		return nil, err
	}
	backupPath := filepath.Join(path+".backups", name)
	info, err := os.Lstat(backupPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrBackupNotRegular
	}
	return os.ReadFile(backupPath)
}

func ConfigRevision(data []byte) string {
	if data == nil {
		return "missing-config"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ConfigFileRevision(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return ConfigRevision(data), nil
}
