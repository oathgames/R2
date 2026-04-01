package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const tiktokAPIBase = "https://business-api.tiktok.com/open_api/v1.3"

// ── Campaign Setup ───────────────────────────────────────────

// tiktokEnsureCampaigns creates the Testing and Scaling campaigns if they don't exist.
// Returns (testingCampaignID, scalingCampaignID).
func tiktokEnsureCampaigns(cfg *Config) (string, string, error) {
	testingID, _ := tiktokFindCampaign(cfg, "Auto CMO - Testing")
	scalingID, _ := tiktokFindCampaign(cfg, "Auto CMO - Scaling")

	if testingID == "" {
		var err error
		testingID, err = tiktokCreateCampaign(cfg, "Auto CMO - Testing", false)
		if err != nil {
			return "", "", fmt.Errorf("cannot create Testing campaign: %w", err)
		}
		fmt.Printf("  Created: Auto CMO - Testing (%s)\n", testingID)
	} else {
		fmt.Printf("  Found: Auto CMO - Testing (%s)\n", testingID)
	}

	if scalingID == "" {
		var err error
		scalingID, err = tiktokCreateCampaign(cfg, "Auto CMO - Scaling", true)
		if err != nil {
			return testingID, "", fmt.Errorf("cannot create Scaling campaign: %w", err)
		}
		fmt.Printf("  Created: Auto CMO - Scaling (%s)\n", scalingID)
	} else {
		fmt.Printf("  Found: Auto CMO - Scaling (%s)\n", scalingID)
	}

	return testingID, scalingID, nil
}

func tiktokCreateCampaign(cfg *Config, name string, cbo bool) (string, error) {
	params := map[string]interface{}{
		"advertiser_id":  cfg.TikTokAdvertiserID,
		"campaign_name":  name,
		"objective_type": "CONVERSIONS",
		"budget_mode":    "BUDGET_MODE_DAY",
		"budget":         50.0, // $50/day campaign-level budget for CBO
	}
	if !cbo {
		// ABO — no campaign-level budget, budget is per ad group
		params["budget_mode"] = "BUDGET_MODE_INFINITE"
		delete(params, "budget")
	}

	return tiktokPost(cfg, "/campaign/create/", params)
}

func tiktokFindCampaign(cfg *Config, name string) (string, error) {
	url := fmt.Sprintf("%s/campaign/get/", tiktokAPIBase)

	params := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"page_size":     100,
	}
	body, _ := json.Marshal(params)

	req, _ := http.NewRequest("GET", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				CampaignID   string `json:"campaign_id"`
				CampaignName string `json:"campaign_name"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)

	for _, c := range result.Data.List {
		if c.CampaignName == name {
			return c.CampaignID, nil
		}
	}
	return "", nil
}

// ── Creative Upload ──────────────────────────────────────────

// tiktokUploadImage uploads an image and returns the image_id.
func tiktokUploadImage(cfg *Config, imagePath string) (string, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("advertiser_id", cfg.TikTokAdvertiserID)
	writer.WriteField("upload_type", "UPLOAD_BY_FILE")
	writer.WriteField("file_name", filepath.Base(imagePath))
	part, _ := writer.CreateFormFile("image_file", filepath.Base(imagePath))
	io.Copy(part, file)
	writer.Close()

	url := fmt.Sprintf("%s/file/image/ad/upload/", tiktokAPIBase)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("TikTok image upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ID      string `json:"id"`
			ImageID string `json:"image_id"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	if result.Code != 0 {
		return "", fmt.Errorf("TikTok image upload error: %s", result.Message)
	}

	imageID := result.Data.ImageID
	if imageID == "" {
		imageID = result.Data.ID
	}
	if imageID == "" {
		return "", fmt.Errorf("no image ID in response: %s", string(body))
	}
	return imageID, nil
}

