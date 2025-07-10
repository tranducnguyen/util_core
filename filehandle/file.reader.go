package filehandle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LineFileHandler manages file operations with line-by-line processing
type ReaderHandler struct {
	filePath     string
	isSyncPos    bool
	positionFile string
	file         *os.File
	mu           sync.RWMutex
	isOpen       bool
	lastAccess   time.Time
	autoCloseMs  int64
	reader       *bufio.Reader
	maxLineSize  int  // Maximum line size in bytes
	openMode     int  // 0=not open, 1=read, 2=write
	appendMode   bool // Tracks append mode for writing
	currentPos   int64
	lineNumber   int64
	isStopped    bool // Flag to indicate if processing is stopped
}

// NewLineFileHandler creates a new file controller optimized for line processing
func NewReaderHandler(path string, maxLineSize int) (*ReaderHandler, error) {
	if maxLineSize <= 0 {
		maxLineSize = 4 * 1024 // Default 4KB for line buffer
	}

	return &ReaderHandler{
		filePath:     path,
		positionFile: path + ".pos", // Optional position file for tracking
		isOpen:       false,
		autoCloseMs:  10000, // Auto-close after 10 seconds of inactivity
		maxLineSize:  maxLineSize,
		openMode:     0,
		appendMode:   false,
		isStopped:    false,
		currentPos:   0,
		lineNumber:   0,
		isSyncPos:    true,
	}, nil
}

func (fc *ReaderHandler) SetSyncPos(value bool) {
	fc.isSyncPos = value
}

// ensureOpenForReading ensures the file is open for reading
func (fc *ReaderHandler) ensureOpenForReading() error {
	// If already open for reading, just update access time
	if fc.isOpen && fc.openMode == 1 && fc.file != nil && fc.reader != nil {
		fc.lastAccess = time.Now()
		return nil
	}

	// If open in a different mode, close it first
	if fc.isOpen && fc.openMode != 1 {
		fc.closeInternal()
	}

	// Open the file
	var err error
	fc.file, err = os.Open(fc.filePath)
	if err != nil {
		return err
	}

	// Get a reader from the pool or create a new one
	reader, ok := readerPool.Get().(*bufio.Reader)
	if !ok || reader == nil {
		reader = bufio.NewReaderSize(fc.file, 4*1024)
	} else {
		reader.Reset(fc.file)
	}
	fc.reader = reader

	fc.isOpen = true
	fc.openMode = 1
	fc.lastAccess = time.Now()
	return nil
}

// ReadLine reads a single line from the file
func (fc *ReaderHandler) ReadLine() (string, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if err := fc.ensureOpenForReading(); err != nil {
		return "", err
	}

	line, err := fc.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}

	// Remove trailing newline if present
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1] // Handle CRLF
		}
	}

	return line, err
}

