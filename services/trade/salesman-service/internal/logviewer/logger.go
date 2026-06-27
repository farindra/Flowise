package logviewer

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"
)

const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I,O,0,1 — easy to read

var (
	logFile *os.File
	mu      sync.Mutex
)

// Init opens the error log file for writing.
func Init(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	logFile = f
	return nil
}

// NewCode generates a random 5-char error code.
func NewCode() string {
	b := make([]byte, 5)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// LogError writes a structured error line and returns the error code.
func LogError(code, context, message string) string {
	line := fmt.Sprintf("%s [ERROR] [%s] %s | %s\n",
		time.Now().UTC().Format("2006-01-02T15:04:05Z"), code, context, message)

	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.WriteString(line)
	}
	return code
}