// tiktokUploadVideo uploads a video and returns the video_id.
func tiktokUploadVideo(cfg *Config, videoPath string) (string, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("advertiser_id", cfg.TikTokAdvertiserID)
	writer.WriteField("upload_type", "UPLOAD_BY_FILE")
	writer.WriteField("file_name", filepath.Base(videoPath))
	part, _ := writer.CreateFormFile("video_file", filepath.Base(videoPath))
	io.Copy(part, file)
	writer.Close()

	url := fmt.Sprintf("%s/file/video/ad/upload/", tiktokAPIBase)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("TikTok video upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			VideoID string `json:"video_id"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	if result.Code != 0 {
		return "", fmt.Errorf("TikTok video upload error: %s", result.Message)
	}
	if result.Data.VideoID == "" {
		return "", fmt.Errorf("no video ID in response: %s", string(body))
	}
	return result.Data.VideoID, nil
}

// ── Ad Creation ──────────────────────────────────────────────

// tiktokCreateAdGroup creates an ad group in a campaign.
// TikTok ad groups = Meta ad sets.
func tiktokCreateAdGroup(cfg *Config, campaignID, name string, dailyBudget float64) (string, error) {
	if dailyBudget <= 0 {
		dailyBudget = 5.0
	}

	// Schedule: start now, run indefinitely
	startTime := time.Now().UTC().Format("2006-01-02 15:04:05")

	params := map[string]interface{}{
		"advertiser_id":  cfg.TikTokAdvertiserID,
		"campaign_id":    campaignID,
		"adgroup_name":   name,
		"placement_type": "PLACEMENT_TYPE_NORMAL",
		"placements":     []string{"PLACEMENT_TIKTOK"},
		"location_ids":   []int{6252001}, // US
		"budget_mode":    "BUDGET_MODE_DAY",
		"budget":         dailyBudget,
		"schedule_type":  "SCHEDULE_FROM_NOW",
		"schedule_start_time": startTime,
		"optimize_goal":  "CONVERT",
		"pacing":         "PACING_MODE_SMOOTH",
		"billing_event":  "OCPM",
		"bid_type":       "BID_TYPE_NO_BID", // Lowest cost
		"operation_status": "ENABLE",
	}

	// Add pixel for conversion tracking
	if cfg.TikTokPixelID != "" {
		params["pixel_id"] = cfg.TikTokPixelID
		params["external_action"] = "COMPLETE_PAYMENT" // Purchase event
	}

	return tiktokPost(cfg, "/adgroup/create/", params)
}

// tiktokCreateImageAd creates a full ad (ad group + ad) in the Testing campaign.
func tiktokCreateImageAd(cfg *Config, campaignID, imageID, headline, body, link string, dailyBudget float64) (string, error) {
	// Step 1: Create Ad Group
	adGroupName := fmt.Sprintf("AutoCMO - %s", time.Now().Format("Jan 02 15:04"))
	adGroupID, err := tiktokCreateAdGroup(cfg, campaignID, adGroupName, dailyBudget)
	if err != nil {
		return "", fmt.Errorf("ad group creation failed: %w", err)
	}
	fmt.Printf("  Ad Group: %s\n", adGroupID)

	// Step 2: Create Ad (creative is inline in TikTok)
	adParams := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"adgroup_id":    adGroupID,
		"creatives": []map[string]interface{}{
			{
				"ad_name":      fmt.Sprintf("AutoCMO Ad - %s", time.Now().Format("Jan 02 15:04")),
				"ad_format":    "SINGLE_IMAGE",
				"image_ids":    []string{imageID},
				"ad_text":      body,
				"display_name": headline,
				"landing_page_url": link,
				"call_to_action": "SHOP_NOW",
			},
		},
	}

	adID, err := tiktokPostAd(cfg, adParams)
	if err != nil {
		return "", fmt.Errorf("ad creation failed: %w", err)
	}
	fmt.Printf("  Ad: %s\n", adID)
	return adID, nil
}

