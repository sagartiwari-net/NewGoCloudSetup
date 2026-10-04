package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// ChatGPT returns 429 when the page asks for the same conversation many times
// after a refresh. One upstream call is shared, and a short copy is reused.

type conversationEntry struct {
	body []byte
	at   time.Time
}

type conversationFlight struct {
	done chan struct{}
}

var (
	conversationMu    sync.Mutex
	conversationCache = map[string]conversationEntry{}
	conversationBusy  = map[string]*conversationFlight{}
)

func conversationKey(r *http.Request) string {
	if r.Method != http.MethodGet {
		return ""
	}
	path := r.URL.Path
	if path == "/backend-api/conversations" || strings.HasPrefix(path, "/backend-api/conversations/") || strings.HasPrefix(path, "/backend-api/conversation/") {
		if strings.Contains(path, "/stream_status") || strings.Contains(path, "/textdocs") {
			return ""
		}
		return path + "?" + r.URL.RawQuery
	}
	return ""
}

func serveConversation(w http.ResponseWriter, key string, maxAge time.Duration) bool {
	if key == "" {
		return false
	}
	conversationMu.Lock()
	entry, ok := conversationCache[key]
	conversationMu.Unlock()
	if !ok || time.Since(entry.at) > maxAge || len(entry.body) == 0 {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=5")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(entry.body)
	return true
}

func joinConversation(r *http.Request) (key string, leader bool, done <-chan struct{}) {
	key = conversationKey(r)
	if key == "" {
		return "", true, nil
	}
	conversationMu.Lock()
	if flight := conversationBusy[key]; flight != nil {
		conversationMu.Unlock()
		return key, false, flight.done
	}
	flight := &conversationFlight{done: make(chan struct{})}
	conversationBusy[key] = flight
	conversationMu.Unlock()
	return key, true, flight.done
}

var conversationListGate = make(chan struct{}, 1)

func isConversationListPath(path string) bool {
	p := strings.Split(path, "?")[0]
	return p == "/backend-api/conversations" || strings.HasSuffix(p, "/conversations")
}

func gateConversationList(path string) func() {
	if !isConversationListPath(path) {
		return func() {}
	}
	conversationListGate <- struct{}{}
	return func() { <-conversationListGate }
}

func finishConversation(key string) {
	if key == "" {
		return
	}
	conversationMu.Lock()
	flight := conversationBusy[key]
	delete(conversationBusy, key)
	conversationMu.Unlock()
	if flight != nil {
		close(flight.done)
	}
}

func storeConversation(r *http.Request, status int, body []byte) {
	key := conversationKey(r)
	if key == "" || status != http.StatusOK || len(body) == 0 || len(body) > 2*1024*1024 {
		return
	}
	copied := append([]byte(nil), body...)
	conversationMu.Lock()
	conversationCache[key] = conversationEntry{body: copied, at: time.Now()}
	conversationMu.Unlock()
}
