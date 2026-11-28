/*
 * tm-config.js — stamped per reseller build (Phase 10).
 * Dev defaults below; panel zip builder overwrites this file.
 */
self.TM_BAR_CONFIG = {
  apiBase: 'http://127.0.0.1:8088',
  /** DNR urlFilter host for injecting auth headers on /v4 */
  proxyHostFilter: '||127.0.0.1:8088/v4/',
  brandName: 'ToolsMandi',
  resellerId: 0,
  buildId: 'dev-local',
  version: '1.0.33',
  /** Panel page (operator) — branded zip */
  panelUrl: 'http://127.0.0.1:3000/ahrefs-bar/download',
  /** Direct zip from bar-api (same as old “get update” button) */
  updateUrl: 'http://127.0.0.1:8088/v1/extension/download.zip',
};
