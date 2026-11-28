package filecache

import (
	"encoding/json"
	"strings"
)

// Compact URL/domain metrics — toolbar fields only (plan §2.1).
type CompactURL struct {
	DR           float64 `json:"dr,omitempty"`
	AR           float64 `json:"ar,omitempty"`
	BL           float64 `json:"bl,omitempty"`
	RD           float64 `json:"rd,omitempty"`
	KW           float64 `json:"kw,omitempty"`
	ST           float64 `json:"st,omitempty"`
	TV           float64 `json:"tv,omitempty"`
	PaidKW       float64 `json:"paid_kw,omitempty"`
	PaidAds      float64 `json:"paid_ads,omitempty"`
	PaidTraffic  float64 `json:"paid_traffic,omitempty"`
	PaidCost     float64 `json:"paid_cost,omitempty"`
	UR           float64 `json:"ur,omitempty"`
	PageBL       float64 `json:"p_bl,omitempty"`
	PageRD       float64 `json:"p_rd,omitempty"`
	PageKW       float64 `json:"p_kw,omitempty"`
	PageST       float64 `json:"p_st,omitempty"`
	PageTV       float64 `json:"p_tv,omitempty"`
	Words        float64 `json:"words,omitempty"`
	PagePaidKW   float64 `json:"p_paid_kw,omitempty"`
	PagePaidAds  float64 `json:"p_paid_ads,omitempty"`
	PagePaidTraf float64 `json:"p_paid_traffic,omitempty"`
	PagePaidCost float64 `json:"p_paid_cost,omitempty"`
}

// Compact keyword metrics — toolbar fields only (plan §2.2).
type CompactKW struct {
	SV     float64 `json:"sv,omitempty"`
	KD     float64 `json:"kd,omitempty"`
	CPC    float64 `json:"cpc,omitempty"`
	TP     float64 `json:"tp,omitempty"`
	Clicks float64 `json:"clicks,omitempty"`
	GV     float64 `json:"gv,omitempty"`
}

// PackAhrefsBody turns Melange ["Ok",…] JSON into compact storage bytes.
func PackAhrefsBody(kind Kind, ahrefsBody []byte) ([]byte, error) {
	trim := strings.TrimSpace(string(ahrefsBody))
	if !strings.HasPrefix(trim, "[") || !strings.Contains(trim, `"Ok"`) {
		return nil, errNotOK
	}
	var arr []any
	if err := json.Unmarshal(ahrefsBody, &arr); err != nil || len(arr) < 2 {
		return nil, errNotOK
	}
	obj, _ := arr[1].(map[string]any)
	if obj == nil {
		return nil, errNotOK
	}
	switch kind {
	case KindKeyword:
		return json.Marshal(compactFromKeywordStats(obj))
	default:
		return json.Marshal(compactFromHeader(obj))
	}
}

// ExpandToAhrefsBody rebuilds Melange response for Header or KeywordStats.
func ExpandToAhrefsBody(kind Kind, compact []byte) ([]byte, error) {
	switch kind {
	case KindKeyword:
		var c CompactKW
		if err := json.Unmarshal(compact, &c); err != nil {
			return nil, err
		}
		return json.Marshal([]any{"Ok", map[string]any{
			"stats": map[string]any{
				"volume":            map[string]any{"value": c.SV},
				"difficulty":        map[string]any{"value": c.KD},
				"cpc":               map[string]any{"value": c.CPC},
				"traffic_potential": map[string]any{"value": c.TP},
				"clicks":            map[string]any{"value": c.Clicks},
				"global_volume":     map[string]any{"value": c.GV},
			},
		}})
	default:
		var c CompactURL
		if err := json.Unmarshal(compact, &c); err != nil {
			return nil, err
		}
		return ExpandHeaderFromCompact(c)
	}
}

