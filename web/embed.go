package web

import "embed"

// frontendFS 内嵌前端构建产物目录 web/frontend/dist，供单二进制直接提供 Web UI。
//
// 采用 all: 前缀以包含以 "." 或 "_" 开头的文件：仓库中提交了占位文件
// dist/.gitkeep，使得在未执行 `npm run build` 的全新克隆 / CI 环境下，
// go build 与 go test 仍能编译通过（此时仅提供占位内容）；一旦执行前端构建，
// 真实产物（index.html 与 assets/）会被一并嵌入并由二进制对外服务。
//
// 注意：vite build 默认清空 dist 目录，dist/.gitkeep 会被删除；因此仓库同时
// 提交了 frontend/public/.gitkeep，构建时由 vite 复制到 dist 下自动重建，
// 保证占位文件不会因一次构建就从版本库中消失。请勿删除这两个文件。
//
//go:embed all:frontend/dist
var frontendFS embed.FS
