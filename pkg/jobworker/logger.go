package jobworker

import (
	"bytes"
	"io"
	"log"
	"sync"
)

type logger struct {
	buffer bytes.Buffer
	mutex  sync.RWMutex
	cond   *sync.Cond
	closed bool
}

func newLogger() *logger {
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
	log.Println("Logger", string(p))

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

// newLogReader creates a new Reader for the given logger
func newLogReader(logger *logger) io.Reader {
	return &logReader{
		logger: logger,
		offset: 0,
	}
}

// Read implements the io.Reader interface
func (lr *logReader) Read(p []byte) (n int, err error) {
	lr.logger.mutex.RLock()
	defer lr.logger.mutex.RUnlock()
	// Read any bytes that are already in the buffer
	// Typically, this will happen when the reader has requested the stream
	// in the middle of data being pushed into the writer. Without this,
	// the reader will need to wait for the next broadcast from the condition variable
	if lr.offset < int64(lr.logger.buffer.Len()) {
		n = copy(p, lr.logger.buffer.Bytes()[lr.offset:])
		lr.offset += int64(n)
		return n, nil
	}

	// Wait for data to be read into the buffer
	if !lr.logger.closed {
		lr.logger.cond.Wait()
	}

	// Exit condition: If the writer is still processing, we return nothing
	// if writer is closed, we return EOF
	if lr.offset >= int64(lr.logger.buffer.Len()) {
		if lr.logger.closed {
			return 0, io.EOF
		}
		return 0, nil
	}

	// Return the data written into the buffer
	n = copy(p, lr.logger.buffer.Bytes()[lr.offset:])
	lr.offset += int64(n)
	return n, nil
}
