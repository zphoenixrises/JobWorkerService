package jobworker

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/rand"
)

func TestConcurrentWriteAndRead(t *testing.T) {
	writer := NewLogger()
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			data := []byte("test")
			_, err := writer.Write(data)
			if err != nil {
				t.Errorf("Error writing data: %v", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		writer.Close()
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			reader := NewLogReader(writer)
			bufferSize := rand.Intn(20) + 1 // Random buffer size between 1 and 20 bytes
			buffer := make([]byte, bufferSize)
			var readData bytes.Buffer

			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					readData.Write(buffer[:n])
				}
				if err == io.EOF {
					break
				}
				// Simulate different reading speeds
				time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond)
			}
			assert.True(t, bytes.Equal(readData.Bytes(), writer.buffer.Bytes()))
		}(i)
	}

	wg.Wait()

	var buffer bytes.Buffer

	io.Copy(&buffer, NewLogReader(writer))

	assert.True(t, bytes.Equal(buffer.Bytes(), writer.buffer.Bytes()))
}
