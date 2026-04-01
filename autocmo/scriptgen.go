package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// generateUGCScript uses Gemini to write a natural-sounding ad voiceover script.
// The script is written in the style of real social media creators — casual, authentic,
// with hooks, filler words, natural pauses, and a call-to-action.
func generateUGCScript(cfg *Config, refs []ReferenceImage, targetSeconds int, feedback ...string) (string, error) {
	if targetSeconds <= 0 {
		targetSeconds = 15 // Default: 15-second UGC script (~35-40 words)
	}

	wordCount := targetSeconds * 3 // ~3 words per second for natural casual speech

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s",
		cfg.GoogleAPIKey,
	)

	parts := []map[string]interface{}{}

	// Add reference images so Gemini can see the actual product
	for _, ref := range refs {
		parts = append(parts, map[string]interface{}{
			"inlineData": map[string]string{
				"mimeType": ref.MimeType,
				"data":     ref.Base64,
			},
		})
	}

	refContext := ""
	if len(refs) > 0 {
		refContext = fmt.Sprintf(
			"I've attached %d reference image(s) of the product. Study them carefully — "+
				"describe what you SEE, not generic marketing copy. ",
			len(refs),
		)
	}

	parts = append(parts, map[string]interface{}{
		"text": fmt.Sprintf(`You are a UGC content creator recording a casual selfie-style video about a product.

%sProduct: %s
Description: %s
URL: %s

Write a voiceover script that sounds EXACTLY like a real person talking to their phone camera.
Target length: %d words (~%d seconds when spoken naturally).

RULES:
- First 3 seconds = attention hook ("okay so", "wait you guys", "I need to tell you about", "so I just found")
- Sound genuinely excited, not salesy. Like telling a friend about something cool.
- Include natural speech patterns: "like", "honestly", "literally", "you guys", brief pauses
- Use "—" for natural pauses and "..." for trailing off / thinking
- ONE specific detail about the product (color, feel, feature) — not generic praise
- End with casual CTA ("link in bio", "go check it out", "trust me on this one")
- Do NOT use hashtags, emojis, or stage directions
- Do NOT say "UGC" or "content" — you're a real person, not a marketer
- Write ONLY the spoken words. Nothing else. No labels, no quotes.

EXAMPLE TONE (do not copy, match the vibe):
"Okay wait — I literally cannot stop wearing this. Like, the fabric is so soft it's actually ridiculous... I wore it to brunch yesterday and got like three compliments? Anyway if you need new basics, go check these out. Trust me."` + feedbackBlock(feedback) + ``,
			refContext,
			cfg.ProductName,
			cfg.ProductDescription,
			cfg.ProductURL,
			wordCount,
			targetSeconds,
		),
	})

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"TEXT"},
			"temperature":        0.9, // High creativity for natural variation
			"topP":               0.95,
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

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini script generation failed: %w", err)
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
				script := strings.TrimSpace(p.Text)
				words := len(strings.Fields(script))
				fmt.Printf("  Script: %d words (~%ds spoken)\n", words, words/3)
				return script, nil
			}
		}
	}

	return "", fmt.Errorf("no script in Gemini response")
}

func feedbackBlock(feedback []string) string {
	if len(feedback) == 0 || feedback[0] == "" {
		return ""
	}
	return "\n\nCRITICAL — Your previous attempt was rejected by quality review:\n" + feedback[0] + "\nFix these specific issues in your new attempt."
}
