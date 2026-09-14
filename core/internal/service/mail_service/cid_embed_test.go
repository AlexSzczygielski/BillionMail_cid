package mail_service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// minimalJPEG is the smallest valid JPEG byte sequence (SOI + EOI markers).
var minimalJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01}

func writeTestImage(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, data, 0644))
	return p
}

// TestRewriteHTMLImages_SelfHosted verifies that an <img> whose src points at
// the BillionMail instance is rewritten to a cid: reference and its bytes are
// returned as an inline attachment.
func TestRewriteHTMLImages_SelfHosted(t *testing.T) {
	dir := t.TempDir()
	writeTestImage(t, dir, "logo.jpg", minimalJPEG)

	html := `<html><body><img src="https://mail.example.com/logo.jpg" alt="logo"></body></html>`
	got, imgs, err := RewriteHTMLImages(html, "https://mail.example.com", dir)

	require.NoError(t, err)
	require.Len(t, imgs, 1)
	require.Contains(t, got, `src="cid:`)
	require.NotContains(t, got, "https://mail.example.com/logo.jpg")
	require.Equal(t, minimalJPEG, imgs[0].Data)
	require.Equal(t, "image/jpeg", imgs[0].MimeType)
	require.NotEmpty(t, imgs[0].ContentID)
}

// TestRewriteHTMLImages_External verifies that an <img> pointing at a
// third-party host is left completely unchanged.
func TestRewriteHTMLImages_External(t *testing.T) {
	html := `<html><body><img src="https://cdn.external.com/banner.png"></body></html>`
	got, imgs, err := RewriteHTMLImages(html, "https://mail.example.com", t.TempDir())

	require.NoError(t, err)
	require.Empty(t, imgs)
	require.Equal(t, html, got)
}

// TestRewriteHTMLImages_NoImages verifies that HTML with no <img> tags passes
// through unchanged.
func TestRewriteHTMLImages_NoImages(t *testing.T) {
	html := `<html><body><p>Hello World</p></body></html>`
	got, imgs, err := RewriteHTMLImages(html, "https://mail.example.com", t.TempDir())

	require.NoError(t, err)
	require.Empty(t, imgs)
	require.Equal(t, html, got)
}

// TestRewriteHTMLImages_PathTraversal verifies that a crafted URL whose derived
// local path escapes the serverRoot is rejected: the tag is left unchanged and
// no file is read or attached.
func TestRewriteHTMLImages_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	// Create a file one level above the serverRoot to make sure it exists on
	// disk (we want to confirm it is NOT read, not just "file missing").
	parent := filepath.Dir(dir)
	_ = os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret"), 0644)

	html := `<img src="https://mail.example.com/../../secret.txt">`
	got, imgs, err := RewriteHTMLImages(html, "https://mail.example.com", dir)

	require.NoError(t, err)
	require.Empty(t, imgs)
	require.Equal(t, html, got)
}

// TestMultipartRelated_Structure verifies that buildMultipartRelated produces a
// well-formed multipart/related message: correct boundary, Content-ID header,
// base64-encoded image data, and a closing boundary delimiter.
func TestMultipartRelated_Structure(t *testing.T) {
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	img := CIDImage{
		ContentID: "test-cid-123@billionmail",
		MimeType:  "image/png",
		Data:      imgData,
	}

	message := Message{
		Title:        "Test Subject",
		Content:      `<html><body><img src="cid:test-cid-123@billionmail"></body></html>`,
		Headers:      map[string]string{"Content-Type": `multipart/related; type="text/html"; boundary="fixedboundary"`},
		InlineImages: []CIDImage{img},
	}

	boundary := "fixedboundary"
	raw := buildMultipartRelated(message, "from@example.com", []string{"to@example.com"}, boundary)
	body := string(raw)

	require.Contains(t, body, "MIME-Version: 1.0")
	require.Contains(t, body, "Content-Type: multipart/related")
	require.Contains(t, body, "--"+boundary)
	require.Contains(t, body, "--"+boundary+"--")
	require.Contains(t, body, "Content-Type: text/html; charset=utf-8")
	require.Contains(t, body, "Content-Type: image/png")
	require.Contains(t, body, "Content-ID: <test-cid-123@billionmail>")
	require.Contains(t, body, "Content-Transfer-Encoding: base64")
	require.Contains(t, body, "Content-Disposition: inline")

	// Verify the image bytes appear as base64 (may be split across lines).
	encoded := base64.StdEncoding.EncodeToString(imgData)
	require.Contains(t, strings.ReplaceAll(body, "\r\n", ""), encoded)
}
