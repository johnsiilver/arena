package arena

import (
	"errors"
	"io"
	"sync"
)

// Reader reads from a block of bytes that came from an Arena. It implements io.Reader.
// It is not safe to hold a reference to the []byte and not hold one to the Reader you got it from.
// IT IS NOT SAFE TO MODIFY THE UNDERLYING SLICE IN A WAY THAT CHANGES THE SIZE OF THE SLICE.
// DO NOT USE APPEND.
type Reader struct {
	block []byte
	at    int

	once sync.Once
	wg   *sync.WaitGroup
}

// Read reads up to len(b) bytes into b. It returns the number of bytes read.
func (r *Reader) Read(b []byte) (int, error) {
	if r.at >= len(r.block) {
		return 0, io.EOF
	}

	n := copy(b, r.block[r.at:])
	r.at += n
	if r.at == len(r.block) {
		return n, io.EOF
	}
	return n, nil
}

// Len returns the number of bytes of the unread portion of the buffer; b.Len() == len(b.Bytes()).
func (r *Reader) Len() int {
	return len(r.block) - r.at
}

// Cap returns the capacity of the buffer's underlying byte slice, that is, the total space allocated for the buffer's data.
func (r *Reader) Cap() int {
	return cap(r.block)
}

// Peek allows peeking ahead n bytes without moving the Reader forward. This will return the next n bytes
// from the underlying slice.
func (r *Reader) Peek(n int) ([]byte, error) {
	if r.at+n > len(r.block) {
		return nil, errors.New("block is not big enough to peek ahead that far")
	}
	return r.block[r.at : r.at+n], nil
}

// Bytes returns a slice of length b.Len() holding the unread portion of the buffer. The slice is valid for use only until
// the next buffer modification (such as Read()).
func (r *Reader) Bytes() []byte {
	return r.block[r.at:]
}

// Block returns the underlying block of bytes.
func (r *Reader) Block() []byte {
	return r.block
}

// Release releases the block back to the arena. You must call this when you are done with the Reader.
// If you are using the underlying []byte, you must not use it after calling Release.
func (r *Reader) Release() {
	if r.wg == nil {
		return
	}
	r.once.Do(
		func() {
			r.wg.Done()
		},
	)
}

// Writer writes to a block of bytes that came from an Arena. It will not allow you to
// write more than the block size. If you need to write more, you will need to get a new
// block from the Arena. It is not safe to hold a reference to the []byte and not hold one
// to the Reader you got it from.
// IT IS NOT SAFE TO MODIFY THE UNDERLYING SLICE IN A WAY THAT CHANGES THE SIZE OF THE SLICE.
// DO NOT USE APPEND.
type Writer struct {
	block []byte
	at    int

	once sync.Once
	wg   *sync.WaitGroup
}

// Write writes len(b) bytes from b to the underlying data stream. It returns the number of bytes written.
// If the length of b is greater than the block size, you will get an error.
func (w *Writer) Write(b []byte) (int, error) {
	if w.at >= len(w.block) {
		return 0, io.ErrShortWrite
	}

	if len(b) > len(w.block)-w.at {
		copy(w.block[w.at:], b[:len(w.block)-w.at])
		return len(w.block) - w.at, io.ErrShortWrite
	}

	n := copy(w.block[w.at:], b)
	w.at += n
	return n, nil
}

// Len returns the number of bytes of the unwritten portion of the buffer.
func (w *Writer) Len() int {
	return len(w.block) - w.at
}

// Size returns the size of the underlying block.
func (w *Writer) Cap() int {
	return len(w.block)
}

// Block returns the underlying block of bytes.
func (w *Writer) Block() []byte {
	return w.block
}

// Reader returns a Reader for the block of bytes stored in Writer reading from the
// 0 position. IT IS UNSAFE TO USE THE WRITER AFTER CALLING THIS.
func (w *Writer) Reader() Reader {
	w.wg.Add(1) // Add for the reader.
	w.Release() // Release the writer.

	return Reader{
		block: w.block,
		wg:    w.wg,
	}
}

// Reset resets the Writer to the beginning of the block.
func (w *Writer) Reset() {
	w.at = 0
}

// Release releases the block back to the arena. You must call this when you are done with the Writer.
// If you are using the underlying []byte, you must not use it after calling Release. If you called
// Reader(), you must call Release on the Reader and not here.
func (w *Writer) Release() {
	if w.wg == nil {
		return
	}
	w.once.Do(
		func() {
			w.wg.Done()
		},
	)
}
