// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
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

func init() {
	caddy.RegisterModule(Handler{})
	httpcaddyfile.RegisterHandlerDirective("wikijs_meta_tags", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("wikijs_meta_tags", httpcaddyfile.After, "encode")
}

// Handler is an example; put your own type here.
type Handler struct {
	// Default values when nothing can be figured out from the page
	// The DefaultImageURL entry must be a valid publicly accessible path
	// The hostname will be automatically added, so it should start with a slash (/)
	DefaultDescription string `json:"default_description,omitempty"`
	DefaultImagePath   string `json:"default_image_path,omitempty"`
	// Only run replacements on responses that match against this ResponseMmatcher.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`
	// Get a logger from the context
	Logger *zap.Logger
}

// Handler performs the necessary insertions
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "http.handlers.wikijs_meta_tags",
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
		h.Logger.Debug("wikijs_meta_tags", zap.Int("Not buffering body. Skipping replacement.", rec.Status()))
		return nil
	}

	h.Logger.Debug("wikijs_meta_tags", zap.String("Default og:description", h.DefaultDescription))
	h.Logger.Debug("wikijs_meta_tags", zap.String("Default og:image", h.DefaultImagePath))
	h.Logger.Debug("wikijs_meta_tags", zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))

	res := rec.Buffer().Bytes()
	tr := h.makeTransformer(res, r)
	// TODO: could potentially use transform.Append here with a pooled byte slice as buffer?
	result, _, err := transform.Bytes(tr, res)
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

func (h *Handler) makeTransformer(res []byte, req *http.Request) transform.Transformer {
	reqReplacer := req.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	tr_desc := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\"\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\""+h.DefaultDescription+"\">", ""),
	)

	pattern := "<img alt=\".*\" src=\"(.+\\.(?:jpg|png|webp|gif))\">"
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(string(res))
	imgReplacement := h.DefaultImagePath
	if len(matches) > 1 {
		imgReplacement = matches[1]
	}
	imgReplacement = "https://" + req.Host + imgReplacement
	h.Logger.Debug("wikijs_meta_tags", zap.String("Chosen og:image", imgReplacement))
	tr_img := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:image\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\""+imgReplacement+"\">", ""),
	)

	transforms := []transform.Transformer{tr_desc, tr_img}

	return transform.Chain(transforms...)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler. Syntax:
//
//	default_description <desc>
//	default_image_path <path>
//
// 'path' has to be a path to jpg, png, webp, or gif image.
// Hostname will be added before it automatically, so it should start with a slash (/)
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	h.DefaultDescription = ""
	h.DefaultImagePath = ""

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
	_ caddy.Provisioner           = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
)
