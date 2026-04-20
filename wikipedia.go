package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const maxSteps = 100

// philosophyTitles maps Wikipedia language codes to the title of the
// "Philosophy" article in that language.
var philosophyTitles = map[string]string{
	"ar": "فلسفة",
	"cs": "Filozofie",
	"da": "Filosofi",
	"de": "Philosophie",
	"en": "Philosophy",
	"es": "Filosofía",
	"fi": "Filosofia",
	"fr": "Philosophie",
	"hu": "Filozófia",
	"it": "Filosofia",
	"ja": "哲学",
	"ko": "철학",
	"nb": "Filosofi",
	"nl": "Filosofie",
	"pl": "Filozofia",
	"pt": "Filosofia",
	"ro": "Filozofie",
	"ru": "Философия",
	"sv": "Filosofi",
	"tr": "Felsefe",
	"uk": "Філософія",
	"zh": "哲学",
}

// wikiAPIBaseURL maps each supported language code to its Wikipedia API
// base URL. These values are static constants — never derived from user
// input — which prevents SSRF via host injection.
var wikiAPIBaseURL = map[string]string{
	"ar": "https://ar.wikipedia.org/w/api.php",
	"cs": "https://cs.wikipedia.org/w/api.php",
	"da": "https://da.wikipedia.org/w/api.php",
	"de": "https://de.wikipedia.org/w/api.php",
	"en": "https://en.wikipedia.org/w/api.php",
	"es": "https://es.wikipedia.org/w/api.php",
	"fi": "https://fi.wikipedia.org/w/api.php",
	"fr": "https://fr.wikipedia.org/w/api.php",
	"hu": "https://hu.wikipedia.org/w/api.php",
	"it": "https://it.wikipedia.org/w/api.php",
	"ja": "https://ja.wikipedia.org/w/api.php",
	"ko": "https://ko.wikipedia.org/w/api.php",
	"nb": "https://nb.wikipedia.org/w/api.php",
	"nl": "https://nl.wikipedia.org/w/api.php",
	"pl": "https://pl.wikipedia.org/w/api.php",
	"pt": "https://pt.wikipedia.org/w/api.php",
	"ro": "https://ro.wikipedia.org/w/api.php",
	"ru": "https://ru.wikipedia.org/w/api.php",
	"sv": "https://sv.wikipedia.org/w/api.php",
	"tr": "https://tr.wikipedia.org/w/api.php",
	"uk": "https://uk.wikipedia.org/w/api.php",
	"zh": "https://zh.wikipedia.org/w/api.php",
}

