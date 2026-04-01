package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fal.ai model slugs for text-to-video
var falVideoModels = map[string]string{
	"kling":    "fal-ai/kling-video/v2.1/master/text-to-video",
	"veo":      "fal-ai/veo3",
	"seedance": "fal-ai/seedance-1.5-pro/text-to-video",
	"minimax":  "fal-ai/minimax/video-01-live",
	"hunyuan":  "fal-ai/hunyuan-video",
	"wan":      "fal-ai/wan/v2.1/1.3b",
}

// Fal.ai model slugs for image-to-video
var falI2VModels = map[string]string{
	"kling":    "fal-ai/kling-video/v2.1/master/image-to-video",
	"veo":      "fal-ai/veo2/image-to-video",
	"seedance": "fal-ai/seedance-1.5-pro/image-to-video",
	"minimax":  "fal-ai/minimax/video-01-live/image-to-video",
	"wan":      "fal-ai/wan/v2.1/1.3b/image-to-video",
}

// falGenerateVideo submits a text-to-video request to fal.ai's queue API,
// polls for completion, and downloads the result.
func falGenerateVideo(cfg *Config, model, prompt, outputPath string, durationSecs int) error {
	// Resolve model slug
	slug, ok := falVideoModels[model]
	if !ok {
		// Allow raw slugs like "fal-ai/kling-video/v2.1/master/text-to-video"
		slug = model
	}

	fmt.Printf("  Model: %s\n", slug)

	// Step 1: Submit to queue
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)

	if durationSecs <= 0 {
		durationSecs = 5
	}

	payload := map[string]interface{}{
		"prompt":       prompt,
		"aspect_ratio": "9:16",
	}

	// Model-specific duration formatting and fields
	switch model {
	case "veo":
		// Veo accepts "4s", "6s", or "8s" — snap to nearest valid
		veoDur := "4s"
		if durationSecs >= 7 {
			veoDur = "8s"
		} else if durationSecs >= 5 {
			veoDur = "6s"
		}
		payload["duration"] = veoDur
	case "kling":
		payload["duration"] = durationSecs
		payload["cfg_scale"] = 0.5
		payload["negative_prompt"] = "blur, distort, low quality, watermark, text overlay"
	default:
		payload["duration"] = durationSecs
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal.ai submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal.ai returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	if err := json.Unmarshal(respBody, &submitResult); err != nil {
		return fmt.Errorf("cannot parse fal.ai submit response: %w", err)
	}

	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in fal.ai response")
	}
	fmt.Printf("  Request: %s\n", submitResult.RequestID)

	// Step 2: Poll for completion — use URLs from submit response (fal shortens the slug)
	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL
	if statusURL == "" {
		statusURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s/status", slug, submitResult.RequestID)
	}
	if resultURL == "" {
		resultURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s", slug, submitResult.RequestID)
	}
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < 120; i++ { // Up to 20 minutes
		time.Sleep(10 * time.Second)

		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)

		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			fmt.Printf("  Poll error (retrying): %v\n", err)
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status        string `json:"status"`
			QueuePosition *int   `json:"queue_position,omitempty"`
		}
		json.Unmarshal(pollBody, &status)

		switch status.Status {
		case "COMPLETED":
			fmt.Printf("  Generation complete (%ds)\n", (i+1)*10)
			// Fetch result
			return falDownloadResult(cfg, resultURL, outputPath)

		case "IN_QUEUE":
			pos := ""
			if status.QueuePosition != nil {
				pos = fmt.Sprintf(", position %d", *status.QueuePosition)
			}
			fmt.Printf("  Queued%s... (%ds elapsed)\n", pos, (i+1)*10)

		case "IN_PROGRESS":
			fmt.Printf("  Generating... (%ds elapsed)\n", (i+1)*10)

		default:
			// Check if it's an error
			var errResp struct {
				Detail string `json:"detail"`
			}
			if json.Unmarshal(pollBody, &errResp) == nil && errResp.Detail != "" {
				return fmt.Errorf("fal.ai error: %s", errResp.Detail)
			}
			fmt.Printf("  Status: %s (%ds elapsed)\n", status.Status, (i+1)*10)
		}
	}

	return fmt.Errorf("timed out after 20 minutes waiting for fal.ai generation")
}

