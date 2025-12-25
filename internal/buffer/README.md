# Buffer

A generic, GC-friendly buffer package for task queues.

## Components

### buffer[T]

Fixed-size linear buffer chunk. Single-use, no index wrapping.

```text
[0] [1] [2] [3] [4]
     ^       ^
  readIdx  writeIdx
```

- **Write**: Appends at `writeIdx`, returns `ErrFull` when full
- **Read**: Returns from `readIdx`, clears slot for GC, returns `ErrEmpty` when exhausted

```go
buf := NewBuffer[int](3)
buf.Write(10)  // [10, _, _]
buf.Write(20)  // [10, 20, _]
v, _ := buf.Read()  // v=10, [0, 20, _]
```

### LinkedBuffer[T]

Unbounded FIFO queue using linked `buffer` chunks. Designed for single-producer single-consumer.

```text
[chunk1] -> [chunk2] -> [chunk3] -> nil
    ^                       ^
  head                    tail
```

- **Write**: Appends to `tail`, allocates new chunk on full (capacity doubles up to `maxCap`)
- **Read**: Returns from `head`, advances to next chunk when empty, releases old chunk for GC

```go
lb := NewLinkedBuffer[int](2, 8)  // initCap=2, maxCap=8
lb.Write(1)
lb.Write(2)
lb.Write(3)  // triggers new chunk (cap=4)

v, _ := lb.Read()  // v=1
lb.Len()           // 2
```

#### **Growth Strategy**

| Current Cap | New Cap                |
|-------------|------------------------|
| < 1024      | `min(cap*2, maxCap)`   |
| ≥ 1024      | `min(cap*1.5, maxCap)` |

## Thread Safety

- `buffer`: Not thread-safe
- `LinkedBuffer`: Single-producer single-consumer only
