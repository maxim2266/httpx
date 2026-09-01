package httpx

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

func TestBuffer(t *testing.T) {
	sizes := [...]int{
		0,
		2749,
		4096,
		7919,
		httpBufferSize - 1,
		httpBufferSize,
		httpBufferSize + 1,
		httpBufferSize + 7919,
		2*httpBufferSize - 1,
		2 * httpBufferSize,
		2*httpBufferSize + 1,
		2*httpBufferSize + 7919,
	}

	data := make([]byte, sizes[len(sizes)-1])

	for i := range len(data) {
		data[i] = 'a' + byte(rand.Int()%26)
	}

	var res bytes.Buffer

	res.Grow(len(data))

	for no, size := range sizes {
		src := data[len(data)-size:] // ensure different data at the start of the buffer
		b := allocBuffer()

		// chunked write
		chunkSize := size / 7

		for i := 0; i < size; i += chunkSize {
			nreq := min(chunkSize, size-i)
			nact, err := b.Write(src[i : i+nreq])

			if err != nil {
				t.Fatalf("(%d) Write @ %d: %s", no, i, err)
			}

			if nact != nreq {
				t.Fatalf("(%d) Write @ %d: written %d bytes instead of %d", no, i, nact, nreq)
			}
		}

		// flush
		count, err := b.flush()

		if err != nil {
			t.Fatalf("(%d) flush: %s", no, err)
		}

		if int64(size) != count {
			t.Fatalf("(%d) flush: written %d bytes instead of %d", no, count, size)
		}

		// write result
		res.Reset()

		if err = b.writeTo(&res); err != nil {
			t.Fatalf("(%d) writeTo: %s", no, err)
		}

		// compare
		if !bytes.Equal(src, res.Bytes()) {
			os.WriteFile("TestBuffer.in", src, 0600)
			os.WriteFile("TestBuffer.out", res.Bytes(), 0600)
			t.Fatalf("(%d) writeTo: data mismatch", no)
		}

		// recycle
		var tmp string

		if b.file != nil {
			tmp = b.file.Name()
		}

		b.recycle()

		if len(tmp) > 0 {
			if _, err = os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf(`(%d) temporary file "%s" is not deleted: %s`, no, tmp, err)
			}
		}
	}
}

func TestBufferString(t *testing.T) {
	b := allocBuffer()

	defer b.recycle()

	data := "this is a test"
	n, err := b.WriteString(data)

	if err != nil {
		t.Fatalf("WriteString: %s", err)
	}

	if n != len(data) {
		t.Fatalf("WriteString: length %d instead of %d", n, len(data))
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

	if s := res.String(); s != data {
		t.Fatalf(`result: "%s" instead of "%s"`, s, data)
	}
}

// compare memory-only vs file-backed for different sizes
func BenchmarkBufferMemoryVsFile(b *testing.B) {
	sizes := []int{
		32 * 1024,
		64 * 1024,
		65 * 1024,
		128 * 1024,
		256 * 1024,
		512 * 1024,
		1024 * 1024,
		2 * 1024 * 1024,
		4 * 1024 * 1024,
		8 * 1024 * 1024,
		16 * 1024 * 1024,
		32 * 1024 * 1024,
	}

	data := bytes.Repeat([]byte("x"), sizes[len(sizes)-1])

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf := allocBuffer()
				buf.Write(data[:size])
				buf.flush()
				buf.writeTo(io.Discard)
				buf.recycle()
			}
		})
	}
}

// helpers
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
