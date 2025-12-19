package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func resetLogger() {
	instance = nil
	once = sync.Once{}
}

func createTempLogFile(t *testing.T) string {
	tmpDir := t.TempDir()
	return filepath.Join(tmpDir, "test.log")
}

func TestInit_Success(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if instance == nil {
		t.Fatal("Expected instance to be initialized")
	}
	if instance.file == nil {
		t.Error("Expected file to be set")
	}
	if instance.logger == nil {
		t.Error("Expected logger to be set")
	}

	Close()
}

func TestInit_InvalidPath(t *testing.T) {
	resetLogger()
	invalidPath := "/invalid/path/that/does/not/exist/test.log"

	err := Init(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid path")
	}

	Close()
}

func TestInit_SingletonBehavior(t *testing.T) {
	resetLogger()
	logPath1 := createTempLogFile(t)

	err := Init(logPath1)
	if err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	firstInstance := instance

	tmpDir := t.TempDir()
	logPath2 := filepath.Join(tmpDir, "second.log")
	err = Init(logPath2)
	if err != nil {
		t.Error("Second init should not error")
	}

	if instance != firstInstance {
		t.Error("Expected singleton pattern to preserve first instance")
	}

	Close()
}

func TestClose_WithInstance(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	err = Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestClose_WithoutInstance(t *testing.T) {
	resetLogger()

	err := Close()
	if err != nil {
		t.Errorf("Close without instance should not error, got %v", err)
	}
}

func TestLogFileOpen(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	testPath := "/test/path/file.json"
	LogFileOpen(testPath)

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "[FILE_OPEN]") {
		t.Error("Expected [FILE_OPEN] tag in log")
	}
	if !strings.Contains(logContent, testPath) {
		t.Errorf("Expected path %s in log", testPath)
	}
}

func TestLogFileOpen_NoInstance(t *testing.T) {
	resetLogger()

	LogFileOpen("/test/path")
}

func TestLogFileWrite(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	testPath := "/test/path/output.json"
	LogFileWrite(testPath)

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "[FILE_WRITE]") {
		t.Error("Expected [FILE_WRITE] tag in log")
	}
	if !strings.Contains(logContent, testPath) {
		t.Errorf("Expected path %s in log", testPath)
	}
}

func TestLogFileWrite_NoInstance(t *testing.T) {
	resetLogger()

	LogFileWrite("/test/path")
}

func TestLogError(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	testOperation := "TestOperation"
	testPath := "/test/path/file.json"
	testError := os.ErrNotExist

	LogError(testOperation, testPath, testError)

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "[ERROR]") {
		t.Error("Expected [ERROR] tag in log")
	}
	if !strings.Contains(logContent, testOperation) {
		t.Errorf("Expected operation %s in log", testOperation)
	}
	if !strings.Contains(logContent, testPath) {
		t.Errorf("Expected path %s in log", testPath)
	}
}

func TestLogError_NoInstance(t *testing.T) {
	resetLogger()

	LogError("operation", "/path", os.ErrNotExist)
}

func TestConcurrentLogging(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	logsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < logsPerGoroutine; j++ {
				LogFileOpen("/test/path/concurrent")
				LogFileWrite("/test/path/concurrent")
				LogError("concurrent", "/test/path", os.ErrNotExist)
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	openCount := strings.Count(logContent, "[FILE_OPEN]")
	writeCount := strings.Count(logContent, "[FILE_WRITE]")
	errorCount := strings.Count(logContent, "[ERROR]")

	expectedCount := numGoroutines * logsPerGoroutine

	if openCount != expectedCount {
		t.Errorf("Expected %d FILE_OPEN logs, got %d", expectedCount, openCount)
	}
	if writeCount != expectedCount {
		t.Errorf("Expected %d FILE_WRITE logs, got %d", expectedCount, writeCount)
	}
	if errorCount != expectedCount {
		t.Errorf("Expected %d ERROR logs, got %d", expectedCount, errorCount)
	}
}

func TestMultipleOperations(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	LogFileOpen("/path1")
	LogFileWrite("/path2")
	LogError("op1", "/path3", os.ErrPermission)
	LogFileOpen("/path4")
	LogFileWrite("/path5")

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if strings.Count(logContent, "[FILE_OPEN]") != 2 {
		t.Error("Expected 2 FILE_OPEN entries")
	}
	if strings.Count(logContent, "[FILE_WRITE]") != 2 {
		t.Error("Expected 2 FILE_WRITE entries")
	}
	if strings.Count(logContent, "[ERROR]") != 1 {
		t.Error("Expected 1 ERROR entry")
	}

	if !strings.Contains(logContent, "/path1") {
		t.Error("Expected /path1 in log")
	}
	if !strings.Contains(logContent, "/path5") {
		t.Error("Expected /path5 in log")
	}
}

func TestLogFileAppend(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	LogFileOpen("/first")
	Close()

	resetLogger()

	err = Init(logPath)
	if err != nil {
		t.Fatalf("Second init failed: %v", err)
	}

	LogFileOpen("/second")
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	if !strings.Contains(logContent, "/first") {
		t.Error("Expected /first from first session")
	}
	if !strings.Contains(logContent, "/second") {
		t.Error("Expected /second from second session")
	}
	if strings.Count(logContent, "[FILE_OPEN]") != 2 {
		t.Error("Expected both log entries to be preserved")
	}
}

func TestInit_CreateNewFile(t *testing.T) {
	resetLogger()
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "newfile.log")

	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("File should not exist before Init")
	}

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("File should exist after Init")
	}

	Close()
}

func TestLogger_ThreadSafety(t *testing.T) {
	resetLogger()
	logPath := createTempLogFile(t)

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var wg sync.WaitGroup
	operations := 100

	for i := 0; i < operations; i++ {
		wg.Add(3)

		go func(n int) {
			defer wg.Done()
			LogFileOpen("/path/open")
		}(i)

		go func(n int) {
			defer wg.Done()
			LogFileWrite("/path/write")
		}(i)

		go func(n int) {
			defer wg.Done()
			LogError("error", "/path/error", os.ErrNotExist)
		}(i)
	}

	wg.Wait()
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	totalLogs := strings.Count(logContent, "\n")

	if totalLogs < operations*3 {
		t.Errorf("Expected at least %d log lines, got %d", operations*3, totalLogs)
	}
}
