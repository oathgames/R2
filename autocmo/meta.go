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

const metaAPIBase = "https://graph.facebook.com/v22.0"

// ── Campaign Setup ───────────────────────────────────────────

// metaEnsureCampaigns creates the Testing (ABO), Scaling (CBO), and Retargeting campaigns.
// Returns (testingCampaignID, scalingCampaignID).
func metaEnsureCampaigns(cfg *Config) (string, string, error) {
	// Check for existing campaigns
	testingID, _ := metaFindCampaign(cfg, "Auto CMO - Testing")
	scalingID, _ := metaFindCampaign(cfg, "Auto CMO - Scaling")

	if testingID == "" {
		var err error
		testingID, err = metaCreateCampaign(cfg, "Auto CMO - Testing", "AUCTION", "OUTCOME_SALES", false)
		if err != nil {
			return "", "", fmt.Errorf("cannot create Testing campaign: %w", err)
		}
		fmt.Printf("  Created: Auto CMO - Testing (%s)\n", testingID)
	} else {
		fmt.Printf("  Found: Auto CMO - Testing (%s)\n", testingID)
	}

	if scalingID == "" {
		var err error
		scalingID, err = metaCreateCampaign(cfg, "Auto CMO - Scaling", "AUCTION", "OUTCOME_SALES", true)
		if err != nil {
			return testingID, "", fmt.Errorf("cannot create Scaling campaign: %w", err)
		}
		fmt.Printf("  Created: Auto CMO - Scaling (%s)\n", scalingID)
	} else {
		fmt.Printf("  Found: Auto CMO - Scaling (%s)\n", scalingID)
	}

	// Retargeting campaign — auto-created if pixel is configured
	retargetingID, _ := metaFindCampaign(cfg, "Auto CMO - Retargeting")
	if retargetingID == "" && cfg.MetaPixelID != "" {
		var err error
		retargetingID, err = metaCreateCampaign(cfg, "Auto CMO - Retargeting", "AUCTION", "OUTCOME_SALES", true)
		if err != nil {
			fmt.Printf("  [WARN] Retargeting campaign failed: %v\n", err)
		} else {
			fmt.Printf("  Created: Auto CMO - Retargeting (%s)\n", retargetingID)
		}
	} else if retargetingID != "" {
		fmt.Printf("  Found: Auto CMO - Retargeting (%s)\n", retargetingID)
	}

	return testingID, scalingID, nil
}

// metaEnsureRetargetingAudiences creates the core retargeting audiences from pixel data.
func metaEnsureRetargetingAudiences(cfg *Config) (siteVisitors, cartAbandoners, viewContent string, err error) {
	if cfg.MetaPixelID == "" {
		return "", "", "", fmt.Errorf("pixel ID required for retargeting audiences")
	}

	siteVisitors, err = metaCreateRetargetingAudience(cfg, "AutoCMO - Site Visitors 7d", "PageView", 7)
	if err != nil {
		return "", "", "", fmt.Errorf("site visitors audience: %w", err)
	}

	cartAbandoners, err = metaCreateRetargetingAudience(cfg, "AutoCMO - Cart Abandoners 14d", "AddToCart", 14)
	if err != nil {
		return siteVisitors, "", "", fmt.Errorf("cart abandoners audience: %w", err)
	}

	viewContent, err = metaCreateRetargetingAudience(cfg, "AutoCMO - View Content 7d", "ViewContent", 7)
	if err != nil {
		return siteVisitors, cartAbandoners, "", fmt.Errorf("view content audience: %w", err)
	}

	return siteVisitors, cartAbandoners, viewContent, nil
}

func metaCreateRetargetingAudience(cfg *Config, name, eventType string, retentionDays int) (string, error) {
	params := map[string]interface{}{
		"name":        name,
		"description": fmt.Sprintf("Auto-created by AutoCMO - %s events, %d day retention", eventType, retentionDays),
		"subtype":     "WEBSITE",
		"rule": map[string]interface{}{
			"inclusions": map[string]interface{}{
				"operator": "or",
				"rules": []map[string]interface{}{
					{
						"event_sources": []map[string]string{
							{"id": cfg.MetaPixelID, "type": "pixel"},
						},
						"retention_seconds": retentionDays * 86400,
						"filter": map[string]interface{}{
							"operator": "and",
							"filters": []map[string]interface{}{
								{"field": "event", "operator": "eq", "value": eventType},
							},
						},
					},
				},
			},
		},
	}

	return metaPost(cfg, fmt.Sprintf("/%s/customaudiences", cfg.MetaAdAccountID), params)
}

