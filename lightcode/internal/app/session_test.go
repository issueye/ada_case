package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSessionRestoresFiles reopens the app and finds the files, the tab,
// the sidebar and the zoom of the last run.
func TestSessionRestoresFiles(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	a.ShowSidebar(2)
	a.SetZoom(1.2)

	a2 := New(dir, "")
	if len(a2.Col().Editors) != 2 {
		t.Fatalf("restored %d tabs, want 2", len(a2.Col().Editors))
	}
	if got := a2.Col().Current(); got == nil || got.Doc.Name() != "util.go" {
		t.Errorf("active tab = %v", got)
	}
	if !a2.SidebarOpen || a2.SidebarView != 2 {
		t.Errorf("sidebar = %v %d", a2.SidebarOpen, a2.SidebarView)
	}
	if a2.Zoom < 1.19 || a2.Zoom > 1.21 {
		t.Errorf("zoom = %v", a2.Zoom)
	}
}

// TestZoomCommand changes the zoom and keeps it for the next run.
func TestZoomCommand(t *testing.T) {
	a, dir := testWorkspace(t)
	a.RunCommand("view.zoomIn")
	if a.Zoom < 1.09 || a.Zoom > 1.11 {
		t.Errorf("zoom after zoomIn = %v", a.Zoom)
	}
	a.RunCommand("view.zoomReset")
	if a.Zoom != 1 {
		t.Errorf("zoom after reset = %v", a.Zoom)
	}
	a.RunCommand("view.zoomOut")
	if a.Zoom < 0.89 || a.Zoom > 0.91 {
		t.Errorf("zoom after zoomOut = %v", a.Zoom)
	}
	a2 := New(dir, "")
	if a2.Zoom < 0.89 || a2.Zoom > 0.91 {
		t.Errorf("restored zoom = %v", a2.Zoom)
	}
}

// TestPreviewTab pins the tab on an edit, and replaces it with the next
// preview.
func TestPreviewTab(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenPreview(filepath.Join(dir, "main.go"))
	a.OpenPreview(filepath.Join(dir, "pkg", "util.go"))
	if len(a.Col().Editors) != 1 {
		t.Fatalf("preview tabs = %d, want 1", len(a.Col().Editors))
	}
	e := a.Col().Current()
	if !e.Preview || e.Doc.Name() != "util.go" {
		t.Errorf("preview = %v %q", e.Preview, e.Doc.Name())
	}
	e.Doc.Buffer.Set("package pkg\n\n// edited\n")
	a.syncPreviews()
	if e.Preview {
		t.Errorf("an edit did not pin the preview tab")
	}
	// A pinned tab stays beside the next preview.
	a.OpenPreview(filepath.Join(dir, "main.go"))
	if len(a.Col().Editors) != 2 {
		t.Errorf("tabs = %d, want the pinned tab and a preview", len(a.Col().Editors))
	}
}

// TestExternalChangeBanner asks what to do when a dirty file changes on
// disk, and keeps the user's text when they choose so.
func TestExternalChangeBanner(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	d := a.Col().Current().Doc
	d.Buffer.Set("package main\n\n// mine\n")
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\n// theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the mtime clearly newer than the read one.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime().Add(2e9), info.ModTime().Add(2e9)); err != nil {
		t.Fatal(err)
	}
	a.checkDisk()
	if !d.ExternalChanged() {
		t.Fatalf("a changed dirty file went unnoticed")
	}
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("main.go 已在磁盘上改变，本窗口还有未保存的修改。") {
		t.Fatalf("no banner; texts %q", tt.Texts())
	}
	if err := tt.Click("保留我的"); err != nil {
		t.Fatal(err)
	}
	if d.ExternalChanged() {
		t.Errorf("banner flag not cleared by 保留我的")
	}
	if tt.HasText("main.go 已在磁盘上改变，本窗口还有未保存的修改。") {
		t.Errorf("banner still shown")
	}
}

// TestExternalReloadClean reloads a clean file changed on disk.
func TestExternalReloadClean(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	d := a.Col().Current().Doc
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\n// theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	os.Chtimes(path, info.ModTime().Add(2e9), info.ModTime().Add(2e9))
	a.checkDisk()
	if d.Dirty() {
		t.Errorf("reloaded document is dirty")
	}
	if got := d.Buffer.String(); got != "package main\n\n// theirs\n" {
		t.Errorf("reloaded text = %q", got)
	}
}
