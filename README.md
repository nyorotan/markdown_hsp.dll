🌐 **English** | [日本語](./README.ja.md)

# markdown_hsp.dll

A Markdown conversion DLL and HSP3 module for displaying Markdown files in an existing `webview2_hsp.dll`. It can be built and distributed separately from the WebView2 host DLL.

The renderer uses Marked.js to parse Markdown and DOMPurify to sanitize generated HTML. Both libraries are embedded in the DLL, so no additional runtime files or network access are required.

## Features

- Converts Markdown files to HTML and displays them in WebView2
- Builds a table of contents from headings (`#` through `######`) and displays it on the left
- Clicking a table-of-contents entry scrolls to the corresponding heading
- Individually collapses table-of-contents entries that contain child headings
- Embeds relative images specified with Markdown image syntax into the generated HTML
- Opens links to local Markdown files in the same directory without leaving the viewer
- Sanitizes generated HTML with DOMPurify
- Supports UTF-8 (including BOM) and the Windows system code page
- Limits input Markdown files to 32 MiB and generated HTML to 64 MiB

## Requirements

- Windows 10 / 11
- HSP 3.6 or later
- Microsoft Edge WebView2 Runtime
- `markdown_hsp.dll` and `webview2_hsp.dll` matching the HSP3 runtime architecture

Relative images specified with Markdown image syntax are embedded in the generated HTML as data URLs, so this module does not depend on a virtual-host mapping function in the WebView2 DLL. External image URLs are kept unchanged. For security, only local images within the Markdown file's directory are embedded. Links to local Markdown files in the same directory switch the displayed document in place. Other relative links resolve to `file:` URLs, but navigation to local files may be restricted by WebView2 security settings.

## Files

- `markdown_hsp.dll` — Markdown conversion DLL
- `hsp3markdown.as` — HSP3 module for calling the Markdown DLL
- `hsp3webview2.as` — WebView2 control module
- `markdown_dll/assets/markdown/` — Marked.js and DOMPurify embedded in the DLL
- `build_markdown.bat` — build script for the 32-bit DLL

## Usage

After creating a WebView2 instance, initialize the Markdown module and display a Markdown file:

```hsp
#include "hsp3webview2.as"
#include "hsp3markdown.as"

    wv2_markdown_init id
    if stat == 0 : dialog "Markdown DLL initialization failed.", 1

    wv2_show_markdown id, "README.md"
    if stat == 0 : dialog "Markdown file could not be displayed.", 1
```

`id` is the WebView2 instance ID returned by `wv2_create`. 

## Build

Building requires Go and a cgo-compatible MinGW-w64 GCC. Run `build_markdown.bat` from the repository root to produce the 32-bit `markdown_hsp.dll`. For standard 32-bit HSP3, use a 32-bit DLL matching the WebView2 DLL.

```cmd
build_markdown.bat
```

For 64-bit HSP3, use 64-bit MinGW-w64 GCC and build with `GOARCH=amd64` and the matching compiler. The architecture must match the HSP3 runtime and WebView2 DLL.

## License

This project is released under the `MIT License`. Feel free to use it for both commercial and non-commercial purposes.

Refer to the license files in `markdown_dll/assets/markdown/` for Marked.js and DOMPurify.

- **Copyright**: © 2026 nyorotan

## Version Information

- **Version**: v1.0.0
- **Author**: nyorotan
