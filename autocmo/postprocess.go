package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// burnCaptions takes a video and a script, generates ASS subtitles with
// Hormozi-style word-by-word captions (large, bold, centered, 1-2 words
// at a time with emphasis words highlighted), and burns them into the video.
func burnCaptions(videoPath, script, outputPath string) error {
	ffmpeg, err := findOrDownloadFFmpeg()
	if err != nil {
		return fmt.Errorf("ffmpeg not available: %w", err)
	}

	probe := ffprobeJSON(videoPath)
	if probe == nil {
		return fmt.Errorf("cannot probe video for captions")
	}
	duration := probe.Duration()
	if duration <= 0 {
		return fmt.Errorf("video has no duration")
	}
	w, h := probe.Resolution()
	if w == 0 {
		w = 1080
	}
	if h == 0 {
		h = 1920
	}

	// Generate ASS subtitle file (Advanced SubStation Alpha — supports rich styling)
	assPath := videoPath + ".captions.ass"
	if err := generateASS(script, duration, w, h, assPath); err != nil {
		return fmt.Errorf("ASS generation failed: %w", err)
	}
	defer os.Remove(assPath)

	// Burn with ffmpeg using the ASS filter
	subtitleFilter := fmt.Sprintf("ass='%s'", escapeFfmpegPath(assPath))

	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", subtitleFilter,
		"-c:a", "copy",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "18",
		"-movflags", "+faststart",
		outputPath,
	}

	fmt.Printf("  Burning captions (Hormozi-style)...\n")
	cmd := exec.Command(ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg captions failed: %w\n%s", err, string(out))
	}

	info, _ := os.Stat(outputPath)
	if info != nil {
		fmt.Printf("  Captioned: %s (%.0f KB)\n", filepath.Base(outputPath), float64(info.Size())/1024)
	}
	return nil
}

// Emphasis words that get highlighted in yellow when they appear in captions.
var emphasisWords = map[string]bool{
	"insane": true, "crazy": true, "literally": true, "actually": true,
	"obsessed": true, "incredible": true, "amazing": true, "perfect": true,
	"love": true, "best": true, "favorite": true, "need": true,
	"trust": true, "premium": true, "quality": true, "fire": true,
	"different": true, "game-changer": true, "recommend": true, "seriously": true,
	"honestly": true, "gorgeous": true, "soft": true, "comfortable": true,
}

// generateASS creates an ASS (Advanced SubStation Alpha) subtitle file with
// Hormozi-style word-by-word captions: large bold text, 1-2 words at a time,
// centered vertically in the lower third, emphasis words in yellow.
func generateASS(script string, totalDuration float64, videoW, videoH int, outputPath string) error {
	words := strings.Fields(script)
	if len(words) == 0 {
		return fmt.Errorf("empty script")
	}

	// Clean words for display (remove pause markers)
	var cleanWords []string
	for _, w := range words {
		w = strings.ReplaceAll(w, "—", "")
		w = strings.ReplaceAll(w, "...", "")
		w = strings.TrimSpace(w)
		if w != "" {
			cleanWords = append(cleanWords, w)
		}
	}

	if len(cleanWords) == 0 {
		return fmt.Errorf("no words after cleaning")
	}

	// Group into 1-2 word chunks for punchy display
	var chunks []string
	for i := 0; i < len(cleanWords); {
		if i+1 < len(cleanWords) {
			// Two-word chunk — but keep single word if it's an emphasis word
			word := cleanWords[i]
			if isEmphasis(word) || len(word) > 8 {
				chunks = append(chunks, strings.ToUpper(word))
				i++
			} else {
				chunks = append(chunks, strings.ToUpper(cleanWords[i]+" "+cleanWords[i+1]))
				i += 2
			}
		} else {
			chunks = append(chunks, strings.ToUpper(cleanWords[i]))
			i++
		}
	}

	// Time each chunk — words per second based on total
	wordsPerSec := float64(len(cleanWords)) / totalDuration
	if wordsPerSec < 1 {
		wordsPerSec = 1
	}

	// Font size scales with resolution
	fontSize := videoH / 18 // ~107px on 1920h, ~60px on 1080h
	if fontSize < 48 {
		fontSize = 48
	}
	if fontSize > 120 {
		fontSize = 120
	}

	// Vertical position: lower third, above the very bottom
	marginV := videoH / 5 // 20% from bottom

	// ASS header
	var ass strings.Builder
	ass.WriteString("[Script Info]\n")
	ass.WriteString("Title: UGC Captions\n")
	ass.WriteString(fmt.Sprintf("PlayResX: %d\n", videoW))
	ass.WriteString(fmt.Sprintf("PlayResY: %d\n", videoH))
	ass.WriteString("ScaledBorderAndShadow: yes\n\n")

	ass.WriteString("[V4+ Styles]\n")
	ass.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	// Normal style: white, bold, black outline, drop shadow
	ass.WriteString(fmt.Sprintf("Style: Default,Arial,%d,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,-1,0,0,0,100,100,2,0,1,4,2,2,40,40,%d,1\n", fontSize, marginV))
	// Emphasis style: yellow, bold, same outline
	ass.WriteString(fmt.Sprintf("Style: Emphasis,Arial,%d,&H0000FFFF,&H000000FF,&H00000000,&H80000000,-1,0,0,0,100,100,2,0,1,4,2,2,40,40,%d,1\n", fontSize+4, marginV))
	ass.WriteString("\n")

	ass.WriteString("[Events]\n")
	ass.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	// Generate timed dialogue events
	currentTime := 0.0
	for _, chunk := range chunks {
		// How many actual words in this chunk
		chunkWords := len(strings.Fields(chunk))
		chunkDur := float64(chunkWords) / wordsPerSec
		if chunkDur < 0.25 {
			chunkDur = 0.25
		}
		if chunkDur > 1.2 {
			chunkDur = 1.2
		}

		endTime := currentTime + chunkDur
		if endTime > totalDuration {
			endTime = totalDuration
		}

		// Determine if this is an emphasis chunk
		style := "Default"
		displayText := chunk
		lowerChunk := strings.ToLower(chunk)
		for word := range emphasisWords {
			if strings.Contains(lowerChunk, word) {
				style = "Emphasis"
				break
			}
		}

		// Strip trailing punctuation for cleaner look (keep ? and !)
		displayText = strings.TrimRight(displayText, ".,;:")

		ass.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,%s,,0,0,0,,%s\n",
			assTimestamp(currentTime), assTimestamp(endTime), style, displayText))

		currentTime = endTime
	}

	return os.WriteFile(outputPath, []byte(ass.String()), 0644)
}

