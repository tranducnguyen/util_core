package filehandle

import (
	"bufio"
	"os"
	"sync"
	"time"
)

type WriteHandler struct {
	filePath    string
	file        *os.File
	mu          sync.RWMutex
	isOpen      bool
	lastAccess  time.Time
	autoCloseMs int64
	writer      *bufio.Writer
	maxLineSize int  // Maximum line size in bytes
	openMode    int  // 0=not open, 1=read, 2=write
	appendMode  bool // Tracks append mode for writing
	lineNumber  int64
	isStopped   bool // Flag to indicate if processing is stopped
}

// NewLineFileHandler creates a new file controller optimized for line processing
func NewWriteHandler(path string, maxLineSize int) (*WriteHandler, error) {
	if maxLineSize <= 0 {
		maxLineSize = 4 * 1024 // Default 4KB for line buffer
	}

	return &WriteHandler{
		filePath:    path,
		isOpen:      false,
		autoCloseMs: 10000, // Auto-close after 10 seconds of inactivity
		maxLineSize: maxLineSize,
		openMode:    0,
		appendMode:  false,
		isStopped:   false,
		lineNumber:  0,
	}, nil
}

func (fc *WriteHandler) ensureOpenForWriting(append bool) error {
	// If already open for writing with same append mode, just update access time
	if fc.isOpen && fc.openMode == 2 && fc.file != nil && fc.writer != nil && fc.appendMode == append {
		fc.lastAccess = time.Now()
		return nil
	}

	// If open in a different mode or different append setting, close it first
	if fc.isOpen && (fc.openMode != 2 || fc.appendMode != append) {
		fc.closeInternal()
	}

	var flag int
	if append {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	} else {
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	var err error
	fc.file, err = os.OpenFile(fc.filePath, flag, 0666)
	if err != nil {
		return err
	}

	// Get a writer from the pool or create a new one
	writer, ok := writerPool.Get().(*bufio.Writer)
	if !ok || writer == nil {
		writer = bufio.NewWriterSize(fc.file, 4*1024)
	} else {
		writer.Reset(fc.file)
	}
	fc.writer = writer

	fc.isOpen = true
	fc.openMode = 2
	fc.appendMode = append
	fc.lastAccess = time.Now()
	return nil
}

func (fc *WriteHandler) WriteLine(line string, append bool) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if err := fc.ensureOpenForWriting(append); err != nil {
		return err
	}

	if _, err := fc.writer.WriteString(line); err != nil {
		return err
	}
	if _, err := fc.writer.WriteString("\n"); err != nil {
		return err
	}

	// In high-concurrency environments, flush after each write
	// to ensure data is written to disk regularly
	return fc.writer.Flush()
}

// Flush ensures all buffered data is written to disk
func (fc *WriteHandler) Flush() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.isOpen && fc.writer != nil {
		return fc.writer.Flush()
	}
	return nil
}

func (fc *WriteHandler) WriteLines(lines []string, append bool) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if err := fc.ensureOpenForWriting(append); err != nil {
		return err
	}

	for _, line := range lines {
		if _, err := fc.writer.WriteString(line); err != nil {
			return err
		}
		if _, err := fc.writer.WriteString("\n"); err != nil {
			return err
		}
	}

	return fc.writer.Flush()
}

// Close closes the file and returns buffers to pool
func (fc *WriteHandler) Close() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	return fc.closeInternal()
}

// closeInternal handles the actual closing logic (must be called with lock held)
func (fc *WriteHandler) closeInternal() error {
	if !fc.isOpen || fc.file == nil {
		// Already closed or never opened
		fc.isOpen = false
		fc.openMode = 0
		fc.writer = nil
		return nil
	}

	var err error

	// Flush writer and return to pool if exists
	if fc.writer != nil {
		if flushErr := fc.writer.Flush(); flushErr != nil {
			err = flushErr
		}
		writerPool.Put(fc.writer)
		fc.writer = nil
	}
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
func (fc *WriteHandler) StartAutoCloseMonitor(checkIntervalMs int64) {
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
			}
			fc.mu.Unlock()
		}
	}()
}

func (fr *WriteHandler) IsStopped() bool {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.isStopped
}

func (fr *WriteHandler) Stop() error {
	if !fr.isOpen {
		return nil // Already stopped
	}

	fr.isStopped = true
	return nil
}
