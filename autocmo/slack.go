package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
"net/http"
	"os"
	"path/filepath"
	"time"
)

func slackPost(cfg *Config, videoPath, voiceoverPath string, meta *VideoMeta, customMessage string) error {
	// Step 1: Upload files via Slack Bot Token (files.upload v2 flow)
	var fileLinks []string

	if videoPath != "" {
		link, err := slackUploadFile(cfg, videoPath, "Ad Video")
		if err != nil {
			fmt.Printf("  [WARN] Video upload failed: %v\n", err)
		} else {
			fileLinks = append(fileLinks, link)
		}
	}

	if voiceoverPath != "" {
		link, err := slackUploadFile(cfg, voiceoverPath, "Voiceover")
		if err != nil {
			fmt.Printf("  [WARN] Voiceover upload failed: %v\n", err)
		} else {
			fileLinks = append(fileLinks, link)
		}
	}

	// Step 2: Post rich message via webhook
	timestamp := time.Now().Format("2006-01-02 15:04")
	scriptPreview := meta.Script
	if len(scriptPreview) > 2000 {
		scriptPreview = scriptPreview[:1997] + "..."
	}
	if scriptPreview == "" {
		scriptPreview = "No script available"
	}

	blocks := []map[string]interface{}{
		{
			"type": "header",
			"text": map[string]string{
				"type": "plain_text",
				"text": fmt.Sprintf("New Ad — %s", cfg.ProductName),
			},
		},
		{
			"type": "context",
			"elements": []map[string]string{
				{"type": "mrkdwn", "text": fmt.Sprintf("Auto-generated on %s", timestamp)},
			},
		},
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Script*\n%s", scriptPreview),
			},
		},
		{
			"type": "section",
			"fields": []map[string]string{
				{"type": "mrkdwn", "text": fmt.Sprintf("*Model*\n%s", stringOr(meta.Model, "unknown"))},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Format*\n%s", stringOr(meta.VideoFormat, "9:16"))},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Duration*\n%ds", meta.Duration)},
			},
		},
	}

	// Add file links if we have them
	if len(fileLinks) > 0 {
		linksText := ""
		for _, l := range fileLinks {
			linksText += l + "\n"
		}
		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Files*\n%s", linksText),
			},
		})
	}

	blocks = append(blocks, map[string]interface{}{
		"type": "context",
		"elements": []map[string]string{
			{"type": "plain_text", "text": "AutoCMO"},
		},
	})

	payload := map[string]interface{}{
		"blocks": blocks,
	}
	if customMessage != "" {
		payload["text"] = customMessage
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", cfg.SlackWebhookURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Slack webhook failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Slack returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// slackUploadFile uploads a file to Slack using the v2 upload flow:
// 1. files.getUploadURLExternal → get a presigned URL
// 2. PUT file data to that URL
// 3. files.completeUploadExternal → finalize and share to channel
func slackUploadFile(cfg *Config, filePath, title string) (string, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	fileName := filepath.Base(filePath)
	fileSize := fileInfo.Size()

	// Step 1: Get upload URL
	getURLPayload := fmt.Sprintf("filename=%s&length=%d", fileName, fileSize)
	req, err := http.NewRequest("POST", "https://slack.com/api/files.getUploadURLExternal",
		bytes.NewReader([]byte(getURLPayload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.SlackBotToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("getUploadURLExternal failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var urlResult struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		UploadURL string `json:"upload_url"`
		FileID   string `json:"file_id"`
	}
	if err := json.Unmarshal(body, &urlResult); err != nil {
		return "", fmt.Errorf("cannot parse getUploadURL response: %w", err)
	}
	if !urlResult.OK {
		return "", fmt.Errorf("getUploadURLExternal error: %s", urlResult.Error)
	}

	// Step 2: PUT file data to the presigned URL
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	putReq, err := http.NewRequest("POST", urlResult.UploadURL, bytes.NewReader(fileData))
	if err != nil {
		return "", err
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")

	uploadClient := &http.Client{Timeout: 120 * time.Second}
	putResp, err := uploadClient.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("file upload PUT failed: %w", err)
	}
	putResp.Body.Close()

	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		return "", fmt.Errorf("file upload PUT returned HTTP %d", putResp.StatusCode)
	}

	// Step 3: Complete the upload and share to channel
	completePayload := map[string]interface{}{
		"files": []map[string]string{
			{"id": urlResult.FileID, "title": title},
		},
		"channel_id": cfg.SlackChannel,
	}
	completeJSON, _ := json.Marshal(completePayload)

	completeReq, err := http.NewRequest("POST", "https://slack.com/api/files.completeUploadExternal",
		bytes.NewReader(completeJSON))
	if err != nil {
		return "", err
	}
	completeReq.Header.Set("Authorization", "Bearer "+cfg.SlackBotToken)
	completeReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	completeResp, err := client.Do(completeReq)
	if err != nil {
		return "", fmt.Errorf("completeUploadExternal failed: %w", err)
	}
	defer completeResp.Body.Close()
	completeBody, _ := io.ReadAll(completeResp.Body)

	var completeResult struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Files []struct {
			ID        string `json:"id"`
			Permalink string `json:"permalink"`
		} `json:"files"`
	}
	if err := json.Unmarshal(completeBody, &completeResult); err != nil {
		return "", fmt.Errorf("cannot parse complete response: %w", err)
	}
	if !completeResult.OK {
		return "", fmt.Errorf("completeUploadExternal error: %s", completeResult.Error)
	}

	permalink := ""
	if len(completeResult.Files) > 0 {
		permalink = completeResult.Files[0].Permalink
	}

	fmt.Printf("  Uploaded: %s\n", fileName)
	return permalink, nil
}

func stringOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