// tiktokCreateVideoAd creates a video ad in the Testing campaign.
func tiktokCreateVideoAd(cfg *Config, campaignID, videoID, headline, body, link string, dailyBudget float64) (string, error) {
	// Step 1: Create Ad Group
	adGroupName := fmt.Sprintf("AutoCMO Video - %s", time.Now().Format("Jan 02 15:04"))
	adGroupID, err := tiktokCreateAdGroup(cfg, campaignID, adGroupName, dailyBudget)
	if err != nil {
		return "", fmt.Errorf("video ad group failed: %w", err)
	}
	fmt.Printf("  Ad Group: %s\n", adGroupID)

	// Step 2: Create Ad
	adParams := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"adgroup_id":    adGroupID,
		"creatives": []map[string]interface{}{
			{
				"ad_name":      fmt.Sprintf("AutoCMO Video Ad - %s", time.Now().Format("Jan 02 15:04")),
				"ad_format":    "SINGLE_VIDEO",
				"video_id":     videoID,
				"ad_text":      body,
				"display_name": headline,
				"landing_page_url": link,
				"call_to_action": "SHOP_NOW",
			},
		},
	}

	adID, err := tiktokPostAd(cfg, adParams)
	if err != nil {
		return "", fmt.Errorf("video ad creation failed: %w", err)
	}
	fmt.Printf("  Ad: %s\n", adID)
	return adID, nil
}

// ── Insights & Optimization ─────────────────────────────────

// TikTokAdInsight holds performance data for a single ad.
type TikTokAdInsight struct {
	AdID        string  `json:"ad_id"`
	AdName      string  `json:"ad_name"`
	Spend       float64 `json:"spend_float"`
	Impressions int     `json:"impressions_int"`
	Clicks      int     `json:"clicks_int"`
	CTR         float64 `json:"ctr_float"`
	CPC         float64 `json:"cpc_float"`
	CPM         float64 `json:"cpm_float"`
	AddToCart   int     `json:"add_to_cart"`
	Purchases   int     `json:"purchases"`
	CostPerATC  float64 `json:"cost_per_atc"`
	Verdict     string  `json:"verdict"` // "KILL", "WINNER", or ""
}

