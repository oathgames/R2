package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// heygenGenerate creates a production-quality talking-head UGC video via HeyGen.
// Uses Avatar IV for realistic facial motion and lip-sync.
// Two modes:
//   - Text mode: HeyGen generates voice + video from script text
//   - Audio mode: Uses your ElevenLabs voiceover + HeyGen avatar visuals
func heygenGenerate(cfg *Config, cmd *Command, script string) (*VideoMeta, error) {
	aspectRatio := "9:16"
	if cmd != nil && cmd.Format != "" {
		aspectRatio = cmd.Format
	}

	// Build voice config
	voice := map[string]interface{}{
		"type":       "text",
		"input_text": script,
		"voice_id":   "4754e1ec667544b0bd18cdf4bec7d6a7", // Brittney — natural, casual
		"speed":      1.0,
	}

	// Build character config — talking_photo (custom image) or pre-built avatar
	var character map[string]interface{}
	avatarLabel := ""

	// Check for user's avatar photo in avatars/ folder
	avatarPhotoPath := findAvatarPhoto(cfg)
	if avatarPhotoPath != "" {
		fmt.Printf("  Uploading avatar photo to HeyGen...\n")
		talkingPhotoID, err := heygenUploadTalkingPhoto(cfg, avatarPhotoPath)
		if err == nil {
			character = map[string]interface{}{
				"type":             "talking_photo",
				"talking_photo_id": talkingPhotoID,
			}
			avatarLabel = "talking_photo (" + filepath.Base(avatarPhotoPath) + ")"
		} else {
			fmt.Printf("  [WARN] Photo upload failed: %v — using pre-built avatar\n", err)
		}
	}

	// Fall back to pre-built avatar
	if character == nil {
		avatarID := "Annie_expressive5_public"
		if cmd != nil && cmd.AvatarID != "" {
			avatarID = cmd.AvatarID
		}
		character = map[string]interface{}{
			"type":         "avatar",
			"avatar_id":    avatarID,
			"avatar_style": "normal",
			"scale":        1,
		}
		avatarLabel = avatarID
	}

	payload := map[string]interface{}{
		"title": fmt.Sprintf("AutoCMO — %s", cfg.ProductName),
		"video_inputs": []map[string]interface{}{
			{
				"character":  character,
				"voice":      voice,
				"background": map[string]interface{}{
					"type":  "color",
					"value": "#f5f5f5",
				},
			},
		},
		"dimension": heygenDimension(aspectRatio),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	fmt.Printf("  Avatar: %s\n", avatarLabel)

	req, err := http.NewRequest("POST", "https://api.heygen.com/v2/video/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", cfg.HeyGenAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HeyGen request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HeyGen returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var createResult struct {
		Error *string `json:"error"`
		Data  struct {
			VideoID string `json:"video_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &createResult); err != nil {
		return nil, fmt.Errorf("cannot parse HeyGen response: %w", err)
	}
	if createResult.Error != nil {
		return nil, fmt.Errorf("HeyGen error: %s", *createResult.Error)
	}
	if createResult.Data.VideoID == "" {
		return nil, fmt.Errorf("no video_id in HeyGen response")
	}

	videoID := createResult.Data.VideoID
	fmt.Printf("  Video ID: %s\n", videoID)

	// Poll for completion
	return heygenPoll(cfg, videoID, script, aspectRatio)
}

// heygenListAvatars returns available avatars for selection.
func heygenListAvatars(cfg *Config) error {
	req, err := http.NewRequest("GET", "https://api.heygen.com/v2/avatars", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", cfg.HeyGenAPIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HeyGen request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("HeyGen returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			Avatars []struct {
				AvatarID   string `json:"avatar_id"`
				AvatarName string `json:"avatar_name"`
				Gender     string `json:"gender"`
			} `json:"avatars"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("cannot parse avatars response: %w", err)
	}

	fmt.Printf("\n  Available HeyGen Avatars (%d):\n", len(result.Data.Avatars))
	fmt.Println("  ─────────────────────────────────────────────────")
	for _, a := range result.Data.Avatars {
		fmt.Printf("  %-12s %-24s %s\n", a.Gender, a.AvatarName, a.AvatarID)
	}
	fmt.Println("  ─────────────────────────────────────────────────")

	return nil
}

func heygenPoll(cfg *Config, videoID, script, aspectRatio string) (*VideoMeta, error) {
	pollURL := fmt.Sprintf("https://api.heygen.com/v1/video_status.get?video_id=%s", videoID)
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < 60; i++ {
		time.Sleep(10 * time.Second)

		pollReq, _ := http.NewRequest("GET", pollURL, nil)
		pollReq.Header.Set("X-Api-Key", cfg.HeyGenAPIKey)

		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			fmt.Printf("  Poll error (retrying): %v\n", err)
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Data struct {
				Status   string  `json:"status"`
				VideoURL string  `json:"video_url"`
				Error    *string `json:"error"`
				Duration float64 `json:"duration"`
			} `json:"data"`
		}

		if err := json.Unmarshal(pollBody, &status); err != nil {
			continue
		}

		switch status.Data.Status {
		case "completed":
			return &VideoMeta{
				ID:          videoID,
				Status:      "completed",
				VideoURL:    status.Data.VideoURL,
				DownloadURL: status.Data.VideoURL,
				Script:      script,
				VideoFormat: aspectRatio,
				Duration:    int(status.Data.Duration),
				Model:       "heygen/avatar-iv",
			}, nil
		case "failed":
			errMsg := "unknown error"
			if status.Data.Error != nil {
				errMsg = *status.Data.Error
			}
			return nil, fmt.Errorf("HeyGen failed: %s", errMsg)
		default:
			fmt.Printf("  Generating... (%ds elapsed)\n", (i+1)*10)
		}
	}

	return nil, fmt.Errorf("timed out after 10 minutes")
}

// heygenUploadTalkingPhoto uploads a photo to HeyGen's asset API and returns the asset ID
// which can be used as a talking_photo_id for video generation.
func heygenUploadTalkingPhoto(cfg *Config, photoPath string) (string, error) {
	data, err := os.ReadFile(photoPath)
	if err != nil {
		return "", fmt.Errorf("cannot read photo: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(photoPath))
	contentType := "image/jpeg"
	if ext == ".png" {
		contentType = "image/png"
	}

	req, err := http.NewRequest("POST", "https://upload.heygen.com/v1/talking_photo", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Api-Key", cfg.HeyGenAPIKey)
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HeyGen upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HeyGen upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			TalkingPhotoID  string `json:"talking_photo_id"`
			TalkingPhotoURL string `json:"talking_photo_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("cannot parse upload response: %w", err)
	}

	if result.Data.TalkingPhotoID == "" {
		return "", fmt.Errorf("no talking_photo_id in response: %s", string(body))
	}

	fmt.Printf("  Talking photo ID: %s\n", result.Data.TalkingPhotoID)
	return result.Data.TalkingPhotoID, nil
}

func heygenDimension(aspectRatio string) map[string]int {
	switch aspectRatio {
	case "16:9":
		return map[string]int{"width": 1920, "height": 1080}
	case "1:1":
		return map[string]int{"width": 1080, "height": 1080}
	default: // 9:16
		return map[string]int{"width": 1080, "height": 1920}
	}
}
