package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// QualityResult is the outcome of a single quality gate.
type QualityResult struct {
	Pass     bool               `json:"pass"`
	Score    *float64           `json:"score,omitempty"`    // AI-scored gates (1-10)
	Reason   string             `json:"reason,omitempty"`   // Why it failed
	Checks   map[string]interface{} `json:"checks,omitempty"` // Technical check details
	Attempts int                `json:"attempts,omitempty"` // How many attempts so far
	Note     string             `json:"note,omitempty"`     // Human-readable context
}

// QAReport collects all gate results for a single run.
type QAReport struct {
	Script    *QualityResult `json:"script,omitempty"`
	Voiceover *QualityResult `json:"voiceover,omitempty"`
	Video     *QualityResult `json:"video,omitempty"`
	Final     *QualityResult `json:"final,omitempty"`
}

// ── Gate 1: Script QA ────────────────────────────────────────

func qaScript(cfg *Config, script string, refs []ReferenceImage) QualityResult {
	// Quick structural checks first
	words := len(strings.Fields(script))
	if words < 10 {
		return QualityResult{Pass: false, Reason: "script too short — only " + strconv.Itoa(words) + " words"}
	}
	if words > 150 {
		return QualityResult{Pass: false, Reason: "script too long — " + strconv.Itoa(words) + " words, max 150"}
	}

	// AI evaluation via Gemini
	if cfg.GoogleAPIKey == "" {
		// Can't AI-check without Gemini — pass on structural checks alone
		return QualityResult{Pass: true, Note: "structural checks only (no Google API key for AI eval)"}
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s",
		cfg.GoogleAPIKey,
	)

	prompt := fmt.Sprintf(`You are a UGC quality reviewer. Score this voiceover script for a product called "%s" (%s).

SCRIPT:
"""%s"""

Score each dimension 1-10:
1. AUTHENTICITY — Sounds like a real person, not an ad? No corporate jargon or salesy clichés?
2. HOOK — First sentence grabs attention within 3 seconds?
3. BRAND_FIT — Mentions the product accurately? No hallucinated features?
4. LENGTH — Appropriate length for a 15-30 second video? (~30-80 words)
5. CTA — Ends with a natural call-to-action?

Respond in EXACTLY this JSON format, nothing else:
{"authenticity":N,"hook":N,"brand_fit":N,"length":N,"cta":N,"overall":N,"issues":"one sentence summary of problems, or 'none'"}`,
		cfg.ProductName, cfg.ProductDescription, script)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"TEXT"},
			"temperature":        0.1,
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return QualityResult{Pass: false, Reason: "AI eval failed (network) — cannot verify quality", Note: "Gemini unreachable. Fix network or disable qualityGate to skip."}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return QualityResult{Pass: false, Reason: "AI eval returned HTTP " + strconv.Itoa(resp.StatusCode), Note: "Gemini returned an error. Check Google API key and quota."}
	}

	// Parse Gemini response
	var geminiResult struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	json.Unmarshal(respBody, &geminiResult)

	for _, c := range geminiResult.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text == "" {
				continue
			}
			// Extract JSON from response (may have markdown fences)
			text := strings.TrimSpace(p.Text)
			text = strings.TrimPrefix(text, "```json")
			text = strings.TrimPrefix(text, "```")
			text = strings.TrimSuffix(text, "```")
			text = strings.TrimSpace(text)

			var scores struct {
				Authenticity float64 `json:"authenticity"`
				Hook         float64 `json:"hook"`
				BrandFit     float64 `json:"brand_fit"`
				Length       float64 `json:"length"`
				CTA          float64 `json:"cta"`
				Overall      float64 `json:"overall"`
				Issues       string  `json:"issues"`
			}
			if err := json.Unmarshal([]byte(text), &scores); err != nil {
				return QualityResult{Pass: false, Reason: "AI eval response unparseable — cannot verify quality", Note: "Gemini returned non-JSON. Retrying may fix."}
			}

			overall := scores.Overall
			if overall == 0 {
				overall = (scores.Authenticity + scores.Hook + scores.BrandFit + scores.Length + scores.CTA) / 5
			}

			checks := map[string]interface{}{
				"authenticity": scores.Authenticity,
				"hook":         scores.Hook,
				"brand_fit":    scores.BrandFit,
				"length":       scores.Length,
				"cta":          scores.CTA,
				"word_count":   words,
			}

			pass := overall >= 7.0
			reason := ""
			if !pass {
				reason = fmt.Sprintf("overall score %.1f/10 (need ≥7): %s", overall, scores.Issues)
			}

			scoreVal := overall
			return QualityResult{
				Pass:   pass,
				Score:  &scoreVal,
				Reason: reason,
				Checks: checks,
				Note:   scores.Issues,
			}
		}
	}

	return QualityResult{Pass: false, Reason: "AI eval returned no scores — cannot verify quality", Note: "Gemini returned empty response. Retrying may fix."}
}

