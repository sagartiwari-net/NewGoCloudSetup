package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"strings"
)

type limitDef struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	ResetDays int    `json:"reset_days"`
}

type seedTool struct {
	slug     string
	name     string
	port     string
	category string
	limits   []limitDef
}

var credits = []limitDef{{Key: "credits", Label: "Credits", ResetDays: 1}}
var writeHumanLimits = []limitDef{
	{Key: "words", Label: "Words", ResetDays: 1},
	{Key: "images", Label: "Images", ResetDays: 1},
}
var creditsExports = []limitDef{
	{Key: "credits", Label: "Credits", ResetDays: 1},
	{Key: "exports", Label: "Exports", ResetDays: 7},
}

var localTools = []seedTool{
	{"canva", "Canva", "4501", "design", nil},
	{"jasper", "Jasper", "4511", "ai", credits},
	{"copywritely", "Copywritely", "4521", "ai", credits},
	{"ilovepdf", "iLovePDF", "4531", "tools", nil},
	{"piktochart", "Piktochart", "4541", "design", nil},
	{"storybase", "Storybase", "4551", "ai", credits},
	{"woorank", "WooRank", "4561", "seo", creditsExports},
	{"linkedin-learning", "LinkedIn Learning", "4571", "learning", nil},
	{"vistacreate", "VistaCreate", "4581", "design", nil},
	{"sellthetrend", "Sell The Trend", "4591", "commerce", credits},
	{"leonardo", "Leonardo", "4601", "ai", credits},
	{"placeit", "Placeit", "4611", "design", nil},
	{"creaitor", "Creaitor", "4621", "ai", credits},
	{"wordtune", "Wordtune", "4631", "ai", credits},
	{"epidemicsound", "Epidemic Sound", "4641", "audio", nil},
	{"merchinformer", "Merch Informer", "4651", "commerce", credits},
	{"seositecheckup", "SeoSite Checkup", "4661", "seo", creditsExports},
	{"flexclip", "FlexClip", "4671", "video", nil},
	{"glorify", "Glorify", "4681", "design", nil},
	{"zonguru", "ZonGuru", "4691", "commerce", credits},
	{"coursera", "Coursera", "4701", "learning", nil},
	{"searchatlas", "Search Atlas", "4711", "seo", creditsExports},
	{"screpy", "Screpy", "4721", "seo", credits},
	{"seobility", "Seobility", "4731", "seo", credits},
	{"copyspace", "Copyspace", "4741", "ai", credits},
	{"prezi", "Prezi", "4751", "design", nil},
	{"scite", "Scite", "4761", "learning", nil},
	{"shortform", "Shortform", "4771", "learning", nil},
	{"sketchgenius", "Sketch Genius", "4781", "design", nil},
	{"scribd", "Scribd", "4791", "learning", nil},
	{"seotesteronline", "SEO Tester Online", "4801", "seo", credits},
	{"rivalflow", "RivalFlow", "4811", "seo", credits},
	{"artistly", "Artistly", "4821", "ai", credits},
	{"grok", "Grok", "4831", "ai", credits},
	{"cramly", "Cramly", "4841", "learning", credits},
	{"educative", "Educative", "4851", "learning", nil},
	{"creattie", "Creattie", "4861", "design", nil},
	{"minvo", "Minvo", "4871", "video", nil},
	{"mojo", "Mojo", "4881", "video", nil},
	{"uncensoredchat", "Uncensored Chat", "4891", "ai", credits},
	{"pixlr", "Pixlr", "4901", "design", nil},
	{"grammarly", "Grammarly", "4911", "ai", credits},
	{"flaticon", "Flaticon", "4921", "design", nil},
	{"answerthepublic", "AnswerThePublic", "4931", "seo", creditsExports},
	{"spyfu", "SpyFu", "4941", "seo", creditsExports},
	{"kalodata", "Kalodata", "4951", "commerce", credits},
	{"ppspy", "PPSpy", "4961", "commerce", credits},
	{"storyblocks", "Storyblocks", "4971", "video", nil},
	{"syntx", "SYNTX", "4981", "ai", nil},
	{"fishaudio", "Fish Audio", "4991", "audio", credits},
	{"chatbotapp", "Chatbot App", "5001", "ai", credits},
	{"slidebean", "Slidebean", "5021", "design", nil},
	{"joggai", "JoggAI", "5031", "video", credits},
	{"videotoblog", "Video to Blog", "5041", "ai", credits},
	{"writecream", "WriteCream", "5051", "ai", credits},
	{"similarweb", "Similarweb", "5071", "seo", credits},
	{"branalyzer", "Branalyzer", "5081", "seo", credits},
	{"imgupscaler", "ImgUpscaler", "5091", "design", credits},
	{"perplexity", "Perplexity", "5101", "ai", credits},
	{"zebracat", "Zebracat", "5111", "video", credits},
	{"magnific", "Magnific", "5121", "design", credits},
	{"seobuddy", "SEO Buddy", "5131", "seo", credits},
	{"semrush", "Semrush", "5141", "seo", creditsExports},
	{"chatgpt", "ChatGPT", "5151", "ai", credits},
	{"selleramp", "SellerAmp", "5161", "commerce", credits},
	{"claude-ai", "Claude AI", "5171", "ai", credits},
	{"airbrush", "Airbrush", "5181", "design", nil},
	{"erank", "eRank", "5191", "commerce", credits},
	{"helium10", "Helium 10", "5201", "commerce", credits},
	{"junglescout", "Jungle Scout", "5211", "commerce", credits},
	{"helium-learning", "Helium Learning", "5221", "learning", nil},
	{"indexification", "Indexification", "5231", "seo", credits},
	{"closerscopy", "ClosersCopy", "5241", "ai", credits},
	{"zikaanalytics", "Zik Analytics", "5251", "commerce", credits},
	{"envato", "Envato", "5261", "design", nil},
	{"digen", "Digen", "5271", "video", credits},
	{"ubersuggest", "Ubersuggest", "5281", "seo", creditsExports},
	{"ahrefs", "Ahrefs", "5291", "seo", creditsExports},
	{"quillbot", "QuillBot", "5311", "ai", credits},
	{"writehuman", "WriteHuman", "5321", "ai", writeHumanLimits},
	{"pictory", "Pictory", "5331", "video", nil},
}

