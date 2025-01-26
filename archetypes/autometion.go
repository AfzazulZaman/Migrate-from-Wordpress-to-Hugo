package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Post struct {
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Date, Modified, Slug string
	Tags, Categories    []int
}

func sanitizeFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	return name
}

func downloadImage(imgURL string) (string, error) {
	parsedURL, err := url.Parse(imgURL)
	if err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), path.Base(parsedURL.Path))
	filePath := filepath.Join("static", "images", fileName)

	os.MkdirAll(filepath.Dir(filePath), 0755)

	resp, err := http.Get(imgURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return "/images/" + fileName, err
}

func convertToMarkdown(html string) string {
	markdown := html

	imgRegex := regexp.MustCompile(`<img\s+[^>]*src=["']([^"']+)["'][^>]*>`)
	markdown = imgRegex.ReplaceAllStringFunc(markdown, func(match string) string {
		imgURL := imgRegex.FindStringSubmatch(match)[1]
		markdownPath, err := downloadImage(imgURL)
		if err != nil {
			log.Printf("Image download failed: %v", err)
			return ""
		}
		return fmt.Sprintf("![](%s)", markdownPath)
	})

	conversions := map[string]string{
		"<h1[^>]*>(.*?)</h1>": "# $1\n",
		"<h2[^>]*>(.*?)</h2>": "## $1\n",
		"<h3[^>]*>(.*?)</h3>": "### $1\n",
		"<p[^>]*>(.*?)</p>":   "$1\n\n",
		"<strong>(.*?)</strong>": "**$1**",
		"<em>(.*?)</em>":         "*$1*",
		"</?[^>]*>":              "",
	}

	for pattern, replacement := range conversions {
		markdown = regexp.MustCompile(pattern).ReplaceAllString(markdown, replacement)
	}

	return markdown
}

func createFrontMatter(post Post) string {
	fm := "---\n"
	fm += fmt.Sprintf("title: \"%s\"\n", post.Title.Rendered)
	fm += fmt.Sprintf("date: %s\n", post.Date)
	fm += fmt.Sprintf("lastmod: %s\n", post.Modified)
	fm += fmt.Sprintf("slug: \"%s\"\n", post.Slug)
	fm += "draft: false\n"

	if len(post.Categories) > 0 {
		fm += "categories:\n"
		for _, cat := range post.Categories {
			fm += fmt.Sprintf("  - %d\n", cat)
		}
	}

	if len(post.Tags) > 0 {
		fm += "tags:\n"
		for _, tag := range post.Tags {
			fm += fmt.Sprintf("  - %d\n", tag)
		}
	}

	fm += "---\n\n"
	return fm
}

func fetchPosts(apiURL string) ([]Post, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var posts []Post
	err = json.NewDecoder(resp.Body).Decode(&posts)
	return posts, err
}

func main() {
	apiURL := "https://animalsjunction.com/wp-json/wp/v2/posts"
	outputDir := "content/posts"

	existingPosts := make(map[string]bool)

	for {
		posts, err := fetchPosts(apiURL)
		if err != nil {
			log.Printf("Fetch error: %v", err)
			time.Sleep(4 * time.Second)
			continue
		}

		// Track current posts
		currentSlugs := make(map[string]bool)
		for _, post := range posts {
			currentSlugs[post.Slug] = true
			
			filename := filepath.Join(outputDir, post.Slug+".md")
			markdown := convertToMarkdown(post.Content.Rendered)
			content := createFrontMatter(post) + markdown

			os.WriteFile(filename, []byte(content), 0644)
			log.Printf("Processed: %s", filename)
		}

		// Remove posts deleted from WordPress
		for slug := range existingPosts {
			if !currentSlugs[slug] {
				filename := filepath.Join(outputDir, slug+".md")
				err := os.Remove(filename)
				if err != nil {
					log.Printf("Failed to delete %s: %v", filename, err)
				} else {
					log.Printf("Deleted post: %s", filename)
				}
			}
		}

		// Update existing posts
		existingPosts = currentSlugs

		time.Sleep(1 * time.Second)
	}
}