func isEmphasis(word string) bool {
	lower := strings.ToLower(strings.TrimRight(word, ".,!?;:"))
	return emphasisWords[lower]
}

func assTimestamp(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	cs := int((seconds - float64(int(seconds))) * 100)
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func escapeFfmpegPath(p string) string {
	// ffmpeg subtitles filter needs escaped backslashes and colons on Windows
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, ":", "\\:")
	return p
}

// cutPlatformVersions takes a 9:16 master video and produces cropped versions
// for different social platforms using ffmpeg.
func cutPlatformVersions(masterPath, runDir string) (map[string]string, error) {
	ffmpeg, err := findOrDownloadFFmpeg()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not available: %w", err)
	}

	// Get master dimensions
	probe := ffprobeJSON(masterPath)
	if probe == nil {
		return nil, fmt.Errorf("cannot probe master video")
	}
	w, h := probe.Resolution()
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("cannot detect master resolution")
	}

	cuts := map[string]struct {
		file  string
		ratio float64 // width/height
		desc  string
	}{
		"4_5":  {"feed_4x5.mp4", 4.0 / 5.0, "Facebook/Instagram Feed (4:5)"},
		"1_1":  {"square_1x1.mp4", 1.0, "Universal Square (1:1)"},
		"16_9": {"landscape_16x9.mp4", 16.0 / 9.0, "YouTube/Landscape (16:9)"},
	}

	results := map[string]string{}

	for key, cut := range cuts {
		outPath := filepath.Join(runDir, cut.file)

		// Calculate crop dimensions from master (9:16)
		// Master is tall/narrow. For wider crops, we crop height and keep width.
		// For equal/narrower crops, we crop width and keep height.
		var cropW, cropH int
		targetRatio := cut.ratio
		masterRatio := float64(w) / float64(h)

		if targetRatio > masterRatio {
			// Target is wider than master — keep full width, crop height
			cropW = w
			cropH = int(float64(w) / targetRatio)
		} else {
			// Target is narrower or equal — keep full height, crop width
			cropH = h
			cropW = int(float64(h) * targetRatio)
		}

		// Ensure even dimensions (required by libx264)
		cropW = cropW - (cropW % 2)
		cropH = cropH - (cropH % 2)

		// Center crop
		cropFilter := fmt.Sprintf("crop=%d:%d:(iw-%d)/2:(ih-%d)/2", cropW, cropH, cropW, cropH)

		args := []string{
			"-y",
			"-i", masterPath,
			"-vf", cropFilter,
			"-c:v", "libx264",
			"-preset", "fast",
			"-crf", "18",
			"-c:a", "copy",
			"-movflags", "+faststart",
			outPath,
		}

		cmd := exec.Command(ffmpeg, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("  [WARN] %s cut failed: %v\n%s\n", key, err, string(out))
			continue
		}

		info, _ := os.Stat(outPath)
		if info != nil {
			fmt.Printf("  ✓ %s (%.0f KB)\n", cut.desc, float64(info.Size())/1024)
			results[key] = outPath
		}
	}

	return results, nil
}