// Existing panel databases are not re-seeded. Insert QuillBot once so the
// Access tool list can open qllb.upays.in.
func ensureQuillbot(db *sql.DB) {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM websites WHERE name='QuillBot' OR domain IN ('qllb.upays.in', 'quillbot.gt4rents.com', 'quillbot.upays.in', '127.0.0.1:5311')`).Scan(&n)
	if n > 0 {
		return
	}
	limits, _ := json.Marshal(credits)
	var toolID int64
	err := db.QueryRow(`SELECT id FROM tools WHERE name=?`, "QuillBot").Scan(&toolID)
	if err != nil {
		res, insErr := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, "QuillBot", "ai", string(limits))
		if insErr != nil {
			log.Printf("quillbot tool: %v", insErr)
			return
		}
		toolID, _ = res.LastInsertId()
	}
	defaults, _ := json.Marshal(map[string]int{"credits": 100})
	res, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?,?,?,?,30,?,1)`, toolID, "QuillBot", "qllb.upays.in", "secret_quillbot", string(defaults))
	if err != nil {
		log.Printf("quillbot website: %v", err)
		return
	}
	websiteID, _ := res.LastInsertId()
	_, _ = db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, "QuillBot", `["5311"]`)
	_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, status, description) VALUES (?,?,?,?,?)`,
		websiteID, "QuillBot 1", "", "active", "Paste the Firebase session JSON into this cookie.")
	log.Printf("added QuillBot website %d at qllb.upays.in", websiteID)
}

// Existing panel databases are not re-seeded. Insert WriteHuman once so the
// Access tool list can open wrth.upays.in.
func ensureWriteHuman(db *sql.DB) {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM websites WHERE name='WriteHuman' OR domain IN ('wrth.upays.in', 'writehuman.gt4rents.com', 'writehuman.upays.in', '127.0.0.1:5321')`).Scan(&n)
	if n > 0 {
		return
	}
	limits, _ := json.Marshal(writeHumanLimits)
	var toolID int64
	err := db.QueryRow(`SELECT id FROM tools WHERE name=?`, "WriteHuman").Scan(&toolID)
	if err != nil {
		res, insErr := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, "WriteHuman", "ai", string(limits))
		if insErr != nil {
			log.Printf("writehuman tool: %v", insErr)
			return
		}
		toolID, _ = res.LastInsertId()
	}
	defaults, _ := json.Marshal(map[string]int{"words": 10000, "images": 50})
	res, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?,?,?,?,30,?,1)`, toolID, "WriteHuman", "wrth.upays.in", "secret_writehuman", string(defaults))
	if err != nil {
		log.Printf("writehuman website: %v", err)
		return
	}
	websiteID, _ := res.LastInsertId()
	_, _ = db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, "WriteHuman", `["writehuman"]`)
	_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, status, description) VALUES (?,?,?,?,?)`,
		websiteID, "WriteHuman 1", "", "active", "Paste the Chrome cookie array into this cookie.")
	log.Printf("added WriteHuman website %d at wrth.upays.in", websiteID)
}

