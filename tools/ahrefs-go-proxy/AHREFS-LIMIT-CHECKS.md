# Ahrefs limit checks

Panel limits: Credits reset daily. Export rows reset weekly.

## Exact counting plan

Confirmed from the tests and from the user: a search is 1 credit, and every later click is also 1 credit. The click is the unit, not every helper request inside that click.

Example. Site Explorer search for `google.com` costs 1 credit. After that, Organic keywords, Backlinks, a filter, a country change, a group, or opening the same report again each cost 1 more credit. The charts that load together with that one click do not add extra credits.

Count 1 credit only on the main request for that click:

- Site Explorer search: the overview data call, once per search.
- Each later Site Explorer option or filter: that option's main API, once per click.
- Keywords Explorer overview: `keGetTopPositionsHistory` with `consumeLimitFor=KeOverview`.
- Keywords Explorer options: `keIdeas`, `keGetTrafficByDomains`, `keGetTrafficByPages`, `keAdsByDomains`, each click.
- Content Explorer search: `ceSearchResults`. Later tabs and a result detail are their own clicks.
- Other tools follow the same rule. The tables below name the main request for each click.

Do not count these: empty tool page, `tkGet*` settings, presets, plan, remaining-searches, and the small chart calls that arrive with the main request.

Export is not a credit. Count data rows only.

A file of 1000 exported records has 1001 lines because line 1 is the heading. Rows added to the weekly export limit are `line count - 1`. If that result is below 1, add 0. UTF-16 CSV uses the same rule on newline pairs, then subtract the heading. The dialog `tkGetExportSettings` adds nothing. The CSV response that returns 200 is the one that adds rows.

Batch Analysis is still one click, but the amount is not always 1. `POST /v4/baTable` costs 1 credit per 200 targets: 1–200 = 1, 201–400 = 2. Its CSV export uses the row rule above, not an extra credit.

Web Analytics, Bot Analytics, Alerts, Local SEO list, Ahrefs Top, and an empty AI Grader page add no credit.

## Keywords Explorer — saved

Test keyword `google`. India (`in`) aur Andorra (`ad`).

| Action | Count request | Limit |
|---|---|---|
| Overview, India | `GET /v4/keGetTopPositionsHistory` with `consumeLimitFor=KeOverview` | 1 credit |
| Overview helpers, no extra count | `keGetKeywordAttrs`, `keGetDifficulty`, `keParentTopic`, `keKeywordOverview`, `keSerpOverview`, `keGetClicksGoogle`, `keGetGlobalVolume`, `keGetSearchVolumeGoogle`, `keGetPaidPositionsHistory`, `keIdeasOverview` | overview ke saath |
| Matching terms | `POST /v4/keIdeas` | 1 credit. `keIdeasKwGroups` is the group list, not a second credit |
| Traffic by domains | `POST /v4/keGetTrafficByDomains` | 1 credit |
| Traffic by pages | `POST /v4/keGetTrafficByPages` | 1 credit |
| Ads history domains | `GET /v4/keAdsByDomains` | 1 credit |
| Country India to Andorra | same overview request, country `ad`, `consumeLimitFor=KeOverview` again | 1 new credit |
| Matching terms, Andorra | `POST /v4/keIdeas` again | 1 credit |
| CSV export, Matching terms | `POST /v4/keIdeasExport?mode=csv-utf16` returned 200, about 59 KB | export rows, alag weekly limit. `tkGetExportSettings` sirf dialog hai |

Same page reload bhi `keIdeas` dubara bhejti hai. User rule ke hisaab se wahi search dubara bhi credit hai, isliye reload ko free mat maano.

Export crash purane MySQL check ki wajah se tha. Ab `db == nil` par woh check nahi chalta, aur export 200 aa chuka hai.

## Site Explorer — saved

Test domain `prezi.com`, mode `subdomains`. Empty Site Explorer page is not a credit. Overview charts share one domain search. `seGetBestFilters` and `seFilterPresetsList` are settings, not a credit.

