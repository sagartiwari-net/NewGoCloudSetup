# Semrush usage notes

Proxy: `http://127.0.0.1:5141`, folder `tool-need-to-work/semrush-new-go-proxy`.

These notes are from two live sessions. Semrush does not print its own credit counter in these logs, so a row is the request that represents the action. Repeat loads of the same report are not treated as a new search. Panel limits are not wired yet.

Do not count: page shell, CSS, JS, source maps, `device-bind`, web vitals, search-bar suggestions, and chart helpers that arrive with one report.

## Session 1

| Action | Request | What it means |
|---|---|---|
| Topic Research search | `POST /topic-research/api/researches/launch/` once | The new topic search. Keyword was the school-outfit query. |
| Same topic cards | `GET /topic-research/api/researches/6abe5f1054200d162d3f3429/` many times | Same report reloading. Not a new search. |
| Earlier research open | `GET` and `POST .../6abe2a7654200d162d3f336a/detailed/` | Opening one existing research. |
| Favorites | `GET /topic-research/api/researches/favorites/` | List only. |
| Create AI Content | `GET /contentshake/activate` then `GET /content/activate/` | One click into ContentShake. |
| Keyword Overview | `GET /analytics/keywordoverview/` plus `POST /kwogw/v2/webapi` | One keyword report. Extra webapi calls are the same page. |
| Keyword Magic Tool | `GET /analytics/keywordmagic/` plus `POST /kwm/rpc` | One tool open. The many RPC calls are groups inside that page. |
| Keyword Manager | `GET /analytics/keywordmanager/` and `list` | List of saved lists. |
| Organic Rankings | `GET /analytics/organic/positions/` | One report page. |
| Top Pages | `GET /analytics/toppages/` | One report page. |
| Domain Overview | `GET /analytics/overview/` plus `POST /dpa/rpc` | One domain. Later RPC calls are charts on that domain. |
| Keyword Gap | `GET /analytics/keywordgap/` plus `POST /spectrum/v1/Gap/KeywordsList` | One gap run. Totals and overlaps are the same run. |
| Backlink Gap | `GET /analytics/gap/backlinks/` and `report/` | One gap page. |
| Competitive Analysis | `POST /analytics/competitors/rpc` | One comparison. |
| Site Audit | `GET /siteaudit/` plus campaigns list and `api/limits` | Opening the project list, not a new crawl. |
| Local Business | `GET /local-business/start/` plus listings countries and current user | Landing page. The dentist search did not send a new report request. |
| Search box typing | `POST /search-bar/api/search` | Suggestions only, not a report. |

## Session 2

| Action | Request | What it means |
|---|---|---|
| AI Visibility overview | `GET /ai-seo/overview/` plus `POST /ai-seo/api/v2/brand/stats_by_llm`, `stats_by_country`, topic totals | One visibility report. The several posts are sections of that report. |
| Competitor Research | `GET /ai-seo/competitor-research/` plus `POST /ai-seo/api/v2/competitor/brand_competitors` | One competitor view. |
| Prompt Research | `GET /ai-seo/prompt-research/?q=group+buys+seo+tools` plus `POST /ai-seo/api/v2/prompt/prompts`, `prompts_totals`, `prompts_by_topic_ids` | One prompt report for that query. Topic and source posts are the same report. |
| Monitor all prompts | `GET /positions/api/campaigns` | This click failed. Semrush answered `400 Request Header Or Cookie Too Large`. The dialog was “Something went wrong”. |
| Backlinks | `GET /analytics/backlinks/` and many `webapi2/overview/*` | One backlink overview. The overview splits are one report. |
| Referring domains | `GET /analytics/refdomains/report/` | One report page. |
| Backlink Audit | `GET /backlink_audit/` plus `POST /backlink_audit/rpc/` | Opening the audit. Many RPC calls are rows of that open. |
| Semrush Rank | `GET /analytics/ranks/rank/` and `GET /sensor/` plus `sensor/api/ranks/` | Rank page. Ranks API failed with `431 Request Header Fields Too Large`. |
| Organic Traffic Insights | `GET /organic_traffic_insights/` and project `31424003` | Opening that project. `siteUrls` was 403 from Semrush. The report GET was blocked by our device check before it left the proxy. |
| Create AI Content again | `GET /content/activate/` and content-shake token | Same ContentShake click as session 1. |

## Session 3

Cookie header is now only the panel account plus Cloudflare cookies, so the earlier header-too-large failures are gone. `GET /positions/api/campaigns` came back `200` here. That was the Monitor click that failed in session 2.

