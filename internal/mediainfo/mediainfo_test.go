package mediainfo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheGeeKing/FileGot/internal/mediainfo"
)

func TestDecodeExposesRawObjectsAndNamingBindings(t *testing.T) {
	input, err := os.Open(filepath.Join("testdata", "movie.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	got, err := mediainfo.Decode(input)
	if err != nil {
		t.Fatal(err)
	}

	if got.Media["Format"] != "Matroska" || got.Video[0]["Format_Profile"] != "Main 10" {
		t.Fatalf("raw objects were not preserved: %#v", got)
	}
	if got.Chapters["00_10_000"] != "Chapter 2" || got.Image["Type"] != "Cover" {
		t.Fatalf("chapter or image objects missing: %#v", got)
	}

	want := map[string]string{
		"cf": "mkv", "vcf": "HEVC", "resolution": "3840x2160",
		"vf": "2160p", "vk": "4K", "bitdepth": "10", "hdr": "HDR10",
		"dovi": "Dolby Vision", "ac": "truehd", "aco": "TrueHD+Atmos",
		"af": "8ch", "channels": "7.1", "audioLanguages": "en",
		"textLanguages": "fr", "mediaTitle": "Dune: Part Two",
		"seconds": "9972", "minutes": "166", "hours": "2:46",
	}
	for name, value := range want {
		if got.Bindings[name] != value {
			t.Errorf("binding %s = %q, want %q", name, got.Bindings[name], value)
		}
	}
}

func TestDecodeRejectsMalformedAndOversizedOutput(t *testing.T) {
	if _, err := mediainfo.DecodeString(`{"media":`); err == nil {
		t.Fatal("malformed JSON should fail")
	}
	oversized := make([]byte, mediainfo.MaxOutputBytes+1)
	if _, err := mediainfo.DecodeBytes(oversized); err == nil {
		t.Fatal("oversized output should fail")
	}
}
