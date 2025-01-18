package arena

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"testing"
)

func TestPool(t *testing.T) {
	const (
		KiB = 1024
		MiB = 1048576
	)

	content := make([]byte, 1024)
	for i := 0; i < len(content); i++ {
		content[i] = byte(i % 256)
	}

	ctx := context.Background()

	pool, err := New(ctx, "testPool", 10*MiB, 1*MiB, 1)
	if err != nil {
		panic(err)
	}

	input := make(chan *Reader, 1)

	go func() {
		for i := 0; i < 20*MiB; i += KiB {
			w, err := pool.GetWriter(ctx, KiB)
			if err != nil {
				panic(err)
			}
			written, err := w.Write(content)
			if err != nil {
				panic(err)
			}
			if written != 1024 {
				panic("wrote wrong number of bytes")
			}
			written, err = w.Write(content)
			if written != 0 && err == nil {
				panic("wrote more bytes than allowed")
			}

			r := w.Reader()
			input <- &r
		}
		close(input)
	}()

	wg := sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range input { // ignore copylocks
				if !bytes.Equal(r.Bytes(), content) {
					panic("content is not the same")
				}
				r.Release()
			}
		}()
	}

	wg.Wait()
}
