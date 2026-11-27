package tools

// 本文件实现 HTML → 可读文本的轻量转换，供 web_fetch 使用。
// 依赖 golang.org/x/net/html（项目已有依赖）做一次真正的 DOM 解析，
// 而不是正则去标签：这样 script/style 子树、实体转义、嵌套结构都能正确处理。

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// htmlBlockTags 块级元素：进入与离开时各补一个换行，保证段落/列表结构可读。
var htmlBlockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "aside": true,
	"header": true, "footer": true, "nav": true, "main": true, "figure": true,
	"figcaption": true, "address": true, "blockquote": true, "pre": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"ul": true, "ol": true, "dl": true, "dt": true, "dd": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// htmlSkipTags 需要连同子树一起丢弃的元素：脚本、样式、内嵌框架等非正文内容。
var htmlSkipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true,
	"svg": true, "canvas": true, "template": true, "head": true,
	"object": true, "embed": true,
}

// htmlToText 把 HTML 字节流转成 (标题, 正文) 两部分文本。
// 解析失败时退化为原始字符串，保证工具不会因为畸形 HTML 直接失败。
func htmlToText(raw []byte) (string, string) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return "", strings.TrimSpace(string(raw))
	}

	title := findHTMLTitle(doc)

	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(collapseHTMLWhitespace(n.Data))
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			if htmlSkipTags[tag] {
				return
			}
			switch tag {
			case "br":
				b.WriteString("\n")
				return
			case "hr":
				b.WriteString("\n---\n")
				return
			case "img":
				if alt := htmlAttr(n, "alt"); alt != "" {
					b.WriteString("[image: " + alt + "]")
				}
				return
			case "li":
				b.WriteString("\n- ")
			}
			// 块级元素先换行，再接标题前缀，避免 "# " 与标题文本之间被换行拆开
			if htmlBlockTags[tag] {
				b.WriteString("\n")
			}
			// 标题前缀：h1 → "# "，h2 → "## "，依此类推
			if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
				b.WriteString(strings.Repeat("#", int(tag[1]-'0')) + " ")
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			// 链接在文本后附上地址，便于模型引用来源；#锚点与 javascript: 无价值，跳过
			if tag == "a" {
				href := htmlAttr(n, "href")
				if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
					b.WriteString(" (" + href + ")")
				}
			}
			// 表格单元格之间用空格分隔，避免列内容粘在一起
			if tag == "td" || tag == "th" {
				b.WriteString(" ")
			}
			if htmlBlockTags[tag] {
				b.WriteString("\n")
			}
		}
	}
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}

	return title, normalizeHTMLText(b.String())
}

// findHTMLTitle 提取 <title> 文本；缺失或为空时返回空串。
func findHTMLTitle(n *html.Node) string {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "title") {
		return strings.TrimSpace(htmlNodeText(n))
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if title := findHTMLTitle(child); title != "" {
			return title
		}
	}
	return ""
}

// htmlNodeText 拼接节点子树内的全部文本。
func htmlNodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

// htmlAttr 读取元素属性值（不存在时返回空串）。
func htmlAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

// collapseHTMLWhitespace 把文本节点中的连续空白压成单个空格。
// 块级元素的换行由遍历逻辑显式插入，这里只处理行内空白。
func collapseHTMLWhitespace(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}
	// 结尾保留一个空格，避免相邻行内元素（如两段 <span>）拼接时粘连
	if pendingSpace && b.Len() > 0 {
		b.WriteByte(' ')
	}
	return b.String()
}

// normalizeHTMLText 逐行 Trim 并压缩空行，输出干净的多行文本。
func normalizeHTMLText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			// 最多保留一个空行，避免转换结果中出现大段空白
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