// tiktokGetInsights fetches yesterday's performance for all ads in a campaign.
func tiktokGetInsights(cfg *Config, campaignID string) ([]TikTokAdInsight, error) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	params := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"report_type":   "BASIC",
		"data_level":    "AUCTION_AD",
		"dimensions":    []string{"ad_id"},
		"metrics": []string{
			"spend", "impressions", "clicks", "ctr", "cpc", "cpm",
			"complete_payment", "total_complete_payment_rate",
			"onsite_shopping", // add to cart
		},
		"start_date": yesterday,
		"end_date":   yesterday,
		"filters": []map[string]interface{}{
			{
				"field_name":   "campaign_ids",
				"filter_type":  "IN",
				"filter_value": fmt.Sprintf("[%s]", campaignID),
			},
		},
		"page_size": 1000,
	}

	body, _ := json.Marshal(params)
	url := fmt.Sprintf("%s/report/integrated/get/", tiktokAPIBase)
	req, _ := http.NewRequest("GET", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TikTok insights HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Dimensions struct {
					AdID string `json:"ad_id"`
				} `json:"dimensions"`
				Metrics struct {
					Spend       string `json:"spend"`
					Impressions string `json:"impressions"`
					Clicks      string `json:"clicks"`
					CTR         string `json:"ctr"`
					CPC         string `json:"cpc"`
					CPM         string `json:"cpm"`
					Purchases   string `json:"complete_payment"`
					AddToCart   string `json:"onsite_shopping"`
				} `json:"metrics"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)

	var insights []TikTokAdInsight
	for _, d := range result.Data.List {
		var spend, ctr, cpc, cpm float64
		var impressions, clicks, addToCart, purchases int
		fmt.Sscanf(d.Metrics.Spend, "%f", &spend)
		fmt.Sscanf(d.Metrics.CTR, "%f", &ctr)
		fmt.Sscanf(d.Metrics.CPC, "%f", &cpc)
		fmt.Sscanf(d.Metrics.CPM, "%f", &cpm)
		fmt.Sscanf(d.Metrics.Impressions, "%d", &impressions)
		fmt.Sscanf(d.Metrics.Clicks, "%d", &clicks)
		fmt.Sscanf(d.Metrics.AddToCart, "%d", &addToCart)
		fmt.Sscanf(d.Metrics.Purchases, "%d", &purchases)

		// Calculate cost per ATC
		var costPerATC float64
		if addToCart > 0 {
			costPerATC = spend / float64(addToCart)
		}

		// Same verdict logic as Meta
		verdict := ""
		if spend >= 5 && ctr < 0.5 {
			verdict = "KILL"
		} else if addToCart > 0 && costPerATC <= 10 && spend >= 3 {
			verdict = "WINNER"
		}

		insights = append(insights, TikTokAdInsight{
			AdID:        d.Dimensions.AdID,
			AdName:      fmt.Sprintf("Ad %s", d.Dimensions.AdID), // TikTok reports don't include ad_name in basic reports
			Spend:       spend,
			Impressions: impressions,
			Clicks:      clicks,
			CTR:         ctr,
			CPC:         cpc,
			CPM:         cpm,
			AddToCart:   addToCart,
			Purchases:   purchases,
			CostPerATC:  costPerATC,
			Verdict:     verdict,
		})
	}

	return insights, nil
}

// tiktokPauseAd disables an ad.
func tiktokPauseAd(cfg *Config, adID string) error {
	params := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"ad_ids":        []string{adID},
		"opt_status":    "DISABLE",
	}

	body, _ := json.Marshal(params)
	url := fmt.Sprintf("%s/ad/status/update/", tiktokAPIBase)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	json.Unmarshal(respBody, &result)

	if result.Code != 0 {
		return fmt.Errorf("TikTok pause failed: %s (%s)", result.Message, string(respBody))
	}
	return nil
}

// tiktokDuplicateAd copies an ad into the Scaling campaign at $20/day.
func tiktokDuplicateAd(cfg *Config, sourceAdID, targetCampaignID string) (string, error) {
	// Get source ad details
	params := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"filtering": map[string]interface{}{
			"ad_ids": []string{sourceAdID},
		},
	}
	body, _ := json.Marshal(params)

	url := fmt.Sprintf("%s/ad/get/", tiktokAPIBase)
	req, _ := http.NewRequest("GET", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var adResult struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				AdID    string `json:"ad_id"`
				AdName  string `json:"ad_name"`
				AdText  string `json:"ad_text"`
				VideoID string `json:"video_id"`
				ImageIDs []string `json:"image_ids"`
				DisplayName string `json:"display_name"`
				LandingPageURL string `json:"landing_page_url"`
				CallToAction string `json:"call_to_action"`
				AdFormat string `json:"ad_format"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &adResult)

	if len(adResult.Data.List) == 0 {
		return "", fmt.Errorf("cannot find ad %s", sourceAdID)
	}
	source := adResult.Data.List[0]

	// Create new ad group in Scaling campaign at $20/day
	adGroupName := fmt.Sprintf("AutoCMO Winner - %s", time.Now().Format("Jan 02"))
	adGroupID, err := tiktokCreateAdGroup(cfg, targetCampaignID, adGroupName, 20.0)
	if err != nil {
		return "", fmt.Errorf("scaling ad group failed: %w", err)
	}

	// Create new ad with the same creative
	creative := map[string]interface{}{
		"ad_name":      fmt.Sprintf("Winner from %s - %s", sourceAdID, time.Now().Format("Jan 02")),
		"ad_format":    source.AdFormat,
		"ad_text":      source.AdText,
		"display_name": source.DisplayName,
		"call_to_action": source.CallToAction,
	}
	if source.LandingPageURL != "" {
		creative["landing_page_url"] = source.LandingPageURL
	}
	if source.VideoID != "" {
		creative["video_id"] = source.VideoID
	}
	if len(source.ImageIDs) > 0 {
		creative["image_ids"] = source.ImageIDs
	}

	adParams := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"adgroup_id":    adGroupID,
		"creatives":     []map[string]interface{}{creative},
	}

	return tiktokPostAd(cfg, adParams)
}

// ── Lookalike Audiences ──────────────────────────────────────

