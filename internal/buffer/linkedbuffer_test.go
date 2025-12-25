package buffer_test

import (
	"testing"

	"github.com/qntx/pond/internal/buffer"
)

func TestLinkedBuffer(t *testing.T) {
	buf := buffer.NewLinkedBuffer[int](2, 2)

	if buf.Len() != 0 {
		t.Errorf("Len() = %d, want 0", buf.Len())
	}

	buf.Write(1)
	buf.Write(2)

	if buf.Len() != 2 {
		t.Errorf("Len() = %d, want 2", buf.Len())
	}

	value, err := buf.Read()
	if err != nil || value != 1 {
		t.Errorf("Read() = (%d, %v), want (1, nil)", value, err)
	}

	value, err = buf.Read()
	if err != nil || value != 2 {
		t.Errorf("Read() = (%d, %v), want (2, nil)", value, err)
	}

	// Test ErrEmpty
	value, err = buf.Read()
	if value != 0 || err != buffer.ErrEmpty {
		t.Errorf("Read() = (%d, %v), want (0, ErrEmpty)", value, err)
	}

	buf.Write(3)

	if buf.Len() != 1 {
		t.Errorf("Len() = %d, want 1", buf.Len())
	}
}

func TestLinkedBufferChunkGrowth(t *testing.T) {
	buf := buffer.NewLinkedBuffer[int](2, 8)

	// Write more than initial capacity to trigger chunk allocation
	for i := range 10 {
		buf.Write(i)
	}

	if buf.Len() != 10 {
		t.Errorf("Len() = %d, want 10", buf.Len())
	}

	// Read all and verify FIFO order
	for i := range 10 {
		v, err := buf.Read()
		if err != nil || v != i {
			t.Errorf("Read() = (%d, %v), want (%d, nil)", v, err, i)
		}
	}

	if buf.Len() != 0 {
		t.Errorf("Len() = %d, want 0", buf.Len())
	}
}

func TestLinkedBufferMultipleChunks(t *testing.T) {
	buf := buffer.NewLinkedBuffer[int](2, 2) // Fixed size chunks

	// Fill multiple chunks
	buf.Write(1)
	buf.Write(2)
	buf.Write(3) // Triggers new chunk
	buf.Write(4)
	buf.Write(5) // Triggers another chunk

	if buf.Len() != 5 {
		t.Errorf("Len() = %d, want 5", buf.Len())
	}

	// Read across chunk boundaries
	for i := 1; i <= 5; i++ {
		v, err := buf.Read()
		if err != nil || v != i {
			t.Errorf("Read() = (%d, %v), want (%d, nil)", v, err, i)
		}
	}

	// Should be empty
	_, err := buf.Read()
	if err != buffer.ErrEmpty {
		t.Errorf("Read() err = %v, want ErrEmpty", err)
	}
}

func TestLinkedBufferDefaultCapacity(t *testing.T) {
	// Test with invalid initial capacity
	buf := buffer.NewLinkedBuffer[int](0, 100)

	// Should use default (16)
	for i := range 20 {
		buf.Write(i)
	}

	if buf.Len() != 20 {
		t.Errorf("Len() = %d, want 20", buf.Len())
	}
}

func TestLinkedBufferMaxCapAdjustment(t *testing.T) {
	// maxCap < initCap should be adjusted
	buf := buffer.NewLinkedBuffer[int](10, 5)

	for i := range 25 {
		buf.Write(i)
	}

	if buf.Len() != 25 {
		t.Errorf("Len() = %d, want 25", buf.Len())
	}
}