// falUploadFile uploads a local file to fal.ai CDN and returns a public URL.
// Uses POST https://v3.fal.media/files/upload (the same endpoint the Python SDK uses).
func falUploadFile(cfg *Config, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	contentType := "application/octet-stream"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".webp":
		contentType = "image/webp"
	case ".mp3":
		contentType = "audio/mpeg"
	case ".wav":
		contentType = "audio/wav"
	case ".m4a":
		contentType = "audio/mp4"
	case ".ogg":
		contentType = "audio/ogg"
	}

	// Sanitize filename — spaces and special chars break fal's URL validation
	fileName := filepath.Base(filePath)
	fileName = strings.ReplaceAll(fileName, " ", "_")
	fileName = strings.ReplaceAll(fileName, "(", "")
	fileName = strings.ReplaceAll(fileName, ")", "")

	// Try the fal REST API upload endpoint first, then CDN fallback
	uploadURLs := []string{
		"https://rest.fal.run/fal-ai/storage/upload",
		"https://v3.fal.media/files/upload",
		"https://fal.media/files/upload",
	}

	var lastErr error
	for _, uploadURL := range uploadURLs {
		req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(data))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-Fal-File-Name", fileName)

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			var result struct {
				AccessURL string `json:"access_url"`
				URL       string `json:"url"`
				FileURL   string `json:"file_url"`
			}
			if json.Unmarshal(body, &result) == nil {
				for _, u := range []string{result.AccessURL, result.URL, result.FileURL} {
					if u != "" {
						fmt.Printf("  Uploaded: %s\n", fileName)
						return u, nil
					}
				}
			}
			urlStr := strings.TrimSpace(string(body))
			if strings.HasPrefix(urlStr, "http") {
				fmt.Printf("  Uploaded: %s\n", fileName)
				return urlStr, nil
			}
		}

		lastErr = fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, uploadURL, string(body))
	}

	// Last resort: use a data URI for small files (<5MB)
	if len(data) < 5*1024*1024 {
		fmt.Printf("  Using inline data URI (%s, %.0f KB)\n", fileName, float64(len(data))/1024)
		dataURI := fmt.Sprintf("data:%s;base64,%s",
			contentType,
			base64.StdEncoding.EncodeToString(data),
		)
		return dataURI, nil
	}

	return "", fmt.Errorf("all upload endpoints failed: %v", lastErr)
}

// falGenerateImageToVideo creates a video from a reference image using fal.ai's i2v models.
func falGenerateImageToVideo(cfg *Config, model, prompt, imageURL, outputPath string, durationSecs int) error {
	slug, ok := falI2VModels[model]
	if !ok {
		// Fall back to text-to-video if no i2v model available
		fmt.Printf("  [NOTE] No image-to-video model for %s, falling back to text-to-video\n", model)
		return falGenerateVideo(cfg, model, prompt, outputPath, durationSecs)
	}

	if durationSecs <= 0 {
		durationSecs = 5
	}

	fmt.Printf("  Model: %s (image-to-video)\n", slug)
	fmt.Printf("  Source: %s\n", imageURL)

	payload := map[string]interface{}{
		"prompt":    prompt,
		"image_url": imageURL,
	}

	// Model-specific duration formatting
	switch model {
	case "veo":
		veoDur := "4s"
		if durationSecs >= 7 {
			veoDur = "8s"
		} else if durationSecs >= 5 {
			veoDur = "6s"
		}
		payload["duration"] = veoDur
	case "kling":
		payload["duration"] = fmt.Sprintf("%d", durationSecs)
		payload["cfg_scale"] = 0.5
	default:
		payload["duration"] = durationSecs
	}

	body, _ := json.Marshal(payload)
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)

	req, _ := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal.ai i2v submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal.ai i2v returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	json.Unmarshal(respBody, &submitResult)

	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in fal i2v response")
	}
	fmt.Printf("  Request: %s\n", submitResult.RequestID)

	// Poll (reuse same polling logic)
	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL
	if statusURL == "" {
		statusURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s/status", slug, submitResult.RequestID)
	}
	if resultURL == "" {
		resultURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s", slug, submitResult.RequestID)
	}

	pollClient := &http.Client{Timeout: 15 * time.Second}
	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)

		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(pollBody, &status)

		switch status.Status {
		case "COMPLETED":
			fmt.Printf("  Generation complete (%ds)\n", (i+1)*10)
			return falDownloadResult(cfg, resultURL, outputPath)
		case "IN_QUEUE":
			fmt.Printf("  Queued... (%ds elapsed)\n", (i+1)*10)
		case "IN_PROGRESS":
			fmt.Printf("  Generating... (%ds elapsed)\n", (i+1)*10)
		default:
			fmt.Printf("  Status: %s (%ds elapsed)\n", status.Status, (i+1)*10)
		}
	}

	return fmt.Errorf("timed out waiting for fal i2v generation")
}

