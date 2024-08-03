package jobworker

import (
	"bytes"
	"io"
	"sync"
)

type logger struct {
	buffer bytes.Buffer
	mutex  sync.RWMutex
	cond   *sync.Cond
	closed bool
}

func NewLogger() *logger {
	l := &logger{}
	l.cond = sync.NewCond(l.mutex.RLocker())
	return l
}

// Write implements the io.Writer interface
func (l *logger) Write(p []byte) (n int, err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.closed {
		return 0, io.ErrClosedPipe
	}

	n, err = l.buffer.Write(p)
	l.cond.Broadcast()
	return n, err
}

func (l *logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.closed = true
	l.cond.Broadcast()
	return nil
}

// logReader implements io.Reader and references the Logger's buffer
type logReader struct {
	logger *logger
	offset int64
}

// NewLogReader creates a new StreamReader for the given StreamWriter
func NewLogReader(logger *logger) io.Reader {
	return &logReader{
		logger: logger,
		offset: 0,
	}
}

// Read implements the io.Reader interface
func (lr *logReader) Read(p []byte) (n int, err error) {
	lr.logger.mutex.RLock()
	defer lr.logger.mutex.RUnlock()
	if !lr.logger.closed {
		lr.logger.cond.Wait()
	}
	if lr.offset >= int64(lr.logger.buffer.Len()) {
		if lr.logger.closed {
			return 0, io.EOF
		}
		return 0, nil
	}

	n = copy(p, lr.logger.buffer.Bytes()[lr.offset:])
	lr.offset += int64(n)
	return n, nil
}
