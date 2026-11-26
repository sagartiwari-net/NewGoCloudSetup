package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Markhor Semrush gates the tool behind a short-lived mh_bp cookie.
// When it expires the upstream returns an HTML "One moment" page whose JS
// completes GET /__mh/b/c + POST /__mh/b/e (ECDSA P-256) and reloads.
// That page's script source contains the string "You have been signed out",
// so logout detectors must ignore it or they block the proof flow.

func markhorOneMomentPage(body []byte) bool {
	if len(body) == 0 || len(body) > 64<<10 {
		return false
	}
	text := strings.ToLower(string(body))
	if strings.Contains(text, `data-mh-bl="1"`) || strings.Contains(text, "data-mh-bl='1'") {
		return true
	}
	return strings.Contains(text, "<title>one moment</title>") ||
		strings.Contains(text, `id="mh-bl-wait"`)
}

func markhorSiteCode(toolOrigin string) string {
	u, err := url.Parse(strings.TrimSpace(toolOrigin))
	if err != nil || u.Host == "" {
		return "sm01"
	}
	host := strings.ToLower(u.Hostname())
	if i := strings.Index(host, "."); i > 0 {
		return host[:i]
	}
	return host
}

func warmMarkhorBrowserProof(r *http.Request, jar, toolOrigin string) string {
	toolOrigin = strings.TrimRight(strings.TrimSpace(toolOrigin), "/")
	if toolOrigin == "" || strings.TrimSpace(jar) == "" {
		return jar
	}
	site := markhorSiteCode(toolOrigin)
	chal, ok := markhorBLChallenge(r, jar, toolOrigin)
	if !ok {
		return jar
	}
	next, ok := markhorBLEnroll(r, jar, toolOrigin, site, chal)
	if !ok {
		log.Printf("[UP] markhor mh_bp enroll failed site=%s", site)
		return jar
	}
	return next
}

func markhorBLChallenge(r *http.Request, jar, toolOrigin string) (string, bool) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, toolOrigin+"/__mh/b/c", nil)
	if err != nil {
		return "", false
	}
	setBareUpstreamHeaders(req, r)
	if u, err := url.Parse(toolOrigin); err == nil {
		req.Host = u.Host
		req.Header.Set("Host", u.Host)
	}
	req.Header.Set("Cookie", jar)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", toolOrigin)
	req.Header.Set("Referer", toolOrigin+"/")
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		log.Printf("[UP] markhor /__mh/b/c failed: %v", err)
		return "", false
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	var out struct {
		C   string `json:"c"`
		Off bool   `json:"off"`
	}
	if json.Unmarshal(raw, &out) != nil || strings.TrimSpace(out.C) == "" || out.Off {
		log.Printf("[UP] markhor /__mh/b/c bad body=%s", truncateForLog(raw, 120))
		return "", false
	}
	return out.C, true
}

func markhorBLEnroll(r *http.Request, jar, toolOrigin, site, chal string) (string, bool) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return jar, false
	}
	x := markhorB64u(markhorPad32(priv.PublicKey.X.Bytes()))
	y := markhorB64u(markhorPad32(priv.PublicKey.Y.Bytes()))
	jwk := map[string]string{"kty": "EC", "crv": "P-256", "x": x, "y": y}
	sum := sha256.Sum256([]byte("P-256|" + x + "|" + y))
	kid := markhorB64u(sum[:])
	if len(kid) > 22 {
		kid = kid[:22]
	}
	msg := []byte("mhbl1|" + chal + "|" + site + "|" + kid)
	hash := sha256.Sum256(msg)
	rr, ss, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return jar, false
	}
	sig := markhorB64u(append(markhorPad32(rr.Bytes()), markhorPad32(ss.Bytes())...))
	body, _ := json.Marshal(map[string]any{"j": jwk, "c": chal, "s": sig})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, toolOrigin+"/__mh/b/e", bytes.NewReader(body))
	if err != nil {
		return jar, false
	}
	setBareUpstreamHeaders(req, r)
	if u, err := url.Parse(toolOrigin); err == nil {
		req.Host = u.Host
		req.Header.Set("Host", u.Host)
	}
	req.Header.Set("Cookie", jar)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", toolOrigin)
	req.Header.Set("Referer", toolOrigin+"/")
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		log.Printf("[UP] markhor /__mh/b/e failed: %v", err)
		return jar, false
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	jar = absorbSetCookies(jar, resp.Header)
	var out struct {
		OK  any `json:"ok"`
		Out any `json:"out"`
		Err any `json:"err"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Out != nil || out.Err != nil {
		log.Printf("[UP] markhor /__mh/b/e rejected body=%s", truncateForLog(raw, 160))
		return jar, false
	}
	if out.OK == nil && !strings.Contains(strings.ToLower(jar), "mh_bp=") {
		log.Printf("[UP] markhor /__mh/b/e no mh_bp body=%s", truncateForLog(raw, 160))
		return jar, false
	}
	log.Printf("[UP] markhor mh_bp enroll ok site=%s", site)
	return jar, true
}

func markhorB64u(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}

func markhorPad32(b []byte) []byte {
	if len(b) > 32 {
		return b[len(b)-32:]
	}
	if len(b) == 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func truncateForLog(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
