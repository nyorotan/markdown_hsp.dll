package main

/*
#include <windows.h>
*/
import "C"

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/sys/windows"
)

//go:embed assets/markdown/marked.min.js assets/markdown/purify.min.js
var markdownAssets embed.FS

const maxMarkdownBytes = 32 * 1024 * 1024
const maxHTMLBytes = 64 * 1024 * 1024

var (
	libraryOnce sync.Once
	markedJS    string
	purifyJS    string
	libraryErr  error
)

func loadLibraries() error {
	libraryOnce.Do(func() {
		marked, err := markdownAssets.ReadFile("assets/markdown/marked.min.js")
		if err != nil {
			libraryErr = fmt.Errorf("read embedded Marked.js: %w", err)
			return
		}
		purify, err := markdownAssets.ReadFile("assets/markdown/purify.min.js")
		if err != nil {
			libraryErr = fmt.Errorf("read embedded DOMPurify: %w", err)
			return
		}
		markedJS = string(marked)
		purifyJS = string(purify)
	})
	return libraryErr
}

func cStringToUTF8(value *C.char) (string, error) {
	if value == nil {
		return "", errors.New("path is nil")
	}
	raw := C.GoBytes(unsafe.Pointer(value), C.int(C.strlen(value)))
	if utf8.Valid(raw) {
		return string(raw), nil
	}
	if len(raw) == 0 {
		return "", nil
	}

	wideLen, err := windows.MultiByteToWideChar(0, 0, &raw[0], int32(len(raw)), nil, 0)
	if err != nil {
		return "", fmt.Errorf("decode path using the system code page: %w", err)
	}
	if wideLen <= 0 {
		return "", errors.New("decode path using the system code page: no characters decoded")
	}
	wide := make([]uint16, wideLen)
	if _, err := windows.MultiByteToWideChar(0, 0, &raw[0], int32(len(raw)), &wide[0], wideLen); err != nil {
		return "", fmt.Errorf("decode path using the system code page: %w", err)
	}
	return windows.UTF16ToString(wide), nil
}

func decodeMarkdown(data []byte) (string, error) {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	if len(data) == 0 {
		return "", nil
	}

	wideLen, err := windows.MultiByteToWideChar(0, 0, &data[0], int32(len(data)), nil, 0)
	if err != nil {
		return "", fmt.Errorf("decode Markdown using the system code page: %w", err)
	}
	if wideLen <= 0 {
		return "", errors.New("decode Markdown using the system code page: no characters decoded")
	}
	wide := make([]uint16, wideLen)
	if _, err := windows.MultiByteToWideChar(0, 0, &data[0], int32(len(data)), &wide[0], wideLen); err != nil {
		return "", fmt.Errorf("decode Markdown using the system code page: %w", err)
	}
	return windows.UTF16ToString(wide), nil
}

func markdownBaseURL(directory string) string {
	path := filepath.ToSlash(filepath.Clean(directory))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func markdownImageDataURLs(markdown, directory string) ([][]string, error) {
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve Markdown directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Markdown directory symlinks: %w", err)
	}

	images := make(map[string]string)
	document := goldmark.New().Parser().Parse(text.NewReader([]byte(markdown)))
	if err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		image, ok := node.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		destination := string(image.Destination)
		if destination == "" {
			return ast.WalkContinue, nil
		}
		parsed, err := url.Parse(destination)
		if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" {
			return ast.WalkContinue, nil
		}
		relativePath, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return ast.WalkContinue, nil
		}
		assetPath, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			return ast.WalkContinue, fmt.Errorf("resolve Markdown image path: %w", err)
		}
		relativeToRoot, err := filepath.Rel(root, assetPath)
		if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
			return ast.WalkContinue, nil
		}
		assetPath, err = filepath.EvalSymlinks(assetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ast.WalkContinue, nil
			}
			return ast.WalkContinue, fmt.Errorf("resolve Markdown image symlinks: %w", err)
		}
		relativeToRoot, err = filepath.Rel(root, assetPath)
		if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
			return ast.WalkContinue, nil
		}
		data, err := os.ReadFile(assetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ast.WalkContinue, nil
			}
			return ast.WalkContinue, fmt.Errorf("read Markdown image: %w", err)
		}
		contentType := mime.TypeByExtension(filepath.Ext(assetPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		images[destination] = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, fmt.Errorf("parse Markdown images: %w", err)
	}

	result := make([][]string, 0, len(images))
	for destination, dataURL := range images {
		result = append(result, []string{destination, dataURL})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i][0] < result[j][0]
	})
	return result, nil
}

