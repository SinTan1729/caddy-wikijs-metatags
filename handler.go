// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"bytes"
	"fmt"
	"io"
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
	"go.uber.org/zap/zapcore"
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
	// The DefaultImageURL entry must be a direct link to an image
	DefaultDescription string `json:"default_description,omitempty"`
	DefaultImageURL    string `json:"default_image_url,omitempty"`
	// Only run replacements on responses that match against this ResponseMmatcher.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`

	logger *zap.Logger
}

// Handler performs the necessary insertions
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.wikijs_meta_tags",
		New: func() caddy.Module { return new(Handler) },
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

	// collect the response from upstream
	err := next.ServeHTTP(rec, r)
	if err != nil {
		return err
	}
	if !rec.Buffered() {
		// Skipped, no need to replace
		if c := h.logger.Check(zapcore.DebugLevel, "not buffering body; skipping replacement"); c != nil {
			c.Write(
				zap.Int("response_status", rec.Status()),
				zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}),
			)
		}
		return nil
	}

	if c := h.logger.Check(zapcore.DebugLevel, "buffered body replacement"); c != nil {
		c.Write(
			zap.Any("og:description", h.DefaultDescription),
			zap.Any("og:image", h.DefaultImageURL),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}),
		)
	}

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

	pattern := "<img alt=\".*\" src=\"(.+\\.[jpg|png|webp])\">"
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(string(res))
	imgReplacement := h.DefaultImageURL
	if matches[1] != "" {
		imgReplacement = matches[1]
	}
	tr_img := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\"\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\""+imgReplacement+"\">", ""),
	)

	transforms := []transform.Transformer{tr_desc, tr_img}

	return transform.Chain(transforms...)
}

// replaceWriter is used for streaming response body replacement. It
// ensures the Content-Length header is removed and writes to tw,
// which should be a transform writer that performs replacements.
type replaceWriter struct {
	*caddyhttp.ResponseWriterWrapper
	wroteHeader bool
	tw          io.WriteCloser
	tr          transform.Transformer
	handler     *Handler
}

func (fw *replaceWriter) WriteHeader(status int) {
	if fw.wroteHeader {
		return
	}
	fw.wroteHeader = true

	if fw.handler.Matcher == nil || fw.handler.Matcher.Match(status, fw.ResponseWriterWrapper.Header()) {
		// we don't know the length after replacements since
		// we're not buffering it all to find out
		fw.Header().Del("Content-Length")
		fw.tw = transform.NewWriter(fw.ResponseWriterWrapper, fw.tr)
	}

	fw.ResponseWriterWrapper.WriteHeader(status)
}

func (fw *replaceWriter) Write(d []byte) (int, error) {
	if !fw.wroteHeader {
		fw.WriteHeader(http.StatusOK)
	}

	if fw.tw != nil {
		return fw.tw.Write(d)
	} else {
		return fw.ResponseWriterWrapper.Write(d)
	}
}

func (fw *replaceWriter) Close() error {
	if fw.tw != nil {
		// Close if we have a transform writer, the underlying one does not need to be closed.
		return fw.tw.Close()
	}
	return nil
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

	_ http.ResponseWriter = (*replaceWriter)(nil)
)
