package zik

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"panel-api/automation"
	"strings"
	"testing"
	"time"
)

func TestLoginStoresAccessAndSession(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	payload, _ := json.Marshal(map[string]any{"exp": exp})
	token := "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Email != "owner@example.com" || body.Password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "SessionToken", Value: "sess-1", Path: "/"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]string{"accessToken": token},
		})
	}))
	defer srv.Close()
	loginURL = srv.URL
	t.Cleanup(func() { loginURL = "https://api.zikanalytics.com/auth/login" })

	cookies, err := Recipe{}.Login(context.Background(), "owner@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	ok, _, err := Recipe{}.Check(context.Background(), cookies)
	if err != nil || !ok {
		t.Fatalf("check ok=%v err=%v", ok, err)
	}
	raw, err := Recipe{}.Document(cookies)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"access"`) || !strings.Contains(raw, "SessionToken") {
		t.Fatalf("document missing session fields: %s", raw)
	}
	if strings.Contains(raw, `"name":"access"`) {
		t.Fatal("access token should stay in localStorage")
	}
}

func TestExpiredTokenNeedsLogin(t *testing.T) {
	exp := time.Now().Add(-time.Hour).Unix()
	payload, _ := json.Marshal(map[string]any{"exp": exp})
	token := "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
	ok, _, err := Recipe{}.Check(context.Background(), []automation.Cookie{
		{Name: "access", Value: token},
		{Name: "SessionToken", Value: "sess"},
	})
	if err != nil || ok {
		t.Fatalf("expired check ok=%v err=%v", ok, err)
	}
}
