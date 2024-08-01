package jobworker

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOutput(t *testing.T) {
	t.Run("Write and ReadAt", func(t *testing.T) {
		output := NewLogger()
		data := []byte("Hello, World!")

		n, err := output.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(data) {
			t.Fatalf("Write returned wrong length: got %d, want %d", n, len(data))
		}

		buf := make([]byte, len(data))
		n, err = output.ReadAt(buf, 0)
		if err != nil {
			t.Fatalf("ReadAt failed: %v", err)
		}
		if n != len(data) {
			t.Fatalf("ReadAt returned wrong length: got %d, want %d", n, len(data))
		}
		if !bytes.Equal(buf, data) {
			t.Fatalf("ReadAt returned wrong data: got %q, want %q", buf, data)
		}
	})

	t.Run("Close", func(t *testing.T) {
		output := NewLogger()
		if err := output.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		_, err := output.Write([]byte("test"))
		if err != io.ErrClosedPipe {
			t.Fatalf("Write after close didn't return ErrClosedPipe: got %v", err)
		}
	})
}

func TestOutputReader(t *testing.T) {
	t.Run("Read", func(t *testing.T) {
		output := NewLogger()
		reader := NewLogReader(output)

		data := "Hello, World!"
		done := make(chan struct{})

		go func() {
			time.Sleep(100 * time.Millisecond)
			output.Write([]byte(data))
			output.Close()
			close(done)
		}()

		buf := new(strings.Builder)
		_, err := io.Copy(buf, reader)
		if err != nil {
			t.Fatalf("io.Copy failed: %v", err)
		}

		<-done

		if buf.String() != data {
			t.Fatalf("Read returned wrong data: got %q, want %q", buf.String(), data)
		}
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		output := NewLogger()
		data := "Hello, World!"
		numReaders := 5

		var wg sync.WaitGroup
		wg.Add(numReaders)

		for i := 0; i < numReaders; i++ {
			go func() {
				defer wg.Done()
				reader := NewLogReader(output)
				buf := new(strings.Builder)
				_, err := io.Copy(buf, reader)
				if err != nil {
					t.Errorf("io.Copy failed: %v", err)
					return
				}
				if buf.String() != data {
					t.Errorf("Read returned wrong data: got %q, want %q", buf.String(), data)
				}
			}()
		}

		time.Sleep(100 * time.Millisecond)
		output.Write([]byte(data))
		output.Close()

		wg.Wait()
	})
}