// tiktokCreateLookalike creates a custom audience from pixel purchasers + lookalike.
func tiktokCreateLookalike(cfg *Config, sourceAdID string) (string, error) {
	if cfg.TikTokPixelID == "" {
		return "", fmt.Errorf("TikTok pixel ID required for lookalike creation")
	}

	// Step 1: Create custom audience from pixel events
	customParams := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"custom_audience_name": fmt.Sprintf("AutoCMO Purchasers - %s", time.Now().Format("Jan 02")),
		"audience_type":        "WEBSITE_CUSTOM",
		"rule": map[string]interface{}{
			"inclusions": map[string]interface{}{
				"operator": "OR",
				"rules": []map[string]interface{}{
					{
						"source":           "pixel",
						"source_id":        cfg.TikTokPixelID,
						"event_type":       "CompletePayment",
						"retention_days":   30,
					},
				},
			},
		},
	}

	customID, err := tiktokPost(cfg, "/dmp/custom_audience/create/", customParams)
	if err != nil {
		return "", fmt.Errorf("custom audience creation failed: %w", err)
	}
	fmt.Printf("  Custom audience: %s\n", customID)

	// Step 2: Create 1% lookalike
	lookalikeParams := map[string]interface{}{
		"advertiser_id": cfg.TikTokAdvertiserID,
		"custom_audience_name": fmt.Sprintf("AutoCMO 1%% Lookalike - %s", time.Now().Format("Jan 02")),
		"source_audience_ids":  []string{customID},
		"location_ids":         []int{6252001}, // US
		"lookalike_type":       "NARROW",       // Most similar = highest quality
	}

	lookalikeID, err := tiktokPost(cfg, "/dmp/custom_audience/lookalike/create/", lookalikeParams)
	if err != nil {
		return "", fmt.Errorf("lookalike creation failed: %w", err)
	}

	fmt.Printf("  Lookalike audience: %s (narrow, US)\n", lookalikeID)
	return lookalikeID, nil
}

// ── Multi-Creative Ad Groups ─────────────────────────────────

// tiktokCreateMultiCreativeAdGroup creates one ad group with multiple ads.
func tiktokCreateMultiCreativeAdGroup(cfg *Config, campaignID string, imageIDs []string, videoIDs []string, headline, bodyText, link string, dailyBudget float64) ([]string, error) {
	if dailyBudget <= 0 {
		dailyBudget = 5.0
	}

	// Create one ad group
	adGroupName := fmt.Sprintf("AutoCMO Multi - %s", time.Now().Format("Jan 02 15:04"))
	adGroupID, err := tiktokCreateAdGroup(cfg, campaignID, adGroupName, dailyBudget)
	if err != nil {
		return nil, fmt.Errorf("multi-creative ad group failed: %w", err)
	}
	fmt.Printf("  Ad Group: %s\n", adGroupID)

	var adIDs []string

	// Create ads for each image
	for i, imgID := range imageIDs {
		adParams := map[string]interface{}{
			"advertiser_id": cfg.TikTokAdvertiserID,
			"adgroup_id":    adGroupID,
			"creatives": []map[string]interface{}{
				{
					"ad_name":           fmt.Sprintf("AutoCMO Img %d - %s", i+1, time.Now().Format("Jan 02")),
					"ad_format":         "SINGLE_IMAGE",
					"image_ids":         []string{imgID},
					"ad_text":           bodyText,
					"display_name":      headline,
					"landing_page_url":  link,
					"call_to_action":    "SHOP_NOW",
				},
			},
		}
		adID, err := tiktokPostAd(cfg, adParams)
		if err != nil {
			fmt.Printf("  [WARN] Image ad %d failed: %v\n", i+1, err)
			continue
		}
		adIDs = append(adIDs, adID)
		fmt.Printf("  Ad %d: %s (image)\n", i+1, adID)
	}

	// Create ads for each video
	for i, vidID := range videoIDs {
		adParams := map[string]interface{}{
			"advertiser_id": cfg.TikTokAdvertiserID,
			"adgroup_id":    adGroupID,
			"creatives": []map[string]interface{}{
				{
					"ad_name":           fmt.Sprintf("AutoCMO Vid %d - %s", i+1, time.Now().Format("Jan 02")),
					"ad_format":         "SINGLE_VIDEO",
					"video_id":          vidID,
					"ad_text":           bodyText,
					"display_name":      headline,
					"landing_page_url":  link,
					"call_to_action":    "SHOP_NOW",
				},
			},
		}
		adID, err := tiktokPostAd(cfg, adParams)
		if err != nil {
			fmt.Printf("  [WARN] Video ad %d failed: %v\n", i+1, err)
			continue
		}
		adIDs = append(adIDs, adID)
		fmt.Printf("  Ad %d: %s (video)\n", len(imageIDs)+i+1, adID)
	}

	return adIDs, nil
}

