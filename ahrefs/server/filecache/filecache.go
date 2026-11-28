// Package filecache is the shared on-disk Ahrefs metrics corpus.
//
// Lookup is O(1): sha256(key) → 1 of 256 shards → map get (never scan crores).
// Hot keys also sit in an in-memory LRU so repeat Hits stay ~sub-ms.
// Density: packed shards + zstd shared dictionary (trained on open).
// Eviction: least-recently-USED (last Get), not oldest write.
package filecache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
)

type Kind string

const (
	KindURL     Kind = "url"
	KindKeyword Kind = "keyword"

	// Hot in-memory entries for O(1) ~1ms Hits (discussed lookup path).
	hotCapDefault = 100_000
)

type Config struct {
	Dir      string
	URLCapGB int
	KWCapGB  int
	TTLHours int
	Enabled  bool
}

type Store struct {
	cfg Config
	// enc/dec kept as aliases to url codec for Enabled() checks
	enc, urlEnc, kwEnc       *zstd.Encoder
	dec, urlDec, kwDec       *zstd.Decoder
	plainDec                 *zstd.Decoder
	mu                       sync.Mutex // shard load/save + dirty + hot
	urlBytes                 atomic.Int64
	kwBytes                  atomic.Int64
	shards                   map[string]*memShard
	hot                      map[string]*Record // kind|hex → record (O(1) fast path)
	hotMax                   int
}

type memShard struct {
	kind  Kind
	id    string // 2-hex
	path  string
	ents  map[string]*Record // sha256 hex → record
	dirty bool
	size  int64 // last compressed size on disk
}

// Record is one compact metrics blob inside a shard.
type Record struct {
	FetchedAt int64           `json:"t"`           // unix sec — when upstream filled
	LastUsed  int64           `json:"u"`           // unix sec — last Get (LRU)
	Source    string          `json:"s,omitempty"` // u=upstream, r=refresh
	Payload   json.RawMessage `json:"p"`           // compact toolbar fields
}

func Open(cfg Config) (*Store, error) {
	if !cfg.Enabled || strings.TrimSpace(cfg.Dir) == "" {
		return &Store{cfg: cfg}, nil
	}
	if cfg.URLCapGB <= 0 {
		cfg.URLCapGB = 35
	}
	if cfg.KWCapGB <= 0 {
		cfg.KWCapGB = 15
	}
	if cfg.TTLHours <= 0 {
		cfg.TTLHours = 72
	}
	for _, sub := range []string{"urls", "keywords", "dict", "meta"} {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	s := &Store{
		cfg:    cfg,
		shards: make(map[string]*memShard),
		hot:    make(map[string]*Record),
		hotMax: hotCapDefault,
	}
	if err := s.ensureDicts(); err != nil {
		return nil, err
	}
	_ = os.WriteFile(filepath.Join(cfg.Dir, "meta", "VERSION"), []byte("3-packed-lru-zdict\n"), 0o644)
	s.recountSizes()
	go s.flushLoop()
	return s, nil
}

// flushLoop persists LastUsed (LRU) touches so hot keys survive restarts.
func (s *Store) flushLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		if !s.Enabled() {
			return
		}
		s.mu.Lock()
		for _, sh := range s.shards {
			if sh.dirty {
				_ = s.flushShardLocked(sh)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Store) Enabled() bool {
	return s != nil && s.cfg.Enabled && s.cfg.Dir != "" && s.enc != nil
}

func (s *Store) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	for _, sh := range s.shards {
		if sh.dirty {
			_ = s.flushShardLocked(sh)
		}
	}
	s.mu.Unlock()
	closeEnc := func(e *zstd.Encoder) {
		if e != nil {
			e.Close()
		}
	}
	closeDec := func(d *zstd.Decoder) {
		if d != nil {
			d.Close()
		}
	}
	closeEnc(s.urlEnc)
	closeEnc(s.kwEnc)
	closeDec(s.urlDec)
	closeDec(s.kwDec)
	closeDec(s.plainDec)
}

func NormalizeURL(raw string) string {
	t := strings.TrimSpace(strings.ToLower(raw))
	t = strings.TrimPrefix(t, "https://")
	t = strings.TrimPrefix(t, "http://")
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t = t[:i]
	}
	for strings.HasSuffix(t, "/") {
		t = strings.TrimSuffix(t, "/")
	}
	return t
}

