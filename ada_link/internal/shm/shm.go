// Package shm maps a file into memory shared by two processes: the child
// writes frames into it, the host reads them, on macOS, Linux and Windows.
//
// A mapped file is not a POSIX shared-memory object: the child creates a
// file in the session's directory, and both sides map it. A file's page
// cache is shared by every process that maps or reads it, so what the child
// writes before it tells the host about it, the host reads.
//
// Unix maps the file with mmap, so Bytes returns the pages themselves and a
// frame costs no copy on either side. Windows, whose mapping needs a
// uintptr-to-pointer conversion pure Go vet refuses without a //go:nocheckptr
// escape, reads and writes the file through the page cache instead:
// Read/Write copy, Bytes is nil. The design notes MapViewOfFile as the
// production path (docs/protocol.md).
package shm

import (
	"fmt"
	"os"
)

// Map is a mapped file.
type Map struct {
	File *os.File
	data []byte
	size int
	// unmap is the platform's Munmap, nil where the file is not mapped.
	unmap func(data []byte) error
}

// Bytes returns the mapped bytes, or nil where the platform reads and
// writes the file instead. They live until Close.
func (m *Map) Bytes() []byte { return m.data }

// Size returns the length of the file.
func (m *Map) Size() int { return m.size }

// Read copies len(p) bytes at off into p.
func (m *Map) Read(p []byte, off int64) error {
	if m.data != nil {
		if off < 0 || off+int64(len(p)) > int64(len(m.data)) {
			return fmt.Errorf("shm: read %d bytes at %d is outside %d", len(p), off, len(m.data))
		}
		copy(p, m.data[off:])
		return nil
	}
	_, err := m.File.ReadAt(p, off)
	return err
}

// Write copies p to off.
func (m *Map) Write(p []byte, off int64) error {
	if m.data != nil {
		if off < 0 || off+int64(len(p)) > int64(len(m.data)) {
			return fmt.Errorf("shm: write %d bytes at %d is outside %d", len(p), off, len(m.data))
		}
		copy(m.data[off:], p)
		return nil
	}
	_, err := m.File.WriteAt(p, off)
	return err
}

// Close unmaps the file and closes it. The bytes are invalid afterwards.
func (m *Map) Close() error {
	var err error
	if m.data != nil {
		err = m.unmap(m.data)
		m.data = nil
	}
	if cerr := m.File.Close(); err == nil {
		err = cerr
	}
	return err
}

// Create truncates path to size and opens it read-write. Permissions are
// 0600: only the user who started the host and its child can read frames.
func Create(path string, size int) (*Map, error) {
	if size <= 0 {
		return nil, fmt.Errorf("shm: size %d", size)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(int64(size)); err != nil {
		f.Close()
		return nil, err
	}
	m, err := mapFile(f, size)
	if err != nil {
		f.Close()
		return nil, err
	}
	m.File = f
	return m, nil
}

// Open opens an existing file of size bytes read-write.
func Open(path string, size int) (*Map, error) {
	if size <= 0 {
		return nil, fmt.Errorf("shm: size %d", size)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	// A file another process truncated may be smaller than the mapping asked
	// for: refuse rather than map bytes that nothing backs.
	if fi, err := f.Stat(); err != nil {
		f.Close()
		return nil, err
	} else if fi.Size() < int64(size) {
		f.Close()
		return nil, fmt.Errorf("shm: %s is %d bytes, not %d", path, fi.Size(), size)
	}
	m, err := mapFile(f, size)
	if err != nil {
		f.Close()
		return nil, err
	}
	m.File = f
	return m, nil
}
