package buffer

import (
	"math"
	"sync/atomic"
)

// LinkedBuffer is an unbounded FIFO buffer using linked chunks.
// Designed for single-producer single-consumer use.
type LinkedBuffer[T any] struct {
	head *buffer[T] // read position
	tail *buffer[T] // write position

	maxCap int

	writes atomic.Uint64
	reads  atomic.Uint64
}

// NewLinkedBuffer creates a LinkedBuffer with initial and max chunk capacity.
func NewLinkedBuffer[T any](initCap, maxCap int) *LinkedBuffer[T] {
	if initCap <= 0 {
		initCap = 16
	}
	if maxCap < initCap {
		maxCap = initCap
	}
	chunk := NewBuffer[T](initCap)
	return &LinkedBuffer[T]{
		head:   chunk,
		tail:   chunk,
		maxCap: maxCap,
	}
}

// Len returns approximate unread count.
func (lb *LinkedBuffer[T]) Len() uint64 {
	w, r := lb.writes.Load(), lb.reads.Load()
	if w >= r {
		return w - r
	}
	return math.MaxUint64 - r + w + 1
}

// Write appends a value. Allocates new chunk if current is full.
func (lb *LinkedBuffer[T]) Write(v T) {
	for lb.tail.Write(v) == ErrFull {
		if lb.tail.next == nil {
			lb.tail.next = NewBuffer[T](lb.nextCap())
		}
		lb.tail = lb.tail.next
	}
	lb.writes.Add(1)
}

// Read retrieves the next value. Returns ErrEmpty if buffer is empty.
func (lb *LinkedBuffer[T]) Read() (T, error) {
	for {
		v, err := lb.head.Read()
		if err != ErrEmpty {
			lb.reads.Add(1)
			return v, nil
		}
		if lb.head.next == nil {
			var zero T
			return zero, ErrEmpty
		}
		// Advance and release old chunk for GC
		old := lb.head
		lb.head = lb.head.next
		old.next = nil // break reference
	}
}

// nextCap calculates next chunk capacity with growth strategy.
func (lb *LinkedBuffer[T]) nextCap() int {
	cur := lb.tail.Cap()
	if cur < 1024 {
		return min(cur*2, lb.maxCap)
	}
	return min(cur+cur/2, lb.maxCap)
}
