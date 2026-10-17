package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNetLogHidesSecretsAndKeepsUsage(t *testing.T) {
	raw := []byte(`{"words":120,"remaining":80,"text":"one two three four","email":"a@b.com","access_token":"header.payload.sig"}`)
	out := summarizeNetBody(raw)
	if !strings.Contains(out, "words=120") || !strings.Contains(out, "remaining=80") {
		t.Fatalf("usage missing: %s", out)
	}
	if strings.Contains(out, "@") || strings.Contains(out, "header.payload") || strings.Contains(out, "one two") {
		t.Fatalf("secret or text leaked: %s", out)
	}
	if shouldLogWriteHumanNet("/_next/static/chunks/app.js") {
		t.Fatal("static js should not be logged")
	}
	if !shouldLogWriteHumanNet("/api/humanize") {
		t.Fatal("humanize should be logged")
	}
}

func TestSupabaseCookieIsBase64URL(t *testing.T) {
	packed := encodeSupabaseCookieValue(`{"access_token":"a+b/c","refresh_token":"r"}`)
	if !strings.HasPrefix(packed, "base64-") || strings.ContainsAny(packed, "+/=") {
		t.Fatalf("cookie encoding = %s", packed)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(packed, "base64-"))
	if err != nil || !strings.Contains(string(raw), "a+b/c") {
		t.Fatalf("decode %q err=%v", raw, err)
	}
	chunks := chunkSupabaseCookie("sb-writehuman-auth-token", strings.Repeat("a", 4000))
	if len(chunks) < 2 || chunks[0][0] != "sb-writehuman-auth-token.0" {
		t.Fatalf("chunks = %#v", chunks)
	}
	body := relaxWriteHumanSession("/api/auth/heartbeat", []byte(`{"data":{"sessionDead":true,"status":"session_limit"}}`))
	if strings.Contains(string(body), "session_limit") || strings.Contains(string(body), "true") {
		t.Fatalf("session still blocked: %s", body)
	}
}

func TestHostLockStaysOnProxy(t *testing.T) {
	in := []byte(`function s(){return"writehuman.ai"}`)
	out := string(rewriteHostLock(in))
	if strings.Contains(out, `return"writehuman.ai"`) || !strings.Contains(out, "return location.hostname") {
		t.Fatalf("host lock not rewritten: %s", out)
	}
	page := []byte(`<script>(function(){var d=['write','human'].join('')+'.ai';location.replace(o+location.pathname+location.search);})()</script>`)
	page = rewriteHostLock(page)
	if strings.Contains(string(page), "location.replace(o+location.pathname+location.search)") {
		t.Fatal("bootstrap redirect still present")
	}
	loc := rewriteOfficialLocation("https://writehuman.ai/pricing?x=1", "https://writehuman.gt4rents.com")
	if loc != "https://writehuman.gt4rents.com/pricing?x=1" {
		t.Fatalf("location = %s", loc)
	}
	cookie := stripCookieDomain("wh_anon_id=abc; Path=/; Domain=writehuman.ai; Secure")
	if strings.Contains(strings.ToLower(cookie), "domain=") || !strings.Contains(cookie, "wh_anon_id=abc") {
		t.Fatalf("cookie = %s", cookie)
	}
}

func TestChromeCookieArrayRestoresSupabaseSession(t *testing.T) {
	sess := `{"access_token":"header.payload.sig","refresh_token":"rtok","token_type":"bearer"}`
	raw := `[{"name":"sb-hicfsbrfkzsxbwayibfm-auth-token","value":"base64-` + base64.StdEncoding.EncodeToString([]byte(sess)) + `"},{"name":"_ga","value":"1"}]`
	header, ls := parseSession(raw)
	if !strings.Contains(header, "sb-hicfsbrfkzsxbwayibfm-auth-token=") {
		t.Fatal("cookie header missing auth cookie")
	}
	if accessTokenFromLS(ls) != "header.payload.sig" {
		t.Fatalf("access token = %q", accessTokenFromLS(ls))
	}
	if refreshTokenFromLS(ls) != "rtok" {
		t.Fatalf("refresh token = %q", refreshTokenFromLS(ls))
	}
}

func TestForceGPTZeroPremiumPlanProfile(t *testing.T) {
	in := []byte(`{"id":"u1","email":"a@b.com","plan":"Free"}`)
	out := forceGPTZeroPremiumPlan("/rest/v1/profiles?id=eq.u1", in)
	if !strings.Contains(string(out), "Premium (Annual)") {
		t.Fatalf("expected Premium plan, got %s", out)
	}
	if !strings.Contains(string(out), "full_plan") {
		t.Fatalf("expected full_plan injection, got %s", out)
	}
}

func TestForceGPTZeroPremiumPlanIgnoresScanJSON(t *testing.T) {
	in := []byte(`{"scanType":"plagiarism","plan":"Free"}`)
	out := forceGPTZeroPremiumPlan("/v3/scan", in)
	if string(out) != string(in) {
		t.Fatalf("scan JSON should be unchanged, got %s", out)
	}
}

func TestNormalizeGPTZeroPlan(t *testing.T) {
	if normalizeGPTZeroPlan("Free") != "Premium (Annual)" {
		t.Fatal("free")
	}
	if normalizeGPTZeroPlan("Premium") != "Premium (Annual)" {
		t.Fatal("premium")
	}
}

func TestStubSupabaseFeatures(t *testing.T) {
	st, body, ok := maybeStubGPTZeroFeatures("/extra-cdn-0/rest/v1/features", 401, []byte(`{"message":"JWT expired"}`))
	if !ok || st != 200 || !strings.Contains(string(body), "generally_available") {
		t.Fatalf("features stub failed st=%d ok=%v body=%s", st, ok, body)
	}
	st, body, ok = maybeStubGPTZeroFeatures("/extra-cdn-0/rest/v1/features_access?email=eq.a", 401, []byte(`{"message":"JWT expired"}`))
	if !ok || st != 200 || string(body) != "[]" {
		t.Fatalf("features_access stub failed st=%d ok=%v body=%s", st, ok, body)
	}
	if gptzeroLooksLoggedOut(401, "/extra-cdn-0/rest/v1/features", []byte(`{"message":"JWT expired"}`)) {
		t.Fatal("features 401 must not trigger account swap")
	}
}

func TestBuildInjectUpsellDismissAndLayoutGuards(t *testing.T) {
	s := buildInject(
		Config{PublicHost: "gptzero.gt4rents.com", HomePath: "/", BlockedPaths: []string{"/login"}},
		map[string]string{"plan": "Free"},
		"Free",
		"tok",
		"http://gptzero.gt4rents.com",
		"sagar",
	)
	need := []string{
		"dismissUpsells",
		"upgrade-plan-modal-close-button",
		`data-testid="upgrade-plan-modal"`,
		"gzIsLayoutChrome",
		"gzUnhideLayout",
		"Premium (Annual)",
		"trialSubscriptionUsed",
	}
	for _, n := range need {
		if !strings.Contains(s, n) {
			t.Fatalf("inject missing %q", n)
		}
	}
	// Free plan must be upgraded in inject PLAN constant.
	if strings.Contains(s, `var PLAN = "Free"`) {
		t.Fatal("PLAN should not stay Free")
	}
}
