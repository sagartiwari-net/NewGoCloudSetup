package filecache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPutGetRoundTripCompact(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, URLCapGB: 1, KWCapGB: 1, TTLHours: 72, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	header := []byte(`["Ok",{"page":{"id":1,"label":"PAGE","stats":{"url_rating":55,"backlinks":100,"refdomains":10,"keywords":5,"traffic":1.5,"value":9,"nr_words":0}},"root_domain":{"id":2,"label":"ROOT DOMAIN","stats":{"domain_rating":96,"ahrefs_rank":28,"backlinks":1000,"refdomains":50,"keywords":20,"traffic":99,"value":88}}}]`)
	key := NormalizeURL("https://Example.COM/Path/")
	if err := s.PutAhrefsBody(KindURL, key, header, "upstream"); err != nil {
		t.Fatal(err)
	}
	out, ok, err := s.GetAhrefsBody(KindURL, key)
	if err != nil || !ok {
		t.Fatalf("get ok=%v err=%v", ok, err)
	}
	var arr []any
	if err := json.Unmarshal(out, &arr); err != nil || arr[0] != "Ok" {
		t.Fatalf("bad expand %s", out)
	}
	obj := arr[1].(map[string]any)
	root := obj["root_domain"].(map[string]any)
	st := root["stats"].(map[string]any)
	if st["domain_rating"].(float64) != 96 {
		t.Fatalf("dr=%v", st["domain_rating"])
	}
	p := s.pathFor(KindURL, key)
	base := strings.TrimSuffix(filepath.Base(p), ".zst")
	if len(base) != 2 {
		t.Fatalf("expected 2-hex shard file, got %q path=%s", base, p)
	}
}

func TestKeywordCompact(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, Enabled: true, TTLHours: 72, URLCapGB: 1, KWCapGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := []byte(`["Ok",{"stats":{"volume":{"value":100},"difficulty":{"value":12},"cpc":{"value":0.4}}}]`)
	key := KeywordKey("Hello World", "US")
	if err := s.PutAhrefsBody(KindKeyword, key, body, "upstream"); err != nil {
		t.Fatal(err)
	}
	out, ok, err := s.GetAhrefsBody(KindKeyword, key)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	var arr []any
	_ = json.Unmarshal(out, &arr)
	st := arr[1].(map[string]any)["stats"].(map[string]any)
	if st["volume"].(map[string]any)["value"].(float64) != 100 {
		t.Fatalf("%v", st)
	}
}

func TestKeywordKey(t *testing.T) {
	k := KeywordKey("  Hello   World ", "US")
	if k != "us\x00hello world" {
		t.Fatalf("got %q", k)
	}
}

func TestManyKeysShareOneShardDense(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, Enabled: true, TTLHours: 72, URLCapGB: 1, KWCapGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := []byte(`["Ok",{"page":{"stats":{"url_rating":1}},"root_domain":{"stats":{"domain_rating":2}}}]`)
	for i := 0; i < 40; i++ {
		key := NormalizeURL(fmt.Sprintf("https://example.com/page-%d", i))
		if err := s.PutAhrefsBody(KindURL, key, body, "upstream"); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.EntryCount(KindURL)
	if err != nil || n != 40 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	// Packed: far under 40×4KiB one-file-per-key waste
	if sz := s.urlBytes.Load(); sz > 32*1024 {
		t.Fatalf("packed size too large: %d bytes for 40 entries", sz)
	}
}

func TestZstdDictFilesCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, Enabled: true, TTLHours: 72, URLCapGB: 1, KWCapGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"url-v1.zdict", "keyword-v1.zdict"} {
		st, err := os.Stat(filepath.Join(dir, "dict", name))
		if err != nil || st.Size() < 64 {
			t.Fatalf("dict %s missing/small: %v", name, err)
		}
	}
	body := []byte(`["Ok",{"page":{"stats":{"url_rating":1}},"root_domain":{"stats":{"domain_rating":2}}}]`)
	key := NormalizeURL("https://dict-test.example/")
	if err := s.PutAhrefsBody(KindURL, key, body, "upstream"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.GetAhrefsBody(KindURL, key); err != nil || !ok {
		t.Fatalf("get after dict put: ok=%v err=%v", ok, err)
	}
}

func TestO1HotLookupUnder1ms(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, Enabled: true, TTLHours: 72, URLCapGB: 1, KWCapGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := []byte(`["Ok",{"page":{"stats":{"url_rating":1}},"root_domain":{"stats":{"domain_rating":50}}}]`)
	key := NormalizeURL("https://fast.example/page")
	if err := s.PutAhrefsBody(KindURL, key, body, "upstream"); err != nil {
		t.Fatal(err)
	}
	// Warm hot cache
	if _, ok, _ := s.GetAhrefsBody(KindURL, key); !ok {
		t.Fatal("warm miss")
	}
	const rounds = 200
	start := time.Now()
	for i := 0; i < rounds; i++ {
		if _, ok, err := s.GetAhrefsBody(KindURL, key); err != nil || !ok {
			t.Fatal(err, ok)
		}
	}
	avg := time.Since(start) / rounds
	if avg > time.Millisecond {
		t.Fatalf("hot O(1) avg %v > 1ms", avg)
	}
	t.Logf("hot Get avg %v", avg)
}

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, Enabled: true, TTLHours: 72, URLCapGB: 1, KWCapGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	body := []byte(`["Ok",{"page":{"stats":{"url_rating":1}},"root_domain":{"stats":{"domain_rating":50}}}]`)
	cold := NormalizeURL("https://cold.example/")
	hot := NormalizeURL("https://hot.example/")
	if err := s.PutAhrefsBody(KindURL, cold, body, "upstream"); err != nil {
		t.Fatal(err)
	}
	if err := s.PutAhrefsBody(KindURL, hot, body, "upstream"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, ok, _ := s.GetAhrefsBody(KindURL, hot); !ok {
		t.Fatal("hot miss")
	}

	s.mu.Lock()
	keepMostRecentlyUsedLocked(s, KindURL, 1)
	s.mu.Unlock()

	if _, ok, _ := s.GetAhrefsBody(KindURL, hot); !ok {
		t.Fatal("hot should survive LRU")
	}
	if _, ok, _ := s.GetAhrefsBody(KindURL, cold); ok {
		t.Fatal("cold should be evicted")
	}
}

func keepMostRecentlyUsedLocked(s *Store, kind Kind, keep int) {
	root := filepath.Join(s.cfg.Dir, "urls")
	if kind == KindKeyword {
		root = filepath.Join(s.cfg.Dir, "keywords")
	}
	type pair struct {
		id, hex string
		used    int64
	}
	var all []pair
	entries, _ := filepath.Glob(filepath.Join(root, "*.zst"))
	for _, p := range entries {
		base := strings.TrimSuffix(filepath.Base(p), ".zst")
		if len(base) != 2 {
			continue
		}
		sh, err := s.loadShardLocked(kind, base)
		if err != nil {
			continue
		}
		for h, rec := range sh.ents {
			u := rec.LastUsed
			if u == 0 {
				u = rec.FetchedAt
			}
			all = append(all, pair{base, h, u})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].used < all[j].used })
	needDrop := len(all) - keep
	for i := 0; i < needDrop; i++ {
		sh, err := s.loadShardLocked(kind, all[i].id)
		if err != nil {
			continue
		}
		delete(sh.ents, all[i].hex)
		s.hotDelLocked(kind, all[i].hex)
		sh.dirty = true
		_ = s.flushShardLocked(sh)
	}
}