// falLipSync takes a real video clip + voiceover audio and produces a lip-synced video.
// Uses Kling LipSync — the person's natural body motion is preserved,
// only the mouth is AI-driven to match the audio. Highest quality lip-sync path.
func falLipSync(cfg *Config, videoURL, audioURL, outputPath string) error {
	slug := "fal-ai/kling-video/lipsync/audio-to-video"

	fmt.Printf("  Model: %s\n", slug)

	payload := map[string]interface{}{
		"video_url": videoURL,
		"audio_url": audioURL,
	}

	body, _ := json.Marshal(payload)
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)

	req, _ := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal lip-sync submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal lip-sync HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	json.Unmarshal(respBody, &submitResult)
	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in lip-sync response")
	}
	fmt.Printf("  Request: %s\n", submitResult.RequestID)

	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL
	if statusURL == "" {
		statusURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s/status", slug, submitResult.RequestID)
	}
	if resultURL == "" {
		resultURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s", slug, submitResult.RequestID)
	}

	pollClient := &http.Client{Timeout: 15 * time.Second}
	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(pollBody, &status)

		switch status.Status {
		case "COMPLETED":
			fmt.Printf("  Lip-sync complete (%ds)\n", (i+1)*10)
			return falDownloadResult(cfg, resultURL, outputPath)
		case "IN_QUEUE":
			fmt.Printf("  Queued... (%ds elapsed)\n", (i+1)*10)
		case "IN_PROGRESS":
			fmt.Printf("  Lip-syncing... (%ds elapsed)\n", (i+1)*10)
		default:
			fmt.Printf("  Status: %s (%ds elapsed)\n", status.Status, (i+1)*10)
		}
	}
	return fmt.Errorf("timed out waiting for lip-sync")
}

// falGenerateAvatar creates a talking-head video using Kling AI Avatar v2.
// Takes a face photo + voiceover audio → generates lip-synced video.
// This is the highest quality path for talking-head ad content.
func falGenerateAvatar(cfg *Config, avatarImageURL, audioURL, prompt, outputPath string) error {
	slug := "fal-ai/kling-video/ai-avatar/v2/standard"

	fmt.Printf("  Model: %s (avatar lip-sync)\n", slug)

	payload := map[string]interface{}{
		"image_url": avatarImageURL,
		"audio_url": audioURL,
	}
	if prompt != "" {
		payload["prompt"] = prompt
	}

	body, _ := json.Marshal(payload)
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)

	req, _ := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal avatar submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal avatar HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	json.Unmarshal(respBody, &submitResult)
	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in avatar response")
	}
	fmt.Printf("  Request: %s\n", submitResult.RequestID)

	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL
	if statusURL == "" {
		statusURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s/status", slug, submitResult.RequestID)
	}
	if resultURL == "" {
		resultURL = fmt.Sprintf("https://queue.fal.run/%s/requests/%s", slug, submitResult.RequestID)
	}

	pollClient := &http.Client{Timeout: 15 * time.Second}
	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(pollBody, &status)

		switch status.Status {
		case "COMPLETED":
			fmt.Printf("  Avatar generation complete (%ds)\n", (i+1)*10)
			return falDownloadResult(cfg, resultURL, outputPath)
		case "IN_QUEUE":
			fmt.Printf("  Queued... (%ds elapsed)\n", (i+1)*10)
		case "IN_PROGRESS":
			fmt.Printf("  Generating avatar... (%ds elapsed)\n", (i+1)*10)
		default:
			fmt.Printf("  Status: %s (%ds elapsed)\n", status.Status, (i+1)*10)
		}
	}
	return fmt.Errorf("timed out waiting for avatar generation")
}

