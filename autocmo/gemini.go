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
	"time"
)

// geminiGenerateImage generates a product image via Gemini, optionally using reference images.
func geminiGenerateImage(cfg *Config, prompt string, outputPath string, refs []ReferenceImage) error {
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-image-preview:generateContent?key=%s",
		cfg.GoogleAPIKey,
	)

	// Build parts: text prompt + any reference images
	parts := []map[string]interface{}{}

	// Add reference images first so Gemini sees them before the instruction
	for _, ref := range refs {
		parts = append(parts, map[string]interface{}{
			"inlineData": map[string]string{
				"mimeType": ref.MimeType,
				"data":     ref.Base64,
			},
		})
	}

	// Build the text prompt — if references exist, tell Gemini to use them
	textPrompt := prompt
	if len(refs) > 0 {
		textPrompt = fmt.Sprintf(
			"I'm providing %d reference image(s) of my product. "+
				"Use them to understand the product's look, branding, colors, and style. "+
				"Then generate this: %s",
			len(refs), prompt,
		)
	}
	parts = append(parts, map[string]interface{}{"text": textPrompt})

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"TEXT", "IMAGE"},
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

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Gemini returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response to find the image data
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text,omitempty"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("cannot parse Gemini response: %w", err)
	}

	if result.Error != nil {
		return fmt.Errorf("Gemini API error: %s", result.Error.Message)
	}

	// Find the image part
	for _, candidate := range result.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				imgData, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
				if err != nil {
					return fmt.Errorf("cannot decode image data: %w", err)
				}

				// Determine extension from mime type
				ext := ".png"
				if part.InlineData.MimeType == "image/jpeg" {
					ext = ".jpg"
				}

				// Update output path with correct extension
				outputPath = outputPath[:len(outputPath)-len(filepath.Ext(outputPath))] + ext

				if err := os.WriteFile(outputPath, imgData, 0644); err != nil {
					return err
				}

				fmt.Printf("  Generated: %s (%.0f KB)\n", filepath.Base(outputPath), float64(len(imgData))/1024)
				return nil
			}
		}
	}

	return fmt.Errorf("no image found in Gemini response")
}

// geminiDescribeForVideo uses Gemini to analyze reference images and build an enriched
// video prompt for Veo (which doesn't accept image input directly).
func geminiDescribeForVideo(cfg *Config, basePrompt string, refs []ReferenceImage) (string, error) {
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s",
		cfg.GoogleAPIKey,
	)

	parts := []map[string]interface{}{}
	for _, ref := range refs {
		parts = append(parts, map[string]interface{}{
			"inlineData": map[string]string{
				"mimeType": ref.MimeType,
				"data":     ref.Base64,
			},
		})
	}
	parts = append(parts, map[string]interface{}{
		"text": fmt.Sprintf(
			"I'm creating a UGC-style social media video. "+
				"Analyze these product reference images carefully. "+
				"Describe the product's appearance, colors, branding, style, and vibe in vivid detail. "+
				"Then write a single, detailed video generation prompt that incorporates those visual details. "+
				"The base concept is: %s\n\n"+
				"Output ONLY the final video prompt, nothing else. Keep it under 300 words.",
			basePrompt,
		),
	})

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"TEXT"},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini describe request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini returned HTTP %d: %s", resp.StatusCode, string(respBody))
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

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("cannot parse Gemini response: %w", err)
	}

	for _, c := range result.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				fmt.Printf("  Enhanced prompt from %d reference image(s)\n", len(refs))
				return p.Text, nil
			}
		}
	}

	return "", fmt.Errorf("no text in Gemini describe response")
}
