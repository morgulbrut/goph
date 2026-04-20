package main

import (
	"testing"

	"golang.org/x/net/html"
)

// wrapInDiv wraps HTML in the mw-parser-output div that Wikipedia returns.
func wrapInDiv(inner string) string {
	return `<div class="mw-parser-output">` + inner + `</div>`
}

func TestExtractFirstLink(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		currentTitle string
		want         string
	}{
		{
			name: "simple first link",
			html: wrapInDiv(`<p>A <a href="/wiki/Machine">machine</a> that computes.</p>`),
			want: "Machine",
		},
		{
			name: "skip link in parentheses",
			html: wrapInDiv(`<p>Text (<a href="/wiki/Skip">skip</a>) and <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "skip italic link",
			html: wrapInDiv(`<p><i><a href="/wiki/Skip">italic</a></i> then <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "skip em italic link",
			html: wrapInDiv(`<p><em><a href="/wiki/Skip">em</a></em> then <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "skip namespace link",
			html: wrapInDiv(`<p><a href="/wiki/File:Foo.jpg">image</a> then <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "skip self-link",
			html:         wrapInDiv(`<p><a href="/wiki/Self">Self</a> then <a href="/wiki/Target">target</a>.</p>`),
			currentTitle: "Self",
			want:         "Target",
		},
		{
			name: "skip sup footnote",
			html: wrapInDiv(`<p>Text<sup><a href="/wiki/Note">1</a></sup> and <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "paren depth restored after closing",
			html: wrapInDiv(`<p>Text (<a href="/wiki/Skip">skip</a>) more (<a href="/wiki/Skip2">skip2</a>) <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "first paragraph with no links, second has link",
			html: wrapInDiv(`<p>No links here.</p><p>But <a href="/wiki/Target">target</a> is here.</p>`),
			want: "Target",
		},
		{
			name: "skip external link",
			html: wrapInDiv(`<p>See <a href="https://example.com">external</a> and <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "link with underscores decoded to spaces",
			html: wrapInDiv(`<p>A <a href="/wiki/Social_science">social science</a>.</p>`),
			want: "Social science",
		},
		{
			name: "URL-encoded link decoded",
			html: wrapInDiv(`<p>A <a href="/wiki/Caf%C3%A9">café</a>.</p>`),
			want: "Café",
		},
		{
			name: "skip div infobox content",
			html: wrapInDiv(`<div><a href="/wiki/Skip">infobox link</a></div><p><a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "bold link is valid",
			html: wrapInDiv(`<p>A <b><a href="/wiki/Target">bold link</a></b>.</p>`),
			want: "Target",
		},
		{
			name: "parens span multiple nodes",
			html: wrapInDiv(`<p>Text (<b>bold</b> <a href="/wiki/Skip">skip</a>) <a href="/wiki/Target">target</a>.</p>`),
			want: "Target",
		},
		{
			name: "no valid link returns empty",
			html: wrapInDiv(`<p>No links at all.</p>`),
			want: "",
		},
		{
			name: "empty content returns empty",
			html: wrapInDiv(``),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractFirstLink(tc.html, tc.currentTitle)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Philosophy", "philosophy"},
		{"Social_Science", "social science"},
		{"  Foo  ", "foo"},
		{"Café", "café"},
	}
	for _, tc := range tests {
		got := normalizeTitle(tc.in)
		if got != tc.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTitlesEqual(t *testing.T) {
	if !titlesEqual("Philosophy", "philosophy") {
		t.Error("expected Philosophy == philosophy")
	}
	if !titlesEqual("Social_Science", "Social Science") {
		t.Error("expected Social_Science == Social Science")
	}
	if titlesEqual("Foo", "Bar") {
		t.Error("expected Foo != Bar")
	}
}

func TestExtractWikiTitle(t *testing.T) {
	tests := []struct {
		name         string
		href         string
		currentTitle string
		want         string
	}{
		{"normal link", "/wiki/Philosophy", "", "Philosophy"},
		{"underscored link", "/wiki/Social_science", "", "Social science"},
		{"namespace link", "/wiki/Category:Foo", "", ""},
		{"file link", "/wiki/File:Bar.jpg", "", ""},
		{"external link", "https://example.com", "", ""},
		{"self link", "/wiki/Self", "Self", ""},
		{"anchor stripped", "/wiki/Philosophy#History", "", "Philosophy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &html.Node{
				Type: html.ElementNode,
				Data: "a",
				Attr: []html.Attribute{{Key: "href", Val: tc.href}},
			}
			got := extractWikiTitle(n, tc.currentTitle)
			if got != tc.want {
				t.Errorf("extractWikiTitle(href=%q, current=%q) = %q, want %q",
					tc.href, tc.currentTitle, got, tc.want)
			}
		})
	}
}

func TestValidLang(t *testing.T) {
	if !validLang("en") {
		t.Error("expected 'en' to be valid")
	}
	if !validLang("de") {
		t.Error("expected 'de' to be valid")
	}
	if validLang("xx") {
		t.Error("expected 'xx' to be invalid")
	}
	if validLang("en.evil.com/path?x=") {
		t.Error("expected SSRF-attempt string to be invalid")
	}
}