func (fc *ReaderHandler) PipeLine(processor func(line string) error) error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if err := fc.ensureOpenForReading(); err != nil {
		return err
	}

	savedPos, savedLine, err := fc.loadPosition()
	if err != nil {
		savedPos = 0
		savedLine = 0
	}

	if _, err := fc.file.Seek(savedPos, io.SeekStart); err != nil {
		return err
	}
	fc.currentPos = savedPos
	fc.lineNumber = savedLine

	for {
		if fc.isStopped {
			return errors.New("processing stopped")
		}

		line, err := fc.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}

		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if errOn := processor(trimmed); errOn != nil {
				return errOn
			}
			fc.lineNumber++
			fc.currentPos += int64(len(line))
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// ReadAllLines reads all lines from the file into a slice
func (fc *ReaderHandler) ReadAllLines() ([]string, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if err := fc.ensureOpenForReading(); err != nil {
		return nil, err
	}

	// Reset to beginning of file
	if _, err := fc.file.Seek(0, 0); err != nil {
		return nil, err
	}
	fc.reader.Reset(fc.file)

	// Pre-allocate slice to reduce reallocations
	lines := make([]string, 0, 1000)

	scanner := bufio.NewScanner(fc.file)
	buf := make([]byte, fc.maxLineSize)
	scanner.Buffer(buf, fc.maxLineSize)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// CountLines counts the number of lines in the file
func (fc *ReaderHandler) CountLines() (int, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if err := fc.ensureOpenForReading(); err != nil {
		return 0, err
	}

	// Reset to beginning of file
	if _, err := fc.file.Seek(0, 0); err != nil {
		return 0, err
	}
	fc.reader.Reset(fc.file)

	lineCount := 0
	scanner := bufio.NewScanner(fc.file)

	// Use a smaller buffer for counting since we don't need the content
	smallBuf := make([]byte, 4096)
	scanner.Buffer(smallBuf, 4096)

	for scanner.Scan() {
		lineCount++
	}

	return lineCount, scanner.Err()
}

// GetLineAt gets a specific line by number (starting from 1)
func (fc *ReaderHandler) GetLineAt(lineNum int) (string, error) {
	if lineNum < 1 {
		return "", errors.New("line number must be positive")
	}

	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if err := fc.ensureOpenForReading(); err != nil {
		return "", err
	}

	// Reset to beginning of file
	if _, err := fc.file.Seek(0, 0); err != nil {
		return "", err
	}
	fc.reader.Reset(fc.file)

	currentLine := 0
	scanner := bufio.NewScanner(fc.file)
	buf := make([]byte, fc.maxLineSize)
	scanner.Buffer(buf, fc.maxLineSize)

	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return scanner.Text(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("line number exceeds file length")
}

// Close closes the file and returns buffers to pool
func (fc *ReaderHandler) Close() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	return fc.closeInternal()
}

// closeInternal handles the actual closing logic (must be called with lock held)
func (fc *ReaderHandler) closeInternal() error {
	if !fc.isOpen || fc.file == nil {
		// Already closed or never opened
		fc.isOpen = false
		fc.openMode = 0
		fc.reader = nil
		return nil
	}

	var err error

	// Return reader to pool if exists
	if fc.reader != nil {
		readerPool.Put(fc.reader)
		fc.reader = nil
	}

	// Save current position and line number
	if saveErr := fc.savePosition(); saveErr != nil {
		err = saveErr
	}
	// Reset file position and line number
	fc.currentPos = 0
	fc.lineNumber = 0

	// Close file
	if closeErr := fc.file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	fc.file = nil
	fc.isOpen = false
	fc.openMode = 0

	return err
}

// StartAutoCloseMonitor starts a goroutine that monitors file access
// and closes the file after a period of inactivity
func (fc *ReaderHandler) StartAutoCloseMonitor(checkIntervalMs int64) {
	if checkIntervalMs <= 0 {
		checkIntervalMs = 1000 // Default to 1 second if invalid
	}

	go func() {
		ticker := time.NewTicker(time.Duration(checkIntervalMs) * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			fc.mu.Lock()
			if fc.isOpen && time.Since(fc.lastAccess).Milliseconds() > fc.autoCloseMs {
				fc.closeInternal()
				fc.savePosition()
			}
			fc.mu.Unlock()
		}
	}()
}
func (fr *ReaderHandler) loadPosition() (int64, int64, error) {
	data, err := os.ReadFile(fr.positionFile)
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Split(strings.TrimSpace(string(data)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid position file format")
	}

	pos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	line, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return pos, line, nil
}

func (fr *ReaderHandler) savePosition() error {
	if !fr.isSyncPos {
		return nil
	}

	data := fmt.Sprintf("%d,%d", fr.currentPos, fr.lineNumber)
	return os.WriteFile(fr.positionFile, []byte(data), 0644)
}

func (fr *ReaderHandler) IsStopped() bool {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.isStopped
}

func (fr *ReaderHandler) Stop() error {
	if !fr.isOpen {
		return nil // Already stopped
	}

	fr.isStopped = true
	return nil
}
