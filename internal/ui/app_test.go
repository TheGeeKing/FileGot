package ui

import (
	"testing"

	"github.com/thegeeking/FileGot/internal/media"
)

func TestReviewParsed(t *testing.T) {
	parsed, err := reviewParsed("TV episode", "The Last of Us", "2023", "1", "3")
	if err != nil {
		t.Fatal(err)
	}
	want := media.Parsed{Kind: media.Episode, Query: "The Last of Us", Year: 2023, Season: 1, Episode: 3}
	if parsed != want {
		t.Fatalf("reviewParsed() = %#v, want %#v", parsed, want)
	}

	if _, err := reviewParsed("TV episode", "Show", "", "", "2"); err == nil {
		t.Fatal("missing season should fail")
	}
}
