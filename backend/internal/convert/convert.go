// Package convert extracts plain text from various document formats.
// Supported: .txt, .md, .html/.htm, .docx, .pdf
package convert

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ErrScannedPDF is returned when a PDF appears to contain no extractable text.
var ErrScannedPDF = fmt.Errorf("PDF appears to be scanned/image-based — no text could be extracted. Convert to a text-based PDF or paste the text directly")

// FromBytes converts document bytes to clean plain text based on the file extension.
// ext should include the dot, e.g. ".pdf", ".docx".
func FromBytes(data []byte, ext string) (string, error) {
	ext = strings.ToLower(ext)
	switch ext {
	case ".txt", ".text":
		return cleanText(string(data)), nil
	case ".md", ".markdown":
		return fromMarkdown(string(data)), nil
	case ".html", ".htm":
		return fromHTML(string(data)), nil
	case ".docx":
		return fromDOCX(data)
	case ".pdf":
		return fromPDF(data)
	default:
		return "", fmt.Errorf("unsupported file type %q — supported: .txt, .md, .html, .docx, .pdf", ext)
	}
}

// ── Plain text ────────────────────────────────────────────────────────────────

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	re := regexp.MustCompile(`\n{3,}`)
	s = re.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// ── Markdown ─────────────────────────────────────────────────────────────────

var (
	mdFencedCode  = regexp.MustCompile("(?ms)^```.*?```")
	mdInlineCode  = regexp.MustCompile("`[^`]+`")
	mdHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBoldItalic  = regexp.MustCompile(`\*{1,3}([^*]+)\*{1,3}`)
	mdLinkImg     = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdHtmlTag     = regexp.MustCompile(`<[^>]+>`)
	mdHorizontal  = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	mdBlockquote  = regexp.MustCompile(`(?m)^>\s?`)
	mdListBullet  = regexp.MustCompile(`(?m)^[\s]*[-*+]\s+`)
	mdListOrdered = regexp.MustCompile(`(?m)^[\s]*\d+\.\s+`)
)

func fromMarkdown(s string) string {
	s = mdFencedCode.ReplaceAllString(s, "")
	s = mdInlineCode.ReplaceAllString(s, "")
	s = mdHeading.ReplaceAllString(s, "")
	// Bold/italic: ***text***, **text**, *text*, __text__, _text_ → just text
	s = regexp.MustCompile(`\*{3}([^*\n]+)\*{3}`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\*{2}([^*\n]+)\*{2}`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\*([^*\n]+)\*`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`_{2}([^_\n]+)_{2}`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`_([^_\n]+)_`).ReplaceAllString(s, "$1")
	s = mdLinkImg.ReplaceAllString(s, "$1")
	s = mdHtmlTag.ReplaceAllString(s, "")
	s = mdHorizontal.ReplaceAllString(s, "")
	s = mdBlockquote.ReplaceAllString(s, "")
	s = mdListBullet.ReplaceAllString(s, "")
	s = mdListOrdered.ReplaceAllString(s, "")
	return cleanText(s)
}

// ── HTML ──────────────────────────────────────────────────────────────────────

func fromHTML(s string) string {
	// Remove script and style blocks entirely
	noScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	noStyle  := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	s = noScript.ReplaceAllString(s, "")
	s = noStyle.ReplaceAllString(s, "")

	// Replace block-level tags with newlines
	blockTags := regexp.MustCompile(`(?i)</?(?:p|div|br|h[1-6]|li|tr|td|th|blockquote|section|article|header|footer|nav|aside|main)[^>]*>`)
	s = blockTags.ReplaceAllString(s, "\n")

	// Strip remaining tags
	allTags := regexp.MustCompile(`<[^>]+>`)
	s = allTags.ReplaceAllString(s, "")

	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&mdash;", "—")
	s = strings.ReplaceAll(s, "&ndash;", "–")

	return cleanText(s)
}

// ── DOCX ──────────────────────────────────────────────────────────────────────
// A .docx file is a ZIP containing word/document.xml.
// We extract that XML and parse <w:t> (text run) elements.

type wbody struct {
	Paragraphs []wparagraph `xml:"body>p"`
}

type wparagraph struct {
	Runs []wrun `xml:"r"`
}

type wrun struct {
	Text string `xml:"t"`
}

func fromDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid .docx file: %w", err)
	}

	var docXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("could not open word/document.xml: %w", err)
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("no word/document.xml found — is this a valid .docx file?")
	}

	// Parse XML — strip namespace prefixes for simpler matching
	// Replace w: prefix tags to plain names for easier struct unmarshaling
	cleaned := bytes.ReplaceAll(docXML, []byte("w:"), []byte("w-"))
	// Use a streaming token decoder to grab all <w-t> text nodes
	dec := xml.NewDecoder(bytes.NewReader(cleaned))
	var sb strings.Builder
	inText := false
	prevWasParagraphEnd := false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // best effort on malformed XML
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "w-t" {
				inText = true
				prevWasParagraphEnd = false
			} else if t.Name.Local == "w-p" {
				if sb.Len() > 0 && !prevWasParagraphEnd {
					sb.WriteString("\n")
				}
			}
		case xml.EndElement:
			if t.Name.Local == "w-t" {
				inText = false
			} else if t.Name.Local == "w-p" {
				sb.WriteString("\n")
				prevWasParagraphEnd = true
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}

	result := cleanText(sb.String())
	if result == "" {
		return "", fmt.Errorf("no text extracted from .docx — the file may be empty or use unsupported formatting")
	}
	return result, nil
}

// ── PDF ───────────────────────────────────────────────────────────────────────
// Shells out to pdftotext (poppler-utils), which must be installed on the server.
// Install: apt-get install -y poppler-utils

func fromPDF(data []byte) (string, error) {
	// Write to a temp file (pdftotext requires a file path)
	tmp, err := os.CreateTemp("", "factcheck-*.pdf")
	if err != nil {
		return "", fmt.Errorf("could not create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	// pdftotext -layout -enc UTF-8 input.pdf - (stdout)
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", tmp.Name(), "-")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %w — %s", err, errBuf.String())
	}

	text := out.String()

	// Check if any real text was extracted (scanned PDFs yield whitespace only)
	meaningful := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, text)

	if len(meaningful) < 50 {
		return "", ErrScannedPDF
	}

	// Clean up common PDF artifacts
	text = removePDFArtifacts(text)
	return cleanText(text), nil
}

// removePDFArtifacts strips common noise from pdftotext output.
func removePDFArtifacts(s string) string {
	lines := strings.Split(s, "\n")
	var kept []string
	// Page break character (form feed)
	pageBreak := regexp.MustCompile(`\f`)
	// Lines that are purely page numbers or headers/footers (heuristic: short + only digits/dashes)
	pageNum := regexp.MustCompile(`^\s*[\d\-–—]+\s*$`)

	for _, line := range lines {
		line = pageBreak.ReplaceAllString(line, "")
		if pageNum.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// ExtFromFilename returns the lowercase extension including the dot.
func ExtFromFilename(name string) string {
	return strings.ToLower(filepath.Ext(name))
}
