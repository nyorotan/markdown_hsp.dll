🌐 [English](./README.md) | **日本語**

# markdown_hsp.dll

HSP3の `webview2_hsp.dll` の画面にMarkdownファイルを表示するための変換DLLとHSP3用モジュールです。`webview2_hsp.dll` 本体のDLLとは別に配布・ビルドできます。

Markdownの解析にはMarked.js、生成HTMLのサニタイズにはDOMPurifyを使用します。両ライブラリはDLLに埋め込まれるため、実行時の追加ファイルやネット接続は不要です。

## 主な機能

- MarkdownファイルをHTMLに変換してWebView2へ表示
- 見出し（`#`〜`######`）から目次を生成し、左側に表示
- 目次のクリックで該当する見出しへ移動
- 子項目を持つ目次項目を個別に折りたたみ
- Markdown形式で指定された相対画像を読み込み、HTMLへ埋め込み
- DOMPurifyによる生成HTMLのサニタイズ
- UTF-8（BOM付き含む）およびWindowsのシステムコードページのMarkdownに対応
- 入力ファイルは最大32 MiB、生成HTMLは最大64 MiB

## 動作要件

- Windows 10 / 11
- HSP 3.6以降
- Microsoft Edge WebView2 Runtime
- HSP3ランタイムとビット数が一致する `markdown_hsp.dll` および `webview2_hsp.dll`

Markdown形式で指定された相対画像はMarkdown DLLがデータURLとしてHTMLへ埋め込むため、WebView2 DLLの仮想ホスト登録関数には依存しません。外部画像URLはそのまま使用します。ローカル画像はMarkdownファイルのフォルダー内にあるものだけを埋め込みます。相対リンクは `file:` URLになりますが、ローカルファイルへの遷移はWebView2のセキュリティ設定により制限される場合があります。

## ファイル

- `markdown_hsp.dll` — Markdown変換DLL
- `hsp3markdown.as` — HSP3からMarkdown変換DLLを呼び出すモジュール
- `hsp3webview2.as` — WebView2操作用モジュール
- `markdown_dll/assets/markdown/` — DLLに埋め込むMarked.jsとDOMPurify
- `build_markdown.bat` — 32bit版DLLのビルドスクリプト

## 使い方

WebView2の作成後、次のように初期化してMarkdownファイルを表示します。

```hsp
#include "hsp3webview2.as"
#include "hsp3markdown.as"

    wv2_markdown_init id
    if stat == 0 : dialog "Markdown DLL initialization failed.", 1

    wv2_show_markdown id, "docs\\guide.md"
    if stat == 0 : dialog "Markdown file could not be displayed.", 1
```

`id` は既存の `wv2_create` で取得したWebView2の識別用IDです。

## ビルド

Goとcgo対応のMinGW-w64 GCCが必要です。リポジトリのルートで `build_markdown.bat` を実行すると、32bit版の `markdown_hsp.dll` が生成されます。標準の32bit版HSP3では、WebView2 DLLと同じ32bit版を使用してください。

```cmd
build_markdown.bat
```

64bit版HSP3で使用する場合は、64bit用のMinGW-w64 GCCを用意し、`GOARCH=amd64` と対応するコンパイラーを指定してビルドしてください。生成したDLLのビット数はHSP3ランタイムおよびWebView2 DLLと一致させてください。

## ライセンス

プロジェクトはMIT Licenseで公開されています。Marked.jsとDOMPurifyのライセンス表記は `markdown_dll/assets/markdown/` 内の各ライセンスファイルを参照してください。
