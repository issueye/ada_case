package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	in := Default()
	in.Workspace = "/w"
	in.Files = []string{"main.go", "pkg/util.go"}
	in.Active = 1
	in.SidebarOpen = false
	in.SidebarView = 2
	in.AIMode = 2
	in.Zoom = 1.2
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Workspace != in.Workspace || out.Active != 1 || out.Zoom != 1.2 {
		t.Errorf("loaded %+v", out)
	}
	if len(out.Files) != 2 || out.Files[1] != "pkg/util.go" {
		t.Errorf("files = %v", out.Files)
	}
	if out.SidebarOpen || out.AIMode != 2 || out.SidebarView != 2 {
		t.Errorf("layout = %+v", out)
	}
}

func TestLoadMissing(t *testing.T) {
	st, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if !st.SidebarOpen || st.AIMode != 0 || st.Zoom != 1 {
		t.Errorf("defaults = %+v", st)
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err == nil {
		t.Fatalf("corrupt file loaded without an error")
	}
	if !st.SidebarOpen || st.Zoom != 1 {
		t.Errorf("corrupt load did not start from defaults: %+v", st)
	}
}

func TestClamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"sidebarView":9,"zoom":42,"active":-3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.SidebarView != 0 || st.Zoom != 1 || st.Active != 0 {
		t.Errorf("clamps = %+v", st)
	}
}

// TestLoadV2State reads a state written before the AI modes: its layout
// still loads, docked.
func TestLoadV2State(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	old := `{"version":2,"workspace":"/w","files":["a.go"],"active":1,"sidebarOpen":false,"sidebarView":2,"aiDocked":false,"zoom":1.5}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != Version || len(st.Files) != 1 || st.SidebarView != 2 || st.Zoom != 1.5 {
		t.Errorf("migrated = %+v", st)
	}
	if st.AIMode != 0 {
		t.Errorf("AI mode = %d, want docked", st.AIMode)
	}
}