// wikiAPIResponse represents the relevant fields from the Wikipedia
// action=parse API JSON response.
type wikiAPIResponse struct {
	Parse struct {
		Title string `json:"title"`
		Text  struct {
			Content string `json:"*"`
		} `json:"text"`
	} `json:"parse"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
}

// traceToPhilosophy starts at the given article and repeatedly follows
// the first valid link until it reaches the Philosophy article (or an
// error condition such as a loop or missing link).
func traceToPhilosophy(ctx context.Context, word, lang string) ([]string, bool, error) {
	if !validLang(lang) {
		return nil, false, fmt.Errorf("unsupported language code %q", lang)
	}

	target := philosophyTitles[lang]

	visited := make(map[string]bool)
	var steps []string
	current := word

	for i := 0; i < maxSteps; i++ {
		select {
		case <-ctx.Done():
			return steps, false, ctx.Err()
		default:
		}

		steps = append(steps, current)

		if titlesEqual(current, target) {
			return steps, true, nil
		}

		key := normalizeTitle(current)
		if visited[key] {
			return steps, false, fmt.Errorf("loop detected: reached %q again", current)
		}
		visited[key] = true

		next, err := getFirstLink(ctx, lang, current)
		if err != nil {
			return steps, false, fmt.Errorf("error processing %q: %w", current, err)
		}
		if next == "" {
			return steps, false, fmt.Errorf("no valid link found in %q", current)
		}

		current = next
	}

	return steps, false, fmt.Errorf("exceeded maximum of %d steps", maxSteps)
}

func titlesEqual(a, b string) bool {
	return normalizeTitle(a) == normalizeTitle(b)
}

func normalizeTitle(title string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), "_", " "))
}

// validLang returns true when lang is in the supported language allowlist.
func validLang(lang string) bool {
	_, ok := wikiAPIBaseURL[lang]
	return ok
}

// getFirstLink fetches the Wikipedia article for the given title in the
// given language and returns the title of the first valid linked article.
func getFirstLink(ctx context.Context, lang, title string) (string, error) {
	baseURL, ok := wikiAPIBaseURL[lang]
	if !ok {
		return "", fmt.Errorf("unsupported language code %q", lang)
	}

	// The base URL is a compile-time constant from wikiAPIBaseURL; only the
	// query parameters contain user-supplied data, which url.Values encodes.
	apiURL := baseURL + "?" + url.Values{
		"action":    {"parse"},
		"page":      {strings.ReplaceAll(title, " ", "_")},
		"prop":      {"text"},
		"format":    {"json"},
		"redirects": {"1"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "goph/1.0 (https://github.com/morgulbrut/goph; Go)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var apiResp wikiAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("Wikipedia API error (%s): %s", apiResp.Error.Code, apiResp.Error.Info)
	}

	htmlContent := apiResp.Parse.Text.Content
	if htmlContent == "" {
		return "", fmt.Errorf("empty article content")
	}

	return extractFirstLink(htmlContent, title)
}

// extractFirstLink parses the HTML of a Wikipedia article and returns the
// title of the first linked article that meets the game rules:
//   - not inside parentheses
//   - not in italics
//   - not a self-link
//   - not a namespaced link (File:, Category:, Help:, etc.)
func extractFirstLink(htmlContent, currentTitle string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("HTML parse error: %w", err)
	}

	content := findDivByClass(doc, "mw-parser-output")
	if content == nil {
		content = doc
	}

	for child := content.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "p" {
			continue
		}

		state := &parseState{currentTitle: currentTitle}
		if link := walkNode(child, state); link != "" {
			return link, nil
		}
	}

	return "", nil
}

// parseState holds traversal state while scanning a paragraph for links.
type parseState struct {
	parenDepth   int
	inItalic     bool
	currentTitle string
}

// walkNode traverses the HTML tree in document order and returns the first
// Wikipedia article link that is outside of parentheses and italics.
//
// Parenthesis depth is tracked across all text nodes; links inside <sup>,
// <sub>, <table>, <div>, and <figure> elements are ignored entirely.
// The children of <a> elements are never walked: this prevents parentheses
// inside link text from polluting the outer paren depth counter.
func walkNode(n *html.Node, state *parseState) string {
	switch n.Type {
	case html.TextNode:
		for _, ch := range n.Data {
			switch ch {
			case '(':
				state.parenDepth++
			case ')':
				if state.parenDepth > 0 {
					state.parenDepth--
				}
			}
		}
		return ""

	case html.ElementNode:
		switch n.Data {
		case "sup", "sub":
			// Footnote references and superscripts – skip entirely.
			return ""
		case "table", "div", "figure":
			// Infoboxes, tables, figures – skip entirely.
			return ""
		case "a":
			// Check the link if it is outside parentheses and italics.
			if !state.inItalic && state.parenDepth == 0 {
				if title := extractWikiTitle(n, state.currentTitle); title != "" {
					return title
				}
			}
			// Do NOT walk into <a> children so parens in link text are ignored.
			return ""
		}

		savedItalic := state.inItalic
		if n.Data == "i" || n.Data == "em" {
			state.inItalic = true
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if link := walkNode(child, state); link != "" {
				state.inItalic = savedItalic
				return link
			}
		}

		state.inItalic = savedItalic
	}

	return ""
}

// extractWikiTitle returns the article title from an anchor element,
// or an empty string if the link should be ignored.
func extractWikiTitle(n *html.Node, currentTitle string) string {
	href := getAttr(n, "href")
	if !strings.HasPrefix(href, "/wiki/") {
		return ""
	}

	raw := strings.TrimPrefix(href, "/wiki/")

	// Remove anchor fragment.
	if idx := strings.Index(raw, "#"); idx >= 0 {
		raw = raw[:idx]
	}

	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}
	decoded = strings.ReplaceAll(decoded, "_", " ")

	// Skip namespace links: Category:, File:, Help:, etc.
	if strings.Contains(decoded, ":") {
		return ""
	}

	// Skip self-links.
	if titlesEqual(decoded, currentTitle) {
		return ""
	}

	return decoded
}

// findDivByClass returns the first <div> whose class attribute contains
// the given class name, searching the tree depth-first.
func findDivByClass(n *html.Node, class string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Key == "class" && strings.Contains(a.Val, class) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findDivByClass(c, class); result != nil {
			return result
		}
	}
	return nil
}

// getAttr returns the value of the named attribute of an HTML node.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
