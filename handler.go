// SPDX-FileCopyrightText: 2025 Sayantan Santra <sayantan.santra689@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package metatags

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/icholy/replace"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

// ServeHTTP implements caddyhttp.MiddlewareHandler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Get a buffer to hold the response body
	respBuf := bufPool.Get().(*bytes.Buffer)
	respBuf.Reset()
	defer bufPool.Put(respBuf)

	// Set up the response recorder
	shouldBuf := func(status int, headers http.Header) bool {
		return true
	}
	rec := caddyhttp.NewResponseRecorder(w, respBuf, shouldBuf)

	// Collect the response from upstream
	err := next.ServeHTTP(rec, r)
	if err != nil {
		return err
	}
	if !rec.Buffered() {
		// Skipped, no need to replace
		h.Logger.Debug("wikijs_metatags", zap.Int("Not buffering body. Skipping replacement.", rec.Status()))
		return nil
	}

	h.Logger.Debug("wikijs_metatags", zap.String("Default og:description", h.DefaultDescription))
	h.Logger.Debug("wikijs_metatags", zap.String("Default og:image", h.DefaultImagePath))
	h.Logger.Debug("wikijs_metatags", zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))

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

	descReplacement := h.DefaultDescription
	if h.InsertTopic {
		topic_matches := h.TopicRegexCompiled.FindStringSubmatch(req.URL.Path)
		if len(topic_matches) > 1 {
			descReplacement += " - " + cases.Title(language.English).String(topic_matches[1])
		}
	}
	h.Logger.Debug("wikijs_metatags", zap.String("Chosen og:description", descReplacement))

	tr_desc := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\"\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:description\" content=\""+descReplacement+"\">", ""),
	)

	img_matches := h.ImageRegexCompiled.FindStringSubmatch(string(res))
	imgReplacement := h.DefaultImagePath
	if len(img_matches) > 1 {
		imgReplacement = img_matches[1]
	}
	// Only add host if need to
	if strings.HasPrefix(imgReplacement, "/") {
		imgReplacement = "https://" + req.Host + imgReplacement
	}
	h.Logger.Debug("wikijs_metatags", zap.String("Chosen og:image", imgReplacement))
	tr_img := replace.String(
		reqReplacer.ReplaceKnown("<meta property=\"og:image\">", ""),
		reqReplacer.ReplaceKnown("<meta property=\"og:image\" content=\""+imgReplacement+"\">", ""),
	)

	transforms := []transform.Transformer{tr_desc, tr_img}

	return transform.Chain(transforms...)
}
