package httpx

import (
	"io"
	"os"
	"sync"
)

// TempDir is a pathname of the directory to use for temporary files. Empty string (default value)
// means that the files are created in the directory returned from [os.TempDir] function.
var TempDir string

type buffer struct {
	wi   int
	file *os.File
	buff [64 * 1024]byte
}

func allocBuffer() *buffer {
	return bufferPool.Get().(*buffer)
}

// Write implements io.Writer interface.
func (b *buffer) Write(data []byte) (n int, err error) {
	switch {
	case len(data) == 0:
		return

	case b.wi+len(data) <= cap(b.buff):
		n = copy(b.buff[b.wi:], data)
		b.wi += n
		return
	}

	// create temp. file if not yet
	if b.file == nil {
		if b.file, err = os.CreateTemp(TempDir, "http-buffer-"); err != nil {
			return
		}
	}

	// write existing bytes
	if b.wi > 0 {
		if _, err = b.file.Write(b.buff[:b.wi]); err != nil {
			return
		}

		b.wi = 0
	}

	// write the data
	if len(data) < 1024 {
		n = copy(b.buff[:], data)
		b.wi = n
		return
	}

	return b.file.Write(data)
}

func (b *buffer) writeTo(w io.Writer) (err error) {
	if b.file == nil {
		_, err = w.Write(b.buff[:b.wi])
	} else {
		_, err = b.file.WriteTo(w)
	}

	return
}

func (b *buffer) complete() (n int64, err error) {
	if b.file == nil {
		n = int64(b.wi)
		return
	}

	// write remaining bytes
	if b.wi > 0 {
		if _, err = b.file.Write(b.buff[:b.wi]); err != nil {
			return
		}

		b.wi = 0
	}

	// file size
	if n, err = b.file.Seek(0, io.SeekCurrent); err == nil {
		// prepare for writeTo() call
		_, err = b.file.Seek(0, io.SeekStart)
	}

	return
}

func (b *buffer) recycle() {
	if b.file != nil {
		b.file.Close()
		os.Remove(b.file.Name())
		b.file = nil
	}

	b.wi = 0
	bufferPool.Put(b)
}

var bufferPool = sync.Pool{
	New: func() any {
		return new(buffer)
	},
}
