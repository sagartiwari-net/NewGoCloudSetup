/*
 * tm-cache.js — browser entity cache (Phase 6)
 * Keys: domain:<host>, keyword:<q>, serp:<engine>:<q>
 * TTL default 72h, LRU cap 500 domain entities.
 * Loaded via importScripts from tm-guard.js
 */
(() => {
  const DB_NAME = 'tm-ahrefs-cache';
  const DB_VER = 1;
  const STORE = 'entities';
  const META = 'meta';
  const DEFAULT_TTL_MS = 72 * 60 * 60 * 1000;
  const MAX_DOMAIN_ENTITIES = 500;

  function openDB() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VER);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(STORE)) {
          const os = db.createObjectStore(STORE, { keyPath: 'key' });
          os.createIndex('by_touched', 'touchedAt', { unique: false });
          os.createIndex('by_kind', 'kind', { unique: false });
        }
        if (!db.objectStoreNames.contains(META)) {
          db.createObjectStore(META, { keyPath: 'id' });
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }

  function txDone(tx) {
    return new Promise((resolve, reject) => {
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error || new Error('aborted'));
    });
  }

  async function getTTL() {
    try {
      const db = await openDB();
      const tx = db.transaction(META, 'readonly');
      const req = tx.objectStore(META).get('settings');
      const row = await new Promise((res, rej) => {
        req.onsuccess = () => res(req.result);
        req.onerror = () => rej(req.error);
      });
      await txDone(tx);
      db.close();
      if (row && row.ttlMs > 0) return row.ttlMs;
    } catch (_) {}
    return DEFAULT_TTL_MS;
  }

  async function setTTLHours(hours) {
    const ttlMs = Math.max(1, Number(hours) || 72) * 60 * 60 * 1000;
    const db = await openDB();
    const tx = db.transaction(META, 'readwrite');
    tx.objectStore(META).put({ id: 'settings', ttlMs, updatedAt: Date.now() });
    await txDone(tx);
    db.close();
    return ttlMs;
  }

  function domainKey(host) {
    host = String(host || '').trim().toLowerCase().replace(/^https?:\/\//, '').replace(/\/.*$/, '').replace(/^www\./, '');
    return host ? 'domain:' + host : '';
  }

  function keywordKey(q) {
    q = String(q || '').trim().toLowerCase();
    return q ? 'keyword:' + q : '';
  }

  function serpKey(engine, q) {
    engine = String(engine || 'google').toLowerCase();
    q = String(q || '').trim().toLowerCase();
    return q ? 'serp:' + engine + ':' + q : '';
  }

  /** Drop null/undefined/empty-string fields to keep IndexedDB compact. */
  function compactData(data) {
    if (!data || typeof data !== 'object' || Array.isArray(data)) return data || {};
    const out = {};
    for (const [k, v] of Object.entries(data)) {
      if (v === null || v === undefined || v === '') continue;
      out[k] = v;
    }
    return out;
  }

  async function putEntity(key, kind, data, source) {
    if (!key) return false;
    const now = Date.now();
    const row = {
      key,
      kind,
      data: compactData(data),
      source: source || 'unknown',
      cachedAt: now,
      touchedAt: now,
    };
    const db = await openDB();
    const tx = db.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).put(row);
    await txDone(tx);
    db.close();
    if (kind === 'domain') await evictLRU();
    return true;
  }

  async function getEntity(key) {
    if (!key) return null;
    const ttl = await getTTL();
    const db = await openDB();
    const tx = db.transaction(STORE, 'readwrite');
    const os = tx.objectStore(STORE);
    const req = os.get(key);
    const row = await new Promise((res, rej) => {
      req.onsuccess = () => res(req.result || null);
      req.onerror = () => rej(req.error);
    });
    if (!row) {
      await txDone(tx);
      db.close();
      return null;
    }
    if (Date.now() - row.cachedAt > ttl) {
      os.delete(key);
      await txDone(tx);
      db.close();
      return null;
    }
    row.touchedAt = Date.now();
    os.put(row);
    await txDone(tx);
    db.close();
    return {
      key: row.key,
      kind: row.kind,
      data: row.data,
      source: row.source,
      cachedAt: row.cachedAt,
      ageMs: Date.now() - row.cachedAt,
      stale: false,
    };
  }

  async function evictLRU() {
    const db = await openDB();
    const tx = db.transaction(STORE, 'readwrite');
    const os = tx.objectStore(STORE);
    const idx = os.index('by_kind');
    const req = idx.getAll('domain');
    const rows = await new Promise((res, rej) => {
      req.onsuccess = () => res(req.result || []);
      req.onerror = () => rej(req.error);
    });
    if (rows.length <= MAX_DOMAIN_ENTITIES) {
      await txDone(tx);
      db.close();
      return;
    }
    rows.sort((a, b) => (a.touchedAt || 0) - (b.touchedAt || 0));
    const drop = rows.length - MAX_DOMAIN_ENTITIES;
    for (let i = 0; i < drop; i++) os.delete(rows[i].key);
    await txDone(tx);
    db.close();
  }

  async function clearAll() {
    const db = await openDB();
    const tx = db.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).clear();
    await txDone(tx);
    db.close();
  }

  async function stats() {
    const ttl = await getTTL();
    const db = await openDB();
    const tx = db.transaction(STORE, 'readonly');
    const req = tx.objectStore(STORE).getAll();
    const rows = await new Promise((res, rej) => {
      req.onsuccess = () => res(req.result || []);
      req.onerror = () => rej(req.error);
    });
    await txDone(tx);
    db.close();
    const byKind = {};
    for (const r of rows) byKind[r.kind] = (byKind[r.kind] || 0) + 1;
    return {
      total: rows.length,
      byKind,
      ttlHours: Math.round(ttl / 3600000),
      maxDomains: MAX_DOMAIN_ENTITIES,
    };
  }

  /** Save icon/metrics payload for a domain target. */
  async function putDomainMetrics(target, metrics, source) {
    const key = domainKey(target);
    if (!key) return false;
    return putEntity(key, 'domain', metrics, source || 'v4');
  }

  async function getDomainMetrics(target) {
    return getEntity(domainKey(target));
  }

  /** SERP prewarm: store serp row + upsert each result domain summary. */
  async function prewarmSerp(engine, query, domains) {
    const sKey = serpKey(engine, query);
    const list = Array.isArray(domains) ? domains : [];
    const domainKeys = [];
    for (const d of list) {
      const host = typeof d === 'string' ? d : (d && (d.host || d.domain || d.target));
      const metrics = typeof d === 'object' && d ? d : {};
      const key = domainKey(host);
      if (!key) continue;
      domainKeys.push(key);
      await putEntity(key, 'domain', {
        domainRating: metrics.domainRating,
        urlRating: metrics.urlRating,
        organicTraffic: metrics.organicTraffic,
        organicKeywords: metrics.organicKeywords,
        backlinks: metrics.backlinks,
        refdomains: metrics.refdomains,
        ahrefsRank: metrics.ahrefsRank,
        target: host,
      }, 'serp-prewarm');
    }
    await putEntity(sKey, 'serp', { domains: domainKeys, query, engine }, 'serp');
    const qKey = keywordKey(query);
    if (qKey && list[0] && typeof list[0] === 'object') {
      await putEntity(qKey, 'keyword', {
        volume: list[0].volume,
        difficulty: list[0].difficulty,
        traffic: list[0].traffic,
      }, 'serp-prewarm');
    }
    return { serpKey: sKey, domains: domainKeys.length };
  }

  self.tmCache = {
    domainKey,
    keywordKey,
    serpKey,
    getEntity,
    putEntity,
    putDomainMetrics,
    getDomainMetrics,
    prewarmSerp,
    clearAll,
    stats,
    setTTLHours,
    getTTL,
    DEFAULT_TTL_MS,
    MAX_DOMAIN_ENTITIES,
  };
})();
