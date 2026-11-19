package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

const sessionCookie = "guoapp_web"
const sessionDuration = 12 * time.Hour

const loginWindow = time.Minute
const maxLoginAttempts = 8
const maxLoginClients = 4096

type loginFailures struct {
	count   int
	resetAt time.Time
}

type webAuth struct {
	password     [sha256.Size]byte
	secure       bool
	trustedProxy netip.Addr
	mu           sync.Mutex
	sessions     map[string]time.Time
	failures     map[netip.Addr]loginFailures
	cleanupAt    time.Time
}

func loginIP(r *http.Request, trustedProxy netip.Addr) netip.Addr {
	peer, _ := netip.ParseAddrPort(r.RemoteAddr)
	address := peer.Addr().WithZone("").Unmap()
	if trustedProxy.IsValid() && address == trustedProxy {
		if forwarded, err := netip.ParseAddr(r.Header.Get("X-Real-IP")); err == nil {
			return forwarded.WithZone("").Unmap()
		}
	}
	return address
}

func (auth *webAuth) authorized(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	return time.Now().Before(auth.sessions[cookie.Value])
}

func (auth *webAuth) login(w http.ResponseWriter, r *http.Request, password string) int {
	hash := sha256.Sum256([]byte(password))
	client := loginIP(r, auth.trustedProxy)
	now := time.Now()
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if !now.Before(auth.cleanupAt) {
		for address, failures := range auth.failures {
			if !now.Before(failures.resetAt) {
				delete(auth.failures, address)
			}
		}
		auth.cleanupAt = now.Add(loginWindow)
	}
	failures := auth.failures[client]
	if !now.Before(failures.resetAt) {
		failures = loginFailures{resetAt: now.Add(loginWindow)}
		delete(auth.failures, client)
	}
	limitUntil := time.Time{}
	if failures.count >= maxLoginAttempts {
		limitUntil = failures.resetAt
	} else if failures.count == 0 && len(auth.failures) >= maxLoginClients {
		limitUntil = auth.cleanupAt
	}
	if now.Before(limitUntil) {
		seconds := (limitUntil.Sub(now) + time.Second - 1) / time.Second
		w.Header().Set("Retry-After", strconv.Itoa(int(seconds)))
		return http.StatusTooManyRequests
	}
	if subtle.ConstantTimeCompare(hash[:], auth.password[:]) != 1 {
		failures.count++
		auth.failures[client] = failures
		return http.StatusUnauthorized
	}
	for token, expires := range auth.sessions {
		if !now.Before(expires) {
			delete(auth.sessions, token)
		}
	}
	if len(auth.sessions) >= 128 {
		return http.StatusServiceUnavailable
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return http.StatusServiceUnavailable
	}
	token := hex.EncodeToString(bytes[:])
	auth.sessions[token] = now.Add(sessionDuration)
	delete(auth.failures, client)
	auth.setCookie(w, token, int(sessionDuration.Seconds()))
	return http.StatusSeeOther
}

func (auth *webAuth) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		auth.mu.Lock()
		delete(auth.sessions, cookie.Value)
		auth.mu.Unlock()
	}
	auth.setCookie(w, "", -1)
}

func (auth *webAuth) setCookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: age,
		HttpOnly: true, Secure: auth.secure, SameSite: http.SameSiteStrictMode,
	})
}
