package mediautil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	MaxAudioSeconds         = 80
	MaxVideoWithAudioSeconds = 80
	MaxVideoNoAudioSeconds  = 120
)

// MediaChunk represents a segment of a media file
type MediaChunk struct {
	Data       []byte
	StartSec   float64
	EndSec     float64
	TotalSec   float64
	SegmentIdx int
	TotalSegs  int
}

// CheckFFmpeg returns an error if ffmpeg is not installed
func CheckFFmpeg() error {
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed; it is required for audio/video embedding (install: https://ffmpeg.org/download.html)")
	}
	return nil
}

// ffprobeResult holds the relevant fields from ffprobe output
type ffprobeResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

// probe gets duration and stream info from a media file
func probe(filePath string) (duration float64, hasAudio bool, err error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		// fallback: use ffmpeg -i for duration
		ffprobePath = ""
	}

	if ffprobePath != "" {
		cmd := exec.Command(ffprobePath,
			"-v", "quiet",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			filePath,
		)
		output, err := cmd.Output()
		if err != nil {
			return 0, false, fmt.Errorf("ffprobe failed: %w", err)
		}

		var result ffprobeResult
		if err := json.Unmarshal(output, &result); err != nil {
			return 0, false, fmt.Errorf("failed to parse ffprobe output: %w", err)
		}

		duration, err = strconv.ParseFloat(result.Format.Duration, 64)
		if err != nil {
			return 0, false, fmt.Errorf("failed to parse duration: %w", err)
		}

		for _, s := range result.Streams {
			if s.CodecType == "audio" {
				hasAudio = true
				break
			}
		}
		return duration, hasAudio, nil
	}

	// Fallback without ffprobe - assume has audio, use ffmpeg to get duration
	return 0, true, fmt.Errorf("ffprobe not found")
}

// MaxDuration returns the max embedding duration in seconds for the given MIME type and audio presence
func MaxDuration(mimeType string, hasAudio bool) int {
	if strings.HasPrefix(mimeType, "audio/") {
		return MaxAudioSeconds
	}
	// video
	if hasAudio {
		return MaxVideoWithAudioSeconds
	}
	return MaxVideoNoAudioSeconds
}

// NeedsSplit checks if a media file needs splitting and returns info
func NeedsSplit(filePath, mimeType string) (needsSplit bool, duration float64, hasAudio bool, maxDur int, err error) {
	duration, hasAudio, err = probe(filePath)
	if err != nil {
		// If we can't probe, assume it might need splitting
		// For audio files, always assume has audio
		if strings.HasPrefix(mimeType, "audio/") {
			hasAudio = true
		}
		maxDur = MaxDuration(mimeType, hasAudio)
		return false, 0, hasAudio, maxDur, err
	}

	maxDur = MaxDuration(mimeType, hasAudio)
	needsSplit = duration > float64(maxDur)
	return needsSplit, duration, hasAudio, maxDur, nil
}

// SplitMedia splits a media file into segments of at most maxSeconds.
// Returns the segments as MediaChunk slices. Caller must not rely on temp files
// as they are read into memory and cleaned up.
func SplitMedia(filePath, mimeType string, maxSeconds int) ([]MediaChunk, error) {
	duration, hasAudio, err := probe(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to probe %s: %w", filePath, err)
	}

	if maxSeconds <= 0 {
		maxSeconds = MaxDuration(mimeType, hasAudio)
	}

	// No split needed
	if duration <= float64(maxSeconds) {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
		}
		return []MediaChunk{{
			Data:       data,
			StartSec:   0,
			EndSec:     duration,
			TotalSec:   duration,
			SegmentIdx: 0,
			TotalSegs:  1,
		}}, nil
	}

	// Create temp directory for segments
	tmpDir, err := os.MkdirTemp("", "ragujuary-media-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ext := strings.ToLower(filepath.Ext(filePath))
	outputPattern := filepath.Join(tmpDir, "seg_%03d"+ext)

	args := []string{
		"-i", filePath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", maxSeconds),
		"-c", "copy",
	}

	// For video, reset timestamps for each segment
	if strings.HasPrefix(mimeType, "video/") {
		args = append(args, "-reset_timestamps", "1")
	}

	args = append(args, outputPattern)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg split failed: %w\n%s", err, output)
	}

	// Read segment files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	// Sort entries by name (they're already sorted as seg_000, seg_001, ...)
	var segFiles []string
	for _, e := range entries {
		if !e.IsDir() {
			segFiles = append(segFiles, filepath.Join(tmpDir, e.Name()))
		}
	}

	if len(segFiles) == 0 {
		return nil, fmt.Errorf("ffmpeg produced no segments")
	}

	totalSegs := len(segFiles)
	chunks := make([]MediaChunk, 0, totalSegs)

	for i, segFile := range segFiles {
		data, err := os.ReadFile(segFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read segment %s: %w", segFile, err)
		}

		startSec := float64(i * maxSeconds)
		endSec := float64((i + 1) * maxSeconds)
		if endSec > duration {
			endSec = duration
		}

		chunks = append(chunks, MediaChunk{
			Data:       data,
			StartSec:   startSec,
			EndSec:     endSec,
			TotalSec:   duration,
			SegmentIdx: i,
			TotalSegs:  totalSegs,
		})
	}

	return chunks, nil
}

// FormatTimeLabel formats a time range as a human-readable label
func FormatTimeLabel(startSec, endSec, totalSec float64) string {
	return fmt.Sprintf("%s-%s of %s",
		formatDuration(startSec),
		formatDuration(endSec),
		formatDuration(totalSec),
	)
}

func formatDuration(sec float64) string {
	m := int(sec) / 60
	s := int(sec) % 60
	if m > 0 {
		return fmt.Sprintf("%d:%02d", m, s)
	}
	return fmt.Sprintf("0:%02d", s)
}