// falGenerateImage generates a product image via Flux Pro on fal.ai.
// Supports portrait (4:5), square (1:1), and landscape formats.
var falImageModels = map[string]string{
	"flux":         "fal-ai/flux-pro/v1.1",
	"ideogram":     "fal-ai/ideogram/v3",
	"recraft":      "fal-ai/recraft/v3",
	"seedream":     "fal-ai/bytedance/seedream/v4.5/text-to-image",
	"imagen":       "fal-ai/imagen4/preview",
	"imagen-ultra": "fal-ai/imagen4/preview/ultra",
	"banana":           "fal-ai/nano-banana-2",
	"banana-edit":      "fal-ai/nano-banana-2/edit",
	"banana-pro":       "fal-ai/nano-banana-pro",
	"banana-pro-edit":  "fal-ai/nano-banana-pro/edit",
}

func falGenerateImage(cfg *Config, prompt, outputPath, format, imageModel string) error {
	slug := "fal-ai/nano-banana-pro"
	if imageModel != "" {
		// Safety: if an "-edit" model reaches text-to-image path, strip to base model
		cleanModel := imageModel
		if strings.HasSuffix(cleanModel, "-edit") {
			cleanModel = strings.TrimSuffix(cleanModel, "-edit")
		}
		if s, ok := falImageModels[cleanModel]; ok {
			slug = s
		}
	}
	fmt.Printf("  Image model: %s\n", slug)

	// Map our formats to fal's image_size enum
	imageSize := "portrait_4_3"
	switch format {
	case "square", "1:1":
		imageSize = "square_hd"
	case "portrait", "4:5":
		imageSize = "portrait_4_3" // Closest to 4:5
	case "landscape", "16:9":
		imageSize = "landscape_16_9"
	}

	payload := map[string]interface{}{
		"prompt":     prompt,
		"num_images": 1,
	}

	// Nano Banana uses aspect_ratio + resolution instead of image_size
	if strings.HasPrefix(slug, "fal-ai/nano-banana") {
		aspectRatio := "4:5"
		switch format {
		case "square", "1:1":
			aspectRatio = "1:1"
		case "portrait", "4:5":
			aspectRatio = "4:5"
		case "landscape", "16:9":
			aspectRatio = "16:9"
		}
		payload["aspect_ratio"] = aspectRatio
		payload["resolution"] = "1K"
	} else {
		payload["image_size"] = imageSize
	}

	body, _ := json.Marshal(payload)

	// Flux Pro is fast enough for synchronous calls
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)
	req, _ := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal image submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal image HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	json.Unmarshal(respBody, &submitResult)

	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in fal image response")
	}

	// Poll for completion
	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL

	pollClient := &http.Client{Timeout: 15 * time.Second}
	for i := 0; i < 30; i++ {
		time.Sleep(3 * time.Second)
		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(pollBody, &status)

		if status.Status == "COMPLETED" {
			// Fetch result
			resReq, _ := http.NewRequest("GET", resultURL, nil)
			resReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
			resResp, err := pollClient.Do(resReq)
			if err != nil {
				return fmt.Errorf("fal image result failed: %w", err)
			}
			resBody, _ := io.ReadAll(resResp.Body)
			resResp.Body.Close()

			var result struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			}
			json.Unmarshal(resBody, &result)

			if len(result.Images) == 0 || result.Images[0].URL == "" {
				return fmt.Errorf("no image URL in fal result")
			}

			// Download the image
			return downloadFile(result.Images[0].URL, outputPath)
		}
	}

	return fmt.Errorf("timed out waiting for fal image generation")
}