| Action | Request | What it means |
|---|---|---|
| Traffic Analytics shell | `GET /analytics/traffic/` | Opening the tool. Not a search. |
| Domain saved | `POST ...trends_dashboards...ProjectService/CreateOrUpdate` once, then `ByID` and `UpdateViewTime` | Opening the saved domain (the google.com list). Repeats are the dashboard refreshing that same project. |
| Traffic overview | `POST ...traffic_analytics...Selector/GetTarget` and `GetTargetTrend` | The overview numbers and trend chart for that domain. |
| Market | `POST ...Selector/GetMarketMetrics`, `GetMarketSizeTrend`, `ListMarketDomains` | One market view. |
| Pages | `POST ...Selector/ListPages` and `ListPagesInsights` | Top pages of the same domain. Extra posts are filters and pages of that list. |
| Sources | `POST ...Selector/ListSources`, `ListSourcesCategories`, `ListSourcesServices`, `GetSourcesServicesTrend` | Source breakdown of the same domain. |
| Destinations | `POST ...Selector/ListDestinations` and `ListDestinationsCategories` | One destinations view. |
| Subfolders and subdomains | `POST ...Selector/ListSubfolders` and `ListSubdomains` | Those two side reports. |
| Keywords on the page | `POST ...DomainAnalytics/ListKeywords` | Keyword rows under the traffic page. Same domain. |
| Page Groups | `POST ...PageGroupsApiGateway/GetPageGroupMetricsTrend`, `ListPageGroupTopSources`, `ListPageGroupTopDestinations`, `ListTopGrowingPages`, `ListPageGroupDistributionByCountries` | One Page Groups report (`report=ads`, entry pages). These returned 200. |
| Page Groups error | `POST ...PageGroupsApiGateway/GetPageGroupMetrics` | This one call sat for 60s and the proxy closed it (`502`, header timeout). The Trend by Device box showed “Something went wrong”. The other Page Groups calls had already loaded. |

AI toolkit calls in this same window (`/semrush-ai-toolkit/api/v1/data/*` and `insights`) are the earlier AI visibility project refreshing in the background, project id `228507`. They are not part of the Traffic Analytics click. One insights post was cut off (`502 context canceled`) because the browser left that page.

## Session 4

Same google.com Traffic Analytics list. Semrush still does not print a unit price, so each row is the request that stands for the click.

| Action | Request | What it means |
|---|---|---|
| Page Groups opened again | `GET /analytics/traffic/page-groups?lid=1322415&report=ads` | One Page Groups report. Entry pages, Aug 2026, all devices. |
| Page Groups widgets | `POST ...PageGroupsApiGateway/GetPageGroupMetrics`, `GetPageGroupMetricsTrend`, `GetPageGroupVisitsByTrafficChannelTrend`, `ListPageGroupTopSources`, `ListPageGroupTopDestinations`, `ListTopGrowingPages`, `ListPageGroupDistributionByCountries` | Sections of that one report. The first wave was canceled around 15s (`502`), then the same calls returned `200` in 15–60s. The chart still showed “Something went wrong” because the proxy treated the gRPC body as text and rewrote it. |
| Audience device and demographics | `POST ...Selector/GetAudienceDeviceDistribution` and `GetAudienceDemographics` | Two audience cards for the same domain. Both `200`. |
| Audience overlap table | `POST ...Selector/ListAudienceDomains` and `ListAudienceDomainsCategories` | The overlap table. First try canceled around 26s, then `200` after about 60s. The spinner was this call. |
| Audience interests and social | `POST ...Selector/ListAudienceInterests` and `ListAudienceSocialMedia` | Helpers on the same audience view. |
| Prompt Tracking | `GET /tracking/landscape/31418707_5574879.html?domain_1=dellahome.com&report_mode_chart=aiVisibility` plus `get_init_command`, campaign `info`, `limits`, `tags`, `topics`, and `GET /positions/api/31418707/targets` | Opening that campaign’s AI visibility chart. These returned `200`. |
| Keyword gather progress | `GET /tracking/web-api/sse/keywords/gather_progress?campaign_id=31418707_5574879` | Live progress. Blocked at the proxy in `0ms` with `401` because EventSource cannot send the device header. The page stayed on “Gathering keywords data: 0/12”. |
| Questions | `GET /ai-seo/questions/` | Opening the Questions page. |
| Trending Websites | `GET /trending-websites/global/all/` | Opening that directory. |

## Not a credit

Search suggestions, folder lists, paywall checks, billing notification, device heartbeat, CSS, JS, and source maps. Source map 404s are debug files Semrush does not ship. They do not load the report.

## Still not checked

A finished Local Business result, Advertising toolkit after Try now, GBP search after a successful submit, Page Groups and Audience Overlap after the gRPC pass-through, Prompt Tracking after the progress stream is allowed, exports (XLSX or PDF that actually download), and Content Creation after the editor opens.
