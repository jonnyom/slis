package zmxctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinaryPrefersBundledSibling(t *testing.T) {
	directory := t.TempDir()
	slisBinary := filepath.Join(directory, "slis")
	bundled := filepath.Join(directory, "zmx")
	override := filepath.Join(directory, "development-zmx")
	for _, path := range []string{bundled, override} {
		if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ResolveBinary(slisBinary, override)
	if err != nil {
		t.Fatal(err)
	}
	if got != bundled {
		t.Fatalf("binary = %q, want %q", got, bundled)
	}
}

func TestResolveBinaryFindsSiblingThroughSlisSymlink(t *testing.T) {
	directory := t.TempDir()
	staged := filepath.Join(directory, "staged")
	bin := filepath.Join(directory, "bin")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	slisBinary := filepath.Join(staged, "slis")
	runtimeBinary := filepath.Join(staged, "zmx")
	for _, path := range []string{slisBinary, runtimeBinary} {
		if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(bin, "slis")
	if err := os.Symlink(slisBinary, link); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveBinary(link, "")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(runtimeBinary)
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Fatalf("binary = %q, want %q", got, expected)
	}
}

func TestResolveBinaryFindsPrivateLibexecRuntime(t *testing.T) {
	directory := t.TempDir()
	bin := filepath.Join(directory, "bin")
	libexec := filepath.Join(directory, "libexec")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libexec, 0o755); err != nil {
		t.Fatal(err)
	}
	runtimeBinary := filepath.Join(libexec, "zmx")
	if err := os.WriteFile(runtimeBinary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveBinary(filepath.Join(bin, "slis"), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != runtimeBinary {
		t.Fatalf("binary = %q, want %q", got, runtimeBinary)
	}
}

func TestResolveBinaryFindsPrivateLibexecRuntimeThroughSlisSymlink(t *testing.T) {
	directory := t.TempDir()
	prefixBin := filepath.Join(directory, "bin")
	cellarBin := filepath.Join(directory, "Cellar", "slis", "0.12.0", "bin")
	cellarLibexec := filepath.Join(directory, "Cellar", "slis", "0.12.0", "libexec")
	for _, path := range []string{prefixBin, cellarBin, cellarLibexec} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	slisBinary := filepath.Join(cellarBin, "slis")
	runtimeBinary := filepath.Join(cellarLibexec, "zmx")
	unrelatedRuntime := filepath.Join(prefixBin, "zmx")
	for _, path := range []string{slisBinary, runtimeBinary, unrelatedRuntime} {
		if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(prefixBin, "slis")
	if err := os.Symlink(slisBinary, link); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveBinary(link, "")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(runtimeBinary)
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Fatalf("binary = %q, want %q", got, expected)
	}
}
