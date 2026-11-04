// Package demo creates synthetic fixtures for future product features.
package demo

import (
	"fmt"
	"os"
)

const fixtures = `{"version":1,"namespace":"demo","synthetic":true,"records":[{"kind":"capsule","label":"Synthetic capsule fixture","implemented":false},{"kind":"switch","label":"Synthetic switch fixture","implemented":false},{"kind":"countdown","label":"Synthetic countdown fixture","implemented":false},{"kind":"window","label":"Synthetic window fixture","implemented":false}]}
`

// Create writes synthetic fixtures below the state directory.
// Existing fixtures remain unchanged.
func Create(stateDir string) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("create demo state directory: %w", err)
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return fmt.Errorf("open demo state directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll("demo", 0700); err != nil {
		return fmt.Errorf("create demo namespace: %w", err)
	}
	info, err := root.Lstat("demo")
	if err != nil {
		return fmt.Errorf("inspect demo namespace: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("demo namespace must be a directory without a symbolic link")
	}
	file, err := root.OpenFile("demo/fixtures.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create synthetic fixtures: %w", err)
	}
	_, writeErr := file.WriteString(fixtures)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write synthetic fixtures: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close synthetic fixtures: %w", closeErr)
	}
	return nil
}

// Clean removes only the explicit demo namespace below the state directory.
func Clean(stateDir, namespace string) error {
	if namespace != "demo" {
		return fmt.Errorf("cleanup requires --namespace demo")
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return fmt.Errorf("open demo state directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err := root.RemoveAll("demo"); err != nil {
		return fmt.Errorf("remove demo namespace: %w", err)
	}
	return nil
}
