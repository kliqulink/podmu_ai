package pod

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleBundle = "testdata/nur-atelier.pod"

func TestLoadValidBundle(t *testing.T) {
	b, err := Load(sampleBundle)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := b.Manifest.Metadata.Slug; got != "nur-atelier" {
		t.Errorf("slug = %q, want nur-atelier", got)
	}
	if got := b.Manifest.Spec.Identity.Brand; got != "Nur Atelier" {
		t.Errorf("brand = %q, want Nur Atelier", got)
	}
	if len(b.Manifest.Spec.Agents) != 3 {
		t.Errorf("agents = %d, want 3", len(b.Manifest.Spec.Agents))
	}
	if b.Thick {
		t.Errorf("sample bundle has no state/, want thin")
	}
	if got := b.Materialization(); got != "thin" {
		t.Errorf("Materialization = %q, want thin", got)
	}
}

func TestLoadValidBundleIsCompatible(t *testing.T) {
	b, err := Load(sampleBundle)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := CheckCompatibility(b.Manifest, CurrentRuntimeVersion); err != nil {
		t.Errorf("CheckCompatibility: %v", err)
	}
}

func TestLoadMissingRefFails(t *testing.T) {
	// Copy the sample bundle, delete a referenced agent file, expect a
	// ref-existence error naming the missing file.
	dst := copyBundle(t, sampleBundle)
	if err := os.Remove(filepath.Join(dst, "agents", "closer.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dst)
	if err == nil {
		t.Fatal("expected error for missing ref, got nil")
	}
	if !strings.Contains(err.Error(), "closer.yaml") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to name the missing closer.yaml", err)
	}
}

func TestLoadDetectsThickBundle(t *testing.T) {
	dst := copyBundle(t, sampleBundle)
	if err := os.MkdirAll(filepath.Join(dst, "state", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dst)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !b.Thick || b.Materialization() != "thick" {
		t.Errorf("expected thick bundle, got Thick=%v Materialization=%q", b.Thick, b.Materialization())
	}
}

func TestLoadNonDirFails(t *testing.T) {
	_, err := Load(filepath.Join(sampleBundle, ManifestFile))
	if err == nil {
		t.Fatal("expected error loading a file as a bundle, got nil")
	}
}

func TestLoadMissingManifestFails(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), ManifestFile) {
		t.Fatalf("expected missing-manifest error, got %v", err)
	}
}

// copyBundle makes a throwaway copy of a bundle under t.TempDir so a test can
// mutate it without touching the shared fixture.
func copyBundle(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "bundle")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyBundle: %v", err)
	}
	return dst
}

func TestValidationErrorsType(t *testing.T) {
	// A bundle load failure for validation reasons should surface as
	// ValidationErrors so callers can inspect fields.
	dst := copyBundle(t, sampleBundle)
	if err := os.Remove(filepath.Join(dst, "workflows", "wa_followup.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dst)
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error is %T, want ValidationErrors", err)
	}
	if len(ve) == 0 {
		t.Fatal("expected at least one validation error")
	}
}
