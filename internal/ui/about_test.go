package ui

import "testing"

func TestFormatAboutTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		appName string
		version string
		want    string
	}{
		{name: "packaged metadata", appName: "FileGot", version: "0.2.0", want: "FileGot 0.2.0"},
		{name: "empty version is dev", appName: "FileGot", version: "", want: "FileGot dev"},
		{name: "empty name defaults", appName: "", version: "1.0.0", want: "FileGot 1.0.0"},
		{name: "both empty", appName: "", version: "", want: "FileGot dev"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := formatAboutTitle(test.appName, test.version)
			if got != test.want {
				t.Fatalf("formatAboutTitle(%q, %q) = %q, want %q", test.appName, test.version, got, test.want)
			}
		})
	}
}
