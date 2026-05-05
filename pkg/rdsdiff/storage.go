// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionDir creates a session directory under workDir and returns its path.
// workDir defaults to os.TempDir() if empty. Session name is rds-diff-<requestID>-<timestamp>.
func SessionDir(workDir, requestID string) (string, error) {
	if workDir == "" {
		workDir = os.TempDir()
	}
	workDir = filepath.Clean(workDir)
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir work dir: %w", err)
	}
	name := fmt.Sprintf("rds-diff-%s-%d", requestID, time.Now().Unix())
	sessionPath := filepath.Join(workDir, name)
	if err := os.MkdirAll(sessionPath, 0o750); err != nil {
		return "", fmt.Errorf("mkdir session: %w", err)
	}
	return sessionPath, nil
}

// SessionID returns the session directory name (last path component) for use as a stable artifact ID.
func SessionID(sessionPath string) string {
	return filepath.Base(sessionPath)
}

// ErrInvalidSessionID is returned when the session ID is invalid (e.g. path traversal).
var ErrInvalidSessionID = errors.New("invalid session id")

// ResolveSessionPath resolves workDir + sessionID to an absolute session path and validates that it
// is under workDir and exists as a directory. sessionID must be a single path segment (no slashes or "..").
func ResolveSessionPath(workDir, sessionID string) (string, error) {
	workDir = filepath.Clean(workDir)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", ErrInvalidSessionID
	}
	if strings.Contains(sessionID, "..") || filepath.IsAbs(sessionID) || strings.ContainsAny(sessionID, `/\`) {
		return "", ErrInvalidSessionID
	}
	sessionPath := filepath.Join(workDir, sessionID)
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve work dir: %w", err)
	}
	absSession, err := filepath.Abs(sessionPath)
	if err != nil {
		return "", fmt.Errorf("resolve session path: %w", err)
	}
	rel, err := filepath.Rel(absWork, absSession)
	if err != nil {
		return "", fmt.Errorf("resolve session path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidSessionID
	}
	info, err := os.Stat(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session not found: %w", err)
		}
		return "", fmt.Errorf("stat session: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", sessionPath)
	}
	return sessionPath, nil
}
