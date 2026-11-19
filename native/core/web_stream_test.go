package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWebStreamCapacityAndReplacement(t *testing.T) {
	stream := newNativeStream(nil, "http://web.invalid")
	media := ProviderMedia{URL: "https://example.com/film.mp4"}
	var tokens []string
	for range nativeStreamCapacity {
		_, token, err := stream.openWeb(t.Context(), media, "")
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if _, _, err := stream.openWeb(t.Context(), media, ""); !errors.Is(err, ErrWebPlaybackBusy) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	for _, token := range tokens {
		if !stream.webPlaybackAvailable(token) {
			t.Fatalf("opening at capacity evicted session %q", token)
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := stream.openWeb(cancelled, media, tokens[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled replacement, got %v", err)
	}
	if !stream.webPlaybackAvailable(tokens[0]) {
		t.Fatal("cancelled replacement released the old stream")
	}
	old := stream.sessions[tokens[0]]
	_, replacement, err := stream.openWeb(t.Context(), media, tokens[0])
	if err != nil {
		t.Fatal(err)
	}
	if stream.webPlaybackAvailable(tokens[0]) || old.ctx.Err() == nil || !stream.webPlaybackAvailable(replacement) {
		t.Fatal("replacement did not close the old stream and publish the new one")
	}
}

func TestWebStreamIdleExpiry(t *testing.T) {
	stream := newNativeStream(nil, "http://web.invalid")
	_, token, err := stream.openWeb(t.Context(), ProviderMedia{URL: "https://example.com/film.mp4"}, "")
	if err != nil {
		t.Fatal(err)
	}
	session := stream.sessions[token]
	session.lastUsed = time.Now().Add(-nativeStreamIdleTimeout - time.Second)
	session.active++
	if !stream.webPlaybackAvailable(token) {
		t.Fatal("an in-flight response should prevent idle expiry")
	}
	session.active--
	if stream.webPlaybackAvailable(token) || session.ctx.Err() == nil {
		t.Fatal("an idle session should expire and be cancelled")
	}
}
