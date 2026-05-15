package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Persistent server-side state under ~/.config/magkal/. Two files:
//
//   config.json   — user-edited params that should stick across restarts
//                   (currently just n0; intentionally narrow — most params
//                   are session-scoped).
//   best_fit.json — the user's saved "known best" filter snapshot: full
//                   state x = (k, l) plus covariance P, written only on
//                   an explicit Save Best action and consulted when the
//                   Actual source's filter is (re-)built.
//
// File I/O is dirt-cheap at the rates exercised here (a handful of
// reads/writes per session), so there's no cache layer; every load
// re-reads from disk so the in-memory view is always authoritative.

const (
	persistConfigSubdir = "magkal"
	bestFileName        = "best_fit.json"
	configFileName      = "config.json"
)

type configFile struct {
	N0 float64 `json:"n0"`
}

type bestFile struct {
	N       int         `json:"n"`
	K       []float64   `json:"k"`
	L       []float64   `json:"l"`
	P       [][]float64 `json:"p"`
	SavedAt time.Time   `json:"savedAt"`
}

// magkalDir returns ~/.config/magkal/, following the XDG-ish convention.
// os.UserConfigDir is used directly so XDG_CONFIG_HOME is honored when
// set, while $HOME/.config is the fallback on Linux/Pi.
func magkalDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, persistConfigSubdir), nil
}

func magkalFile(name string) (string, error) {
	dir, err := magkalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// loadConfig reads ~/.magkal/config.json. Returns (nil, nil) when the
// file is absent (first-run case); errors only on parse/permission
// problems.
func loadConfig() (*configFile, error) {
	path, err := magkalFile(configFileName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c configFile
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

func saveConfig(c *configFile) error {
	return writeJSON(configFileName, c)
}

// loadBest reads the saved best-fit snapshot. Returns (nil, nil) when absent.
func loadBest() (*bestFile, error) {
	path, err := magkalFile(bestFileName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b bestFile
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &b, nil
}

func saveBestFile(b *bestFile) error {
	return writeJSON(bestFileName, b)
}

// deleteBestFile removes the saved best-fit snapshot. Idempotent — returns
// nil if the file was already absent.
func deleteBestFile() error {
	path, err := magkalFile(bestFileName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeJSON(name string, v any) error {
	dir, err := magkalDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	return os.WriteFile(path, data, 0o644)
}