// falGenerateImageEdit generates a product image using reference images via an edit model.
// imageURLs should be fal CDN URLs of the reference images.
// Defaults to Nano Banana Pro Edit if no specific slug is resolved.
func falGenerateImageEdit(cfg *Config, prompt, outputPath, format string, imageURLs []string) error {
	slug := "fal-ai/nano-banana-pro/edit"
	fmt.Printf("  Image model: %s (image-to-image)\n", slug)
	fmt.Printf("  Reference images: %d\n", len(imageURLs))

	aspectRatio := "4:5"
	switch format {
	case "square", "1:1":
		aspectRatio = "1:1"
	case "portrait", "4:5":
		aspectRatio = "4:5"
	case "landscape", "16:9":
		aspectRatio = "16:9"
	}

	payload := map[string]interface{}{
		"prompt":       prompt,
		"image_urls":   imageURLs,
		"aspect_ratio": aspectRatio,
		"resolution":   "1K",
		"num_images":   1,
	}

	body, _ := json.Marshal(payload)
	submitURL := fmt.Sprintf("https://queue.fal.run/%s", slug)

	req, _ := http.NewRequest("POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal image-edit submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal image-edit HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResult struct {
		RequestID   string `json:"request_id"`
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	json.Unmarshal(respBody, &submitResult)
	if submitResult.RequestID == "" {
		return fmt.Errorf("no request_id in fal image-edit response")
	}
	fmt.Printf("  Request: %s\n", submitResult.RequestID)

	statusURL := submitResult.StatusURL
	resultURL := submitResult.ResponseURL
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < 30; i++ {
		time.Sleep(3 * time.Second)
		pollReq, _ := http.NewRequest("GET", statusURL, nil)
		pollReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		json.Unmarshal(pollBody, &status)

		if status.Status == "COMPLETED" {
			resReq, _ := http.NewRequest("GET", resultURL, nil)
			resReq.Header.Set("Authorization", "Key "+cfg.FalAPIKey)
			resResp, err := pollClient.Do(resReq)
			if err != nil {
				return fmt.Errorf("fal image-edit result failed: %w", err)
			}
			resBody, _ := io.ReadAll(resResp.Body)
			resResp.Body.Close()

			var result struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			}
			json.Unmarshal(resBody, &result)

			if len(result.Images) == 0 || result.Images[0].URL == "" {
				return fmt.Errorf("no image URL in fal image-edit result")
			}
			return downloadFile(result.Images[0].URL, outputPath)
		}
	}
	return fmt.Errorf("timed out waiting for fal image-edit generation")
}

// falDownloadResult fetches the completed video from fal.ai and saves it.
func falDownloadResult(cfg *Config, resultURL, outputPath string) error {
	req, _ := http.NewRequest("GET", resultURL, nil)
	req.Header.Set("Authorization", "Key "+cfg.FalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fal.ai result fetch failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal.ai result HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse result — video URL is in result.video.url
	var result struct {
		Video *struct {
			URL string `json:"url"`
		} `json:"video,omitempty"`
		// Some models return an array
		Videos []struct {
			URL string `json:"url"`
		} `json:"videos,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("cannot parse fal.ai result: %w", err)
	}

	videoURL := ""
	if result.Video != nil && result.Video.URL != "" {
		videoURL = result.Video.URL
	} else if len(result.Videos) > 0 && result.Videos[0].URL != "" {
		videoURL = result.Videos[0].URL
	}

	if videoURL == "" {
		return fmt.Errorf("no video URL in fal.ai result: %s", string(respBody))
	}

	// Download the video file
	return downloadFile(videoURL, outputPath)
}
