package mediainfo_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/TheGeeKing/FileGot/internal/mediainfo"
)

func TestProbeExecutesMediaInfoAndCachesByFileIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows command fixture")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "mediainfo.cmd")
	fixture := filepath.Join(root, "output.json")
	source := filepath.Join(root, "movie.mkv")
	sample, err := os.ReadFile(filepath.Join("testdata", "movie.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("@type \"%FILEGOT_MEDIAINFO_FIXTURE%\"\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, sample, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILEGOT_MEDIAINFO_FIXTURE", fixture)

	first, err := mediainfo.Probe(context.Background(), executable, source)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bindings["vf"] != "2160p" {
		t.Fatalf("vf = %q", first.Bindings["vf"])
	}

	if err := os.WriteFile(fixture, []byte(`invalid`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mediainfo.Probe(context.Background(), executable, source); err != nil {
		t.Fatalf("unchanged source should use cache: %v", err)
	}
	if err := os.WriteFile(source, []byte("changed media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mediainfo.Probe(context.Background(), executable, source); err == nil {
		t.Fatal("changed source should invalidate cache")
	}
}

func TestProbeReportsMissingExecutable(t *testing.T) {
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mediainfo.Probe(context.Background(), filepath.Join(t.TempDir(), "missing"), source); err == nil {
		t.Fatal("missing executable should fail")
	}
}
