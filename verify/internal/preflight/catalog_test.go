package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckS3TablesCatalogKept(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, S3TablesCatalogFile)
	manifest := filepath.Join(dir, "manifest.json")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := CheckS3TablesCatalogKept(false, record, manifest); err != nil {
		t.Errorf("a run interrupted before deploying, with no catalog before, may end without one: %v", err)
	}
	if err := CheckS3TablesCatalogKept(true, record, manifest); err != nil {
		t.Errorf("a kept catalog passes: %v", err)
	}

	write(record, "true\n")
	if err := CheckS3TablesCatalogKept(false, record, manifest); err == nil {
		t.Error("a catalog that existed before the run must survive teardown")
	}

	write(record, "false\n")
	write(manifest, "{}")
	if err := CheckS3TablesCatalogKept(false, record, manifest); err == nil {
		t.Error("a deployed run created the catalog, so it must survive teardown")
	}
}

func TestRecordS3TablesCatalogKeepsOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), S3TablesCatalogFile)
	if err := RecordS3TablesCatalog(path, false); err != nil {
		t.Fatal(err)
	}
	if err := RecordS3TablesCatalog(path, true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path) // #nosec G304 -- test temp directory
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "false\n" {
		t.Errorf("a rerun of preflight must keep the original record, got %q", b)
	}
}
