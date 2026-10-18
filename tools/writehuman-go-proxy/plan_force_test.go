package main

import (
	"encoding/base64"
	"net/http"
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

func TestChunkedChromeCookieRestoresSession(t *testing.T) {
	sess := `{"access_token":"header.payload.sig","refresh_token":"rtok","token_type":"bearer"}`
	packed := encodeSupabaseCookieValue(sess)
	mid := len(packed) / 2
	raw := `[{"name":"sb-hicfsbrfkzsxbwayibfm-auth-token.0","value":"` + packed[:mid] + `"},{"name":"sb-hicfsbrfkzsxbwayibfm-auth-token.1","value":"` + packed[mid:] + `"}]`
	_, ls := parseSession(raw)
	if accessTokenFromLS(ls) != "header.payload.sig" || refreshTokenFromLS(ls) != "rtok" {
		t.Fatalf("chunked cookie not restored: %#v", ls)
	}
	markDeadRefresh(9, "old-rt")
	if isDeadRefresh(9, "rtok") {
		t.Fatal("a newly pasted refresh token must be tried")
	}
	if !isDeadRefresh(9, "old-rt") {
		t.Fatal("the failed refresh token should stay blocked")
	}
	clearDeadRefresh(9)
}

func TestRequestPublicBaseKeepsForeignHostOff(t *testing.T) {
	cfg := Config{PublicHost: "writehuman.gt4rents.com", PublicScheme: "http"}
	req, err := http.NewRequest(http.MethodGet, "http://writehuman.gt4rents.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "writehuman.gt4rents.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := requestPublicBase(req, cfg); got != "https://writehuman.gt4rents.com" {
		t.Fatalf("same host = %s", got)
	}
	req.Host = "evil.example"
	if got := requestPublicBase(req, cfg); got != "https://writehuman.gt4rents.com" {
		t.Fatalf("foreign host = %s", got)
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

func TestWriteHumanBillUsesWordsAndImages(t *testing.T) {
	text := `{"text":"one two three","tone":"standard","enhanced":false,"numberOfVariations":5,"sequenceId":1}`
	key, label, n := writeHumanBill(http.MethodPost, "/api/humanize", []byte(text))
	if key != "words" || label != "Words" || n != 3 {
		t.Fatalf("humanize = %s %s %d", key, label, n)
	}
	key, _, n = writeHumanBill(http.MethodPost, "/api/detect", []byte(`{"text":"alpha beta"}`))
	if key != "words" || n != 2 {
		t.Fatalf("detect = %s %d", key, n)
	}
	key, label, n = writeHumanBill(http.MethodPost, "/api/detect-image", []byte("not-json"))
	if key != "images" || label != "Images" || n != 1 {
		t.Fatalf("image = %s %s %d", key, label, n)
	}
	if key, _, n = writeHumanBill(http.MethodPost, "/api/humanize/prepare", []byte(text)); key != "" || n != 0 {
		t.Fatalf("prepare should not bill: %s %d", key, n)
	}
	if key, _, n = writeHumanBill(http.MethodGet, "/api/usage", nil); key != "" || n != 0 {
		t.Fatalf("usage read should not bill: %s %d", key, n)
	}
	if key, _, n = writeHumanBill(http.MethodPost, "/api/humanize", []byte(`{"text":"hi"}`)); n != 1 {
		t.Fatalf("short text words = %d key=%s", n, key)
	}
	if key, _, n = writeHumanBill(http.MethodPost, "/api/humanize", nil); key != "" || n != 0 {
		t.Fatalf("humanize before the body is read must not bill: %s %d", key, n)
	}
	if !writeHumanWordPath(http.MethodPost, "/api/humanize") || !writeHumanWordPath(http.MethodPost, "/api/detect?x=1") {
		t.Fatal("humanize and detect are word paths")
	}
	if writeHumanWordPath(http.MethodPost, "/api/detect-image") {
		t.Fatal("image scans are not a word path")
	}
}

func TestBustChunkQuerySurvivesFlightSplit(t *testing.T) {
	in := []byte(`<script src="/_next/static/chunks/page.js?dpl=dpl_abc"></script><script>self.__next_f.push([1,"/_next/static/chunks/b1262d9a671fb619.js?d"])</script><script>self.__next_f.push([1,"pl=dpl_abc\",\"/_next/static/chunks/other.js?dpl=dpl_abc"])</script>`)
	out := string(bustChunkURLs(in))
	if strings.Contains(out, "?dpl=") {
		t.Fatalf("unbusted query left:\n%s", out)
	}
	joined := strings.ReplaceAll(out, `"])</script><script>self.__next_f.push([1,"`, "")
	if !strings.Contains(joined, "b1262d9a671fb619.js?whhost=2&dpl=dpl_abc") {
		t.Fatalf("split chunk query not joined:\n%s", out)
	}
	if strings.Count(out, "page.js?whhost=2&dpl=dpl_abc") != 1 || strings.Count(out, "other.js?whhost=2&dpl=dpl_abc") != 1 {
		t.Fatalf("contiguous urls changed: %s", out)
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
		"location.assign(dest)",
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
