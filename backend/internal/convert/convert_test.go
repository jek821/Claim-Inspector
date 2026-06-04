package convert

import (
	"strings"
	"testing"
)

func TestMarkdown(t *testing.T) {
	md := "# Heading\n\nFrogs are **remarkable** vertebrates.\n\n- bullet one\n\nThe earliest frog appeared *190 million years ago*."
	out, err := FromBytes([]byte(md), ".md")
	if err != nil { t.Fatal(err) }
	if strings.Contains(out, "#") { t.Error("heading marker not stripped") }
	if strings.Contains(out, "**") { t.Error("bold markers not stripped") }
	if strings.Contains(out, "*190") { t.Error("italic markers not stripped") }
	if !strings.Contains(out, "remarkable") { t.Error("content missing") }
	if !strings.Contains(out, "190 million") { t.Error("content missing") }
	t.Logf("markdown output:\n%s", out)
}

func TestHTML(t *testing.T) {
	html := "<html><head><style>body{color:red}</style></head><body><h1>Title</h1><p>Frogs are <b>remarkable</b> vertebrates.</p><script>alert(1)</script></body></html>"
	out, err := FromBytes([]byte(html), ".html")
	if err != nil { t.Fatal(err) }
	if strings.Contains(out, "<") { t.Error("HTML tags not stripped") }
	if strings.Contains(out, "alert") { t.Error("script content not stripped") }
	if !strings.Contains(out, "remarkable") { t.Error("content missing") }
	t.Logf("html output:\n%s", out)
}

func TestUnsupportedExt(t *testing.T) {
	_, err := FromBytes([]byte("data"), ".xlsx")
	if err == nil { t.Error("expected error for unsupported ext") }
}

func TestCleanText(t *testing.T) {
	out, err := FromBytes([]byte("Hello\r\nWorld\n\n\n\nFoo"), ".txt")
	if err != nil { t.Fatal(err) }
	// 4 newlines should collapse to max 2 (one blank line)
	if strings.Count(out, "\n\n\n") > 0 { t.Error("triple+ newlines not collapsed") }
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "World") || !strings.Contains(out, "Foo") {
		t.Error("content missing")
	}
	t.Logf("clean text:\n%s", out)
}