// Existing panel databases are not re-seeded. Insert Pictory once so the
// Access tool list can open pctr.upays.in.
func ensurePictory(db *sql.DB) {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM websites WHERE name='Pictory' OR domain IN ('pctr.upays.in', 'pictory.gt4rents.com', 'pictory.upays.in', '127.0.0.1:5331')`).Scan(&n)
	if n > 0 {
		return
	}
	var toolID int64
	err := db.QueryRow(`SELECT id FROM tools WHERE name=?`, "Pictory").Scan(&toolID)
	if err != nil {
		res, insErr := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES (?,?,?)`, "Pictory", "video", "[]")
		if insErr != nil {
			log.Printf("pictory tool: %v", insErr)
			return
		}
		toolID, _ = res.LastInsertId()
	}
	res, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?,?,?,?,30,?,1)`, toolID, "Pictory", "pctr.upays.in", "secret_pictory", "{}")
	if err != nil {
		log.Printf("pictory website: %v", err)
		return
	}
	websiteID, _ := res.LastInsertId()
	_, _ = db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, "Pictory", `["pictory"]`)
	_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, status, description) VALUES (?,?,?,?,?)`,
		websiteID, "Pictory 1", "", "active", "Paste a localStorage JSON object, or a referer export that contains storage.localStorage. A Chrome cookie array also works.")
	log.Printf("added Pictory website %d at pctr.upays.in", websiteID)
}

func ensureVeo3(db *sql.DB) {
	_ = db
}

func defaultSeedLimit(key string) int {
	switch key {
	case "exports":
		return 20
	case "words":
		return 10000
	case "images":
		return 50
	default:
		return 100
	}
}

// The live WriteHuman row was inserted with a placeholder credits meter.
// Add the two real meters once, and leave any number already saved in the panel.
func ensureWriteHumanLimits(db *sql.DB) {
	rawDefs, _ := json.Marshal(writeHumanLimits)
	var toolID int
	var toolRaw string
	err := db.QueryRow(`SELECT id, COALESCE(limits_json, '') FROM tools WHERE name=?`, "WriteHuman").Scan(&toolID, &toolRaw)
	if err != nil {
		return
	}
	if !strings.Contains(toolRaw, `"words"`) {
		_, _ = db.Exec(`UPDATE tools SET limits_json=? WHERE id=?`, string(rawDefs), toolID)
	}
	rows, err := db.Query(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE tool_id=?`, toolID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var raw string
		if rows.Scan(&id, &raw) != nil {
			continue
		}
		defaults := map[string]int{}
		_ = json.Unmarshal([]byte(raw), &defaults)
		changed := false
		if _, ok := defaults["words"]; !ok {
			defaults["words"] = 10000
			delete(defaults, "credits")
			changed = true
		}
		if _, ok := defaults["images"]; !ok {
			defaults["images"] = 50
			changed = true
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(defaults)
		if err != nil {
			continue
		}
		_, _ = db.Exec(`UPDATE websites SET default_limits_json=? WHERE id=?`, string(encoded), id)
		log.Printf("WriteHuman website %d default limits set to 10000 words and 50 images", id)
	}
}

func ensureResellers(db *sql.DB) {
	type spec struct {
		name  string
		chats []string
		tools []string
	}
	groups := []spec{
		{"ToolWaly", []string{"ToolWaly 1", "ToolWaly 2", "ToolWaly 3"}, nil},
		{"SemrushToolz", []string{"SemrushToolz"}, []string{"Semrush", "ChatGPT", "Envato", "Ubersuggest", "Helium 10"}},
		{"SeoGroupBuy", []string{"SeoGroupBuy 1", "SeoGroupBuy 2"}, []string{"Semrush"}},
		{"AmzPremiumToolz", []string{"AmzPremiumToolz"}, []string{"Helium 10", "ChatGPT", "Claude AI"}},
	}
	var semrushID int
	_ = db.QueryRow(`SELECT w.id FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.domain=?`, "127.0.0.1:5141").Scan(&semrushID)
	chatSeq := 2001
	for _, group := range groups {
		var existing int
		if db.QueryRow(`SELECT id FROM operators WHERE username=?`, group.name).Scan(&existing) == nil {
			continue
		}
		res, err := db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES (?,?,?,?)`,
			group.name, hashPassword("toolsmandi"), "reseller", "active")
		if err != nil {
			log.Printf("reseller %s: %v", group.name, err)
			continue
		}
		opID, _ := res.LastInsertId()
		var destIDs []int
		for _, label := range group.chats {
			d, err := db.Exec(`INSERT INTO telegram_destinations (reseller_id, label, chat_id, enabled) VALUES (?,?,?,1)`,
				opID, label, "-100"+strconv.Itoa(chatSeq))
			chatSeq++
			if err != nil {
				continue
			}
			id, _ := d.LastInsertId()
			destIDs = append(destIDs, int(id))
		}
		var ownSites []int
		for _, toolName := range group.tools {
			var websiteID int
			err := db.QueryRow(`SELECT w.id FROM websites w JOIN tools t ON t.id=w.tool_id WHERE t.name=?`, toolName).Scan(&websiteID)
			if err != nil {
				continue
			}
			_, _ = db.Exec(`INSERT IGNORE INTO operator_websites (operator_id, website_id) VALUES (?,?)`, opID, websiteID)
			ownSites = append(ownSites, websiteID)
		}
		for _, websiteID := range ownSites {
			if group.name == "SeoGroupBuy" && websiteID == semrushID {
				continue
			}
			attachRoute(db, websiteID, destIDs)
		}
		if group.name == "SeoGroupBuy" && semrushID > 0 {
			attachRoute(db, semrushID, destIDs)
		}
	}
}

