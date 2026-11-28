package filecache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

const (
	urlDictName = "url-v1.zdict"
	kwDictName  = "keyword-v1.zdict"
	urlDictID = uint32(0x544d5501) // "TMU\x01"
	kwDictID  = uint32(0x544d4b01) // "TMK\x01"
)

func (s *Store) dictPath(name string) string {
	return filepath.Join(s.cfg.Dir, "dict", name)
}

// ensureDicts loads or trains zstd dictionaries so repetitive compact JSON packs tight from day one.
func (s *Store) ensureDicts() error {
	urlDict, err := s.loadOrTrainDict(urlDictName, urlDictID, seedURLSamples())
	if err != nil {
		return fmt.Errorf("url dict: %w", err)
	}
	kwDict, err := s.loadOrTrainDict(kwDictName, kwDictID, seedKWSamples())
	if err != nil {
		return fmt.Errorf("keyword dict: %w", err)
	}
	urlEnc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderDict(urlDict))
	if err != nil {
		return err
	}
	urlDec, err := zstd.NewReader(nil, zstd.WithDecoderDicts(urlDict))
	if err != nil {
		urlEnc.Close()
		return err
	}
	kwEnc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderDict(kwDict))
	if err != nil {
		urlEnc.Close()
		urlDec.Close()
		return err
	}
	kwDec, err := zstd.NewReader(nil, zstd.WithDecoderDicts(kwDict))
	if err != nil {
		urlEnc.Close()
		urlDec.Close()
		kwEnc.Close()
		return err
	}
	// Plain fallback decoder for any pre-dict shards left on disk.
	plainDec, err := zstd.NewReader(nil)
	if err != nil {
		urlEnc.Close()
		urlDec.Close()
		kwEnc.Close()
		kwDec.Close()
		return err
	}
	s.urlEnc, s.urlDec = urlEnc, urlDec
	s.kwEnc, s.kwDec = kwEnc, kwDec
	s.plainDec = plainDec
	s.enc, s.dec = urlEnc, urlDec // default aliases
	return nil
}

func (s *Store) loadOrTrainDict(name string, id uint32, samples [][]byte) ([]byte, error) {
	path := s.dictPath(name)
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 64 {
		return raw, nil
	}
	var history []byte
	for _, s := range samples {
		if len(history) > 64*1024 {
			break
		}
		history = append(history, s...)
	}
	dict, err := zstd.BuildDict(zstd.BuildDictOptions{
		ID:       id,
		Contents: samples,
		History:  history,
		Offsets:  [3]int{1, 4, 8},
		Level:    zstd.SpeedDefault,
	})
	if err != nil {
		return nil, err
	}
	if len(dict) < 8 {
		return nil, fmt.Errorf("dictionary of size %d < 8", len(dict))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, dict, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return dict, nil
}

func (s *Store) encoderFor(kind Kind) *zstd.Encoder {
	if kind == KindKeyword && s.kwEnc != nil {
		return s.kwEnc
	}
	if s.urlEnc != nil {
		return s.urlEnc
	}
	return s.enc
}

func (s *Store) decodeShardBytes(kind Kind, raw []byte) ([]byte, error) {
	dec := s.urlDec
	if kind == KindKeyword {
		dec = s.kwDec
	}
	if dec != nil {
		if plain, err := dec.DecodeAll(raw, nil); err == nil {
			return plain, nil
		}
	}
	// Fallback: shards written before dict, or wrong dict id.
	if s.plainDec != nil {
		return s.plainDec.DecodeAll(raw, nil)
	}
	if s.dec != nil {
		return s.dec.DecodeAll(raw, nil)
	}
	return nil, fmt.Errorf("no decoder")
}

func seedURLSamples() [][]byte {
	out := make([][]byte, 0, 512)
	for i := 0; i < 512; i++ {
		c := CompactURL{
			DR: float64(i%100) + 0.1, AR: float64(1000 + i), BL: float64(i * 10),
			RD: float64(i % 500), KW: float64(i % 200), ST: float64(i) * 0.3, TV: float64(i) * 1.1,
			UR: float64(i%100) / 2, PageBL: float64(i), PageRD: float64(i % 40),
			PageKW: float64(i % 30), PageST: float64(i) * 0.05, PageTV: float64(i) * 0.2,
			Words: float64(200 + i%800),
		}
		rec := Record{FetchedAt: 1700000000 + int64(i), LastUsed: 1700000000 + int64(i), Source: "u"}
		rec.Payload, _ = json.Marshal(c)
		b, _ := json.Marshal(map[string]*Record{fmt.Sprintf("%064x", i): &rec})
		out = append(out, b)
		pb, _ := json.Marshal(c)
		out = append(out, pb)
	}
	return out
}

func seedKWSamples() [][]byte {
	out := make([][]byte, 0, 512)
	for i := 0; i < 512; i++ {
		c := CompactKW{
			SV: float64(i * 10), KD: float64(i % 100), CPC: float64(i%50) / 10,
			TP: float64(i * 3), Clicks: float64(i * 2), GV: float64(i * 12),
		}
		rec := Record{FetchedAt: 1700000000 + int64(i), LastUsed: 1700000000 + int64(i), Source: "u"}
		rec.Payload, _ = json.Marshal(c)
		b, _ := json.Marshal(map[string]*Record{fmt.Sprintf("%064x", i): &rec})
		out = append(out, b)
		pb, _ := json.Marshal(c)
		out = append(out, pb)
	}
	return out
}
