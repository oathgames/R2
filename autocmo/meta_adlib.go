package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AdLibAd represents a single ad from the Meta Ad Library.
type AdLibAd struct {
	ID                   string   `json:"id"`
	PageName             string   `json:"page_name"`
	PageID               string   `json:"page_id"`
	AdCreativeBodies     []string `json:"ad_creative_bodies"`
	AdCreativeLinkTitles []string `json:"ad_creative_link_titles"`
	AdCreativeLinkDescs  []string `json:"ad_creative_link_descriptions"`
	AdCreativeLinkCaps   []string `json:"ad_creative_link_captions"`
	AdSnapshotURL        string   `json:"ad_snapshot_url"`
	AdDeliveryStart      string   `json:"ad_delivery_start_time"`
	AdDeliveryStop       string   `json:"ad_delivery_stop_time"`
	PublisherPlatforms   []string `json:"publisher_platforms"`
	MediaType            string   `json:"media_type,omitempty"`
}

// AdLibResult is the output of a competitor scan.
type AdLibResult struct {
	PageName  string     `json:"page_name"`
	PageID    string     `json:"page_id"`
	AdCount   int        `json:"ad_count"`
	Ads       []AdLibAd  `json:"ads"`
}

// adLibSearch queries the Meta Ad Library API for active ads matching search terms.
// Uses ad_reached_countries=['GB'] to access commercial ads (EU/UK transparency rules).
func adLibSearch(accessToken, searchTerms string, limit int) ([]AdLibAd, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	params := url.Values{}
	params.Set("search_terms", searchTerms)
	params.Set("ad_reached_countries", "['GB']")
	params.Set("ad_active_status", "ACTIVE")
	params.Set("ad_type", "all")
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("fields", "id,page_name,page_id,ad_creative_bodies,ad_creative_link_titles,ad_creative_link_descriptions,ad_creative_link_captions,ad_snapshot_url,ad_delivery_start_time,publisher_platforms")
	params.Set("access_token", accessToken)

	reqURL := "https://graph.facebook.com/v22.0/ads_archive?" + params.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("Ad Library request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ad Library HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []AdLibAd `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != nil {
		return nil, fmt.Errorf("Ad Library error: %s", result.Error.Message)
	}

	return result.Data, nil
}

// adLibSearchByPage queries ads for a specific Facebook page ID.
func adLibSearchByPage(accessToken, pageID string, limit int) ([]AdLibAd, error) {
	if limit <= 0 {
		limit = 10
	}

	params := url.Values{}
	params.Set("search_page_ids", pageID)
	params.Set("ad_reached_countries", "['GB']")
	params.Set("ad_active_status", "ACTIVE")
	params.Set("ad_type", "all")
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("fields", "id,page_name,page_id,ad_creative_bodies,ad_creative_link_titles,ad_creative_link_descriptions,ad_creative_link_captions,ad_snapshot_url,ad_delivery_start_time,publisher_platforms")
	params.Set("access_token", accessToken)

	reqURL := "https://graph.facebook.com/v22.0/ads_archive?" + params.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("Ad Library request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ad Library HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []AdLibAd `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != nil {
		return nil, fmt.Errorf("Ad Library error: %s", result.Error.Message)
	}

	return result.Data, nil
}

// adLibFindPage searches for a Facebook page by name and returns the page ID.
// Uses the Ad Library page search to find the correct page.
func adLibFindPage(accessToken, brandName string) (string, string, error) {
	params := url.Values{}
	params.Set("search_terms", brandName)
	params.Set("ad_reached_countries", "['GB']")
	params.Set("ad_active_status", "ACTIVE")
	params.Set("ad_type", "all")
	params.Set("limit", "5")
	params.Set("fields", "page_name,page_id")
	params.Set("access_token", accessToken)

	reqURL := "https://graph.facebook.com/v22.0/ads_archive?" + params.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			PageName string `json:"page_name"`
			PageID   string `json:"page_id"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	if len(result.Data) == 0 {
		return "", "", fmt.Errorf("no ads found for '%s' in UK/EU", brandName)
	}

	// Return first match
	return result.Data[0].PageID, result.Data[0].PageName, nil
}

// ── CLI Entry Points ────────────────────────────────────────

func runCompetitorScan(cfg *Config, cmd *Command) {
	// Use Meta access token for Ad Library (same token works)
	token := cfg.MetaAccessToken
	if token == "" {
		log.Fatal("[ERROR] metaAccessToken required for competitor ad scanning (Ad Library API)")
	}

	fmt.Println("============================================================")
	fmt.Println("  Competitor Ad Intelligence — Meta Ad Library")
	fmt.Println("============================================================")

	// Competitor names come from blogBody (comma-separated brand names)
	var brandNames []string
	if cmd != nil && cmd.BlogBody != "" {
		for _, b := range strings.Split(cmd.BlogBody, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				brandNames = append(brandNames, b)
			}
		}
	}
	if len(brandNames) == 0 {
		log.Fatal("[ERROR] provide competitor brand names: {\"blogBody\": \"Madhappy,Pangaia,Teddy Fresh\"}")
	}

	adsPerBrand := 5
	if cmd != nil && cmd.ImageCount > 0 {
		adsPerBrand = cmd.ImageCount
	}

	var allResults []AdLibResult

	for _, brand := range brandNames {
		fmt.Printf("\n  Searching: %s\n", brand)

		ads, err := adLibSearch(token, brand, adsPerBrand)
		if err != nil {
			fmt.Printf("    [WARN] %v\n", err)
			continue
		}

		if len(ads) == 0 {
			fmt.Printf("    No active ads found in UK/EU\n")
			continue
		}

		// Group by page (search may return ads from multiple pages)
		pageAds := map[string]*AdLibResult{}
		for _, ad := range ads {
			key := ad.PageID
			if _, ok := pageAds[key]; !ok {
				pageAds[key] = &AdLibResult{
					PageName: ad.PageName,
					PageID:   ad.PageID,
				}
			}
			pageAds[key].Ads = append(pageAds[key].Ads, ad)
			pageAds[key].AdCount++
		}

		for _, result := range pageAds {
			fmt.Printf("    Page: %s (%s) — %d ads\n", result.PageName, result.PageID, result.AdCount)
			for i, ad := range result.Ads {
				// Extract hook from ad copy
				hook := ""
				if len(ad.AdCreativeBodies) > 0 {
					hook = ad.AdCreativeBodies[0]
					if len(hook) > 100 {
						hook = hook[:100] + "..."
					}
				}
				headline := ""
				if len(ad.AdCreativeLinkTitles) > 0 {
					headline = ad.AdCreativeLinkTitles[0]
				}

				platforms := strings.Join(ad.PublisherPlatforms, ",")
				fmt.Printf("      %d. [%s] %s\n", i+1, platforms, hook)
				if headline != "" {
					fmt.Printf("         Headline: %s\n", headline)
				}
				if ad.AdSnapshotURL != "" {
					fmt.Printf("         Snapshot: %s\n", ad.AdSnapshotURL)
				}
			}
			allResults = append(allResults, *result)
		}
	}

	fmt.Println("\n============================================================")
	fmt.Printf("  Scanned %d brand(s), found %d result(s)\n", len(brandNames), len(allResults))
	fmt.Println("============================================================")

	// Output JSON for Claude to parse
	jsonData, _ := json.MarshalIndent(allResults, "", "  ")
	fmt.Printf("\n%s\n", string(jsonData))
}