// ToolWaly uses the same tool ports as gt4rents, on its own domains.
// Cookies, limits, logs, and Telegram stay on these website rows.
func ensureToolWalySites(db *sql.DB) {
	type site struct {
		tool   string
		name   string
		domain string
		owner  string
	}
	sites := []site{
		{"Ahrefs", "ToolWaly Ahrefs", "refs.toolwaly.com", "refs.upays.in"},
		{"Semrush", "ToolWaly Semrush", "smrs.toolwaly.com", "smrs.upays.in"},
		{"Canva", "ToolWaly Canva", "cnva.toolwaly.com", "cnva.upays.in"},
		{"Grammarly", "ToolWaly Grammarly", "grammarly.toolwaly.com", "grmm.upays.in"},
	}
	var opID int64
	if db.QueryRow(`SELECT id FROM operators WHERE username=?`, "ToolWaly").Scan(&opID) != nil || opID <= 0 {
		log.Printf("ToolWaly reseller is missing; websites were not assigned")
		return
	}
	var destIDs []int
	rows, err := db.Query(`SELECT id FROM telegram_destinations WHERE reseller_id=? AND enabled=1`, opID)
	if err == nil {
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				destIDs = append(destIDs, id)
			}
		}
		rows.Close()
	}
	for _, site := range sites {
		var toolID int64
		if db.QueryRow(`SELECT id FROM tools WHERE name=?`, site.tool).Scan(&toolID) != nil || toolID <= 0 {
			log.Printf("ToolWaly %s: tool row missing", site.tool)
			continue
		}
		var websiteID int64
		err := db.QueryRow(`SELECT id FROM websites WHERE lower(trim(domain))=lower(trim(?))`, site.domain).Scan(&websiteID)
		if err != nil {
			limits := "{}"
			_ = db.QueryRow(`SELECT COALESCE(default_limits_json, '{}') FROM websites WHERE lower(trim(domain))=lower(trim(?))`, site.owner).Scan(&limits)
			if strings.TrimSpace(limits) == "" {
				limits = "{}"
			}
			res, insErr := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
				VALUES (?,?,?,?,30,?,1)`, toolID, site.name, site.domain, "secret_"+strings.ReplaceAll(site.domain, ".", "_"), limits)
			if insErr != nil {
				log.Printf("ToolWaly website %s: %v", site.domain, insErr)
				continue
			}
			websiteID, _ = res.LastInsertId()
			_, _ = db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, site.name, `["toolwaly"]`)
			_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, status, description) VALUES (?,?,?,?,?)`,
				websiteID, site.name+" 1", "", "active", "Paste this website's own cookie. It is not shared with gt4rents.")
			log.Printf("added %s website %d at %s", site.name, websiteID, site.domain)
		}
		_, _ = db.Exec(`INSERT IGNORE INTO operator_websites (operator_id, website_id) VALUES (?,?)`, opID, websiteID)
		if len(destIDs) > 0 {
			attachRoute(db, int(websiteID), destIDs)
		}
	}
}