func linkedMarkdownSources(markdown, currentPath string) (map[string]string, error) {
	root, err := filepath.EvalSymlinks(filepath.Dir(currentPath))
	if err != nil {
		return nil, fmt.Errorf("resolve Markdown directory symlinks: %w", err)
	}

	type source struct {
		markdown string
	}
	sources := []source{{markdown: markdown}}
	visited := map[string]string{currentPath: markdown}
	linked := make(map[string]string)
	totalBytes := len(markdown)

	for index := 0; index < len(sources); index++ {
		current := sources[index]
		document := goldmark.New().Parser().Parse(text.NewReader([]byte(current.markdown)))
		if err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			link, ok := node.(*ast.Link)
			if !ok {
				return ast.WalkContinue, nil
			}
			destination := string(link.Destination)
			parsed, err := url.Parse(destination)
			if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || !strings.EqualFold(filepath.Ext(parsed.Path), ".md") {
				return ast.WalkContinue, nil
			}
			relativePath, err := url.PathUnescape(parsed.Path)
			if err != nil {
				return ast.WalkContinue, nil
			}
			targetPath, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relativePath)))
			if err != nil {
				return ast.WalkContinue, fmt.Errorf("resolve linked Markdown path: %w", err)
			}
			relativeToRoot, err := filepath.Rel(root, targetPath)
			if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
				return ast.WalkContinue, nil
			}
			targetPath, err = filepath.EvalSymlinks(targetPath)
			if err != nil {
				if os.IsNotExist(err) {
					return ast.WalkContinue, nil
				}
				return ast.WalkContinue, fmt.Errorf("resolve linked Markdown symlinks: %w", err)
			}
			relativeToRoot, err = filepath.Rel(root, targetPath)
			if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
				return ast.WalkContinue, nil
			}
			if filepath.Dir(relativeToRoot) != "." {
				return ast.WalkContinue, nil
			}

			if linkedSource, ok := visited[targetPath]; ok {
				linked[destination] = linkedSource
				return ast.WalkContinue, nil
			}

			data, err := os.ReadFile(targetPath)
			if err != nil {
				if os.IsNotExist(err) {
					return ast.WalkContinue, nil
				}
				return ast.WalkContinue, fmt.Errorf("read linked Markdown file: %w", err)
			}
			totalBytes += len(data)
			if totalBytes > maxMarkdownBytes {
				return ast.WalkStop, fmt.Errorf("linked Markdown files exceed %d bytes", maxMarkdownBytes)
			}
			linkedMarkdown, err := decodeMarkdown(data)
			if err != nil {
				return ast.WalkStop, err
			}
			visited[targetPath] = linkedMarkdown
			sources = append(sources, source{markdown: linkedMarkdown})
			linked[destination] = linkedMarkdown
			return ast.WalkContinue, nil
		}); err != nil {
			return nil, fmt.Errorf("parse linked Markdown files: %w", err)
		}
	}
	return linked, nil
}

