// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"bytes"
	"fmt"
	"net/http"
	// "regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/icholy/replace"
	"go.uber.org/zap"
	"golang.org/x/text/transform"
)

var k, s string
var logger *zap.Logger

func init() {
	k = "wikijs_meta_tags"
	s = "stage"
	logger, _ = zap.NewDevelopment()
	logger.Info(k, zap.String(s, "init"))
	caddy.RegisterModule(Handler{})
	httpcaddyfile.RegisterHandlerDirective("wikijs_meta_tags", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("wikijs_meta_tags", httpcaddyfile.After, "encode")
}

// Handler is an example; put your own type here.
type Handler struct {
	// Default values when nothing can be figured out from the page
	// The DefaultImageURL entry must be a direct link to an image
	DefaultDescription string `json:"default_description,omitempty"`
	DefaultImageURL    string `json:"default_image_url,omitempty"`
	// Only run replacements on responses that match against this ResponseMmatcher.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`
}

// Handler performs the necessary insertions
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "http.handlers.wikijs_meta_tags",
		New: func() caddy.Module {
			logger.Info(k, zap.String(s, "New"))
			return new(Handler)
		},
	}
}

// Provision implements caddy.Validator
func (h *Handler) Validate() error {
	// Make sure that the provided default image, if any,
	// is valid for og:image meta tags
	if h.DefaultImageURL != "" {
		url := h.DefaultImageURL
		startsHttps := strings.HasPrefix(url, "https://")
		endsJPG := strings.HasSuffix(url, ".jpg")
		endsPNG := strings.HasSuffix(url, ".png")
		endsGIF := strings.HasSuffix(url, ".gif")
		endsWEBP := strings.HasSuffix(url, ".webp")

		if !startsHttps || (!endsJPG && !endsPNG && !endsGIF && !endsWEBP) {
			return fmt.Errorf("Default Image URL is invalid. Only secure (https) jpg, png, gif, and webp links work.")
		}
	}

	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Get a buffer to hold the response body
	respBuf := bufPool.Get().(*bytes.Buffer)
	respBuf.Reset()
	defer bufPool.Put(respBuf)

	// Set up the response recorder
	shouldBuf := func(status int, headers http.Header) bool {
		if h.Matcher != nil {
			return h.Matcher.Match(status, headers)
		} else {
			// Always insert if no matcher is specified
			return true
		}
	}
	rec := caddyhttp.NewResponseRecorder(w, respBuf, shouldBuf)

	// Collect the response from upstream
	err := next.ServeHTTP(rec, r)
	if err != nil {
		return err
	}
	if !rec.Buffered() {
		// Skipped, no need to replace
		logger.Info(k, zap.Int("Not buffering body. Skipping replacement.", rec.Status()))
		return nil
	}

	logger.Info(k, zap.String("og:description", h.DefaultDescription))
	logger.Info(k, zap.String("og:image", h.DefaultImageURL))
	logger.Info(k, zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))

	tr := h.makeTransformer(r)
	// TODO: could potentially use transform.Append here with a pooled byte slice as buffer?
	result, _, err := transform.Bytes(tr, rec.Buffer().Bytes())
	if err != nil {
		return err
	}

	// make sure length is correct, otherwise bad things can happen
	if w.Header().Get("Content-Length") != "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(result)))
	}

	if status := rec.Status(); status > 0 {
		w.WriteHeader(status)
	}
	w.Write(result)

	return nil
}

func (h *Handler) makeTransformer(req *http.Request) transform.Transformer {
	reqReplacer := req.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	tr_desc := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\"\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\""+h.DefaultDescription+"\">", ""),
	)

	// pattern := "<img alt=\".*\" src=\"(.+\\.[jpg|png|webp])\">"
	// re := regexp.MustCompile(pattern)
	// matches := re.FindStringSubmatch(string(res))
	imgReplacement := h.DefaultImageURL
	// if matches[1] != "" {
	// 	imgReplacement = matches[1]
	// }
	tr_img := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\"\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\""+imgReplacement+"\">", ""),
	)

	transforms := []transform.Transformer{tr_desc, tr_img}

	return transform.Chain(transforms...)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler. Syntax:
//
//	default_description <desc>
//	default_image_url <url>
//
// 'url' has to be a secure (https) link to jpg, png, webp, or gif image.
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	h.DefaultDescription = ""
	h.DefaultImageURL = ""

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
			case "default_image_url":
				{
					if !d.NextArg() {
						return d.ArgErr()
					}
					h.DefaultImageURL = d.Val()
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

// Interface guards
var (
	_ caddy.Validator             = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
)
