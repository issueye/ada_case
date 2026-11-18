//go:build windows

package shm

import "os"

// mapFile leaves the file unmapped on Windows: Map.Read and Map.Write go
// through the file's page cache, which every process that opens the file
// shares. Mapping it would need MapViewOfFile's uintptr as an unsafe.Pointer,
// which go vet refuses; see the package comment.
func mapFile(f *os.File, size int) (*Map, error) {
	return &Map{size: size}, nil
}
