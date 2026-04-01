package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const shopifyAPIVersion = "2024-10"

// shopifyBaseURL returns the admin API base for the configured store.
func shopifyBaseURL(cfg *Config) string {
	store := cfg.ShopifyStore
	if !strings.Contains(store, ".") {
		store = store + ".myshopify.com"
	}
	return fmt.Sprintf("https://%s/admin/api/%s", store, shopifyAPIVersion)
}

// shopifyRequest makes an authenticated request to the Shopify Admin API.
func shopifyRequest(cfg *Config, method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	url := shopifyBaseURL(cfg) + endpoint
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Shopify-Access-Token", cfg.ShopifyAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Shopify API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// shopifyFindBlog finds the default blog (usually "News") or creates one called "Blog".
// Returns the blog ID.
func shopifyFindBlog(cfg *Config) (int64, error) {
	data, err := shopifyRequest(cfg, "GET", "/blogs.json", nil)
	if err != nil {
		return 0, err
	}

	var result struct {
		Blogs []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"blogs"`
	}
	json.Unmarshal(data, &result)

	// Prefer "News" (Shopify default) or first blog found
	for _, b := range result.Blogs {
		if strings.EqualFold(b.Title, "News") || strings.EqualFold(b.Title, "Blog") {
			return b.ID, nil
		}
	}
	if len(result.Blogs) > 0 {
		return result.Blogs[0].ID, nil
	}

	// No blog exists — create one
	createBody := map[string]interface{}{
		"blog": map[string]interface{}{
			"title": "Blog",
		},
	}
	data, err = shopifyRequest(cfg, "POST", "/blogs.json", createBody)
	if err != nil {
		return 0, fmt.Errorf("cannot create blog: %w", err)
	}

	var created struct {
		Blog struct {
			ID int64 `json:"id"`
		} `json:"blog"`
	}
	json.Unmarshal(data, &created)

	if created.Blog.ID == 0 {
		return 0, fmt.Errorf("blog creation returned no ID")
	}
	return created.Blog.ID, nil
}

// shopifyCreateArticle creates a blog post with optional featured image and meta description.
func shopifyCreateArticle(cfg *Config, blogID int64, title, bodyHTML, tags, summaryHTML, imagePathOrURL string) (int64, string, error) {
	article := map[string]interface{}{
		"title":     title,
		"body_html": bodyHTML,
		"tags":      tags,
		"published": true,
	}

	// Meta description / excerpt
	if summaryHTML != "" {
		article["summary_html"] = summaryHTML
	}

	// Attach image if provided
	if imagePathOrURL != "" {
		if strings.HasPrefix(imagePathOrURL, "http") {
			article["image"] = map[string]string{
				"src": imagePathOrURL,
				"alt": title,
			}
		} else {
			// Local file — base64 encode
			imgData, err := os.ReadFile(imagePathOrURL)
			if err == nil {
				ext := strings.ToLower(filepath.Ext(imagePathOrURL))
				filename := filepath.Base(imagePathOrURL)
				_ = ext
				article["image"] = map[string]string{
					"attachment": base64.StdEncoding.EncodeToString(imgData),
					"filename":  filename,
					"alt":       title,
				}
			}
		}
	}

	payload := map[string]interface{}{
		"article": article,
	}

	data, err := shopifyRequest(cfg, "POST", fmt.Sprintf("/blogs/%d/articles.json", blogID), payload)
	if err != nil {
		return 0, "", err
	}

	var result struct {
		Article struct {
			ID     int64  `json:"id"`
			Handle string `json:"handle"`
		} `json:"article"`
	}
	json.Unmarshal(data, &result)

	store := cfg.ShopifyStore
	if !strings.Contains(store, ".") {
		store = store + ".myshopify.com"
	}
	articleURL := fmt.Sprintf("https://%s/blogs/news/%s", store, result.Article.Handle)

	return result.Article.ID, articleURL, nil
}

// shopifyListArticles lists recent blog articles.
func shopifyListArticles(cfg *Config, blogID int64, limit int) error {
	if limit <= 0 {
		limit = 10
	}
	data, err := shopifyRequest(cfg, "GET", fmt.Sprintf("/blogs/%d/articles.json?limit=%d", blogID, limit), nil)
	if err != nil {
		return err
	}

	var result struct {
		Articles []struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			Handle      string `json:"handle"`
			Tags        string `json:"tags"`
			PublishedAt string `json:"published_at"`
		} `json:"articles"`
	}
	json.Unmarshal(data, &result)

	if len(result.Articles) == 0 {
		fmt.Println("  No articles found.")
		return nil
	}

	fmt.Printf("\n  %-8s %-40s %-20s %s\n", "ID", "TITLE", "PUBLISHED", "TAGS")
	fmt.Println("  ────────────────────────────────────────────────────────────────────────────")
	for _, a := range result.Articles {
		pubDate := a.PublishedAt
		if len(pubDate) > 10 {
			pubDate = pubDate[:10]
		}
		title := a.Title
		if len(title) > 38 {
			title = title[:38] + ".."
		}
		tags := a.Tags
		if len(tags) > 30 {
			tags = tags[:30] + ".."
		}
		fmt.Printf("  %-8d %-40s %-20s %s\n", a.ID, title, pubDate, tags)
	}
	return nil
}

// ── Product SEO Functions ───────────────────────────────────

// ShopifyProduct holds the SEO-relevant fields of a Shopify product.
type ShopifyProduct struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Handle      string `json:"handle"`
	BodyHTML    string `json:"body_html"`
	ProductType string `json:"product_type"`
	Tags        string `json:"tags"`
	Images      []struct {
		ID  int64  `json:"id"`
		Src string `json:"src"`
		Alt string `json:"alt"`
	} `json:"images"`
}

// shopifyGetProducts fetches all products from the store.
func shopifyGetProducts(cfg *Config) ([]ShopifyProduct, error) {
	var allProducts []ShopifyProduct
	page := 1
	for {
		data, err := shopifyRequest(cfg, "GET", fmt.Sprintf("/products.json?limit=250&page=%d", page), nil)
		if err != nil {
			return allProducts, err
		}

		var result struct {
			Products []ShopifyProduct `json:"products"`
		}
		json.Unmarshal(data, &result)

		if len(result.Products) == 0 {
			break
		}
		allProducts = append(allProducts, result.Products...)
		if len(result.Products) < 250 {
			break
		}
		page++
	}
	return allProducts, nil
}

// shopifyUpdateImageAlt updates the alt text of a product image.
// ONLY use this to ADD alt text where none exists. Never overwrite existing alt text.
func shopifyUpdateImageAlt(cfg *Config, productID, imageID int64, altText string) error {
	payload := map[string]interface{}{
		"image": map[string]interface{}{
			"id":  imageID,
			"alt": altText,
		},
	}
	_, err := shopifyRequest(cfg, "PUT", fmt.Sprintf("/products/%d/images/%d.json", productID, imageID), payload)
	return err
}

// shopifySEOAudit runs a quick audit and returns a summary as JSON.
// NEVER modifies anything — read-only scan. Only flags images with empty alt text as fixable.
func shopifySEOAudit(cfg *Config) error {
	fmt.Println("============================================================")
	fmt.Println("  Shopify SEO Audit")
	fmt.Println("============================================================")

	products, err := shopifyGetProducts(cfg)
	if err != nil {
		return fmt.Errorf("cannot fetch products: %w", err)
	}

	thinDescriptions := 0
	missingAltText := 0
	totalImages := 0

	type issue struct {
		ProductID int64  `json:"product_id"`
		Title     string `json:"title"`
		Issue     string `json:"issue"`
		FixType   string `json:"fix_type"` // "auto" or "recommend"
	}
	var issues []issue

	for _, p := range products {
		// Check description length (informational only — NEVER auto-fix)
		descWords := len(strings.Fields(stripHTML(p.BodyHTML)))
		if descWords < 30 {
			thinDescriptions++
			issues = append(issues, issue{
				ProductID: p.ID,
				Title:     p.Title,
				Issue:     fmt.Sprintf("description only %d words (report only — do NOT modify)", descWords),
				FixType:   "info",
			})
		}

		// Check image alt text — ONLY fixable issue (add where empty, never overwrite)
		for _, img := range p.Images {
			totalImages++
			if img.Alt == "" {
				missingAltText++
				issues = append(issues, issue{
					ProductID: p.ID,
					Title:     p.Title,
					Issue:     fmt.Sprintf("image %d missing alt text (will add)", img.ID),
					FixType:   "auto",
				})
			}
		}
	}

	fmt.Printf("\n  Products:           %d\n", len(products))
	fmt.Printf("  Thin descriptions:  %d\n", thinDescriptions)
	fmt.Printf("  Missing alt text:   %d / %d images\n", missingAltText, totalImages)
	fmt.Printf("  Auto-fixable:       %d issues\n", len(issues))

	// Output as JSON for Claude to parse and write seo.md
	jsonData, _ := json.MarshalIndent(map[string]interface{}{
		"products_total":    len(products),
		"thin_descriptions": thinDescriptions,
		"missing_alt_text":  missingAltText,
		"total_images":      totalImages,
		"issues":            issues,
	}, "", "  ")
	fmt.Printf("\n%s\n", string(jsonData))

	return nil
}

// stripHTML removes HTML tags for word counting.
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ── CLI Entry Points ────────────────────────────────────────

func runBlogPost(cfg *Config, cmd *Command) {
	if cfg.ShopifyStore == "" || cfg.ShopifyAccessToken == "" {
		log.Fatal("[ERROR] shopifyStore and shopifyAccessToken required for blog posting")
	}

	if cmd.BlogTitle == "" || cmd.BlogBody == "" {
		log.Fatal("[ERROR] blogTitle and blogBody required")
	}

	fmt.Println("============================================================")
	fmt.Println("  Shopify Blog — Publishing Article")
	fmt.Println("============================================================")

	blogID, err := shopifyFindBlog(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	fmt.Printf("  Blog ID: %d\n", blogID)

	articleID, articleURL, err := shopifyCreateArticle(cfg, blogID, cmd.BlogTitle, cmd.BlogBody, cmd.BlogTags, cmd.BlogSummary, cmd.BlogImage)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	fmt.Printf("  Published: %s\n", cmd.BlogTitle)
	fmt.Printf("  Article ID: %d\n", articleID)
	fmt.Printf("  URL: %s\n", articleURL)
	fmt.Println("============================================================")
}

func runBlogList(cfg *Config) {
	if cfg.ShopifyStore == "" || cfg.ShopifyAccessToken == "" {
		log.Fatal("[ERROR] shopifyStore and shopifyAccessToken required")
	}

	fmt.Println("============================================================")
	fmt.Println("  Shopify Blog — Recent Articles")
	fmt.Println("============================================================")

	blogID, err := shopifyFindBlog(cfg)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	if err := shopifyListArticles(cfg, blogID, 10); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
}
