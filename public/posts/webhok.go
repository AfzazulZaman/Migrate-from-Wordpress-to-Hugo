package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"regexp"
)

// Post structure for incoming webhook data
type Post struct {
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Date     string `json:"date"`
	Modified string `json:"modified"`
	Slug     string `json:"slug"`
	Status   string `json:"status"`
}

// sanitizeFilename removes special characters from filenames
func sanitizeFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	name = reg.ReplaceAllString(name, "")
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")
	return name
}

// convertHTMLToMarkdown converts HTML to Markdown
func convertHTMLToMarkdown(html string) string {
	markdown := html
	markdown = regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`).ReplaceAllString(markdown, "# $1\n")
	markdown = regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`).ReplaceAllString(markdown, "## $1\n")
	markdown = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`).ReplaceAllString(markdown, "### $1\n")
	markdown = regexp.MustCompile(`<p[^>]*>(.*?)</p>`).ReplaceAllString(markdown, "$1\n\n")
	markdown = regexp.MustCompile(`<strong>(.*?)</strong>`).ReplaceAllString(markdown, "**$1**")
	markdown = regexp.MustCompile(`<em>(.*?)</em>`).ReplaceAllString(markdown, "*$1*")
	return markdown
}

// createHugoContent generates Hugo Markdown content with front matter
func createHugoContent(post Post) string {
	frontMatter := fmt.Sprintf(`---
title: "%s"
date: %s
lastmod: %s
slug: "%s"
draft: false
---
`, post.Title.Rendered, post.Date, post.Modified, post.Slug)

	markdown := convertHTMLToMarkdown(post.Content.Rendered)
	return frontMatter + markdown
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var post Post
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(body, &post)
	if err != nil {
		http.Error(w, "Failed to parse JSON", http.StatusInternalServerError)
		return
	}

	if post.Status != "publish" {
		log.Println("Skipping non-published post")
		w.WriteHeader(http.StatusOK)
		return
	}

	filename := sanitizeFilename(post.Slug) + ".md"
	outputDir := "content/posts"
	filepath := filepath.Join(outputDir, filename)

	content := createHugoContent(post)

	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		http.Error(w, "Failed to create content directory", http.StatusInternalServerError)
		return
	}

	err = ioutil.WriteFile(filepath, []byte(content), 0644)
	if err != nil {
		http.Error(w, "Failed to write content file", http.StatusInternalServerError)
		return
	}

	// Trigger Hugo rebuild
	cmd := exec.Command("hugo")
	cmd.Dir = "./" // Change to your Hugo site directory
	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to rebuild Hugo site", http.StatusInternalServerError)
		return
	}

	log.Printf("Updated post: %s\n", filename)
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/webhook", handleWebhook)
	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