// T4 Toolz Ahrefs shares the Ahrefs process. Its own website keeps a separate cookie and the button name t4refs.
func ensureT4ToolzAhrefs(db *sql.DB) {
	const domain = "ah2.t4toolz.com"
	const slug = "t4refs"
	const name = "T4 Toolz Ahrefs"
	var toolID int64
	if db.QueryRow(`SELECT id FROM tools WHERE name=?`, "Ahrefs").Scan(&toolID) != nil || toolID <= 0 {
		log.Printf("T4 Toolz Ahrefs: tool row missing")
		return
	}
	limits := "{}"
	sessionMinutes := 30
	_ = db.QueryRow(`SELECT COALESCE(default_limits_json, '{}'), COALESCE(session_duration, 30) FROM websites WHERE lower(trim(domain)) IN ('refs.upays.in', 'refs.gt4rents.com') ORDER BY CASE WHEN domain LIKE '%.upays.in' THEN 0 ELSE 1 END LIMIT 1`).Scan(&limits, &sessionMinutes)
	if strings.TrimSpace(limits) == "" {
		limits = "{}"
	}
	if sessionMinutes < 1 {
		sessionMinutes = 30
	}
	var websiteID int64
	var currentSlug string
	err := db.QueryRow(`SELECT id, COALESCE(access_slug, '') FROM websites WHERE lower(trim(domain))=lower(trim(?))`, domain).Scan(&websiteID, &currentSlug)
	if err != nil {
		var taken int
		if db.QueryRow(`SELECT id FROM websites WHERE lower(access_slug)=?`, slug).Scan(&taken) == nil {
			log.Printf("T4 Toolz Ahrefs: button name %s is already used by website %d", slug, taken)
			return
		}
		res, insErr := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
			VALUES (?,?,?,?,?,?,1,?)`, toolID, name, domain, "secret_ah2_t4toolz_com", sessionMinutes, limits, slug)
		if insErr != nil {
			log.Printf("T4 Toolz Ahrefs website: %v", insErr)
			return
		}
		websiteID, _ = res.LastInsertId()
		_, _ = db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, name, `["t4toolz"]`)
		_, _ = db.Exec(`INSERT INTO accounts (website_id, name, cookie, status, description) VALUES (?,?,?,?,?)`,
			websiteID, name+" 1", "", "active", "Paste this website's own cookie. It is not shared with gt4rents.")
		log.Printf("added %s website %d at %s", name, websiteID, domain)
		return
	}
	if strings.TrimSpace(currentSlug) == slug {
		return
	}
	if strings.TrimSpace(currentSlug) != "" && strings.TrimSpace(currentSlug) != "ah2" {
		return
	}
	var taken int
	if db.QueryRow(`SELECT id FROM websites WHERE lower(access_slug)=? AND id<>?`, slug, websiteID).Scan(&taken) == nil {
		log.Printf("T4 Toolz Ahrefs: button name %s is already used by website %d", slug, taken)
		return
	}
	_, _ = db.Exec(`UPDATE websites SET access_slug=? WHERE id=?`, slug, websiteID)
}

func attachRoute(db *sql.DB, websiteID int, destIDs []int) {
	var routeID int64
	err := db.QueryRow(`SELECT id FROM telegram_routes WHERE website_id=?`, websiteID).Scan(&routeID)
	if err != nil {
		res, insErr := db.Exec(`INSERT INTO telegram_routes (website_id, events_json) VALUES (?,?)`, websiteID, `["logout","spam"]`)
		if insErr != nil {
			return
		}
		routeID, _ = res.LastInsertId()
	}
	for _, destID := range destIDs {
		_, _ = db.Exec(`INSERT IGNORE INTO telegram_route_chats (route_id, destination_id) VALUES (?,?)`, routeID, destID)
	}
}
