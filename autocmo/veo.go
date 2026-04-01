package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// veoGenerateVideo generates a short video via Google Veo and saves it as an MP4.
// Uses a long-running operation pattern: submit → poll → download.
func veoGenerateVideo(cfg *Config, prompt string, outputPath string) error {
	// Submit generation job
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/veo-3.0-fast-generate-001:predictLongRunning?key=%s",
		cfg.GoogleAPIKey,
	)

	payload := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"aspectRatio":    "9:16",
			"durationSeconds": 8,
			"sampleCount":    1,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Veo request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Veo returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse operation name
	var opResult struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &opResult); err != nil {
		return fmt.Errorf("cannot parse Veo response: %w", err)
	}
	if opResult.Name == "" {
		return fmt.Errorf("no operation name in Veo response")
	}

	fmt.Printf("  Operation: %s\n", opResult.Name)

	// Poll for completion
	pollURL := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/%s?key=%s",
		opResult.Name, cfg.GoogleAPIKey,
	)
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < 60; i++ {
		time.Sleep(10 * time.Second)

		pollReq, err := http.NewRequest("GET", pollURL, nil)
		if err != nil {
			return err
		}

		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			fmt.Printf("  Poll error (retrying): %v\n", err)
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Done  bool `json:"done"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
			Response *struct {
				GenerateVideoResponse *struct {
					GeneratedSamples []struct {
						Video *struct {
							URI string `json:"uri,omitempty"`
							B64 string `json:"bytesBase64Encoded,omitempty"`
						} `json:"video,omitempty"`
					} `json:"generatedSamples"`
				} `json:"generateVideoResponse,omitempty"`
			} `json:"response,omitempty"`
		}

		if err := json.Unmarshal(pollBody, &status); err != nil {
			fmt.Printf("  Parse error (retrying): %v\n", err)
			continue
		}

		if status.Error != nil {
			return fmt.Errorf("Veo generation failed: %s", status.Error.Message)
		}

		if !status.Done {
			fmt.Printf("  Generating... (%ds elapsed)\n", (i+1)*10)
			continue
		}

		// Done — extract video
		gvr := status.Response.GenerateVideoResponse
		if status.Response == nil || gvr == nil || len(gvr.GeneratedSamples) == 0 {
			return fmt.Errorf("no generated samples in Veo response")
		}

		sample := gvr.GeneratedSamples[0]
		if sample.Video == nil {
			return fmt.Errorf("no video in Veo sample")
		}

		// Base64 encoded
		if sample.Video.B64 != "" {
			videoData, err := base64.StdEncoding.DecodeString(sample.Video.B64)
			if err != nil {
				return fmt.Errorf("cannot decode video data: %w", err)
			}
			if err := os.WriteFile(outputPath, videoData, 0644); err != nil {
				return err
			}
			fmt.Printf("  Generated: video.mp4 (%.0f KB)\n", float64(len(videoData))/1024)
			return nil
		}

		// URI — download with auth header (Veo file URIs need the API key)
		if sample.Video.URI != "" {
			return downloadFileWithKey(sample.Video.URI, outputPath, cfg.GoogleAPIKey)
		}

		return fmt.Errorf("video has neither base64 data nor URI")
	}

	return fmt.Errorf("timed out after 10 minutes waiting for Veo generation")
}
