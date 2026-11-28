package main

import (
	"database/sql"
	"net/url"
	"strings"
)

type portalRow struct {
	ID           int64
	ResellerID   int
	Name         string
	SiteURL      string
	CookieName   string
	CheckURL     string
	ProductIDs   string
	SharedSecret string
	Enabled      bool
}

func normalizeSiteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + strings.ToLower(u.Host)
}

func (s *server) findPortalBySiteURL(siteURL string) (*portalRow, error) {
	want := normalizeSiteURL(siteURL)
	if want == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`
SELECT id, reseller_id, name, site_url, cookie_name, check_url, product_ids, COALESCE(shared_secret,''), enabled
FROM ahrefs_bar_portals WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p portalRow
		var en int
		if err := rows.Scan(&p.ID, &p.ResellerID, &p.Name, &p.SiteURL, &p.CookieName, &p.CheckURL, &p.ProductIDs, &p.SharedSecret, &en); err != nil {
			return nil, err
		}
		p.Enabled = en == 1
		if normalizeSiteURL(p.SiteURL) == want {
			return &p, nil
		}
	}
	return nil, rows.Err()
}

func (s *server) listEnabledPortalsPublic() ([]map[string]any, error) {
	rows, err := s.db.Query(`
SELECT id, name, site_url, cookie_name FROM ahrefs_bar_portals WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, site, cookie string
		if err := rows.Scan(&id, &name, &site, &cookie); err != nil {
			return nil, err
		}
		if cookie == "" {
			cookie = "PHPSESSID"
		}
		out = append(out, map[string]any{
			"id":          id,
			"name":        name,
			"site_url":    site,
			"cookie_name": cookie,
		})
	}
	return out, rows.Err()
}

func (s *server) seedDevPortal() error {
	if !s.cfg.DevAuth {
		return nil
	}
	// Local "portal" = ToolsMandi panel API. Same cookie path as aMember later:
	// panel login sets panel_session → bar auths; panel logout clears it → bar parks.
	site := "http://127.0.0.1:18090"
	cookie := "panel_session"
	check := "http://" + s.cfg.Addr + "/v1/dev/tm-check"

	// Retire the old synthetic local.dev portal (was auto-minted; broke logout).
	_, _ = s.db.Exec(`UPDATE ahrefs_bar_portals SET enabled = 0 WHERE site_url = ?`, "https://local.dev")

	var id int64
	err := s.db.QueryRow(`SELECT id FROM ahrefs_bar_portals WHERE site_url = ? LIMIT 1`, site).Scan(&id)
	if err == nil {
		_, err = s.db.Exec(`
UPDATE ahrefs_bar_portals
SET check_url = ?, shared_secret = ?, cookie_name = ?, enabled = 1, name = 'Local Panel', reseller_id = 1
WHERE id = ?`, check, s.cfg.DevCheckSecret, cookie, id)
		return err
	}
	if err != sql.ErrNoRows {
		// table may lack shared_secret on very old db — ignore and insert attempt
	}
	_, err = s.db.Exec(`
INSERT INTO ahrefs_bar_portals (reseller_id, name, site_url, cookie_name, check_url, product_ids, shared_secret, enabled)
VALUES (1, 'Local Panel', ?, ?, ?, '', ?, 1)`, site, cookie, check, s.cfg.DevCheckSecret)
	return err
}
