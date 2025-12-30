package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var (
	instance *Logger
	once     sync.Once
)

type Logger struct {
	file      *os.File
	logger    *log.Logger
	mu        sync.Mutex
	memBuffer []string
}

func Init(logPath string) error {
	var initErr error
	once.Do(func() {
		if logPath != "" {
			file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				initErr = fmt.Errorf("failed to open log file: %w", err)
				return
			}

			instance = &Logger{
				file:      file,
				logger:    log.New(file, "", log.LstdFlags),
				memBuffer: make([]string, 0, 1000),
			}
		} else {
			instance = &Logger{
				memBuffer: make([]string, 0, 1000),
			}
		}
	})
	return initErr
}

func Close() error {
	if instance != nil && instance.file != nil {
		return instance.file.Close()
	}
	return nil
}

func LogFileOpen(path string) {
	if instance == nil {
		return
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf("%s [FILE_OPEN] %s", timestamp, path)
	instance.memBuffer = append(instance.memBuffer, msg)
	if instance.logger != nil {
		instance.logger.Printf("[FILE_OPEN] %s", path)
	}
}

func LogFileWrite(path string) {
	if instance == nil {
		return
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf("%s [FILE_WRITE] %s", timestamp, path)
	instance.memBuffer = append(instance.memBuffer, msg)
	if instance.logger != nil {
		instance.logger.Printf("[FILE_WRITE] %s", path)
	}
}

func LogError(operation, path string, err error) {
	if instance == nil {
		return
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf("%s [ERROR] %s: %s - %v", timestamp, operation, path, err)
	instance.memBuffer = append(instance.memBuffer, msg)
	if instance.logger != nil {
		instance.logger.Printf("[ERROR] %s: %s - %v", operation, path, err)
	}
}

func GetLogs() []string {
	if instance == nil {
		return []string{"Logger not initialized"}
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()

	logsCopy := make([]string, len(instance.memBuffer))
	copy(logsCopy, instance.memBuffer)
	return logsCopy
}
