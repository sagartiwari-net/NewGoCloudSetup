package erank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"testing"
)

func TestLoginStoresSidEr(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/sanctum/csrf-cookie", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "csrf-1", Path: "/"})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-XSRF-TOKEN") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Email != "owner@example.com" || body.Password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sid_er", Value: "sid-1", Path: "/"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sid_er"); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]bool{"logged_in": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"logged_in": true})
	})
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	origin = srv.URL
	t.Cleanup(func() { origin = "https://members.erank.com" })

	cookies, err := Recipe{}.Login(context.Background(), "owner@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !hasName(cookies, "sid_er") {
		t.Fatal("sid_er missing")
	}
	ok, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !ok {
		t.Fatalf("check ok=%v err=%v", ok, err)
	}
	tool, err := Recipe{}.Capture(context.Background(), cookies, srv.URL)
	if err != nil || !hasName(tool, "sid_er") {
		t.Fatalf("capture err=%v", err)
	}
}

func TestForbiddenLoginShowsTheSiteMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`"Unauthorized to login."`))
			return
		}
		if r.URL.Path == "/sanctum/csrf-cookie" {
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "csrf-1", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	origin = srv.URL
	t.Cleanup(func() { origin = "https://members.erank.com" })

	_, err := Recipe{}.Login(context.Background(), "owner@example.com", "secret")
	if err == nil || !strings.Contains(err.Error(), "Unauthorized to login") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingSessionIsLoggedOut(t *testing.T) {
	ok, _, err := Recipe{}.Check(context.Background(), []automation.Cookie{{Name: "plain", Value: "x"}})
	if err != nil || ok {
		t.Fatalf("check ok=%v err=%v", ok, err)
	}
}