// ── Gate 2: Voiceover QA ─────────────────────────────────────

func qaVoiceover(audioPath string) QualityResult {
	checks := map[string]interface{}{}

	// File size check
	info, err := os.Stat(audioPath)
	if err != nil {
		return QualityResult{Pass: false, Reason: "audio file not found"}
	}
	sizeKB := float64(info.Size()) / 1024
	checks["sizeKB"] = sizeKB

	if sizeKB < 10 {
		return QualityResult{Pass: false, Reason: "audio file too small (<10KB) — likely empty or corrupt", Checks: checks}
	}
	if sizeKB > 20*1024 {
		return QualityResult{Pass: false, Reason: "audio file too large (>20MB)", Checks: checks}
	}

	// ffprobe checks
	probe := ffprobeJSON(audioPath)
	if probe != nil {
		duration := probe.Duration()
		checks["duration"] = duration
		checks["sampleRate"] = probe.SampleRate()

		if duration < 3 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("audio too short (%.1fs, need ≥3s)", duration), Checks: checks}
		}
		if duration > 45 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("audio too long (%.1fs, max 45s)", duration), Checks: checks}
		}
		if sr := probe.SampleRate(); sr > 0 && sr < 22050 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("sample rate too low (%d Hz, need ≥22050)", sr), Checks: checks}
		}

		// Silence detection via ffmpeg
		silenceRatio := detectSilenceRatio(audioPath, duration)
		checks["silenceRatio"] = silenceRatio
		if silenceRatio > 0.4 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("%.0f%% silence detected — audio may be glitchy", silenceRatio*100), Checks: checks}
		}
	}

	return QualityResult{Pass: true, Checks: checks}
}

// ── Gate 3: Video QA ─────────────────────────────────────────

func qaVideo(cfg *Config, videoPath string, refs []ReferenceImage) QualityResult {
	checks := map[string]interface{}{}

	// File size check
	info, err := os.Stat(videoPath)
	if err != nil {
		return QualityResult{Pass: false, Reason: "video file not found"}
	}
	sizeKB := float64(info.Size()) / 1024
	checks["sizeKB"] = sizeKB

	if sizeKB < 100 {
		return QualityResult{Pass: false, Reason: "video file too small (<100KB) — likely corrupt", Checks: checks}
	}
	if sizeKB > 200*1024 {
		return QualityResult{Pass: false, Reason: "video file too large (>200MB)", Checks: checks}
	}

	// ffprobe checks
	probe := ffprobeJSON(videoPath)
	if probe != nil {
		duration := probe.Duration()
		width, height := probe.Resolution()
		checks["duration"] = duration
		checks["width"] = width
		checks["height"] = height
		checks["hasVideo"] = probe.HasVideo()

		if !probe.HasVideo() {
			return QualityResult{Pass: false, Reason: "no video stream found in file", Checks: checks}
		}
		if duration < 3 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("video too short (%.1fs, need ≥3s)", duration), Checks: checks}
		}
		if width < 480 || height < 480 {
			return QualityResult{Pass: false, Reason: fmt.Sprintf("resolution too low (%dx%d, need ≥480x480)", width, height), Checks: checks}
		}
	}

	// Visual quality check via Gemini (if refs available and Google key set)
	if cfg.GoogleAPIKey != "" && len(refs) > 0 {
		framePath := videoPath + ".qaframe.jpg"
		if err := ffmpegExtractFrame(videoPath, framePath, 0.5); err == nil {
			defer os.Remove(framePath)
			score, note := geminiVisualQA(cfg, framePath, refs)
			checks["visualScore"] = score
			checks["visualNote"] = note
			if score > 0 && score < 6 {
				return QualityResult{
					Pass:   false,
					Score:  &score,
					Reason: fmt.Sprintf("visual quality score %.0f/10: %s", score, note),
					Checks: checks,
				}
			}
		}
	}

	return QualityResult{Pass: true, Checks: checks}
}

// ── Gate 4: Final Merged QA ──────────────────────────────────

