package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"duanjuapp/native/core"
)

func (app *webApp) signCover(id, address string) []byte {
	mac := hmac.New(sha256.New, app.coverKey[:])
	_, _ = mac.Write([]byte(id + "\x00" + address))
	return mac.Sum(nil)
}

func (app *webApp) coverURL(drama core.LibraryDrama) string {
	address, err := url.Parse(drama.Cover)
	if len(drama.Cover) > 4096 || err != nil || !core.ValidNativeCoverURL(address) {
		return ""
	}
	return "/cover?" + url.Values{
		"id": {drama.ID}, "url": {drama.Cover},
		"key": {hex.EncodeToString(app.signCover(drama.ID, drama.Cover))},
	}.Encode()
}

func (app *webApp) cover(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	id, address := values.Get("id"), values.Get("url")
	key, err := hex.DecodeString(values.Get("key"))
	if err != nil || !hmac.Equal(key, app.signCover(id, address)) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	image, mediaType, err := app.library.OpenCover(ctx, id, address)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer image.Close()
	if !strings.HasPrefix(mediaType, "image/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, max-age=600")
	http.ServeContent(w, r, "", time.Time{}, image)
}
