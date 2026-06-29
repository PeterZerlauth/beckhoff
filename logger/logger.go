package logger

import (
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	file *os.File
	log  *slog.Logger
	path string
}

var (
	instance *Logger
	once     sync.Once
)

// GetLogger returns the singleton instance
func GetLogger(path string, keep int) *Logger {
	once.Do(func() {
		instance = newLogger(path, keep)
	})
	return instance
}

// internal constructor
func newLogger(path string, keep int) *Logger {
	today := time.Now().Format("2006-01-02")
	currentPath := today + ".log"

	file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	multi := io.MultiWriter(os.Stdout, file)

	handler := slog.NewTextHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	l := &Logger{
		file: file,
		log:  slog.New(handler),
		path: currentPath,
	}

	l.startRotation(keep)

	return l
}

func (l *Logger) startRotation(keep int) {
	go func() {
		for {
			time.Sleep(time.Minute)
			l.rotateDaily(keep)
		}
	}()
}

func (l *Logger) rotateDaily(keep int) {
	today := time.Now().Format("2006-01-02")
	newPath := today + ".log"

	if l.path == newPath {
		return
	}

	_ = l.file.Close()

	file, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return
	}

	l.file = file
	l.path = newPath

	multi := io.MultiWriter(os.Stdout, file)
	handler := slog.NewTextHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	l.log = slog.New(handler)

	l.cleanupOldLogs(keep)
}

func (l *Logger) cleanupOldLogs(keep int) {
	files, err := os.ReadDir(".")
	if err != nil {
		return
	}

	var logs []string

	for _, f := range files {
		name := f.Name()

		if strings.HasSuffix(name, ".log") &&
			len(name) == len("2006-01-02.log") {
			logs = append(logs, name)
		}
	}

	sort.Strings(logs)

	if len(logs) <= keep {
		return
	}

	for _, f := range logs[:len(logs)-keep] {
		_ = os.Remove(f)
	}
}

func (l *Logger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}

func (l *Logger) Log() *slog.Logger {
	return l.log
}