func qaFinal(finalPath, videoPath, audioPath string) QualityResult {
	checks := map[string]interface{}{}

	// Basic file existence + size
	finalInfo, err := os.Stat(finalPath)
	if err != nil {
		return QualityResult{Pass: false, Reason: "merged file not found"}
	}
	videoInfo, _ := os.Stat(videoPath)

	finalSize := finalInfo.Size()
	checks["finalSizeKB"] = float64(finalSize) / 1024

	// Merged file should be bigger than raw video (audio was added)
	if videoInfo != nil && finalSize <= videoInfo.Size() {
		checks["videoSizeKB"] = float64(videoInfo.Size()) / 1024
		return QualityResult{Pass: false, Reason: "merged file not larger than raw video — audio may not be included", Checks: checks}
	}

	// ffprobe: verify both streams
	probe := ffprobeJSON(finalPath)
	if probe != nil {
		checks["hasVideo"] = probe.HasVideo()
		checks["hasAudio"] = probe.HasAudio()
		checks["duration"] = probe.Duration()

		if !probe.HasVideo() {
			return QualityResult{Pass: false, Reason: "merged file missing video stream", Checks: checks}
		}
		if !probe.HasAudio() {
			return QualityResult{Pass: false, Reason: "merged file missing audio stream — merge failed silently", Checks: checks}
		}

		// Duration sanity: should roughly match audio
		if audioPath != "" {
			audioProbe := ffprobeJSON(audioPath)
			if audioProbe != nil {
				audioDur := audioProbe.Duration()
				finalDur := probe.Duration()
				checks["audioDuration"] = audioDur
				diff := math.Abs(finalDur - audioDur)
				if diff > 3 && finalDur > 0 {
					checks["durationDiff"] = diff
					return QualityResult{
						Pass:   false,
						Reason: fmt.Sprintf("duration mismatch: final=%.1fs, audio=%.1fs (diff %.1fs)", finalDur, audioDur, diff),
						Checks: checks,
					}
				}
			}
		}
	}

	return QualityResult{Pass: true, Checks: checks}
}

// ── ffprobe helpers ──────────────────────────────────────────

type ProbeData struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width,omitempty"`
		Height     int    `json:"height,omitempty"`
		SampleRateStr string `json:"sample_rate,omitempty"`
	} `json:"streams"`
	Format struct {
		DurationStr string `json:"duration"`
		Size        string `json:"size"`
	} `json:"format"`
}

func (p *ProbeData) Duration() float64 {
	if p == nil {
		return 0
	}
	d, _ := strconv.ParseFloat(p.Format.DurationStr, 64)
	return d
}

func (p *ProbeData) Resolution() (int, int) {
	if p == nil {
		return 0, 0
	}
	for _, s := range p.Streams {
		if s.CodecType == "video" && s.Width > 0 {
			return s.Width, s.Height
		}
	}
	return 0, 0
}

func (p *ProbeData) HasVideo() bool {
	if p == nil {
		return false
	}
	for _, s := range p.Streams {
		if s.CodecType == "video" {
			return true
		}
	}
	return false
}

func (p *ProbeData) HasAudio() bool {
	if p == nil {
		return false
	}
	for _, s := range p.Streams {
		if s.CodecType == "audio" {
			return true
		}
	}
	return false
}

func (p *ProbeData) SampleRate() int {
	if p == nil {
		return 0
	}
	for _, s := range p.Streams {
		if s.CodecType == "audio" && s.SampleRateStr != "" {
			sr, _ := strconv.Atoi(s.SampleRateStr)
			return sr
		}
	}
	return 0
}

func ffprobeJSON(filePath string) *ProbeData {
	probePath := findFFprobe()
	if probePath == "" {
		return nil
	}

	cmd := exec.Command(probePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var data ProbeData
	if err := json.Unmarshal(out, &data); err != nil {
		return nil
	}
	return &data
}

func findFFprobe() string {
	// Check PATH
	if path, err := exec.LookPath("ffprobe"); err == nil {
		return path
	}

	// Check next to ffmpeg (our auto-downloaded copy)
	toolsDir := ""
	if exe, err := os.Executable(); err == nil {
		toolsDir = filepath.Dir(exe)
	}
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name = "ffprobe.exe"
	}
	local := filepath.Join(toolsDir, name)
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}

