package buffer

import "errors"

// ErrFull is returned when the buffer is full and cannot accept more writes.
// ErrEmpty is returned when the buffer is empty and cannot provide more reads.
var (
	ErrFull  = errors.New("buffer full")
	ErrEmpty = errors.New("buffer empty")
)

// buffer is a fixed-size chunk in a linked buffer chain.
// It stores elements in a linear slice without wrapping indices.
// Not thread-safe.
type buffer[T any] struct {
	data     []T
	writeIdx int
	readIdx  int
	next     *buffer[T]
	zero     T // zero value for clearing read slots
}

// NewBuffer creates a new buffer chunk with the given capacity.
func NewBuffer[T any](capacity int) *buffer[T] {
	return &buffer[T]{
		data: make([]T, capacity),
	}
}

// Cap returns the capacity of this buffer chunk.
func (b *buffer[T]) Cap() int {
	return cap(b.data)
}

// Len returns the number of readable elements in this buffer chunk.
func (b *buffer[T]) Len() int {
	return b.writeIdx - b.readIdx
}

// Write appends a value to this buffer chunk.
// Returns ErrFull if the chunk has no remaining space.
func (b *buffer[T]) Write(value T) error {
	if b.writeIdx >= b.Cap() {
		return ErrFull
	}
	b.data[b.writeIdx] = value
	b.writeIdx++
	return nil
}

// Read retrieves the next value from this buffer chunk.
// Returns ErrEmpty if no more elements are available in this chunk
// (either fully read or empty and no next chunk).
func (b *buffer[T]) Read() (T, error) {
	if b.readIdx >= b.writeIdx {
		return b.zero, ErrEmpty
	}
	value := b.data[b.readIdx]
	// Clear reference to allow GC of large elements (e.g., tasks).
	// Reference: common practice in task queues to prevent leaks.
	b.data[b.readIdx] = b.zero
	b.readIdx++
	return value, nil
}
