package httpx

import (
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"unsafe"
)

// TempDir is a pathname of the directory to use for temporary files. Empty string (default value)
// means that the files are created in the directory returned from [os.TempDir] function.
var TempDir string

const httpBufferSize = 64 * 1024

type buffer struct {
	wi   int
	file *os.File
	buff [httpBufferSize]byte
}

func allocBuffer() *buffer {
	return bufferPool.Get().(*buffer)
}

// Write implements [io.Writer] interface.
func (b *buffer) Write(data []byte) (n int, err error) {
	switch {
	case len(data) == 0:
		return

	case b.wi+len(data) <= cap(b.buff):
		n = copy(b.buff[b.wi:], data)
		b.wi += n
		return
	}

	// create temporary file if not yet
	if b.file == nil {
		if b.file, err = os.CreateTemp(TempDir, "http-buffer-"); err != nil {
			return
		}
	}

	// write existing bytes
	if err = write(b.file, b.buff[:b.wi]); err != nil {
		return
	}

	// write the data
	if len(data) <= 2*1024 {
		n = copy(b.buff[:], data)
		b.wi = n
		return
	}

	b.wi = 0

	if err = write(b.file, data); err != nil {
		return
	}

	n = len(data)
	return
}

// WriteString implements [io.StringWriter] interface.
func (b *buffer) WriteString(s string) (int, error) {
	return b.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
}

func (b *buffer) writeTo(w io.Writer) (err error) {
	// buffer-only write
	if b.file == nil {
		return write(w, b.buff[:b.wi])
	}

	// rewind the file
	if _, err = b.file.Seek(0, io.SeekStart); err != nil {
		return
	}

	// for HTTPS/TLS file.WriteTo allocates a buffer for copying,
	// but here we have already got the buffer we can use
	buff := b.buff[:]

	// unfortunately, io.CopyBuffer still wants to call file.WriteTo method,
	// so we have to implement the copy loop here
	var nr int

	for nr, err = b.file.Read(buff); err == nil; nr, err = b.file.Read(buff) {
		if err = write(w, buff[:nr]); err != nil {
			return
		}
	}

	if err == io.EOF {
		err = nil
	}

	return
}

func (b *buffer) flush() (int64, error) {
	if b.file == nil {
		return int64(b.wi), nil
	}

	// write remaining bytes
	if err := write(b.file, b.buff[:b.wi]); err != nil {
		return 0, err
	}

	b.wi = 0

	// file size
	return b.file.Seek(0, io.SeekCurrent)
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

func write(w io.Writer, buff []byte) error {
	if len(buff) == 0 {
		return nil
	}

	n, err := w.Write(buff)

	if err != nil {
		return err
	}

	// I'm not sure about this check, but it's present in the standard io.Copy routines
	if n != len(buff) {
		return errors.New("invalid write: wanted " +
			strconv.Itoa(len(buff)) +
			" bytes, but actually wrote " +
			strconv.Itoa(n))
	}

	return nil
}

var bufferPool = sync.Pool{
	New: func() any {
		return new(buffer)
	},
}