// detectSilenceRatio uses ffmpeg silencedetect to calculate what fraction of audio is silence.
func detectSilenceRatio(audioPath string, totalDuration float64) float64 {
	if totalDuration <= 0 {
		return 0
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		// Check local
		if exe, err2 := os.Executable(); err2 == nil {
			name := "ffmpeg"
			if runtime.GOOS == "windows" {
				name = "ffmpeg.exe"
			}
			local := filepath.Join(filepath.Dir(exe), name)
			if _, err3 := os.Stat(local); err3 == nil {
				ffmpeg = local
			}
		}
		if ffmpeg == "" {
			return 0
		}
	}

	cmd := exec.Command(ffmpeg,
		"-i", audioPath,
		"-af", "silencedetect=noise=-30dB:d=0.5",
		"-f", "null", "-",
	)
	// silencedetect outputs to stderr
	out, _ := cmd.CombinedOutput()
	output := string(out)

	// Parse silence_duration entries
	var totalSilence float64
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "silence_duration:") {
			parts := strings.Split(line, "silence_duration:")
			if len(parts) > 1 {
				valStr := strings.TrimSpace(parts[1])
				// May have additional text after the number
				if idx := strings.IndexAny(valStr, " \t\r\n"); idx > 0 {
					valStr = valStr[:idx]
				}
				val, err := strconv.ParseFloat(valStr, 64)
				if err == nil {
					totalSilence += val
				}
			}
		}
	}

	return totalSilence / totalDuration
}

// ffmpegExtractFrame extracts a single frame from a video at the given position (0.0-1.0).
func ffmpegExtractFrame(videoPath, outputPath string, position float64) error {
	ffmpeg, _ := findOrDownloadFFmpeg()
	if ffmpeg == "" {
		return fmt.Errorf("ffmpeg not available")
	}

	// Get duration to calculate timestamp
	probe := ffprobeJSON(videoPath)
	if probe == nil {
		return fmt.Errorf("cannot probe video duration")
	}
	timestamp := probe.Duration() * position

	cmd := exec.Command(ffmpeg,
		"-y",
		"-ss", fmt.Sprintf("%.2f", timestamp),
		"-i", videoPath,
		"-vframes", "1",
		"-q:v", "2",
		outputPath,
	)
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// geminiVisualQA sends a video frame + reference images to Gemini for visual quality assessment.
func geminiVisualQA(cfg *Config, framePath string, refs []ReferenceImage) (float64, string) {
	frameData, err := os.ReadFile(framePath)
	if err != nil {
		return 0, "cannot read frame"
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s",
		cfg.GoogleAPIKey,
	)

	parts := []map[string]interface{}{}

	// Add reference images
	for _, ref := range refs {
		parts = append(parts, map[string]interface{}{
			"inlineData": map[string]string{
				"mimeType": ref.MimeType,
				"data":     ref.Base64,
			},
		})
	}

	// Add the video frame
	parts = append(parts, map[string]interface{}{
		"inlineData": map[string]string{
			"mimeType": "image/jpeg",
			"data":     base64.StdEncoding.EncodeToString(frameData),
		},
	})

	parts = append(parts, map[string]interface{}{
		"text": `The first images are reference product photos. The last image is a frame from a generated UGC video.

Score the video frame 1-10:
- VISUAL_QUALITY: Is it clear, well-lit, not blurry/artifacted?
- BRAND_RELEVANCE: Does it relate to the product shown in the references?
- APPROPRIATE: Free of inappropriate, offensive, or off-brand content?

Respond in EXACTLY this JSON: {"score":N,"note":"one sentence"}
The score should be the average of all three dimensions.`,
	})

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"TEXT"},
			"temperature":        0.1,
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "visual QA request failed"
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return 0, "visual QA returned HTTP " + strconv.Itoa(resp.StatusCode)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	json.Unmarshal(respBody, &result)

	for _, c := range result.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text == "" {
				continue
			}
			text := strings.TrimSpace(p.Text)
			text = strings.TrimPrefix(text, "```json")
			text = strings.TrimPrefix(text, "```")
			text = strings.TrimSuffix(text, "```")
			text = strings.TrimSpace(text)

			var scores struct {
				Score float64 `json:"score"`
				Note  string  `json:"note"`
			}
			if json.Unmarshal([]byte(text), &scores) == nil {
				return scores.Score, scores.Note
			}
		}
	}

	return 0, "could not parse visual QA"
}

// saveQAReport writes the quality report to the run folder.
func saveQAReport(runDir string, report *QAReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(runDir, "qa_report.json"), data, 0644)
}
