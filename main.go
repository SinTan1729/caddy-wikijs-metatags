// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(Handler{})
	httpcaddyfile.RegisterHandlerDirective("wikijs_metatags", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("wikijs_metatags", httpcaddyfile.After, "encode")
}

// Handler is an example; put your own type here.
type Handler struct {
	// Default values when nothing can be figured out from the page
	// The DefaultImageURL entry must be a valid publicly accessible path
	// The hostname will be automatically added, so it should start with a slash (/)
	// If InsertTopic is true (it is false by default), an attempt will
	// be made to insert a topic after the description. It probably isn't worth it.
	DefaultDescription string `json:"default_description,omitempty"`
	DefaultImagePath   string `json:"default_image_path,omitempty"`
	InsertTopic        bool   `json:"insert_topic"`
	// Only run replacements on responses that match against this ResponseMmatcher.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`
	// Get a logger from the context
	Logger *zap.Logger `json:"logger,omitempty"`
}

// Handler performs the necessary insertions
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "http.handlers.wikijs_metatags",
		New: func() caddy.Module {
			return new(Handler)
		},
	}
}

// Provision implements caddy.Provisioner
func (h *Handler) Provision(ctx caddy.Context) error {
	h.Logger = ctx.Logger()
	// Make sure that the provided default image, if any,
	// is valid for og:image meta tags
	if h.DefaultImagePath != "" {
		url := h.DefaultImagePath
		startsSlash := strings.HasPrefix(url, "/")
		endsJPG := strings.HasSuffix(url, ".jpg")
		endsPNG := strings.HasSuffix(url, ".png")
		endsGIF := strings.HasSuffix(url, ".gif")
		endsWEBP := strings.HasSuffix(url, ".webp")

		if !startsSlash || (!endsJPG && !endsPNG && !endsGIF && !endsWEBP) {
			return fmt.Errorf("Default Image Path is invalid. Only jpg, png, gif, and webp links work.")
		}
	}

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
)
