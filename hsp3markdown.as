#ifndef __HSP3_MARKDOWN__
#define __HSP3_MARKDOWN__

// Markdown DLLの初期化関数を登録します。
#uselib "markdown_hsp.dll"
#func global _MD_Init "MD_Init"
// MarkdownファイルをHTMLへ変換します。戻り値は成功時のHTML長、バッファ不足時は必要容量の負数、失敗時は0です。
#func global _MD_ConvertFile "MD_ConvertFile" sptr, var, int

#module hsp3markdown

// Markdown DLLを初期化します。
// 戻り値: 初期化成功時は1、失敗時は0。
#deffunc md_init
    _MD_Init
    return stat

// WebView2でMarkdownを表示するための初期化を行います。
// id: 対象のWebView2識別用ID。
// 戻り値: Markdown DLLの初期化成功時は1、失敗時は0。
#deffunc wv2_markdown_init int id
    md_init
    return stat

// Markdownファイルを変換し、指定したWebView2に表示します。
// id: 表示先のWebView2識別用ID。
// markdown_path: 表示するMarkdownファイルのパス。
// 戻り値: HTMLの設定に成功した場合は成功値、変換または表示に失敗した場合は0。
#deffunc wv2_show_markdown int id, str markdown_path
    sdim markdown_html, 1
    _MD_ConvertFile markdown_path, markdown_html, 1
    html_length = stat
    if html_length >= 0 : return 0

    html_capacity = 0 - html_length
    if html_capacity > 67108865 : return 0
    sdim markdown_html, html_capacity
    _MD_ConvertFile markdown_path, markdown_html, html_capacity
    if stat <= 0 : return 0

    wv2_set_html id, markdown_html
    return stat

#global

#endif
