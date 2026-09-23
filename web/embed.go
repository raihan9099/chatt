// Package web 内嵌前端构建产物。
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
