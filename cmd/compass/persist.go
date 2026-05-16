package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	persistConfigSubdir = "magkal"
	bestFileName        = "best_fit.json"
	configFileName      = "config.json"
	alignFileName       = "align.json"
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

// alignFile is the on-disk schema for the per-mount alignment. R is the
// sensor→vehicle rotation (rows = [forward, right, down] in sensor frame)
// captured at Align time; AlignHeadingDeg is the magnetic heading the user
// (or the GPS track) supplied as truth. A file with R all zeros is treated
// as "no alignment yet" — the binary continues without heading until Align
// is pressed.
type alignFile struct {
	R               [3][3]float64 `json:"r"`
	AlignHeadingDeg float64       `json:"alignHeadingDeg"`
	SavedAt         time.Time     `json:"savedAt"`
}

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

func loadAlign() (*alignFile, error) {
	path, err := magkalFile(alignFileName)
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
	var a alignFile
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &a, nil
}

func saveAlign(a *alignFile) error {
	return writeJSONAtomic(alignFileName, a)
}

func writeJSONAtomic(name string, v any) error {
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
	final := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, name+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, final)
}
