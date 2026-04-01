package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VideoMeta holds the Arcads API response fields we care about.
type VideoMeta struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	VideoURL    string `json:"video_url"`
	DownloadURL string `json:"download_url"`
	Script      string `json:"script"`
	VideoFormat string `json:"video_format"`
	Duration    int    `json:"duration"`
	Error       string `json:"error"`
	Model       string `json:"model"` // e.g. "fal/veo", "fal/kling", "veo-direct", "arcads", "heygen"
}

func arcadsGenerate(cfg *Config, cmd *Command) (*VideoMeta, error) {
	// Defaults
	format := "9:16"
	language := "en"
	scriptType := "auto"

	// Apply command overrides
	if cmd != nil {
		if cmd.Format != "" {
			format = cmd.Format
		}
		if cmd.Language != "" {
			language = cmd.Language
		}
		if cmd.Script != "" {
			scriptType = "custom"
		}
	}

	payload := map[string]interface{}{
		"product_url":         cfg.ProductURL,
		"product_name":        cfg.ProductName,
		"product_description": cfg.ProductDescription,
		"script_type":         scriptType,
		"video_format":        format,
		"language":            language,
	}

	// If Claude provided a custom script, include it
	if cmd != nil && cmd.Script != "" {
		payload["script"] = cmd.Script
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.arcads.ai/v1/videos", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.ArcadsAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, fmt.Errorf("Arcads returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var meta VideoMeta
	if err := json.Unmarshal(respBody, &meta); err != nil {
		return nil, fmt.Errorf("cannot parse Arcads response: %w", err)
	}

	// Poll for completion
	jobID := meta.ID
	if jobID == "" {
		return nil, fmt.Errorf("no job ID in Arcads response")
	}

	fmt.Printf("  Job created: %s\n", jobID)
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < 60; i++ { // ~10 min max
		time.Sleep(10 * time.Second)

		pollReq, err := http.NewRequest("GET", fmt.Sprintf("https://api.arcads.ai/v1/videos/%s", jobID), nil)
		if err != nil {
			return nil, err
		}
		pollReq.Header.Set("Authorization", "Bearer "+cfg.ArcadsAPIKey)

		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			fmt.Printf("  Poll error (retrying): %v\n", err)
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status VideoMeta
		if err := json.Unmarshal(pollBody, &status); err != nil {
			fmt.Printf("  Parse error (retrying): %v\n", err)
			continue
		}

		switch status.Status {
		case "completed":
			fmt.Println("  Video generation complete.")
			return &status, nil
		case "failed":
			return nil, fmt.Errorf("video generation failed: %s", status.Error)
		default:
			fmt.Printf("  Waiting... (%s)\n", status.Status)
		}
	}

	return nil, fmt.Errorf("timed out after 10 minutes waiting for video generation")
}
