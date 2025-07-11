// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"bytes"
	"sync"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// UnmarshalCaddyfile implements caddyfile.Unmarshaler. Syntax:
//
//	default_description <desc>
//	default_image_path <path>
//	insert_topic
//
// 'path' has to be a path to jpg, png, webp, or gif image.
// Hostname will be added before it automatically, so it should start with a slash (/)
// Insert topic tries to insert a topic name after the description, if enabled.
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	h.DefaultDescription = ""
	h.DefaultImagePath = ""
	h.InsertTopic = false

	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "default_description":
				{
					if !d.NextArg() {
						return d.ArgErr()
					}
					h.DefaultDescription = d.Val()
					if d.NextArg() {
						return d.ArgErr()
					}
				}
			case "default_image_path":
				{
					if !d.NextArg() {
						return d.ArgErr()
					}
					h.DefaultImagePath = d.Val()
					if d.NextArg() {
						return d.ArgErr()
					}
				}
			case "insert_topic":
				{
					if h.InsertTopic {
						return d.Err("Repeated args: insert_topic")
					}
					h.InsertTopic = true
					if d.NextArg() {
						return d.ArgErr()
					}
				}
			case "topic_regex":
				{
					if !d.NextArg() {
						return d.ArgErr()
					}
					h.TopicRegex = d.Val()
					if d.NextArg() {
						return d.ArgErr()
					}
				}
			default:
				return d.Err("Unknown argument" + d.Val())
			}
		}
	}

	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	handler := new(Handler)
	err := handler.UnmarshalCaddyfile(h.Dispenser)
	return handler, err
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}