| Action | Count request | Limit |
|---|---|---|
| Overview | page `/site-explorer/overview`. Charts together: `seGetMetrics`, `seGetDomainRating`, `seBacklinksStats`, `seTrafficSummary`, `seGetPageInfo` | 1 credit for the domain search |
| Organic keywords | `GET /v4/seGetOrganicKeywords` | 1 credit. Country split `seGetOrganicKeywordsByCountry` is the same report |
| Positions / SERP inside organic keywords | `seGetOrganicPositions`, `seGetPositionsHistory`, `seSerpOverview` | 1 credit for this option |
| Top pages | `GET /v4/seGetTopPages` | 1 credit |
| Organic competitors | `GET /v4/seGetOrganicCompetitors` | 1 credit |
| Backlinks | `GET /v4/seBacklinks` | 1 credit |
| Backlinks group | `POST /v4/seBacklinksGroup` | 1 credit, filter/group |
| Broken backlinks | `GET /v4/seBrokenBacklinks` | 1 credit |
| Referring domains | `POST /v4/seRefdomains` | 1 credit |
| Anchors | `POST /v4/seAnchors` | 1 credit |
| Authors | `POST /v4/seAuthors` | 1 credit |
| Authors CSV export | `GET /v4/seAuthorsExport` returned 200, about 2.5 KB | export rows. `tkGetExportSettings` is only the dialog |
| Referring IPs | `POST /v4/seRefIPs` returned 200. Same page reopened | 1 credit each open |
| Linked anchors | `POST /v4/seLinkedAnchors` from `/site-explorer/opportunities` | 1 credit |
| Position movements | `GET /v4/seGetPositionsMovements` | 1 credit |
| Site structure | `GET /v4/seGetSiteStructure` | 1 credit |
| HTML snapshot | `GET /v4/seGetHtmlSnapshot` with `seGetHtmlCalendar` and `seGetHtmlTimeline` | 1 credit |
| Paid keywords | `GET /v4/seGetPaidKeywordsV2` | 1 credit |
| Ads | `GET /v4/seGetAdsV2` | 1 credit |

## Competitive Analysis — saved

| Action | Count request | Limit |
|---|---|---|
| Tool home | `GET /competitive-analysis/` plus `caGetProjects` | no credit |
| Keyword comparison | `POST /v4/caGetKeywords` | 1 credit. `caGetKeywordsTotals` is the same search |
| Link Intersect | `POST /v4/caLinkIntersect` | 1 credit. Chart `caLinkIntersectChart` is the same run |

## Content Explorer — saved

Search term `trade license`.

| Action | Count request | Limit |
|---|---|---|
| Tool page | `cePlan`, `ceHistory`, `ceGetPresets` | no credit |
| Search | `GET /v4/ceSearchResults` | 1 credit |
| Search breakdowns, no extra count | `ceSearchResultsPublishType`, `ceSearchResultsPlatformType`, `ceSearchResultsLanguage`, `ceSearchResultsPageType` | search ke saath |
| Result row charts | `ceInlineDomainRating`, `ceInlinePositionsMetricsChart`, `ceInlineRefDomainsChart`, `ceInlineURLRatingChart` | no extra credit |
| Authors | `GET /v4/ceAuthorsReport` | 1 credit |
| Websites | `GET /v4/ceWebsitesReport` | 1 credit |
| Languages | `GET /v4/ceLanguagesReport` | 1 credit |
| One result detail | `ceSearchDetailsKeywords`, `ceSearchDetailsRefDomains`, `ceSearchDetailsAnchors`, `ceSearchDetailsBacklinks` | 1 credit. Trend charts and `ceSearchDetailsTabTotals` are the same detail |
| CSV export | `POST /v4/ceSearchResultsExport?mode=csv-utf16` returned 200, about 4.4 KB | export rows. `tkGetExportSettings` is only the dialog |

## Site Audit — saved

Existing project `10408774`, site `takweenadvisory.ae`. Opening the project did not start a new crawl.

| Action | Count request | Limit |
|---|---|---|
| Projects list | `POST /v4/saProjects` with `saPlan` | no credit |
| Open a crawl | `saGetProject`, `saGetCrawl`, `saCharts`, `saOverviewIssueCharts`, `saGetProjectIssues` | 1 credit for opening the audit |
| Page explorer / filters | `POST /v4/saDeTable` and `saGetCountsByFilters` | 1 credit per filter or view change |
| URL detail inside page explorer | `POST /v4/saGetUrlDetails` and `saGetUrlIssues` | 1 credit for opening that URL |
| Crawl log | `POST /v4/saGetCrawlLog` | 1 credit |
| Timeframe metrics | `POST /v4/saTimeframesMetrics` | 1 credit |
| Export affected pages | `POST /v4/saExportAffectedPages` returned 200, about 11 bytes | export rows |

