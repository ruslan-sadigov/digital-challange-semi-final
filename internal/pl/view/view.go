// Package view embeds the HTML templates (MVC "V") and static assets.
package view

import "embed"

//go:embed templates/*.html
var Templates embed.FS

//go:embed static
var Static embed.FS
