package jobworker

import (
	"io"
	"sync"
)

type Logger struct {
	buffer []byte
	mutex  sync.RWMutex
	cond   *sync.Cond
	closed bool
}

func NewLogger() *Logger {
	l := &Logger{}
	l.cond = sync.NewCond(&l.mutex)
	return l
}

func (l *Logger) Write(p []byte) (n int, err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.closed {
		return 0, io.ErrClosedPipe
	}

	l.buffer = append(l.buffer, p...)
	l.cond.Broadcast()
	return len(p), nil
}

func (l *Logger) ReadAt(p []byte, off int64) (n int, err error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if off >= int64(len(l.buffer)) {
		if l.closed {
			return 0, io.EOF
		}
		return 0, nil
	}

	n = copy(p, l.buffer[off:])
	if n < len(p) && l.closed {
		err = io.EOF
	}
	return
}

func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.closed = true
	l.cond.Broadcast()
	return nil
}

type logReader struct {
	logger *Logger
	offset int64
}

func NewLogReader(logger *Logger) io.Reader {
	return &logReader{logger: logger}
}

func (lr *logReader) Read(p []byte) (n int, err error) {
	for {
		n, err = lr.logger.ReadAt(p, lr.offset)
		lr.offset += int64(n)

		if n > 0 || err != nil {
			return n, err
		}

		lr.logger.mutex.Lock()
		if lr.logger.closed {
			lr.logger.mutex.Unlock()
			return 0, io.EOF
		}
		lr.logger.cond.Wait()
		lr.logger.mutex.Unlock()
	}
}