// metaCopyWinnerToRetargeting duplicates a scaling winner's creative into the retargeting campaign,
// targeting cart abandoners (highest intent warm audience) at $10/day.
func metaCopyWinnerToRetargeting(cfg *Config, sourceAdID string) (string, error) {
	retargetingID, _ := metaFindCampaign(cfg, "Auto CMO - Retargeting")
	if retargetingID == "" {
		return "", fmt.Errorf("retargeting campaign not found - run meta-setup first")
	}

	// Get source ad's creative
	url := fmt.Sprintf("%s/%s?fields=creative{id}&access_token=%s",
		metaAPIBase, sourceAdID, cfg.MetaAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var adInfo struct {
		Creative struct {
			ID string `json:"id"`
		} `json:"creative"`
	}
	json.Unmarshal(body, &adInfo)

	if adInfo.Creative.ID == "" {
		return "", fmt.Errorf("cannot read creative from ad %s", sourceAdID)
	}

	// Get or create cart abandoner audience
	_, cartAudienceID, _, err := metaEnsureRetargetingAudiences(cfg)
	if err != nil {
		return "", fmt.Errorf("retargeting audience setup failed: %w", err)
	}

	// Create ad set targeting cart abandoners
	adSetParams := map[string]interface{}{
		"name":              fmt.Sprintf("AutoCMO Retarget - %s", time.Now().Format("Jan 02")),
		"campaign_id":       retargetingID,
		"daily_budget":      1000, // $10/day for retargeting
		"billing_event":     "IMPRESSIONS",
		"optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy":      "LOWEST_COST_WITHOUT_CAP",
		"targeting": map[string]interface{}{
			"geo_locations": map[string]interface{}{
				"countries": []string{"US"},
			},
			"custom_audiences": []map[string]string{
				{"id": cartAudienceID},
			},
		},
		"publisher_platforms":  []string{"facebook", "instagram"},
		"facebook_positions":  []string{"feed", "story", "reels"},
		"instagram_positions": []string{"stream", "story", "reels"},
		"status":              "ACTIVE",
	}
	if cfg.MetaPixelID != "" {
		adSetParams["promoted_object"] = map[string]interface{}{
			"pixel_id":          cfg.MetaPixelID,
			"custom_event_type": "PURCHASE",
		}
	}

	adSetID, err := metaPost(cfg, fmt.Sprintf("/%s/adsets", cfg.MetaAdAccountID), adSetParams)
	if err != nil {
		return "", fmt.Errorf("retargeting ad set failed: %w", err)
	}

	adParams := map[string]interface{}{
		"name":     fmt.Sprintf("Retarget from %s - %s", sourceAdID, time.Now().Format("Jan 02")),
		"adset_id": adSetID,
		"creative": map[string]string{"creative_id": adInfo.Creative.ID},
		"status":   "ACTIVE",
	}

	return metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
}

func metaCreateCampaign(cfg *Config, name, buyingType, objective string, cbo bool) (string, error) {
	params := map[string]interface{}{
		"name":          name,
		"objective":     objective,
		"buying_type":   buyingType,
		"status":        "PAUSED",
		"special_ad_categories": []string{},
	}
	if cbo {
		params["bid_strategy"] = "LOWEST_COST_WITHOUT_CAP"
	}

	return metaPost(cfg, fmt.Sprintf("/%s/campaigns", cfg.MetaAdAccountID), params)
}

func metaFindCampaign(cfg *Config, name string) (string, error) {
	url := fmt.Sprintf("%s/%s/campaigns?fields=id,name&filtering=[{\"field\":\"name\",\"operator\":\"CONTAIN\",\"value\":\"%s\"}]&access_token=%s",
		metaAPIBase, cfg.MetaAdAccountID, name, cfg.MetaAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	for _, c := range result.Data {
		if c.Name == name {
			return c.ID, nil
		}
	}
	return "", nil
}

// ── Creative Upload ──────────────────────────────────────────

// metaUploadImage uploads an image to the ad account and returns the image hash.
func metaUploadImage(cfg *Config, imagePath string) (string, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("filename", filepath.Base(imagePath))
	io.Copy(part, file)
	writer.WriteField("access_token", cfg.MetaAccessToken)
	writer.Close()

	url := fmt.Sprintf("%s/%s/adimages", metaAPIBase, cfg.MetaAdAccountID)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Meta image upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Response: {"images":{"filename.jpg":{"hash":"abc123"}}}
	var result struct {
		Images map[string]struct {
			Hash string `json:"hash"`
		} `json:"images"`
	}
	json.Unmarshal(body, &result)

	for _, img := range result.Images {
		if img.Hash != "" {
			return img.Hash, nil
		}
	}
	return "", fmt.Errorf("no image hash in response: %s", string(body))
}

// metaUploadVideo uploads a video to the ad account and returns the video ID.
func metaUploadVideo(cfg *Config, videoPath string) (string, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("source", filepath.Base(videoPath))
	io.Copy(part, file)
	writer.WriteField("access_token", cfg.MetaAccessToken)
	writer.Close()

	url := fmt.Sprintf("%s/%s/advideos", metaAPIBase, cfg.MetaAdAccountID)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Meta video upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &result)

	if result.ID == "" {
		return "", fmt.Errorf("no video ID in response: %s", string(body))
	}
	return result.ID, nil
}

// ── Ad Creation ──────────────────────────────────────────────

// metaCreateImageAd creates a full ad (ad set + ad creative + ad) in the Testing campaign.
func metaCreateImageAd(cfg *Config, campaignID, imageHash, headline, body, link string, dailyBudget float64) (string, error) {
	if dailyBudget <= 0 {
		dailyBudget = 5.0
	}
	budgetCents := int(dailyBudget * 100)

	// Create Ad Set (ABO — budget per ad set)
	// Optimized for purchases, Feed + Stories + Reels placement, broad targeting
	pixelID := cfg.MetaPixelID
	adSetParams := map[string]interface{}{
		"name":              fmt.Sprintf("AutoCMO - %s", time.Now().Format("Jan 02 15:04")),
		"campaign_id":       campaignID,
		"daily_budget":      budgetCents,
		"billing_event":     "IMPRESSIONS",
		"optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy":      "LOWEST_COST_WITHOUT_CAP",
		"targeting": map[string]interface{}{
			"geo_locations": map[string]interface{}{
				"countries": []string{"US"},
			},
		},
		"publisher_platforms": []string{"facebook", "instagram"},
		"facebook_positions": []string{"feed", "story", "reels"},
		"instagram_positions": []string{"stream", "story", "reels"},
		"status": "ACTIVE",
	}
	if pixelID != "" {
		adSetParams["promoted_object"] = map[string]interface{}{
			"pixel_id":        pixelID,
			"custom_event_type": "PURCHASE",
		}
	}

	adSetID, err := metaPost(cfg, fmt.Sprintf("/%s/adsets", cfg.MetaAdAccountID), adSetParams)
	if err != nil {
		return "", fmt.Errorf("ad set creation failed: %w", err)
	}
	fmt.Printf("  Ad Set: %s\n", adSetID)

	// Create Ad Creative
	creativeParams := map[string]interface{}{
		"name": fmt.Sprintf("AutoCMO Creative - %s", time.Now().Format("Jan 02")),
		"object_story_spec": map[string]interface{}{
			"page_id": cfg.MetaPageID,
			"link_data": map[string]interface{}{
				"image_hash":  imageHash,
				"link":        link,
				"message":     body,
				"name":        headline,
				"call_to_action": map[string]interface{}{
					"type":  "SHOP_NOW",
					"value": map[string]string{"link": link},
				},
			},
		},
	}

	creativeID, err := metaPost(cfg, fmt.Sprintf("/%s/adcreatives", cfg.MetaAdAccountID), creativeParams)
	if err != nil {
		return "", fmt.Errorf("creative creation failed: %w", err)
	}
	fmt.Printf("  Creative: %s\n", creativeID)

	// Create Ad
	adParams := map[string]interface{}{
		"name":     fmt.Sprintf("AutoCMO Ad - %s", time.Now().Format("Jan 02 15:04")),
		"adset_id": adSetID,
		"creative": map[string]string{"creative_id": creativeID},
		"status":   "ACTIVE",
	}

	adID, err := metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
	if err != nil {
		return "", fmt.Errorf("ad creation failed: %w", err)
	}
	fmt.Printf("  Ad: %s\n", adID)

	return adID, nil
}

// metaCreateVideoAd creates a video ad in the Testing campaign.
func metaCreateVideoAd(cfg *Config, campaignID, videoID, headline, body, link string, dailyBudget float64) (string, error) {
	if dailyBudget <= 0 {
		dailyBudget = 5.0
	}
	budgetCents := int(dailyBudget * 100)

	pixelID := cfg.MetaPixelID
	adSetParams := map[string]interface{}{
		"name":              fmt.Sprintf("AutoCMO Video - %s", time.Now().Format("Jan 02 15:04")),
		"campaign_id":       campaignID,
		"daily_budget":      budgetCents,
		"billing_event":     "IMPRESSIONS",
		"optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy":      "LOWEST_COST_WITHOUT_CAP",
		"targeting": map[string]interface{}{
			"geo_locations": map[string]interface{}{
				"countries": []string{"US"},
			},
		},
		"publisher_platforms":  []string{"facebook", "instagram"},
		"facebook_positions":  []string{"feed", "story", "reels"},
		"instagram_positions": []string{"stream", "story", "reels"},
		"status":              "ACTIVE",
	}
	if pixelID != "" {
		adSetParams["promoted_object"] = map[string]interface{}{
			"pixel_id":          pixelID,
			"custom_event_type": "PURCHASE",
		}
	}

	adSetID, err := metaPost(cfg, fmt.Sprintf("/%s/adsets", cfg.MetaAdAccountID), adSetParams)
	if err != nil {
		return "", fmt.Errorf("video ad set failed: %w", err)
	}
	fmt.Printf("  Ad Set: %s\n", adSetID)

	creativeParams := map[string]interface{}{
		"name": fmt.Sprintf("AutoCMO Video Creative - %s", time.Now().Format("Jan 02")),
		"object_story_spec": map[string]interface{}{
			"page_id": cfg.MetaPageID,
			"video_data": map[string]interface{}{
				"video_id":    videoID,
				"message":     body,
				"title":       headline,
				"link_description": headline,
				"call_to_action": map[string]interface{}{
					"type":  "SHOP_NOW",
					"value": map[string]string{"link": link},
				},
			},
		},
	}

	creativeID, err := metaPost(cfg, fmt.Sprintf("/%s/adcreatives", cfg.MetaAdAccountID), creativeParams)
	if err != nil {
		return "", fmt.Errorf("video creative failed: %w", err)
	}
	fmt.Printf("  Creative: %s\n", creativeID)

	adParams := map[string]interface{}{
		"name":     fmt.Sprintf("AutoCMO Video Ad - %s", time.Now().Format("Jan 02 15:04")),
		"adset_id": adSetID,
		"creative": map[string]string{"creative_id": creativeID},
		"status":   "ACTIVE",
	}

	return metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
}

// ── Insights & Optimization ─────────────────────────────────

// MetaAdInsight holds performance data for a single ad.
type MetaAdInsight struct {
	AdID           string  `json:"ad_id"`
	AdName         string  `json:"ad_name"`
	Spend          float64 `json:"spend_float"`
	Impressions    int     `json:"impressions_int"`
	Clicks         int     `json:"clicks_int"`
	CTR            float64 `json:"ctr_float"`
	CPC            float64 `json:"cpc_float"`
	CPM            float64 `json:"cpm_float"`
	AddToCart      int     `json:"add_to_cart"`
	Purchases      int     `json:"purchases"`
	PurchaseValue  float64 `json:"purchase_value"`
	ROAS           float64 `json:"roas"`
	CostPerATC     float64 `json:"cost_per_atc"`
	Verdict        string  `json:"verdict"`         // "KILL", "WINNER", "FATIGUE", or ""
	Campaign       string  `json:"campaign"`        // "testing" or "scaling"
}

// metaGetInsights fetches yesterday's performance for all ads in a campaign.
// campaignLabel is "testing" or "scaling" — stored on each insight for cross-campaign reporting.
func metaGetInsights(cfg *Config, campaignID, campaignLabel string) ([]MetaAdInsight, error) {
	url := fmt.Sprintf("%s/%s/insights?fields=ad_id,ad_name,spend,impressions,clicks,ctr,cpc,cpm,actions,action_values,cost_per_action_type,purchase_roas&level=ad&date_preset=yesterday&access_token=%s",
		metaAPIBase, campaignID, cfg.MetaAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Meta insights HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			AdID        string `json:"ad_id"`
			AdName      string `json:"ad_name"`
			Spend       string `json:"spend"`
			Impressions string `json:"impressions"`
			Clicks      string `json:"clicks"`
			CTR         string `json:"ctr"`
			CPC         string `json:"cpc"`
			CPM         string `json:"cpm"`
			Actions     []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"actions"`
			ActionValues []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"action_values"`
			CostPerAction []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"cost_per_action_type"`
			PurchaseROAS []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"purchase_roas"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	var insights []MetaAdInsight
	for _, d := range result.Data {
		var spend, ctr, cpc, cpm float64
		var impressions, clicks int
		fmt.Sscanf(d.Spend, "%f", &spend)
		fmt.Sscanf(d.CTR, "%f", &ctr)
		fmt.Sscanf(d.CPC, "%f", &cpc)
		fmt.Sscanf(d.CPM, "%f", &cpm)
		fmt.Sscanf(d.Impressions, "%d", &impressions)
		fmt.Sscanf(d.Clicks, "%d", &clicks)

		var addToCart, purchases int
		var costPerATC, purchaseValue, roas float64
		for _, a := range d.Actions {
			switch a.ActionType {
			case "add_to_cart":
				fmt.Sscanf(a.Value, "%d", &addToCart)
			case "purchase", "offsite_conversion.fb_pixel_purchase":
				fmt.Sscanf(a.Value, "%d", &purchases)
			}
		}
		for _, av := range d.ActionValues {
			if av.ActionType == "purchase" || av.ActionType == "offsite_conversion.fb_pixel_purchase" {
				fmt.Sscanf(av.Value, "%f", &purchaseValue)
			}
		}
		for _, c := range d.CostPerAction {
			if c.ActionType == "add_to_cart" {
				fmt.Sscanf(c.Value, "%f", &costPerATC)
			}
		}
		// ROAS from purchase_roas field (most accurate)
		for _, r := range d.PurchaseROAS {
			if r.ActionType == "omni_purchase" || r.ActionType == "purchase" {
				fmt.Sscanf(r.Value, "%f", &roas)
			}
		}
		// Fallback: calculate ROAS from purchase_value / spend
		if roas == 0 && purchaseValue > 0 && spend > 0 {
			roas = purchaseValue / spend
		}

		// Apply verdicts
		verdict := ""
		if campaignLabel == "testing" {
			if spend >= 5 && ctr < 0.5 {
				verdict = "KILL"
			} else if addToCart > 0 && costPerATC <= 10 && spend >= 3 {
				verdict = "WINNER"
			}
		} else if campaignLabel == "scaling" {
			// Scaling ads: kill if ROAS drops below 1.0 after $20 spend (losing money)
			if spend >= 20 && roas > 0 && roas < 1.0 {
				verdict = "KILL"
			}
			// Massive winner: sustained 3.0+ ROAS after $1000+ total spend → lookalike
			// (Note: this checks yesterday's spend only; cumulative tracked by Claude in memory)
		}

		insights = append(insights, MetaAdInsight{
			AdID:          d.AdID,
			AdName:        d.AdName,
			Spend:         spend,
			Impressions:   impressions,
			Clicks:        clicks,
			CTR:           ctr,
			CPC:           cpc,
			CPM:           cpm,
			AddToCart:     addToCart,
			Purchases:     purchases,
			PurchaseValue: purchaseValue,
			ROAS:          roas,
			CostPerATC:    costPerATC,
			Verdict:       verdict,
			Campaign:      campaignLabel,
		})
	}

	return insights, nil
}

// metaGetInsightsMultiDay fetches N days of performance for creative fatigue detection.
// Returns insights aggregated per ad over the date range.
func metaGetInsightsMultiDay(cfg *Config, campaignID, campaignLabel string, days int) ([]MetaAdInsight, error) {
	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -(days - 1))

	url := fmt.Sprintf("%s/%s/insights?fields=ad_id,ad_name,spend,impressions,clicks,ctr,cpc,cpm,actions,action_values,cost_per_action_type,purchase_roas&level=ad&time_range={\"since\":\"%s\",\"until\":\"%s\"}&access_token=%s",
		metaAPIBase, campaignID,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"),
		cfg.MetaAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Meta multi-day insights HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Same parsing as metaGetInsights
	var result struct {
		Data []struct {
			AdID        string `json:"ad_id"`
			AdName      string `json:"ad_name"`
			Spend       string `json:"spend"`
			Impressions string `json:"impressions"`
			Clicks      string `json:"clicks"`
			CTR         string `json:"ctr"`
			CPC         string `json:"cpc"`
			CPM         string `json:"cpm"`
			Actions     []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"actions"`
			ActionValues []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"action_values"`
			CostPerAction []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"cost_per_action_type"`
			PurchaseROAS []struct {
				ActionType string `json:"action_type"`
				Value      string `json:"value"`
			} `json:"purchase_roas"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	var insights []MetaAdInsight
	for _, d := range result.Data {
		var spend, ctr, cpc, cpm float64
		var impressions, clicks int
		fmt.Sscanf(d.Spend, "%f", &spend)
		fmt.Sscanf(d.CTR, "%f", &ctr)
		fmt.Sscanf(d.CPC, "%f", &cpc)
		fmt.Sscanf(d.CPM, "%f", &cpm)
		fmt.Sscanf(d.Impressions, "%d", &impressions)
		fmt.Sscanf(d.Clicks, "%d", &clicks)

		var addToCart, purchases int
		var costPerATC, purchaseValue, roas float64
		for _, a := range d.Actions {
			switch a.ActionType {
			case "add_to_cart":
				fmt.Sscanf(a.Value, "%d", &addToCart)
			case "purchase", "offsite_conversion.fb_pixel_purchase":
				fmt.Sscanf(a.Value, "%d", &purchases)
			}
		}
		for _, av := range d.ActionValues {
			if av.ActionType == "purchase" || av.ActionType == "offsite_conversion.fb_pixel_purchase" {
				fmt.Sscanf(av.Value, "%f", &purchaseValue)
			}
		}
		for _, c := range d.CostPerAction {
			if c.ActionType == "add_to_cart" {
				fmt.Sscanf(c.Value, "%f", &costPerATC)
			}
		}
		for _, r := range d.PurchaseROAS {
			if r.ActionType == "omni_purchase" || r.ActionType == "purchase" {
				fmt.Sscanf(r.Value, "%f", &roas)
			}
		}
		if roas == 0 && purchaseValue > 0 && spend > 0 {
			roas = purchaseValue / spend
		}

		insights = append(insights, MetaAdInsight{
			AdID:          d.AdID,
			AdName:        d.AdName,
			Spend:         spend,
			Impressions:   impressions,
			Clicks:        clicks,
			CTR:           ctr,
			CPC:           cpc,
			CPM:           cpm,
			AddToCart:     addToCart,
			Purchases:     purchases,
			PurchaseValue: purchaseValue,
			ROAS:          roas,
			CostPerATC:    costPerATC,
			Campaign:      campaignLabel,
		})
	}

	return insights, nil
}

// metaDetectFatigue compares 3-day performance to yesterday. If CTR dropped >40%, flag as FATIGUE.
func metaDetectFatigue(cfg *Config, campaignID, campaignLabel string) ([]MetaAdInsight, error) {
	threeDayInsights, err := metaGetInsightsMultiDay(cfg, campaignID, campaignLabel, 3)
	if err != nil {
		return nil, err
	}
	yesterdayInsights, err := metaGetInsights(cfg, campaignID, campaignLabel)
	if err != nil {
		return nil, err
	}

	// Build lookup: 3-day avg CTR per ad
	avgCTR := map[string]float64{}
	for _, i := range threeDayInsights {
		if i.Impressions > 100 { // Need enough data
			avgCTR[i.AdID] = i.CTR
		}
	}

	// Compare yesterday to 3-day average
	var fatigued []MetaAdInsight
	for _, y := range yesterdayInsights {
		avg, ok := avgCTR[y.AdID]
		if !ok || avg <= 0 {
			continue
		}
		// CTR dropped more than 40% from 3-day average → fatigue
		if y.CTR < avg*0.6 && y.Spend >= 3 {
			y.Verdict = "FATIGUE"
			fatigued = append(fatigued, y)
		}
	}

	return fatigued, nil
}

// metaPauseAd pauses (kills) an ad.
func metaPauseAd(cfg *Config, adID string) error {
	params := map[string]interface{}{"status": "PAUSED"}
	url := fmt.Sprintf("%s/%s", metaAPIBase, adID)

	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	q := req.URL.Query()
	q.Add("access_token", cfg.MetaAccessToken)
	req.URL.RawQuery = q.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Meta pause HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// metaDuplicateAd copies an ad's creative into a new ad set in the target campaign.
func metaDuplicateAd(cfg *Config, sourceAdID, targetCampaignID string) (string, error) {
	// Get the source ad's creative
	url := fmt.Sprintf("%s/%s?fields=creative{id},adset{daily_budget}&access_token=%s",
		metaAPIBase, sourceAdID, cfg.MetaAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var adInfo struct {
		Creative struct {
			ID string `json:"id"`
		} `json:"creative"`
	}
	json.Unmarshal(body, &adInfo)

	if adInfo.Creative.ID == "" {
		return "", fmt.Errorf("cannot read creative from ad %s", sourceAdID)
	}

	// Create new ad set in scaling campaign — $20/day, purchase optimized
	adSetParams := map[string]interface{}{
		"name":              fmt.Sprintf("AutoCMO Winner - %s", time.Now().Format("Jan 02")),
		"campaign_id":       targetCampaignID,
		"daily_budget":      2000, // $20/day for winners
		"billing_event":     "IMPRESSIONS",
		"optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy":      "LOWEST_COST_WITHOUT_CAP",
		"targeting": map[string]interface{}{
			"geo_locations": map[string]interface{}{
				"countries": []string{"US"},
			},
		},
		"publisher_platforms":  []string{"facebook", "instagram"},
		"facebook_positions":  []string{"feed", "story", "reels"},
		"instagram_positions": []string{"stream", "story", "reels"},
		"status":              "ACTIVE",
	}
	if cfg.MetaPixelID != "" {
		adSetParams["promoted_object"] = map[string]interface{}{
			"pixel_id":          cfg.MetaPixelID,
			"custom_event_type": "PURCHASE",
		}
	}

	adSetID, err := metaPost(cfg, fmt.Sprintf("/%s/adsets", cfg.MetaAdAccountID), adSetParams)
	if err != nil {
		return "", fmt.Errorf("scaling ad set failed: %w", err)
	}

	// Create new ad with the winner's creative
	adParams := map[string]interface{}{
		"name":     fmt.Sprintf("Winner from %s - %s", sourceAdID, time.Now().Format("Jan 02")),
		"adset_id": adSetID,
		"creative": map[string]string{"creative_id": adInfo.Creative.ID},
		"status":   "ACTIVE",
	}

	return metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
}

// ── Lookalike Audiences ──────────────────────────────────────

// metaCreateLookalike creates a lookalike audience from pixel purchasers.
// Only triggered for massive winners: sustained 3.0+ ROAS after $1000+ spend.
func metaCreateLookalike(cfg *Config, sourceAdID string) (string, error) {
	if cfg.MetaPixelID == "" {
		return "", fmt.Errorf("pixel ID required for lookalike creation")
	}

	// Step 1: Create custom audience from pixel purchasers
	customAudienceParams := map[string]interface{}{
		"name":        fmt.Sprintf("AutoCMO Purchasers - %s", time.Now().Format("Jan 02")),
		"description": fmt.Sprintf("Auto-created from winner ad %s", sourceAdID),
		"subtype":     "WEBSITE",
		"rule": map[string]interface{}{
			"inclusions": map[string]interface{}{
				"operator": "or",
				"rules": []map[string]interface{}{
					{
						"event_sources": []map[string]string{
							{"id": cfg.MetaPixelID, "type": "pixel"},
						},
						"retention_seconds": 2592000, // 30 days
						"filter": map[string]interface{}{
							"operator": "and",
							"filters": []map[string]interface{}{
								{"field": "event", "operator": "eq", "value": "Purchase"},
							},
						},
					},
				},
			},
		},
	}

	customAudienceID, err := metaPost(cfg, fmt.Sprintf("/%s/customaudiences", cfg.MetaAdAccountID), customAudienceParams)
	if err != nil {
		return "", fmt.Errorf("custom audience creation failed: %w", err)
	}
	fmt.Printf("  Custom audience: %s\n", customAudienceID)

	// Step 2: Create 1% lookalike from that audience
	lookalikeParams := map[string]interface{}{
		"name":       fmt.Sprintf("AutoCMO 1%% Lookalike - %s", time.Now().Format("Jan 02")),
		"subtype":    "LOOKALIKE",
		"origin_audience_id": customAudienceID,
		"lookalike_spec": map[string]interface{}{
			"type":    "similarity",
			"country": "US",
			"ratio":   0.01, // 1% lookalike
		},
	}

	lookalikeID, err := metaPost(cfg, fmt.Sprintf("/%s/customaudiences", cfg.MetaAdAccountID), lookalikeParams)
	if err != nil {
		return "", fmt.Errorf("lookalike creation failed: %w", err)
	}

	fmt.Printf("  Lookalike audience: %s (1%% US)\n", lookalikeID)
	return lookalikeID, nil
}

// ── Multi-Creative Ad Sets ───────────────────────────────────

// metaCreateMultiCreativeAdSet creates an ad set with up to 3 creatives.
// Meta optimizes delivery across the creatives automatically.
func metaCreateMultiCreativeAdSet(cfg *Config, campaignID string, imageHashes []string, videoIDs []string, headline, bodyText, link string, dailyBudget float64) ([]string, error) {
	if dailyBudget <= 0 {
		dailyBudget = 5.0
	}
	budgetCents := int(dailyBudget * 100)

	// Create one ad set
	adSetParams := map[string]interface{}{
		"name":              fmt.Sprintf("AutoCMO Multi - %s", time.Now().Format("Jan 02 15:04")),
		"campaign_id":       campaignID,
		"daily_budget":      budgetCents,
		"billing_event":     "IMPRESSIONS",
		"optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy":      "LOWEST_COST_WITHOUT_CAP",
		"targeting": map[string]interface{}{
			"geo_locations": map[string]interface{}{
				"countries": []string{"US"},
			},
		},
		"publisher_platforms":  []string{"facebook", "instagram"},
		"facebook_positions":  []string{"feed", "story", "reels"},
		"instagram_positions": []string{"stream", "story", "reels"},
		"status":              "ACTIVE",
	}
	if cfg.MetaPixelID != "" {
		adSetParams["promoted_object"] = map[string]interface{}{
			"pixel_id":          cfg.MetaPixelID,
			"custom_event_type": "PURCHASE",
		}
	}

	adSetID, err := metaPost(cfg, fmt.Sprintf("/%s/adsets", cfg.MetaAdAccountID), adSetParams)
	if err != nil {
		return nil, fmt.Errorf("multi-creative ad set failed: %w", err)
	}
	fmt.Printf("  Ad Set: %s\n", adSetID)

	var adIDs []string

	// Create ads for each image
	for i, hash := range imageHashes {
		creativeParams := map[string]interface{}{
			"name": fmt.Sprintf("AutoCMO Img %d - %s", i+1, time.Now().Format("Jan 02")),
			"object_story_spec": map[string]interface{}{
				"page_id": cfg.MetaPageID,
				"link_data": map[string]interface{}{
					"image_hash": hash,
					"link":       link,
					"message":    bodyText,
					"name":       headline,
					"call_to_action": map[string]interface{}{
						"type":  "SHOP_NOW",
						"value": map[string]string{"link": link},
					},
				},
			},
		}
		creativeID, err := metaPost(cfg, fmt.Sprintf("/%s/adcreatives", cfg.MetaAdAccountID), creativeParams)
		if err != nil {
			fmt.Printf("  [WARN] Creative %d failed: %v\n", i+1, err)
			continue
		}

		adParams := map[string]interface{}{
			"name":     fmt.Sprintf("AutoCMO Ad %d - %s", i+1, time.Now().Format("Jan 02 15:04")),
			"adset_id": adSetID,
			"creative": map[string]string{"creative_id": creativeID},
			"status":   "ACTIVE",
		}
		adID, err := metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
		if err != nil {
			fmt.Printf("  [WARN] Ad %d failed: %v\n", i+1, err)
			continue
		}
		adIDs = append(adIDs, adID)
		fmt.Printf("  Ad %d: %s (image)\n", i+1, adID)
	}

	// Create ads for each video
	for i, vid := range videoIDs {
		creativeParams := map[string]interface{}{
			"name": fmt.Sprintf("AutoCMO Vid %d - %s", i+1, time.Now().Format("Jan 02")),
			"object_story_spec": map[string]interface{}{
				"page_id": cfg.MetaPageID,
				"video_data": map[string]interface{}{
					"video_id":        vid,
					"message":         bodyText,
					"title":           headline,
					"link_description": headline,
					"call_to_action": map[string]interface{}{
						"type":  "SHOP_NOW",
						"value": map[string]string{"link": link},
					},
				},
			},
		}
		creativeID, err := metaPost(cfg, fmt.Sprintf("/%s/adcreatives", cfg.MetaAdAccountID), creativeParams)
		if err != nil {
			fmt.Printf("  [WARN] Video creative %d failed: %v\n", i+1, err)
			continue
		}

		adParams := map[string]interface{}{
			"name":     fmt.Sprintf("AutoCMO Video Ad %d - %s", i+1, time.Now().Format("Jan 02 15:04")),
			"adset_id": adSetID,
			"creative": map[string]string{"creative_id": creativeID},
			"status":   "ACTIVE",
		}
		adID, err := metaPost(cfg, fmt.Sprintf("/%s/ads", cfg.MetaAdAccountID), adParams)
		if err != nil {
			fmt.Printf("  [WARN] Video ad %d failed: %v\n", i+1, err)
			continue
		}
		adIDs = append(adIDs, adID)
		fmt.Printf("  Ad %d: %s (video)\n", len(imageHashes)+i+1, adID)
	}

	return adIDs, nil
}

// ── Helper ───────────────────────────────────────────────────

func metaPost(cfg *Config, endpoint string, params map[string]interface{}) (string, error) {
	params["access_token"] = cfg.MetaAccessToken

	body, _ := json.Marshal(params)
	url := metaAPIBase + endpoint

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Meta API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &result)

	if result.Error != nil {
		return "", fmt.Errorf("Meta API error: %s", result.Error.Message)
	}
	if result.ID == "" {
		return "", fmt.Errorf("no ID in Meta response: %s", string(respBody))
	}

	return result.ID, nil
}

// ── Pipeline Integration ─────────────────────────────────────

func runMetaPush(cfg *Config, cmd *Command) {
	if cfg.MetaAccessToken == "" {
		log.Fatal("[ERROR] metaAccessToken required — get a System User token from Business Manager")
	}
	if cfg.MetaAdAccountID == "" {
		log.Fatal("[ERROR] metaAdAccountId required — format: act_XXXXXXXXX")
	}
	if cfg.MetaPageID == "" {
		log.Fatal("[ERROR] metaPageId required — your Facebook Page ID")
	}

	fmt.Println("============================================================")
	fmt.Println("  Meta Ads — Pushing Creative")
	fmt.Println("============================================================")

	// Ensure campaigns exist
	testingID, _, err := metaEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	// Determine creative type — image or video
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
		// Video ad
		fmt.Printf("  Uploading video: %s\n", filepath.Base(videoPath))
		videoID, err := metaUploadVideo(cfg, videoPath)
		if err != nil {
			log.Fatalf("[ERROR] Video upload failed: %v", err)
		}
		fmt.Printf("  Video ID: %s\n", videoID)
		fmt.Printf("  Creating video ad ($%.0f/day)...\n", budget)
		adID, err = metaCreateVideoAd(cfg, testingID, videoID, headline, adBody, link, budget)
		if err != nil {
			log.Fatalf("[ERROR] Video ad creation failed: %v", err)
		}
	} else {
		// Image ad
		fmt.Printf("  Uploading image: %s\n", filepath.Base(imagePath))
		imageHash, err := metaUploadImage(cfg, imagePath)
		if err != nil {
			log.Fatalf("[ERROR] Image upload failed: %v", err)
		}
		fmt.Printf("  Image hash: %s\n", imageHash)
		fmt.Printf("  Creating image ad ($%.0f/day)...\n", budget)
		adID, err = metaCreateImageAd(cfg, testingID, imageHash, headline, adBody, link, budget)
		if err != nil {
			log.Fatalf("[ERROR] Image ad creation failed: %v", err)
		}
	}

	fmt.Println("\n============================================================")
	fmt.Printf("  Ad live: %s\n", adID)
	fmt.Printf("  Campaign: Auto CMO - Testing\n")
	fmt.Printf("  Budget: $%.0f/day\n", budget)
	fmt.Printf("  Optimization: Purchase conversions\n")
	fmt.Printf("  Placement: Feed + Stories + Reels\n")
	fmt.Println("============================================================")
}

func runMetaInsights(cfg *Config, cmd *Command) {
	if cfg.MetaAccessToken == "" {
		log.Fatal("[ERROR] metaAccessToken required")
	}

	fmt.Println("============================================================")
	fmt.Println("  Meta Ads — Yesterday's Performance (All Campaigns)")
	fmt.Println("============================================================")

	testingID, scalingID, err := metaEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	// Query both Testing and Scaling campaigns
	var allInsights []MetaAdInsight

	testingInsights, err := metaGetInsights(cfg, testingID, "testing")
	if err != nil {
		fmt.Printf("  [WARN] Testing insights failed: %v\n", err)
	} else {
		allInsights = append(allInsights, testingInsights...)
	}

	scalingInsights, err := metaGetInsights(cfg, scalingID, "scaling")
	if err != nil {
		fmt.Printf("  [WARN] Scaling insights failed: %v\n", err)
	} else {
		allInsights = append(allInsights, scalingInsights...)
	}

	// Also check retargeting campaign if it exists
	retargetingID, _ := metaFindCampaign(cfg, "Auto CMO - Retargeting")
	if retargetingID != "" {
		retInsights, err := metaGetInsights(cfg, retargetingID, "retargeting")
		if err != nil {
			fmt.Printf("  [WARN] Retargeting insights failed: %v\n", err)
		} else {
			allInsights = append(allInsights, retInsights...)
		}
	}

	if len(allInsights) == 0 {
		fmt.Println("  No data yet — ads need at least 24 hours to report.")
		return
	}

	// Creative fatigue detection
	fatigued, _ := metaDetectFatigue(cfg, testingID, "testing")
	scalingFatigued, _ := metaDetectFatigue(cfg, scalingID, "scaling")
	fatigued = append(fatigued, scalingFatigued...)

	// Merge fatigue verdicts into allInsights
	fatigueSet := map[string]bool{}
	for _, f := range fatigued {
		fatigueSet[f.AdID] = true
	}
	for i := range allInsights {
		if fatigueSet[allInsights[i].AdID] && allInsights[i].Verdict == "" {
			allInsights[i].Verdict = "FATIGUE"
		}
	}

	fmt.Printf("\n  %-8s %-8s %-6s %-6s %-6s %-5s %-5s %-6s %-8s %-8s %s\n",
		"CAMP", "SPEND", "IMPR", "CLICK", "CTR", "ATC", "PURCH", "ROAS", "$/ATC", "VERDICT", "AD")
	fmt.Println("  ────────────────────────────────────────────────────────────────────────────────────")
	for _, i := range allInsights {
		atcStr := "-"
		if i.CostPerATC > 0 {
			atcStr = fmt.Sprintf("$%.2f", i.CostPerATC)
		}
		roasStr := "-"
		if i.ROAS > 0 {
			roasStr = fmt.Sprintf("%.1fx", i.ROAS)
		}
		campLabel := i.Campaign
		if len(campLabel) > 7 {
			campLabel = campLabel[:7]
		}
		fmt.Printf("  %-8s $%-7.2f %-6d %-6d %-5.1f%% %-5d %-5d %-6s %-8s %-8s %s\n",
			campLabel, i.Spend, i.Impressions, i.Clicks, i.CTR, i.AddToCart, i.Purchases, roasStr, atcStr, i.Verdict, i.AdName)
	}
	fmt.Println("  ────────────────────────────────────────────────────────────────────────────────────")

	// Output as JSON for Claude to parse
	jsonData, _ := json.MarshalIndent(allInsights, "", "  ")
	fmt.Printf("\n%s\n", string(jsonData))
}

func runMetaSetup(cfg *Config) {
	if cfg.MetaAccessToken == "" {
		log.Fatal("[ERROR] metaAccessToken required")
	}
	if cfg.MetaAdAccountID == "" {
		log.Fatal("[ERROR] metaAdAccountId required")
	}

	fmt.Println("============================================================")
	fmt.Println("  Meta Ads — Campaign Setup")
	fmt.Println("============================================================")

	_, _, err := metaEnsureCampaigns(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	fmt.Println("\n  Campaigns ready. Use 'meta-push' to create ads.")
}
