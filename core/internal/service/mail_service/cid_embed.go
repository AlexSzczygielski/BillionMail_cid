package mail_service

import (
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// CIDImage holds the data for one inline image attachment.
type CIDImage struct {
	ContentID string // without angle brackets, e.g. "abc@billionmail"
	MimeType  string // e.g. "image/jpeg"
	Data      []byte
}

// imageFileCache stores CIDImage values keyed by absolute file path so that the
// same image file is read from disk at most once per process lifetime, regardless
// of how many recipients share a campaign.
var imageFileCache sync.Map // map[string]CIDImage

var (
	imgTagRe = regexp.MustCompile(`(?is)<img\b[^>]*?>`)
	dqSrcRe  = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]*)"`)
	sqSrcRe  = regexp.MustCompile(`(?i)\bsrc\s*=\s*'([^']*)'`)
)

// RewriteHTMLImages scans html for <img src="…"> tags whose URL host matches
// the host of baseURL (images served by this BillionMail instance). For each
// such image it:
//   - validates that the derived local path stays inside serverRoot (path
//     traversal attempts are silently skipped and the tag is left unchanged),
//   - reads the file from disk on first encounter and caches the bytes keyed
//     by absolute path (cache is process-scoped via imageFileCache),
//   - replaces src="https://…" with src="cid:<contentID>" in the returned HTML,
//   - appends a CIDImage to the returned slice.
//
// Images pointing at other hosts are left completely unchanged.
// Empty or unparseable baseURL is a no-op (html returned as-is, nil slice).
// serverRoot is the filesystem directory GoFrame serves statically ("public/dist").
func RewriteHTMLImages(html, baseURL, serverRoot string) (string, []CIDImage, error) {
	if baseURL == "" {
		return html, nil, nil
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Host == "" {
		return html, nil, nil
	}
	ownHost := parsedBase.Host

	absRoot, err := filepath.Abs(serverRoot)
	if err != nil {
		return html, nil, nil
	}
	// Ensure the prefix check includes a trailing separator so that
	// "public/dist-evil/…" is not mistaken for "public/dist/…".
	rootPrefix := absRoot + string(filepath.Separator)

	// cidMap deduplicates within one HTML document: if the same image URL
	// appears in multiple <img> tags we reuse its content-ID and don't add
	// another copy to the images slice.
	cidMap := make(map[string]string) // imgURL → contentID
	var images []CIDImage

	result := imgTagRe.ReplaceAllStringFunc(html, func(imgTag string) string {
		imgURL, quoteChar := extractSrc(imgTag)
		if imgURL == "" {
			return imgTag
		}

		parsed, err := url.Parse(imgURL)
		if err != nil || parsed.Host != ownHost {
			return imgTag // external or unparseable — leave untouched
		}

		// Duplicate URL within this document — reuse existing content-ID.
		if cid, ok := cidMap[imgURL]; ok {
			return rewriteSrc(imgTag, imgURL, cid, quoteChar)
		}

		// Derive and validate the local file path.
		relPath := filepath.FromSlash(parsed.Path)
		localPath := filepath.Join(absRoot, relPath)
		absPath, err := filepath.Abs(localPath)
		if err != nil {
			return imgTag
		}
		// Reject any path that escapes the serverRoot (path traversal guard).
		if !strings.HasPrefix(absPath, rootPrefix) {
			return imgTag
		}

		// Cache hit: file was already read by an earlier campaign or tag.
		if cached, ok := imageFileCache.Load(absPath); ok {
			img := cached.(CIDImage)
			cidMap[imgURL] = img.ContentID
			images = append(images, img)
			return rewriteSrc(imgTag, imgURL, img.ContentID, quoteChar)
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			// File not found or unreadable — leave the tag unchanged.
			return imgTag
		}

		mt := mime.TypeByExtension(filepath.Ext(absPath))
		if mt == "" {
			mt = "application/octet-stream"
		}

		img := CIDImage{
			ContentID: uuid.New().String() + "@billionmail",
			MimeType:  mt,
			Data:      data,
		}
		imageFileCache.Store(absPath, img)
		cidMap[imgURL] = img.ContentID
		images = append(images, img)

		return rewriteSrc(imgTag, imgURL, img.ContentID, quoteChar)
	})

	return result, images, nil
}

// extractSrc returns the src attribute value from an img tag string and the
// quote character used (" or ').
func extractSrc(imgTag string) (value, quote string) {
	if m := dqSrcRe.FindStringSubmatch(imgTag); m != nil {
		return m[1], `"`
	}
	if m := sqSrcRe.FindStringSubmatch(imgTag); m != nil {
		return m[1], `'`
	}
	return "", ""
}

// rewriteSrc replaces exactly one occurrence of src=<quote><oldURL><quote>
// with src=<quote>cid:<contentID><quote> inside imgTag.
func rewriteSrc(imgTag, oldURL, contentID, quote string) string {
	old := "src=" + quote + oldURL + quote
	repl := "src=" + quote + "cid:" + contentID + quote
	return strings.Replace(imgTag, old, repl, 1)
}
