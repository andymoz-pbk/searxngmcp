package main

import (
	"bytes"
	"net/url"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
)

// extractReadable uses Mozilla Readability to extract the main article
// content from an HTML page. Returns plain text and cleaned HTML.
// If extraction fails or returns empty, both values are empty strings.
func extractReadable(html, pageURL string) (textContent, htmlContent string) {
	var u *url.URL
	if pageURL != "" {
		if parsed, err := url.Parse(pageURL); err == nil {
			u = parsed
		}
	}

	article, err := readability.FromReader(strings.NewReader(html), u)
	if err != nil {
		return "", ""
	}

	var buf bytes.Buffer
	if err := article.RenderText(&buf); err != nil {
		return "", ""
	}
	textContent = strings.TrimSpace(buf.String())

	buf.Reset()
	if err := article.RenderHTML(&buf); err != nil {
		return textContent, ""
	}
	htmlContent = strings.TrimSpace(buf.String())

	if textContent == "" && htmlContent == "" {
		return "", ""
	}

	return textContent, htmlContent
}
