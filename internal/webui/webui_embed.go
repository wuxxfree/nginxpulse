//go:build embed

package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist/** dist_mobile/**
var embedded embed.FS

func AssetFS() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}

func MobileAssetFS() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist_mobile")
	if err != nil {
		return nil, false
	}
	return sub, true
}
