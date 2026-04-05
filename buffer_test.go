package httpx

import (
	"bytes"
	"slices"
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