func NormalizeKeyword(keyword, country string) (string, string) {
	kw := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(keyword))), " ")
	co := strings.ToLower(strings.TrimSpace(country))
	if co == "" {
		co = "us"
	}
	return kw, co
}

func KeywordKey(keyword, country string) string {
	kw, co := NormalizeKeyword(keyword, country)
	return co + "\x00" + kw
}

func hashKey(key string) (shardID, fullHex string) {
	sum := sha256.Sum256([]byte(key))
	fullHex = hex.EncodeToString(sum[:])
	return fullHex[:2], fullHex
}

func (s *Store) shardPath(kind Kind, shardID string) string {
	root := "urls"
	if kind == KindKeyword {
		root = "keywords"
	}
	return filepath.Join(s.cfg.Dir, root, shardID+".zst")
}

func (s *Store) shardMemKey(kind Kind, shardID string) string {
	if kind == KindKeyword {
		return "keywords/" + shardID
	}
	return "urls/" + shardID
}

func (s *Store) loadShardLocked(kind Kind, shardID string) (*memShard, error) {
	mk := s.shardMemKey(kind, shardID)
	if sh, ok := s.shards[mk]; ok {
		return sh, nil
	}
	path := s.shardPath(kind, shardID)
	sh := &memShard{kind: kind, id: shardID, path: path, ents: make(map[string]*Record)}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.shards[mk] = sh
			return sh, nil
		}
		return nil, err
	}
	sh.size = int64(len(raw))
	plain, err := s.decodeShardBytes(kind, raw)
	if err != nil {
		return nil, err
	}
	var ents map[string]*Record
	if err := json.Unmarshal(plain, &ents); err != nil {
		return nil, err
	}
	if ents == nil {
		ents = make(map[string]*Record)
	}
	sh.ents = ents
	s.shards[mk] = sh
	return sh, nil
}

