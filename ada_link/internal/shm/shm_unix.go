//go:build !windows

package shm

import (
	"fmt"
	"os"
	"syscall"
)

// mapFile maps the file f read-write with mmap(MAP_SHARED), so that what
// one process writes another reads without copying it either side.
func mapFile(f *os.File, size int) (*Map, error) {
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("shm: mmap: %w", err)
	}
	return &Map{
		data: data,
		size: size,
		unmap: func(data []byte) error {
			return syscall.Munmap(data)
		},
	}, nil
}
