package buffer_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/qntx/pond/internal/buffer"
)

func TestBuffer(t *testing.T) {
	buf := buffer.NewBuffer[int](10)

	if buf.Cap() != 10 {
		t.Errorf("Cap() = %d, want 10", buf.Cap())
	}

	// Read should return zero value and ErrEmpty
	value, err := buf.Read()
	if value != 0 {
		t.Errorf("Read() value = %d, want 0", value)
	}
	if err != buffer.ErrEmpty {
		t.Errorf("Read() err = %v, want ErrEmpty", err)
	}

	// Write should succeed
	if err := buf.Write(1); err != nil {
		t.Errorf("Write(1) err = %v, want nil", err)
	}
	if err := buf.Write(2); err != nil {
		t.Errorf("Write(2) err = %v, want nil", err)
	}

	// Read should return written values
	value, err = buf.Read()
	if value != 1 || err != nil {
		t.Errorf("Read() = (%d, %v), want (1, nil)", value, err)
	}

	value, err = buf.Read()
	if value != 2 || err != nil {
		t.Errorf("Read() = (%d, %v), want (2, nil)", value, err)
	}

	// Read should return zero and ErrEmpty
	value, err = buf.Read()
	if value != 0 || err != buffer.ErrEmpty {
		t.Errorf("Read() = (%d, %v), want (0, ErrEmpty)", value, err)
	}
}

func TestBufferFull(t *testing.T) {
	buf := buffer.NewBuffer[int](3)

	for i := range 3 {
		if err := buf.Write(i); err != nil {
			t.Fatalf("Write(%d) err = %v", i, err)
		}
	}

	if err := buf.Write(99); err != buffer.ErrFull {
		t.Errorf("Write on full buffer err = %v, want ErrFull", err)
	}

	if buf.Len() != 3 {
		t.Errorf("Len() = %d, want 3", buf.Len())
	}
}

func TestBufferWithPointerToLargeObject(t *testing.T) {
	var m runtime.MemStats

	type Payload struct {
		data []byte
	}

	dataSize := 10 * 1024 * 1024
	data := &Payload{
		data: make([]byte, dataSize),
	}

	buf := buffer.NewBuffer[*Payload](10)

	runtime.ReadMemStats(&m)
	before := int64(m.Alloc)

	buf.Write(data)

	runtime.ReadMemStats(&m)
	after := int64(m.Alloc)

	// Verify large object wasn't copied when writing
	if after-before >= int64(dataSize) {
		t.Errorf("Write copied large object: alloc diff = %d", after-before)
	}

	runtime.ReadMemStats(&m)
	before = int64(m.Alloc)

	readData, err := buf.Read()

	runtime.ReadMemStats(&m)
	after = int64(m.Alloc)

	// Verify large object wasn't copied while reading
	if err != nil {
		t.Fatalf("Read() err = %v", err)
	}
	if cap(readData.data) != cap(data.data) {
		t.Errorf("Read data cap = %d, want %d", cap(readData.data), cap(data.data))
	}
	if after-before >= int64(dataSize) {
		t.Errorf("Read copied large object: alloc diff = %d", after-before)
	}

	runtime.ReadMemStats(&m)
	before = int64(m.Alloc)

	// Remove references to the large object
	data.data = make([]byte, 0)
	readData.data = make([]byte, 0)

	runtime.GC()

	runtime.ReadMemStats(&m)
	after = int64(m.Alloc)

	// Verify large object was garbage collected
	if before-after < int64(dataSize)/2 {
		t.Errorf("Large object not GC'd: freed = %d, want >= %d", before-after, dataSize/2)
	}

	// Keep references to prevent premature GC
	fmt.Printf("%#v, %#v\n", data.data, readData.data)
}
