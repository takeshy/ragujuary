package fileutil

import "testing"

func TestDetectMimeTypeCaseInsensitive(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "photo.PNG", want: "image/png"},
		{path: "scan.PDF", want: "application/pdf"},
		{path: "clip.MP4", want: "video/mp4"},
		{path: "sound.WAV", want: "audio/wav"},
	}

	for _, tt := range tests {
		if got := detectMimeType(tt.path); got != tt.want {
			t.Fatalf("detectMimeType(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