// ExpandHeaderFromCompact → tbGetHeaderV3 Ok body.
func ExpandHeaderFromCompact(c CompactURL) ([]byte, error) {
	return json.Marshal([]any{"Ok", map[string]any{
		"page": map[string]any{
			"id": 1, "label": "PAGE",
			"stats": map[string]any{
				"url_rating":    c.UR,
				"backlinks":     c.PageBL,
				"refdomains":    c.PageRD,
				"keywords":      c.PageKW,
				"traffic":       c.PageST,
				"value":         c.PageTV,
				"nr_words":      c.Words,
				"paid_keywords": c.PagePaidKW,
				"ads":           c.PagePaidAds,
				"paid_traffic":  c.PagePaidTraf,
				"paid_value":    c.PagePaidCost,
			},
		},
		"root_domain": map[string]any{
			"id": 2, "label": "ROOT DOMAIN",
			"stats": map[string]any{
				"domain_rating": c.DR,
				"ahrefs_rank":   c.AR,
				"backlinks":     c.BL,
				"refdomains":    c.RD,
				"keywords":      c.KW,
				"traffic":       c.ST,
				"value":         c.TV,
				"paid_keywords": c.PaidKW,
				"ads":           c.PaidAds,
				"paid_traffic":  c.PaidTraffic,
				"paid_value":    c.PaidCost,
			},
		},
	}})
}

// ExpandUrlIconFromCompact → tbGetUrlV3 Ok body (icon DR chip).
func ExpandUrlIconFromCompact(c CompactURL) ([]byte, error) {
	return json.Marshal([]any{"Ok", map[string]any{
		"stats": map[string]any{
			"domain_rating": c.DR,
			"ahrefs_rank":   int(c.AR),
			"url_rating":    int(c.UR),
		},
	}})
}

func compactFromHeader(obj map[string]any) CompactURL {
	var c CompactURL
	if root, ok := obj["root_domain"].(map[string]any); ok {
		if st, ok := root["stats"].(map[string]any); ok {
			c.DR = num(st["domain_rating"])
			c.AR = num(st["ahrefs_rank"])
			c.BL = num(st["backlinks"])
			c.RD = num(st["refdomains"])
			c.KW = num(st["keywords"])
			c.ST = num(st["traffic"])
			c.TV = num(st["value"])
			c.PaidKW = num(st["paid_keywords"])
			c.PaidAds = num(st["ads"])
			c.PaidTraffic = num(st["paid_traffic"])
			c.PaidCost = num(st["paid_value"])
		}
	}
	if page, ok := obj["page"].(map[string]any); ok {
		if st, ok := page["stats"].(map[string]any); ok {
			c.UR = num(st["url_rating"])
			c.PageBL = num(st["backlinks"])
			c.PageRD = num(st["refdomains"])
			c.PageKW = num(st["keywords"])
			c.PageST = num(st["traffic"])
			c.PageTV = num(st["value"])
			c.Words = num(st["nr_words"])
			c.PagePaidKW = num(st["paid_keywords"])
			c.PagePaidAds = num(st["ads"])
			c.PagePaidTraf = num(st["paid_traffic"])
			c.PagePaidCost = num(st["paid_value"])
		}
	}
	return c
}

func compactFromKeywordStats(obj map[string]any) CompactKW {
	var c CompactKW
	st, _ := obj["stats"].(map[string]any)
	if st == nil {
		return c
	}
	c.SV = nestedValue(st, "volume")
	c.KD = nestedValue(st, "difficulty")
	c.CPC = nestedValue(st, "cpc")
	c.TP = nestedValue(st, "traffic_potential")
	c.Clicks = nestedValue(st, "clicks")
	c.GV = nestedValue(st, "global_volume")
	return c
}

func nestedValue(st map[string]any, key string) float64 {
	if m, ok := st[key].(map[string]any); ok {
		return num(m["value"])
	}
	return num(st[key])
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

var errNotOK = errPack("not_ok_body")

type errPack string

func (e errPack) Error() string { return string(e) }
