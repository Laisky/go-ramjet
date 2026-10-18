package tasks

import (
	"os"
	"testing"
)

// TestMain isolates every existing synthetic worker failure from service journals.
func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "ramjet-worker-journal-tests-")
	if err != nil {
		panic(err)
	}
	if err = os.Chmod(directory, 0700); err != nil {
		panic(err)
	}
	if err = os.Setenv("CRAWLER_FAILURE_LOG_DIR", directory); err != nil {
		panic(err)
	}
	code := m.Run()
	if err = os.RemoveAll(directory); err != nil {
		panic(err)
	}
	os.Exit(code)
}
