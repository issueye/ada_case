package link

import (
	"os"
	"path/filepath"
	"testing"
)

// pixels makes w×h RGBA pixels of one color, plus n of another.
func pixels(w, h int, n int) []byte {
	pix := make([]byte, 4*w*h)
	for i := range pix {
		pix[i] = 0x20
	}
	for i := 0; i < n; i++ {
		pix[4*i] = 0xff
	}
	return pix
}

func TestDamageIdentical(t *testing.T) {
	w, h := 8, 4
	a := pixels(w, h, 0)
	if _, changed := Damage(a, append([]byte(nil), a...), w, h, 4*w); changed {
		t.Fatal("identical frames report damage")
	}
}

func TestDamageOnePixel(t *testing.T) {
	w, h := 8, 4
	a := pixels(w, h, 0)
	b := append([]byte(nil), a...)
	b[4*(2*w+3)] = 0xff
	box, changed := Damage(a, b, w, h, 4*w)
	if !changed {
		t.Fatal("a changed pixel reports none")
	}
	if box != (Rect{X: 3, Y: 2, W: 1, H: 1}) {
		t.Errorf("damage %+v", box)
	}
}

func TestDamageBox(t *testing.T) {
	w, h := 10, 6
	a := pixels(w, h, 0)
	b := append([]byte(nil), a...)
	for y := 1; y <= 4; y++ {
		for x := 2; x <= 7; x++ {
			b[4*(y*w+x)] = 0xff
		}
	}
	box, changed := Damage(a, b, w, h, 4*w)
	if !changed {
		t.Fatal("a changed box reports none")
	}
	if box != (Rect{X: 2, Y: 1, W: 6, H: 4}) {
		t.Errorf("damage %+v", box)
	}
}

func TestDamageMismatch(t *testing.T) {
	if _, changed := Damage([]byte{1}, []byte{1, 2}, 4, 4, 16); changed {
		t.Fatal("frames of different lengths report damage")
	}
}

func TestRingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w, h := 16, 8
	ring, err := CreateRing(dir, "f.bin", w, h, RingSlots, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	pix := pixels(w, h, 33)
	if err := ring.WriteSlot(1, pix); err != nil {
		t.Fatal(err)
	}
	host, err := OpenRing(dir, FrameInfo{File: "f.bin", Slots: RingSlots, W: w, H: h, Stride: 4 * w, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	got := make([]byte, 4*w*h)
	if err := host.ReadSlot(1, got); err != nil {
		t.Fatal(err)
	}
	for i := range got {
		if got[i] != pix[i] {
			t.Fatalf("byte %d is %#x, not %#x", i, got[i], pix[i])
		}
	}
}

func TestOpenRingRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	w, h := 4, 4
	m, err := CreateRing(dir, "ok.bin", w, h, RingSlots, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	// A file of the right size that is not a ring.
	if err := writeGarbage(dir, "bad.bin", ringHeaderSize+RingSlots*4*w*h); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRing(dir, FrameInfo{File: "bad.bin", Slots: RingSlots, W: w, H: h, Stride: 4 * w}); err == nil {
		t.Fatal("garbage opened as a ring")
	}
	if _, err := OpenRing(dir, FrameInfo{File: "../bad.bin", Slots: RingSlots, W: w, H: h, Stride: 4 * w}); err == nil {
		t.Fatal("a path opened as a ring name")
	}
}

func TestSlotBounds(t *testing.T) {
	dir := t.TempDir()
	ring, err := CreateRing(dir, "f.bin", 4, 4, RingSlots, 1)
	if err != nil {
		t.Fatal(err)
	}
	ring.Close()
	ring2, err := CreateRing(dir, "g.bin", 4, 4, RingSlots, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer ring2.Close()
	if err := ring2.WriteSlot(RingSlots, make([]byte, 64)); err == nil {
		t.Fatal("slot past the ring written")
	}
	if err := ring2.WriteSlot(0, make([]byte, 4)); err == nil {
		t.Fatal("a short frame written")
	}
}

func BenchmarkWriteSlot1280(b *testing.B) {
	dir := b.TempDir()
	w, h := 1280, 800
	ring, err := CreateRing(dir, "f.bin", w, h, RingSlots, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer ring.Close()
	pix := pixels(w, h, 1000)
	b.SetBytes(int64(len(pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ring.WriteSlot(i%RingSlots, pix); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadSlot1280(b *testing.B) {
	dir := b.TempDir()
	w, h := 1280, 800
	ring, err := CreateRing(dir, "f.bin", w, h, RingSlots, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer ring.Close()
	pix := pixels(w, h, 1000)
	if err := ring.WriteSlot(0, pix); err != nil {
		b.Fatal(err)
	}
	dst := make([]byte, len(pix))
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ring.ReadSlot(0, dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDamageUnchanged1280(b *testing.B) {
	w, h := 1280, 800
	a := pixels(w, h, 0)
	b.SetBytes(int64(len(a)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Damage(a, a, w, h, 4*w)
	}
}

// writeGarbage writes a file of size bytes in dir.
func writeGarbage(dir, name string, size int) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(make([]byte, size))
	return err
}