// ── Helpers ─────────────────────────────────────────────────

// tiktokPost is the universal JSON POST helper for TikTok API.
// Auth is via Access-Token header (not body/query param like Meta).
func tiktokPost(cfg *Config, endpoint string, params map[string]interface{}) (string, error) {
	body, _ := json.Marshal(params)
	url := tiktokAPIBase + endpoint

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("TikTok API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// TikTok wraps everything in {"code": 0, "message": "OK", "data": {...}}
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			// Campaign/AdGroup/Audience creation returns IDs in different fields
			CampaignID string   `json:"campaign_id"`
			AdGroupID  string   `json:"adgroup_id"`
			AdIDs      []string `json:"ad_ids"`
			AudienceID string   `json:"custom_audience_id"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)

	if result.Code != 0 {
		return "", fmt.Errorf("TikTok API error %d: %s", result.Code, result.Message)
	}

	// Return whichever ID field is populated
	if result.Data.CampaignID != "" {
		return result.Data.CampaignID, nil
	}
	if result.Data.AdGroupID != "" {
		return result.Data.AdGroupID, nil
	}
	if len(result.Data.AdIDs) > 0 {
		return result.Data.AdIDs[0], nil
	}
	if result.Data.AudienceID != "" {
		return result.Data.AudienceID, nil
	}

	return "", fmt.Errorf("no ID in TikTok response: %s", string(respBody))
}

// tiktokPostAd creates an ad and returns the first ad ID.
// Separate from tiktokPost because /ad/create/ has a different response shape.
func tiktokPostAd(cfg *Config, params map[string]interface{}) (string, error) {
	body, _ := json.Marshal(params)
	url := fmt.Sprintf("%s/ad/create/", tiktokAPIBase)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Access-Token", cfg.TikTokAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("TikTok ad create HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AdIDs []string `json:"ad_ids"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)

	if result.Code != 0 {
		return "", fmt.Errorf("TikTok ad error %d: %s", result.Code, result.Message)
	}
	if len(result.Data.AdIDs) == 0 {
		return "", fmt.Errorf("no ad IDs in response: %s", string(respBody))
	}
	return result.Data.AdIDs[0], nil
}

// ── Pipeline Integration ─────────────────────────────────────

