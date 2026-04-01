package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func elevenLabsSpeak(cfg *Config, cmd *Command, text, outputPath string) error {
	// UGC-optimized defaults: low stability = natural variation,
	// high style = expressive, high similarity = stay close to cloned voice
	stability := 0.25
	similarity := 0.80
	style := 0.70

	if cmd != nil {
		if cmd.Stability > 0 {
			stability = cmd.Stability
		}
		switch cmd.VoiceStyle {
		case "casual":
			stability = 0.20
			style = 0.75
		case "energetic":
			stability = 0.15
			style = 0.95
			similarity = 0.70
		case "serious":
			stability = 0.55
			style = 0.30
		case "warm":
			stability = 0.30
			style = 0.60
		case "natural":
			stability = 0.20
			style = 0.80
		}
	}

	payload := map[string]interface{}{
		"text":     text,
		"model_id": "eleven_multilingual_v2",
		"voice_settings": map[string]interface{}{
			"stability":         stability,
			"similarity_boost":  similarity,
			"style":             style,
			"use_speaker_boost": true,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", cfg.ElevenLabsVoiceID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", cfg.ElevenLabsAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ElevenLabs request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ElevenLabs returned HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("  Voiceover: %.0f KB\n", float64(n)/1024)
	return nil
}

// elevenLabsCloneVoice uploads audio samples to ElevenLabs and creates a cloned voice.
// Returns the new voice ID. Supports .mp3, .wav, .m4a, .ogg, .webm files.
// ElevenLabs instant clone needs at least 1 sample; up to 25 for better quality.
func elevenLabsCloneVoice(cfg *Config, voiceName string, samplePaths []string) (string, error) {
	if len(samplePaths) == 0 {
		return "", fmt.Errorf("no voice samples provided")
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Name field
	if err := writer.WriteField("name", voiceName); err != nil {
		return "", err
	}
	if err := writer.WriteField("description", "Cloned voice for AutoCMO"); err != nil {
		return "", err
	}

	// Attach each audio file
	for _, samplePath := range samplePaths {
		data, err := os.ReadFile(samplePath)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", samplePath, err)
		}

		fileName := filepath.Base(samplePath)
		part, err := writer.CreateFormFile("files", fileName)
		if err != nil {
			return "", err
		}
		if _, err := part.Write(data); err != nil {
			return "", err
		}
		fmt.Printf("  Sample: %s (%.0f KB)\n", fileName, float64(len(data))/1024)
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.elevenlabs.io/v1/voices/add", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("xi-api-key", cfg.ElevenLabsAPIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ElevenLabs clone request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ElevenLabs returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		VoiceID string `json:"voice_id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("cannot parse clone response: %w", err)
	}
	if result.VoiceID == "" {
		return "", fmt.Errorf("no voice_id in clone response")
	}

	return result.VoiceID, nil
}

// elevenLabsListVoices returns all voices available on the account.
func elevenLabsListVoices(cfg *Config) error {
	req, err := http.NewRequest("GET", "https://api.elevenlabs.io/v1/voices", nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", cfg.ElevenLabsAPIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ElevenLabs request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("ElevenLabs returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Voices []struct {
			VoiceID  string `json:"voice_id"`
			Name     string `json:"name"`
			Category string `json:"category"`
			Labels   map[string]string `json:"labels"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("cannot parse voices response: %w", err)
	}

	fmt.Printf("\n  Available Voices (%d):\n", len(result.Voices))
	fmt.Println("  ─────────────────────────────────────────────────────────")
	for _, v := range result.Voices {
		category := v.Category
		if category == "cloned" {
			category = "CLONED"
		}
		desc := ""
		if accent, ok := v.Labels["accent"]; ok {
			desc = accent
		}
		if gender, ok := v.Labels["gender"]; ok {
			if desc != "" {
				desc += ", "
			}
			desc += gender
		}
		if desc != "" {
			desc = " (" + desc + ")"
		}
		marker := "  "
		if v.VoiceID == cfg.ElevenLabsVoiceID {
			marker = "→ "
		}
		fmt.Printf("  %s%-8s  %-24s  %s%s\n", marker, category, v.Name, v.VoiceID, desc)
	}
	fmt.Println("  ─────────────────────────────────────────────────────────")
	fmt.Printf("  → = currently active in config\n")

	return nil
}

// elevenLabsDeleteVoice removes a cloned voice from the account.
func elevenLabsDeleteVoice(cfg *Config, voiceID string) error {
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/voices/%s", voiceID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", cfg.ElevenLabsAPIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ElevenLabs returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// scanVoiceSamples finds audio files in a directory.
func scanVoiceSamples(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	audioExts := map[string]bool{".mp3": true, ".wav": true, ".m4a": true, ".ogg": true, ".webm": true, ".flac": true}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if audioExts[ext] {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}
