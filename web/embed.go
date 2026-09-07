package web

import "embed"

// frontendFS 内嵌前端构建产物目录 web/frontend/dist，供单二进制直接提供 Web UI。
//
// 采用 all: 前缀以包含以 "." 或 "_" 开头的文件：仓库中提交了占位文件
// dist/.gitkeep，使得在未执行 `npm run build` 的全新克隆 / CI 环境下，
// go build 与 go test 仍能编译通过（此时仅提供占位内容）；一旦执行前端构建，
// 真实产物（index.html 与 assets/）会被一并嵌入并由二进制对外服务。
//
//go:embed all:frontend/dist
var frontendFS embed.FS