func runTikTokPush(cfg *Config, cmd *Command) {
	if cfg.TikTokAccessToken == "" {
		log.Fatal("[ERROR] tiktokAccessToken required — get it from TikTok Ads Manager API settings")
	}
	if cfg.TikTokAdvertiserID == "" {
		log.Fatal("[ERROR] tiktokAdvertiserId required — your TikTok advertiser account ID")
	}

	fmt.Println("============================================================")
	fmt.Println("  TikTok Ads — Pushing Creative")
	fmt.Println("============================================================")

	testingID, _, err := tiktokEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	imagePath := cmd.AdImagePath
	videoPath := cmd.AdVideoPath

	if imagePath == "" && videoPath == "" {
		log.Fatal("[ERROR] adImagePath or adVideoPath required")
	}

	headline := cmd.AdHeadline
	if headline == "" {
		headline = cfg.ProductName
	}
	adBody := cmd.AdBody
	if adBody == "" {
		adBody = cfg.ProductDescription
	}
	link := cmd.AdLink
	if link == "" {
		link = cfg.ProductURL
	}
	budget := cmd.DailyBudget
	if budget <= 0 {
		budget = 5.0
	}

	var adID string

	if videoPath != "" {
		fmt.Printf("  Uploading video: %s\n", filepath.Base(videoPath))
		videoID, err := tiktokUploadVideo(cfg, videoPath)
		if err != nil {
			log.Fatalf("[ERROR] Video upload failed: %v", err)
		}
		fmt.Printf("  Video ID: %s\n", videoID)
		fmt.Printf("  Creating video ad ($%.0f/day)...\n", budget)
		adID, err = tiktokCreateVideoAd(cfg, testingID, videoID, headline, adBody, link, budget)
		if err != nil {
			log.Fatalf("[ERROR] Video ad creation failed: %v", err)
		}
	} else {
		fmt.Printf("  Uploading image: %s\n", filepath.Base(imagePath))
		imageID, err := tiktokUploadImage(cfg, imagePath)
		if err != nil {
			log.Fatalf("[ERROR] Image upload failed: %v", err)
		}
		fmt.Printf("  Image ID: %s\n", imageID)
		fmt.Printf("  Creating image ad ($%.0f/day)...\n", budget)
		adID, err = tiktokCreateImageAd(cfg, testingID, imageID, headline, adBody, link, budget)
		if err != nil {
			log.Fatalf("[ERROR] Image ad creation failed: %v", err)
		}
	}

	fmt.Println("\n============================================================")
	fmt.Printf("  Ad live: %s\n", adID)
	fmt.Printf("  Campaign: Auto CMO - Testing\n")
	fmt.Printf("  Budget: $%.0f/day\n", budget)
	fmt.Printf("  Optimization: Conversions (payment)\n")
	fmt.Printf("  Placement: TikTok\n")
	fmt.Println("============================================================")
}

func runTikTokInsights(cfg *Config, cmd *Command) {
	if cfg.TikTokAccessToken == "" {
		log.Fatal("[ERROR] tiktokAccessToken required")
	}

	fmt.Println("============================================================")
	fmt.Println("  TikTok Ads — Yesterday's Performance")
	fmt.Println("============================================================")

	testingID, _, err := tiktokEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	insights, err := tiktokGetInsights(cfg, testingID)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	if len(insights) == 0 {
		fmt.Println("  No data yet — ads need at least 24 hours to report.")
		return
	}

	fmt.Printf("\n  %-8s %-6s %-6s %-6s %-5s %-5s %-8s %-8s %s\n", "SPEND", "IMPR", "CLICK", "CTR", "ATC", "PURCH", "$/ATC", "VERDICT", "AD")
	fmt.Println("  ──────────────────────────────────────────────────────────────────────────")
	for _, i := range insights {
		atcStr := "-"
		if i.CostPerATC > 0 {
			atcStr = fmt.Sprintf("$%.2f", i.CostPerATC)
		}
		fmt.Printf("  $%-7.2f %-6d %-6d %-5.1f%% %-5d %-5d %-8s %-8s %s\n",
			i.Spend, i.Impressions, i.Clicks, i.CTR, i.AddToCart, i.Purchases, atcStr, i.Verdict, i.AdName)
	}
	fmt.Println("  ──────────────────────────────────────────────────────────────────────────")

	// Output as JSON for Claude to parse
	jsonData, _ := json.MarshalIndent(insights, "", "  ")
	fmt.Printf("\n%s\n", string(jsonData))
}

func runTikTokSetup(cfg *Config) {
	if cfg.TikTokAccessToken == "" {
		log.Fatal("[ERROR] tiktokAccessToken required")
	}
	if cfg.TikTokAdvertiserID == "" {
		log.Fatal("[ERROR] tiktokAdvertiserId required")
	}

	fmt.Println("============================================================")
	fmt.Println("  TikTok Ads — Campaign Setup")
	fmt.Println("============================================================")

	_, _, err := tiktokEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	fmt.Println("\n  Campaigns ready. Use 'tiktok-push' to create ads.")
}
