// Package logging handles application-level error and debug logging to the local filesystem.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
)

const (
	maxLogFileSizeBytes = 10 * 1024 * 1024
	maxLogBackups       = 5
	maxLogDirBytes      = 100 * 1024 * 1024
	maxLogAge           = 14 * 24 * time.Hour
)

// Options configures the logging system initialization.
type Options struct {
	Now     func() time.Time
	HomeDir string
	Debug   bool
}

// Init initializes the application logger based on the provided options.
// It returns the logger instance, the absolute path to the active log file, and any error encountered.
func Init(opts Options) (*slog.Logger, string, error) {
	home := opts.HomeDir
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, "", fmt.Errorf("resolve home dir: %w", err)
		}
		home = h
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	logDir := filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "logs")
	if err := os.MkdirAll(logDir, domain.DefaultDirPerm); err != nil {
		return nil, "", fmt.Errorf("create log dir: %w", err)
	}

	fileName := "error.log"
	level := slog.LevelWarn
	if opts.Debug {
		fileName = "debug.log"
		level = slog.LevelDebug
	}

	logPath := filepath.Join(logDir, fileName)
	if err := rotateIfNeeded(logPath, maxLogFileSizeBytes, maxLogBackups); err != nil {
		return nil, "", fmt.Errorf("rotate log file: %w", err)
	}
	if err := cleanupLogs(logDir, nowFn(), maxLogAge, maxLogDirBytes); err != nil {
		return nil, "", fmt.Errorf("cleanup logs: %w", err)
	}

	// #nosec G304 - Intentional log file opening
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, domain.DefaultFilePerm)
	if err != nil {
		return nil, "", fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewTextHandler(io.MultiWriter(f), &slog.HandlerOptions{Level: level})
	return slog.New(handler), logPath, nil
}

func rotateIfNeeded(logPath string, maxSize int64, backups int) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() < maxSize {
		return nil
	}

	for i := backups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		if i == backups {
			_ = os.Remove(src)
			continue
		}
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	return os.Rename(logPath, fmt.Sprintf("%s.1", logPath))
}

func cleanupLogs(logDir string, now time.Time, maxAge time.Duration, maxTotalBytes int64) error {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}

	type fileInfo struct {
		mod  time.Time
		path string
		size int64
	}
	files := make([]fileInfo, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") && !strings.Contains(name, ".log.") {
			continue
		}
		p := filepath.Join(logDir, name)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if maxAge > 0 && now.Sub(st.ModTime()) > maxAge {
			_ = os.Remove(p)
			continue
		}
		files = append(files, fileInfo{path: p, size: st.Size(), mod: st.ModTime()})
	}

	var total int64
	for _, f := range files {
		total += f.size
	}
	if total <= maxTotalBytes {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.Before(files[j].mod)
	})

	for _, f := range files {
		if total <= maxTotalBytes {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
	return nil
}
