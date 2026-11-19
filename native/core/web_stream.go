package core

import (
	"context"
	"time"
)

func (stream *nativeStreamServer) openWeb(ctx context.Context, media ProviderMedia, previous string) (string, string, error) {
	address, token, session := stream.prepare(media)
	if len(media.CENCKey) > 0 {
		if err := stream.prepareCENC(ctx, session, media); err != nil {
			session.cancel()
			return "", "", err
		}
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if err := ctx.Err(); err != nil {
		session.cancel()
		return "", "", err
	}
	now := time.Now()
	for id := range stream.sessions {
		stream.webSessionLocked(id, now)
	}
	old := stream.sessions[previous]
	if old == nil && len(stream.sessions) >= nativeStreamCapacity {
		session.cancel()
		return "", "", ErrWebPlaybackBusy
	}
	if old != nil {
		stream.nativeCancel(old, "release")
		delete(stream.sessions, previous)
	}
	session.lastUsed = now
	stream.sessions[token] = session
	return address, token, nil
}

func (stream *nativeStreamServer) webPlaybackAvailable(token string) bool {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.webSessionLocked(token, time.Now()) != nil
}

func (stream *nativeStreamServer) webSessionLocked(token string, now time.Time) *nativeStreamSession {
	session := stream.sessions[token]
	if session == nil {
		return nil
	}
	if session.active == 0 && now.Sub(session.lastUsed) >= nativeStreamIdleTimeout {
		stream.nativeCancel(session, "idle_expired")
		delete(stream.sessions, token)
		return nil
	}
	if credentials := session.credentials; credentials != nil && !credentials.Expires.IsZero() && !now.Before(credentials.Expires) {
		// Let a running response finish even if new reads would need fresh credentials.
		if session.active == 0 {
			stream.nativeCancel(session, "credentials_expired")
			delete(stream.sessions, token)
		}
		return nil
	}
	return session
}