Clicking an in-page option was sending the browser to `/`, and Ahrefs turns `/` into `/dashboard`. That navigation now stays on the Site Audit page.

## AI Content Helper — saved

Document from `https://www.demandsage.com/semrush-coupon/`.

| Action | Count request | Limit |
|---|---|---|
| Tool home | `GET /content-helper`, `cmGetDocuments`, `cmLimits` | no credit |
| New document from a URL | `POST /v4/cmCreateDocument` then `cmGetInitialDocumentContent` | 1 credit |
| Content score | `POST /v4/cmGetContentScore` | part of the same document, not a new credit |
| Topics and terms | `POST /v4/cmGetTopicsTerms` | same document |
| Competitors | `POST /v4/cmSelectCompetitors` and `cmGetCompetitorData` | 1 credit when competitors are chosen |
| Typing in the editor | `POST /v4/cmUpdateDocumentContent` | no extra credit |

Content score options (Topics, AI chat, Title tag, Meta description, Headings, Competitors) were sending the browser to `/dashboard`. Those clicks now stay inside Content Helper.

## Brand Radar — saved

Brand `Semrush` / `semrush.com`. Competitors `Ahrefs` and `Moz`.

| Action | Count request | Limit |
|---|---|---|
| Tool home | `GET /brand-radar`, `brGetProjects`, `brGetReports` | no credit |
| Overview | `GET /v4/brGetOverviewStats` | 1 credit. Timelines `brGetOverviewTimelines` are the same overview |
| Search demand | `GET /v4/brGetSearchDemand` | 1 credit |
| Web visibility | `GET /v4/brGetWebVisibility` and `brGetWebPagesResults` | 1 credit |
| YouTube | `GET /v4/brGetYoutubeVisibility` and `brGetYoutubeVideosResults` | 1 credit |
| TikTok | `GET /v4/brGetTikTokResults` | 1 credit |
| Reddit | `GET /v4/brGetRedditResults` | 1 credit |
| Share of voice | `GET /v4/brGetReportsShareOfVoice` | 1 credit |

The top Dashboard link is a real `/dashboard` navigation. Tool-option clicks that only go to `/` still stay on the current tool.

## Credit rule

Har naya search, naya report, country change, filter, group, aur usi result ka alag option **1 credit** hai. Wahi action dubara bhi 1 credit hai. Jo charts usi click ke saath aate hain, settings, presets, aur plan unme credit nahi hai.

Export file credit nahi hai. Weekly export rows = CSV lines minus 1 heading. 1000 records become a 1001-line file, so 1000 rows are counted. `tkGetExportSettings` adds nothing.

Batch Analysis: `POST /v4/baTable` is 1 credit per 200 targets. Its CSV still uses the row rule.

## Baaki tools — saved

| Action | Count request | Limit |
|---|---|---|
| Batch Analysis page | `GET /batch-analysis`, `baSettings` | no credit |
| Batch Analysis run | `POST /v4/baTable`. Do runs aaye. Body log mein nahi thi, isliye exact target count yahan se nahi pata | 1 credit per 200 targets |
| Batch Analysis export | `POST /v4/baTableExport` returned 200, CSV about 126 KB | export rows |
| Web Analytics project `10447961` | `waChart`, `waStats`, `waFunnels`, `waSiteStructure` | no SEO credit |
| Bot Analytics | `GET /bot-analytics` | no SEO credit |
| Alerts | `alertsGetBacklinksAlerts`, keywords, mentions, rank tracker lists | no SEO credit |
| Local SEO | `lsGetData`, `lsGetPerformanceBusinesses` | no SEO credit. Naya business add is session mein nahi hua |
| Ahrefs Top | `ahrefsTopGetList` | no SEO credit |
| AI Grader | `GET /ai-grader` only | no SEO credit. Koi grade run nahi hua |
| Rank Tracker widgets | `daGetRtProject`, `rtGetKeywordsStatsHistory` sirf dashboard data the | naya keyword add is session mein nahi hua |
| GSC charts | `daGetGscPerfMetrics` project dashboard ke saath aaya | alag GSC report run nahi hua |
| Portfolios list | `daGetPortfolios` | no SEO credit. Naya portfolio nahi bana |

## Har test par yeh likhna hai

1. Tool aur exact click.
2. Credit pehle aur baad. Farq 0 ho to free.
3. Export rows pehle aur baad.
4. Jo request logs mein nayi aaye, sirf wahi.