func (s *Store) flushShardLocked(sh *memShard) error {
	if sh == nil {
		return nil
	}
	if len(sh.ents) == 0 {
		if sh.size > 0 {
			_ = os.Remove(sh.path)
			if sh.kind == KindKeyword {
				s.kwBytes.Add(-sh.size)
			} else {
				s.urlBytes.Add(-sh.size)
			}
			sh.size = 0
		}
		sh.dirty = false
		return nil
	}
	plain, err := json.Marshal(sh.ents)
	if err != nil {
		return err
	}
	enc := s.encoderFor(sh.kind)
	comp := enc.EncodeAll(plain, make([]byte, 0, len(plain)/2))
	if err := os.MkdirAll(filepath.Dir(sh.path), 0o755); err != nil {
		return err
	}
	tmp := sh.path + ".tmp"
	if err := os.WriteFile(tmp, comp, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, sh.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	delta := int64(len(comp)) - sh.size
	sh.size = int64(len(comp))
	sh.dirty = false
	if sh.kind == KindKeyword {
		s.kwBytes.Add(delta)
	} else {
		s.urlBytes.Add(delta)
	}
	return nil
}

func hotKey(kind Kind, hexed string) string {
	return string(kind) + "|" + hexed
}

func (s *Store) hotGetLocked(kind Kind, hexed string) (*Record, bool) {
	rec, ok := s.hot[hotKey(kind, hexed)]
	return rec, ok
}

func (s *Store) hotPutLocked(kind Kind, hexed string, rec *Record) {
	if s.hotMax <= 0 || rec == nil {
		return
	}
	if len(s.hot) >= s.hotMax {
		// Drop ~10% arbitrary entries (Go map order) — hot path stays O(1).
		n := s.hotMax / 10
		if n < 1 {
			n = 1
		}
		for k := range s.hot {
			delete(s.hot, k)
			n--
			if n <= 0 {
				break
			}
		}
	}
	cp := *rec
	cp.Payload = append(json.RawMessage(nil), rec.Payload...)
	s.hot[hotKey(kind, hexed)] = &cp
}

func (s *Store) hotDelLocked(kind Kind, hexed string) {
	delete(s.hot, hotKey(kind, hexed))
}

func (s *Store) Get(kind Kind, key string) (*Record, bool, error) {
	if !s.Enabled() || key == "" {
		return nil, false, nil
	}
	shardID, hexed := hashKey(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	ttl := time.Duration(s.cfg.TTLHours) * time.Hour
	now := time.Now().UTC().Unix()

	// O(1) hot path — no disk, no shard decompress (~sub-ms).
	if rec, ok := s.hotGetLocked(kind, hexed); ok && rec != nil {
		if ttl > 0 && rec.FetchedAt > 0 && time.Since(time.Unix(rec.FetchedAt, 0)) > ttl {
			s.hotDelLocked(kind, hexed)
		} else {
			rec.LastUsed = now
			// Mirror touch onto shard if already loaded (for disk LRU eviction).
			if sh, ok := s.shards[s.shardMemKey(kind, shardID)]; ok {
				if live, ok := sh.ents[hexed]; ok && live != nil {
					live.LastUsed = now
					sh.dirty = true
				}
			}
			out := *rec
			out.Payload = append(json.RawMessage(nil), rec.Payload...)
			return &out, true, nil
		}
	}

	// O(1) disk path: hash → exactly one shard file → map get (never scan corpus).
	sh, err := s.loadShardLocked(kind, shardID)
	if err != nil {
		return nil, false, err
	}
	rec, ok := sh.ents[hexed]
	if !ok || rec == nil {
		return nil, false, nil
	}
	if ttl > 0 && rec.FetchedAt > 0 {
		if time.Since(time.Unix(rec.FetchedAt, 0)) > ttl {
			return nil, false, nil
		}
	}
	if rec.LastUsed != now {
		rec.LastUsed = now
		sh.dirty = true
	}
	s.hotPutLocked(kind, hexed, rec)
	out := *rec
	out.Payload = append(json.RawMessage(nil), rec.Payload...)
	return &out, true, nil
}

// GetAhrefsBody returns Melange-shaped ["Ok",…] JSON expanded from compact storage.
// endpointHint: "tbGetUrlV3" → icon shape; otherwise Header/Keyword shape.
func (s *Store) GetAhrefsBody(kind Kind, key string, endpointHint ...string) ([]byte, bool, error) {
	rec, ok, err := s.Get(kind, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	hint := ""
	if len(endpointHint) > 0 {
		hint = endpointHint[0]
	}
	if kind == KindURL && hint == "tbGetUrlV3" {
		var c CompactURL
		if err := json.Unmarshal(rec.Payload, &c); err != nil {
			return nil, false, err
		}
		body, err := ExpandUrlIconFromCompact(c)
		return body, err == nil, err
	}
	body, err := ExpandToAhrefsBody(kind, rec.Payload)
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// PutAhrefsBody packs Melange ["Ok",…] into compact fields then stores zstd shard.
func (s *Store) PutAhrefsBody(kind Kind, key string, ahrefsBody []byte, source string) error {
	compact, err := PackAhrefsBody(kind, ahrefsBody)
	if err != nil {
		return err
	}
	return s.Put(kind, key, compact, source)
}

func (s *Store) Put(kind Kind, key string, compact []byte, source string) error {
	if !s.Enabled() || key == "" || len(compact) == 0 {
		return nil
	}
	src := "u"
	if source == "refresh" || source == "fresh" {
		src = "r"
	}
	now := time.Now().UTC().Unix()
	shardID, hexed := hashKey(key)

	s.mu.Lock()
	defer s.mu.Unlock()
	sh, err := s.loadShardLocked(kind, shardID)
	if err != nil {
		return err
	}
	rec := &Record{
		FetchedAt: now,
		LastUsed:  now, // new write counts as used
		Source:    src,
		Payload:   append(json.RawMessage(nil), compact...),
	}
	sh.ents[hexed] = rec
	sh.dirty = true
	s.hotPutLocked(kind, hexed, rec)
	if err := s.flushShardLocked(sh); err != nil {
		return err
	}
	s.enforceCapLocked(kind)
	return nil
}

func (s *Store) Stats() map[string]any {
	if s == nil || !s.Enabled() {
		return map[string]any{"enabled": false}
	}
	s.mu.Lock()
	hotN := len(s.hot)
	s.mu.Unlock()
	return map[string]any{
		"enabled":    true,
		"dir":        s.cfg.Dir,
		"format":     "packed-shard-v3-zdict",
		"lookup":     "O(1) hash→shard→map + hot LRU",
		"zstd_dict":  true,
		"hot_entries": hotN,
		"hot_cap":    s.hotMax,
		"url_bytes":  s.urlBytes.Load(),
		"kw_bytes":   s.kwBytes.Load(),
		"url_cap_gb": s.cfg.URLCapGB,
		"kw_cap_gb":  s.cfg.KWCapGB,
		"ttl_hours":  s.cfg.TTLHours,
	}
}

func (s *Store) recountSizes() {
	s.urlBytes.Store(dirSize(filepath.Join(s.cfg.Dir, "urls")))
	s.kwBytes.Store(dirSize(filepath.Join(s.cfg.Dir, "keywords")))
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		n += info.Size()
		return nil
	})
	return n
}

type evictCand struct {
	shardID string
	hex     string
	used    int64
	approx  int64 // approx compressed bytes for this entry
}

// enforceCapLocked drops least-recently-USED entries until under the pool cap.
// Caller must hold s.mu.
func (s *Store) enforceCapLocked(kind Kind) {
	capBytes := int64(s.cfg.URLCapGB) << 30
	cur := s.urlBytes.Load()
	root := filepath.Join(s.cfg.Dir, "urls")
	if kind == KindKeyword {
		capBytes = int64(s.cfg.KWCapGB) << 30
		cur = s.kwBytes.Load()
		root = filepath.Join(s.cfg.Dir, "keywords")
	}
	if cur <= capBytes {
		return
	}

	// Flush dirty so disk sizes match memory.
	for _, sh := range s.shards {
		if sh.kind == kind && sh.dirty {
			_ = s.flushShardLocked(sh)
		}
	}
	cur = s.urlBytes.Load()
	if kind == KindKeyword {
		cur = s.kwBytes.Load()
	}
	if cur <= capBytes {
		return
	}

	var cands []evictCand
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(p, ".zst") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(p), ".zst")
		if len(base) != 2 {
			return nil // skip legacy one-file-per-key leftovers
		}
		sh, err := s.loadShardLocked(kind, base)
		if err != nil || len(sh.ents) == 0 {
			return nil
		}
		approx := sh.size / int64(len(sh.ents))
		if approx < 32 {
			approx = 32
		}
		for hexed, rec := range sh.ents {
			used := rec.LastUsed
			if used == 0 {
				used = rec.FetchedAt
			}
			cands = append(cands, evictCand{shardID: base, hex: hexed, used: used, approx: approx})
		}
		return nil
	})
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].used == cands[j].used {
			return cands[i].hex < cands[j].hex
		}
		return cands[i].used < cands[j].used // oldest use first
	})

	dirtyShards := map[string]struct{}{}
	for _, c := range cands {
		if cur <= capBytes {
			break
		}
		sh, err := s.loadShardLocked(kind, c.shardID)
		if err != nil {
			continue
		}
		if _, ok := sh.ents[c.hex]; !ok {
			continue
		}
		delete(sh.ents, c.hex)
		s.hotDelLocked(kind, c.hex)
		sh.dirty = true
		dirtyShards[c.shardID] = struct{}{}
		cur -= c.approx
	}
	for id := range dirtyShards {
		sh, err := s.loadShardLocked(kind, id)
		if err != nil {
			continue
		}
		_ = s.flushShardLocked(sh)
	}
}

func (s *Store) String() string {
	if !s.Enabled() {
		return "filecache=off"
	}
	return fmt.Sprintf("filecache=on packed-lru-zdict dir=%s urls=%dMB/%dGB kw=%dMB/%dGB",
		s.cfg.Dir,
		s.urlBytes.Load()>>20, s.cfg.URLCapGB,
		s.kwBytes.Load()>>20, s.cfg.KWCapGB)
}

// pathFor is test helper — packed shard file for this key.
func (s *Store) pathFor(kind Kind, key string) string {
	id, _ := hashKey(key)
	return s.shardPath(kind, id)
}

// EntryCount returns in-memory+loaded entry count for tests (loads all shards).
func (s *Store) EntryCount(kind Kind) (int, error) {
	if !s.Enabled() {
		return 0, nil
	}
	root := filepath.Join(s.cfg.Dir, "urls")
	if kind == KindKeyword {
		root = filepath.Join(s.cfg.Dir, "keywords")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(p, ".zst") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(p), ".zst")
		if len(base) != 2 {
			return nil
		}
		sh, err := s.loadShardLocked(kind, base)
		if err != nil {
			return err
		}
		n += len(sh.ents)
		return nil
	})
	return n, err
}