func convertMarkdownFile(path string) ([]byte, error) {
	if err := loadLibraries(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Markdown file: %w", err)
	}
	if len(data) > maxMarkdownBytes {
		return nil, fmt.Errorf("Markdown file exceeds %d bytes", maxMarkdownBytes)
	}
	markdown, err := decodeMarkdown(data)
	if err != nil {
		return nil, err
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve Markdown file path: %w", err)
	}
	markdownJSON, err := json.Marshal(markdown)
	if err != nil {
		return nil, fmt.Errorf("encode Markdown content: %w", err)
	}
	imageDataURLs, err := markdownImageDataURLs(markdown, filepath.Dir(absolutePath))
	if err != nil {
		return nil, err
	}
	linkedMarkdown, err := linkedMarkdownSources(markdown, absolutePath)
	if err != nil {
		return nil, err
	}
	for _, linkedSource := range linkedMarkdown {
		linkedImages, err := markdownImageDataURLs(linkedSource, filepath.Dir(absolutePath))
		if err != nil {
			return nil, err
		}
		imageDataURLs = append(imageDataURLs, linkedImages...)
	}
	imageDataURLMap := make(map[string]string, len(imageDataURLs))
	for _, image := range imageDataURLs {
		imageDataURLMap[image[0]] = image[1]
	}
	imageDataURLs = imageDataURLs[:0]
	for destination, dataURL := range imageDataURLMap {
		imageDataURLs = append(imageDataURLs, []string{destination, dataURL})
	}
	imageDataURLsJSON, err := json.Marshal(imageDataURLs)
	if err != nil {
		return nil, fmt.Errorf("encode Markdown images: %w", err)
	}
	linkedMarkdownJSON, err := json.Marshal(linkedMarkdown)
	if err != nil {
		return nil, fmt.Errorf("encode linked Markdown files: %w", err)
	}

	baseURL := html.EscapeString(markdownBaseURL(filepath.Dir(absolutePath)))
	title := html.EscapeString(filepath.Base(path))
	page := fmt.Sprintf(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<base href="%s">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
body { font-family: "Segoe UI", "Meiryo", sans-serif; line-height: 1.65; margin: 0; color: #24292f; }
img { max-width: 100%%; height: auto; }
pre { overflow: auto; padding: 1rem; background: #f6f8fa; border-radius: 6px; }
code { font-family: Consolas, "Courier New", monospace; }
blockquote { margin-left: 0; padding: 0 1rem; color: #57606a; border-left: 4px solid #d0d7de; }
table { border-collapse: collapse; } th, td { border: 1px solid #d0d7de; padding: .4rem .7rem; }
.markdown-layout { display: grid; grid-template-columns: minmax(200px, 260px) minmax(0, 960px); gap: 2rem; align-items: start; max-width: 1280px; margin: 2rem auto; padding: 0 1.5rem; }
.markdown-toc { position: sticky; top: 1rem; max-height: calc(100vh - 2rem); overflow: auto; padding: 1rem 1.25rem; background: #f6f8fa; border-radius: 6px; }
.markdown-toc[hidden] { display: none; }
.markdown-toc ul { margin: .5rem 0 0; padding-left: 0; }
.markdown-toc li { list-style: none; }
.markdown-toc li > button, .markdown-toc li > span { display: inline-block; width: 1.5rem; padding: 0; border: 0; background: transparent; color: #57606a; text-align: center; }
.markdown-toc li > button { cursor: pointer; }
.markdown-toc li > button:hover { color: #0969da; }
.markdown-toc li > ul { margin: 0 0 0 1rem; }
.markdown-toc a { color: #0969da; text-decoration: none; }
.markdown-toc a:hover { text-decoration: underline; }
@media (max-width: 760px) {
    .markdown-layout { grid-template-columns: minmax(0, 1fr); gap: 1rem; margin: 1rem auto; padding: 0 1rem; }
    .markdown-toc { position: static; max-height: 40vh; }
}
</style>
</head>
<body><div class="markdown-layout"><nav id="toc" class="markdown-toc" aria-label="目次" hidden></nav><main id="content"></main></div>
<script>%s</script>
<script>%s</script>
<script>
const markdown = %s;
const imageDataURLs = new Map(%s);
const content = document.getElementById("content");
const linkedMarkdown = %s;
const toc = document.getElementById("toc");
function renderMarkdown(source) {
    content.innerHTML = globalThis.DOMPurify.sanitize(globalThis.marked.parse(source));
    toc.replaceChildren();
    content.querySelectorAll("img").forEach(image => {
        const dataURL = imageDataURLs.get(image.getAttribute("src"));
        if (dataURL) {
            image.src = dataURL;
        }
    });
    const headings = content.querySelectorAll("h1, h2, h3, h4, h5, h6");
    if (headings.length === 0) {
        toc.hidden = true;
        return;
    }
    const usedIds = new Set(Array.from(content.querySelectorAll("[id]"), element => element.id));
    toc.hidden = false;
    const tocTitle = document.createElement("strong");
    tocTitle.textContent = "目次";
    toc.appendChild(tocTitle);
    const tocList = document.createElement("ul");
    const tocItems = [];
    const hierarchy = [];

    headings.forEach(heading => {
        if (!heading.id) {
            const baseId = heading.textContent
                .normalize("NFKC")
                .toLowerCase()
                .replace(/[^\p{L}\p{N}_-]+/gu, "-")
                .replace(/^-+|-+$/g, "") || "section";
            let id = baseId;
            let suffix = 2;
            while (usedIds.has(id)) {
                id = baseId + "-" + suffix;
                suffix++;
            }
            heading.id = id;
            usedIds.add(id);
        }

        const item = document.createElement("li");
        const level = Number(heading.tagName.slice(1));
        while (hierarchy.length > 0 && hierarchy[hierarchy.length - 1].level >= level) {
            hierarchy.pop();
        }
        const parent = hierarchy.length > 0 ? hierarchy[hierarchy.length - 1] : null;
        const list = parent ? (parent.childList || (parent.childList = document.createElement("ul"))) : tocList;
        if (parent && !parent.childList.parentNode) {
            parent.item.appendChild(parent.childList);
        }

        const link = document.createElement("a");
        link.href = "#" + heading.id;
        link.textContent = heading.textContent;
        link.addEventListener("click", event => {
            event.preventDefault();
            heading.scrollIntoView({ behavior: "smooth", block: "start" });
        });
        item.appendChild(link);
        list.appendChild(item);
        const tocItem = { level, item, link, childList: null };
        hierarchy.push(tocItem);
        tocItems.push(tocItem);
    });

    toc.appendChild(tocList);
    tocItems.forEach((tocItem, index) => {
        if (!tocItem.childList) {
            const placeholder = document.createElement("span");
            placeholder.className = "toc-toggle-placeholder";
            placeholder.setAttribute("aria-hidden", "true");
            tocItem.item.insertBefore(placeholder, tocItem.link);
            return;
        }

        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.className = "toc-toggle";
        toggle.textContent = "−";
        toggle.setAttribute("aria-expanded", "true");
        toggle.setAttribute("aria-label", "折りたたむ: " + tocItem.link.textContent);
        const baseChildrenId = "toc-children-" + index;
        let childrenId = baseChildrenId;
        let suffix = 2;
        while (usedIds.has(childrenId)) {
            childrenId = baseChildrenId + "-" + suffix;
            suffix++;
        }
        tocItem.childList.id = childrenId;
        usedIds.add(childrenId);
        toggle.setAttribute("aria-controls", childrenId);
        toggle.addEventListener("click", () => {
            const expanded = toggle.getAttribute("aria-expanded") === "true";
            toggle.setAttribute("aria-expanded", String(!expanded));
            toggle.setAttribute("aria-label", (expanded ? "展開する: " : "折りたたむ: ") + tocItem.link.textContent);
            toggle.textContent = expanded ? "+" : "−";
            tocItem.childList.hidden = expanded;
        });
        tocItem.item.insertBefore(toggle, tocItem.link);
    });
}
content.addEventListener("click", event => {
    const anchor = event.target instanceof Element ? event.target.closest("a") : null;
    if (!anchor || !content.contains(anchor)) {
        return;
    }
    const destination = anchor.getAttribute("href");
    if (Object.prototype.hasOwnProperty.call(linkedMarkdown, destination)) {
        event.preventDefault();
        const activeElement = document.activeElement;
        if (activeElement && content.contains(activeElement)) {
            activeElement.blur();
        }
        const source = linkedMarkdown[destination];
        window.setTimeout(() => renderMarkdown(source), 0);
    }
});
renderMarkdown(markdown);
</script>
</body>
</html>`, baseURL, title, markedJS, purifyJS, markdownJSON, imageDataURLsJSON, linkedMarkdownJSON)
	result := []byte(page)
	if len(result) > maxHTMLBytes {
		return nil, fmt.Errorf("converted HTML exceeds %d bytes", maxHTMLBytes)
	}
	return result, nil
}

//export MD_Init
func MD_Init() C.int {
	if err := loadLibraries(); err != nil {
		return 0
	}
	return 1
}

// MD_ConvertFile converts a Markdown file to UTF-8 HTML.
// It returns the HTML byte length on success, 0 on error, or the negative
// required buffer size (including the terminating NUL) when the buffer is small.
//
//export MD_ConvertFile
func MD_ConvertFile(path *C.char, outBuf *C.char, capacity C.int) C.int {
	if path == nil || capacity < 0 || int64(capacity) > maxHTMLBytes+1 {
		return 0
	}
	if capacity > 0 && outBuf == nil {
		return 0
	}

	filePath, err := cStringToUTF8(path)
	if err != nil {
		return 0
	}
	result, err := convertMarkdownFile(filePath)
	if err != nil {
		return 0
	}
	required := len(result) + 1
	if capacity < C.int(required) {
		return -C.int(required)
	}

	dst := unsafe.Slice((*byte)(unsafe.Pointer(outBuf)), int(capacity))
	copy(dst, result)
	dst[len(result)] = 0
	return C.int(len(result))
}

func main() {}
