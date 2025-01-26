package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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

// Post represents the structure of WordPress post data
type Post struct {
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Date          string `json:"date"`
	Modified      string `json:"modified"`
	Slug          string `json:"slug"`
	Status        string `json:"status"`
	Tags          []int  `json:"tags"`
	Categories    []int  `json:"categories"`
	FeaturedMedia int    `json:"featured_media"`
}

// sanitizeFilename removes special characters from filenames
func sanitizeFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	reg := regexp.MustCompile("[^a-z0-9-]")
	name = reg.ReplaceAllString(name, "")
	reg = regexp.MustCompile("-+")
	name = reg.ReplaceAllString(name, "-")
	return name
}

// convertHTMLToMarkdown converts HTML to Markdown and handles images
func convertHTMLToMarkdown(html string, staticDir string) string {
	markdown := html

	imgTagRegex := regexp.MustCompile(`<img\s+[^>]*src=["']([^"']+)["'][^>]*>`)
	markdown = imgTagRegex.ReplaceAllStringFunc(markdown, func(match string) string {
		imgURL := imgTagRegex.FindStringSubmatch(match)[1]
		markdownImagePath, err := downloadImage(imgURL, staticDir)
		if err != nil {
			log.Printf("Failed to download image: %v", err)
			return ""
		}
		return fmt.Sprintf("![](%s)", markdownImagePath)
	})

	markdown = regexp.MustCompile("<h1[^>]*>(.*?)</h1>").ReplaceAllString(markdown, "# $1\n")
	markdown = regexp.MustCompile("<h2[^>]*>(.*?)</h2>").ReplaceAllString(markdown, "## $1\n")
	markdown = regexp.MustCompile("<h3[^>]*>(.*?)</h3>").ReplaceAllString(markdown, "### $1\n")
	markdown = regexp.MustCompile("<p[^>]*>(.*?)</p>").ReplaceAllString(markdown, "$1\n\n")
	markdown = regexp.MustCompile("<strong>(.*?)</strong>").ReplaceAllString(markdown, "**$1**")
	markdown = regexp.MustCompile("<em>(.*?)</em>").ReplaceAllString(markdown, "*$1*")
	markdown = regexp.MustCompile("</?[^>]*>").ReplaceAllString(markdown, "")

	return markdown
}

// createHugoFrontMatter generates Hugo front matter in YAML format
func createHugoFrontMatter(post Post) string {
	frontMatter := "---\n"
	frontMatter += fmt.Sprintf("title: \"%s\"\n", post.Title.Rendered)
	frontMatter += fmt.Sprintf("date: %s\n", post.Date)
	frontMatter += fmt.Sprintf("lastmod: %s\n", post.Modified)
	frontMatter += fmt.Sprintf("slug: \"%s\"\n", post.Slug)
	frontMatter += "draft: false\n"
	if len(post.Categories) > 0 {
		frontMatter += "categories:\n"
		for _, cat := range post.Categories {
			frontMatter += fmt.Sprintf("  - %d\n", cat)
		}
	}
	if len(post.Tags) > 0 {
		frontMatter += "tags:\n"
		for _, tag := range post.Tags {
			frontMatter += fmt.Sprintf("  - %d\n", tag)
		}
	}
	frontMatter += "---\n\n"
	return frontMatter
}

// downloadImage fetches an image from a URL and saves it locally in the Hugo project
func downloadImage(imgURL string, outputDir string) (string, error) {
	parsedURL, err := url.Parse(imgURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse image URL: %v", err)
	}
	fileName := path.Base(parsedURL.Path)
	filePath := filepath.Join(outputDir, "images", fileName)

	err = os.MkdirAll(filepath.Join(outputDir, "images"), os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create image directory: %v", err)
	}

	resp, err := http.Get(imgURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	imgData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %v", err)
	}

	err = ioutil.WriteFile(filePath, imgData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write image file: %v", err)
	}

	return "/images/" + fileName, nil
}

// fetchPosts fetches WordPress posts from the API
func fetchPosts(apiURL string) ([]Post, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var posts []Post
	err = json.Unmarshal(body, &posts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return posts, nil
}

func main() {
	apiURL := "https://animalsjunction.com/wp-json/wp/v2/posts"
	outputDir := "content/posts"

	lastModified := make(map[string]string)
	existingPosts := make(map[string]bool)

	for {
		posts, err := fetchPosts(apiURL)
		if err != nil {
			log.Printf("Error fetching posts: %v", err)
			time.Sleep(4 * time.Second)
			continue
		}

		currentSlugs := make(map[string]bool)
		for _, post := range posts {
			currentSlugs[post.Slug] = true
		}

		for slug := range existingPosts {
			if !currentSlugs[slug] {
				filename := filepath.Join(outputDir, slug+".md")
				err := os.Remove(filename)
				if err != nil {
					log.Printf("Failed to remove deleted post %s: %v", filename, err)
				} else {
					fmt.Printf("Deleted: %s\n", filename)
				}
				delete(existingPosts, slug)
			}
		}

		for _, post := range posts {
			existingPosts[post.Slug] = true
			if lastModified[post.Slug] != post.Modified {
				filename := post.Slug
				if filename == "" {
					filename = sanitizeFilename(post.Title.Rendered)
				}
				filename = filepath.Join(outputDir, filename+".md")

				markdown := convertHTMLToMarkdown(post.Content.Rendered, outputDir)
				hugoContent := createHugoFrontMatter(post) + markdown

				err := ioutil.WriteFile(filename, []byte(hugoContent), 0644)
				if err != nil {
					log.Printf("Failed to write file %s: %v", filename, err)
				} else {
					fmt.Printf("Saved: %s\n", filename)
				}
				lastModified[post.Slug] = post.Modified
			}
		}

		time.Sleep(4 * time.Second)
	}
}
