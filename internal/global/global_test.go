package global

import (
	"testing"
)

// setTestDir overrides the airlock directory for the duration of a test.
func setTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := overrideDir
	overrideDir = dir
	t.Cleanup(func() { overrideDir = old })
	return dir
}

// --- GlobalConfig tests ---

func TestLoadConfigDefaults(t *testing.T) {
	setTestDir(t)
	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.Version != 1 {
		t.Errorf("expected version 1, got %d", c.Version)
	}
}

func TestSaveLoadConfigRoundTrip(t *testing.T) {
	setTestDir(t)
	c := &GlobalConfig{Version: 1, Engine: "docker", DefaultIdentity: "myid"}
	if err := SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.Engine != "docker" {
		t.Errorf("engine: want docker, got %s", got.Engine)
	}
	if got.DefaultIdentity != "myid" {
		t.Errorf("defaultIdentity: want myid, got %s", got.DefaultIdentity)
	}
}

// --- ProjectIndex tests ---

func TestLoadProjectIndexEmpty(t *testing.T) {
	setTestDir(t)
	idx, err := LoadProjectIndex()
	if err != nil {
		t.Fatalf("LoadProjectIndex: %v", err)
	}
	if len(idx.Projects) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx.Projects))
	}
}

func TestProjectIndexBindForgetRoundTrip(t *testing.T) {
	setTestDir(t)
	idx, _ := LoadProjectIndex()

	e := &ProjectEntry{Name: "myproj", Image: "ubuntu:24.04", Identity: "default"}
	idx.Bind("/home/user/repos/myproj", e)

	if err := idx.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	idx2, err := LoadProjectIndex()
	if err != nil {
		t.Fatalf("LoadProjectIndex after save: %v", err)
	}
	got, ok := idx2.Find("/home/user/repos/myproj")
	if !ok {
		t.Fatal("expected to find /home/user/repos/myproj")
	}
	if got.Name != "myproj" {
		t.Errorf("name: want myproj, got %s", got.Name)
	}
	if got.Image != "ubuntu:24.04" {
		t.Errorf("image: want ubuntu:24.04, got %s", got.Image)
	}

	// Forget and verify
	idx2.Forget("/home/user/repos/myproj")
	if err := idx2.Save(); err != nil {
		t.Fatalf("Save after forget: %v", err)
	}
	idx3, _ := LoadProjectIndex()
	if _, ok := idx3.Find("/home/user/repos/myproj"); ok {
		t.Error("expected entry to be forgotten")
	}
}

func TestProjectIndexFindByName(t *testing.T) {
	setTestDir(t)
	idx, _ := LoadProjectIndex()
	idx.Bind("/a/myproj", &ProjectEntry{Name: "myproj", Image: "img1", Identity: "default"})
	idx.Bind("/b/myproj", &ProjectEntry{Name: "myproj", Image: "img2", Identity: "default"})
	idx.Bind("/c/other", &ProjectEntry{Name: "other", Image: "img3", Identity: "default"})

	paths := idx.FindByName("myproj")
	if len(paths) != 2 {
		t.Errorf("expected 2 paths for 'myproj', got %d", len(paths))
	}
	paths2 := idx.FindByName("other")
	if len(paths2) != 1 {
		t.Errorf("expected 1 path for 'other', got %d", len(paths2))
	}
	paths3 := idx.FindByName("nope")
	if len(paths3) != 0 {
		t.Errorf("expected 0 paths for 'nope', got %d", len(paths3))
	}
}

func TestProjectIndexUsedImages(t *testing.T) {
	setTestDir(t)
	idx, _ := LoadProjectIndex()
	idx.Bind("/a", &ProjectEntry{Name: "a", Image: "ubuntu:24.04", Identity: "d"})
	idx.Bind("/b", &ProjectEntry{Name: "b", Image: "golang:1.22", Identity: "d"})
	idx.Bind("/c", &ProjectEntry{Name: "c", Image: "ubuntu:24.04", Identity: "d"}) // duplicate

	imgs := idx.UsedImages()
	if len(imgs) != 2 {
		t.Errorf("expected 2 distinct images, got %d: %v", len(imgs), imgs)
	}
}

func TestProjectEntryResolvedImage(t *testing.T) {
	e1 := &ProjectEntry{Image: "ubuntu:24.04"}
	if e1.ResolvedImage() != "ubuntu:24.04" {
		t.Errorf("expected ubuntu:24.04, got %s", e1.ResolvedImage())
	}
	e2 := &ProjectEntry{Containerfile: "/path/Containerfile", ImageTag: "myproj:latest"}
	if e2.ResolvedImage() != "myproj:latest" {
		t.Errorf("expected myproj:latest, got %s", e2.ResolvedImage())
	}
}

// --- Identity tests ---

func TestCreateListGetIdentity(t *testing.T) {
	setTestDir(t)

	id, err := CreateIdentity("testid")
	if err != nil {
		t.Fatalf("CreateIdentity: %v", err)
	}
	if id.Name != "testid" {
		t.Errorf("name: want testid, got %s", id.Name)
	}

	got, err := GetIdentity("testid")
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if got.HomeDir != id.HomeDir {
		t.Errorf("homeDir mismatch: %s vs %s", got.HomeDir, id.HomeDir)
	}

	ids, err := ListIdentities()
	if err != nil {
		t.Fatalf("ListIdentities: %v", err)
	}
	if len(ids) != 1 || ids[0].Name != "testid" {
		t.Errorf("expected [testid], got %v", ids)
	}
}

func TestGetIdentityNotFound(t *testing.T) {
	setTestDir(t)
	_, err := GetIdentity("ghost")
	if err == nil {
		t.Error("expected error for missing identity")
	}
}

func TestRemoveIdentityRequiresForce(t *testing.T) {
	setTestDir(t)
	if _, err := CreateIdentity("todel"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIdentity("todel", false); err == nil {
		t.Error("expected error without --force")
	}
	if err := RemoveIdentity("todel", true); err != nil {
		t.Errorf("RemoveIdentity with force: %v", err)
	}
	if IdentityExists("todel") {
		t.Error("identity still exists after removal")
	}
}

func TestListIdentitiesEmpty(t *testing.T) {
	setTestDir(t)
	ids, err := ListIdentities()
	if err != nil {
		t.Fatalf("ListIdentities: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %d", len(ids))
	}
}
