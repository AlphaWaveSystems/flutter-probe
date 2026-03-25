package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCSV_ValidFile(t *testing.T) {
	dir := t.TempDir()
	csv := `email,password,expected
user@test.com,pass123,Dashboard
admin@test.com,admin456,Admin Panel
`
	path := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(path, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	ex, err := loadCSVExamples(dir, "data.csv")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(ex.Headers) != 3 {
		t.Fatalf("headers: got %d, want 3", len(ex.Headers))
	}
	if ex.Headers[0] != "email" || ex.Headers[1] != "password" || ex.Headers[2] != "expected" {
		t.Errorf("headers: %v", ex.Headers)
	}
	if len(ex.Rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(ex.Rows))
	}
	if ex.Rows[0][0] != "user@test.com" {
		t.Errorf("row 0 col 0: %q", ex.Rows[0][0])
	}
}

func TestLoadCSV_MissingFile(t *testing.T) {
	_, err := loadCSVExamples(t.TempDir(), "nonexistent.csv")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadCSV_HeaderOnly(t *testing.T) {
	dir := t.TempDir()
	csv := "email,password\n"
	if err := os.WriteFile(filepath.Join(dir, "empty.csv"), []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadCSVExamples(dir, "empty.csv")
	if err == nil {
		t.Error("expected error for header-only CSV")
	}
}

func TestLoadCSV_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	csv := `a,b
1,2
`
	path := filepath.Join(dir, "abs.csv")
	if err := os.WriteFile(path, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	ex, err := loadCSVExamples("/some/other/dir", path)
	if err != nil {
		t.Fatalf("absolute path should work: %v", err)
	}
	if len(ex.Rows) != 1 {
		t.Errorf("rows: %d", len(ex.Rows))
	}
}
