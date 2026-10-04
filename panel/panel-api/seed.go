package main

import (
	"database/sql"
	"log"
	"strconv"
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
	{"speechify", "Speechify", "5011", "audio", credits},
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
}

func ensureResellers(db *sql.DB) {
	type spec struct {
		name  string
		chats []string
		tools []string
	}
	groups := []spec{
		{"ToolWaly", []string{"ToolWaly 1", "ToolWaly 2", "ToolWaly 3"}, []string{"Semrush", "Envato", "ChatGPT"}},
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
			_, _ = db.Exec(`INSERT OR IGNORE INTO operator_websites (operator_id, website_id) VALUES (?,?)`, opID, websiteID)
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
		_, _ = db.Exec(`INSERT OR IGNORE INTO telegram_route_chats (route_id, destination_id) VALUES (?,?)`, routeID, destID)
	}
}
