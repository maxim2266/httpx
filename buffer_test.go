package httpx

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"testing"
)

func TestBufferSmall(t *testing.T) {
	data := []byte("this is a test")
	b := allocBuffer()

	defer b.recycle()

	n, err := b.Write(data)

	if err != nil {
		t.Fatalf("Write: %s", err)
	}

	if n != len(data) {
		t.Fatalf("Write: length %d instead of %d", n, len(data))
	}

	size, err := b.flush()

	if err != nil {
		t.Fatalf("flush: %s", err)
	}

	if size != int64(len(data)) {
		t.Fatalf("flush: length %d instead of %d", size, len(data))
	}

	var res bytes.Buffer

	if err = b.writeTo(&res); err != nil {
		t.Fatalf("writeTo: %s", err)
	}

	if s := res.Bytes(); !bytes.Equal(s, data) {
		t.Fatalf(`result: "%s" instead of "%s"`, s, data)
	}
}

func TestBufferLarge(t *testing.T) {
	const N = 32*1024 + 5

	data := []byte("_xyz/")
	b := allocBuffer()

	defer b.recycle()

	for i := range N {
		n, err := b.Write(data)

		if err != nil {
			t.Fatalf("(%d) Write: %s", i, err)
		}

		if n != len(data) {
			t.Fatalf("(%d) Write: length %d instead of %d", i, n, len(data))
		}
	}

	n, err := b.flush()

	if err != nil {
		t.Fatalf("flush: %s", err)
	}

	if data = slices.Repeat(data, N); n != int64(len(data)) {
		t.Fatalf("flush: length %d instead of %d", n, len(data))
	}

	var res bytes.Buffer

	if err = b.writeTo(&res); err != nil {
		t.Fatalf("writeTo: %s", err)
	}

	if s := res.Bytes(); !bytes.Equal(s, data) {
		t.Fatalf("result: mismatch, length %d (expected %d)", len(s), len(data))
	}
}

// benchmark specifically the buffer's file backing behavior
func BenchmarkBufferFileBacking(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"UnderBuffer_32KB", 32 * 1024},
		{"ExactBuffer_64KB", 64 * 1024},
		{"JustOverBuffer_65KB", 65 * 1024},
		{"DoubleBuffer_128KB", 128 * 1024},
		{"QuadBuffer_256KB", 256 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			testData := bytes.Repeat([]byte("x"), size.bytes)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf := allocBuffer()

				// Write data
				written, err := buf.Write(testData)
				if err != nil {
					b.Fatalf("Write failed: %v", err)
				}
				if written != size.bytes {
					b.Fatalf("write mismatch: got %d, want %d", written, size.bytes)
				}

				// Flush to get size
				length, err := buf.flush()
				if err != nil {
					b.Fatalf("Flush failed: %v", err)
				}
				if int(length) != size.bytes {
					b.Fatalf("length mismatch: got %d, want %d", length, size.bytes)
				}

				// Write to a discard writer
				err = buf.writeTo(io.Discard)
				if err != nil {
					b.Fatalf("writeTo failed: %v", err)
				}

				buf.recycle()
			}
		})
	}
}

// compare memory-only vs file-backed for different sizes
func BenchmarkBufferMemoryVsFile(b *testing.B) {
	sizes := []int{32 * 1024, 64 * 1024, 65 * 1024, 128 * 1024, 256 * 1024, 512 * 1024}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			testData := bytes.Repeat([]byte("x"), size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf := allocBuffer()
				buf.Write(testData)
				buf.flush()
				buf.writeTo(io.Discard)
				buf.recycle()
			}
		})
	}
}

// helper function
func formatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return strconv.Itoa(bytes) + "B"
	case bytes < 1024*1024:
		return strconv.Itoa(bytes/1024) + "KB"
	case bytes < 1024*1024*1024:
		return strconv.Itoa(bytes/(1024*1024)) + "MB"
	default:
		return strconv.Itoa(bytes/(1024*1024*1024)) + "GB"
	}
}
