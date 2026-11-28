/**
 * tm-kw-enrich.js — SV / KD / CPC on Keywords ideas rows.
 *
 * Official Ahrefs toolbar Keywords ideas lists related keywords only (plain text).
 * Volume/CPC/KD for the *search box* bar is separate (W6). Per-row metrics need
 * this overlay — same UX as our working builds / what you expect from Markhor shots.
 *
 * Do NOT move #ah-tb-serp-side-panel__wrapper (breaks React). Paint into live rows.
 */
(function tmKwEnrich() {
  if (!/google\./i.test(location.hostname)) return;

  const ROW = ".ah_tb-serp-tabs-body-kw-items";
  const MARK = "data-tm-kw";
  const cache = new Map();
  const pending = new Set();
  const failUntil = new Map();
  const queue = [];
  let inflight = 0;
  let scheduled = false;
  let lastQ = "";
  let lastLimitMsg = "";
  let lastLimitUntil = 0;
  const MAX_PARALLEL = 3;
  const FAIL_BACKOFF_MS = 60000;

  function fmt(n) {
    if (n == null || n === "") return "—";
    const x = Number(n);
    if (!Number.isFinite(x)) return "—";
    if (x >= 1e6) return (x / 1e6).toFixed(1).replace(/\.0$/, "") + "M";
    if (x >= 1e3) return (x / 1e3).toFixed(1).replace(/\.0$/, "") + "K";
    return String(Math.round(x));
  }

  function countryFromPage() {
    try {
      const host = location.hostname || "";
      const m = host.match(/google\.([a-z.]+)$/i);
      if (!m) return "us";
      const tld = m[1].toLowerCase();
      if (tld === "co.in") return "in";
      if (tld === "com") return "us";
      if (tld.length === 2) return tld;
      return "us";
    } catch (_) {
      return "us";
    }
  }

  function queryText() {
    try {
      return (new URLSearchParams(location.search).get("q") || "").trim().toLowerCase();
    } catch (_) {
      return "";
    }
  }

  function ensureStyles() {
    if (document.getElementById("tm-kw-enrich-css")) return;
    const s = document.createElement("style");
    s.id = "tm-kw-enrich-css";
    s.textContent =
      ".ah_tb-serp-tabs-body-kw-items{display:flex!important;align-items:center!important;" +
      "justify-content:space-between;gap:8px;padding-right:8px!important;}" +
      ".tm-kw-metrics{margin-left:auto;display:flex;gap:10px;font-size:11px;" +
      "color:var(--tb-text-color-secondary,#9aa0a6);font-variant-numeric:tabular-nums;" +
      "white-space:nowrap;flex-shrink:0;}" +
      ".tm-kw-metrics b{color:var(--tb-text-color-primary,#e8eaed);font-weight:600;}" +
      ".tm-kw-metrics.tm-kw-loading{opacity:.55;}";
    (document.head || document.documentElement).appendChild(s);
  }

  function rowKeyword(el) {
    // Official row __html is the keyword only; strip our metrics if present.
    const clone = el.cloneNode(true);
    clone.querySelectorAll(".tm-kw-metrics").forEach((n) => n.remove());
    return (clone.textContent || "").trim().toLowerCase();
  }

  function paint(el, stats, loading) {
    if (!el || !el.isConnected) return;
    ensureStyles();
    let box = el.querySelector(":scope > .tm-kw-metrics");
    if (!box) {
      box = document.createElement("span");
      box.className = "tm-kw-metrics";
      el.appendChild(box);
    }
    box.classList.toggle("tm-kw-loading", !!loading && !stats);
    if (loading && !stats) {
      box.innerHTML = "<span>SV <b>…</b></span><span>KD <b>…</b></span><span>CPC <b>…</b></span>";
      return;
    }
    const vol = stats && stats.volume && stats.volume.value;
    const kd = stats && stats.difficulty && stats.difficulty.value;
    const cpc = stats && stats.cpc && stats.cpc.value;
    box.innerHTML =
      "<span>SV <b>" +
      fmt(vol) +
      "</b></span><span>KD <b>" +
      fmt(kd) +
      "</b></span><span>CPC <b>" +
      (cpc == null || cpc === "" ? "—" : Number(cpc).toFixed(2)) +
      "</b></span>";
  }

  function paintAll(keyword, stats, loading) {
    const rows = document.querySelectorAll(ROW);
    for (let i = 0; i < rows.length; i++) {
      const el = rows[i];
      if (rowKeyword(el) !== keyword) continue;
      paint(el, stats, loading);
      if (stats) el.setAttribute(MARK, "ok:" + keyword);
    }
  }

  function paintLimit(keyword, message) {
    const rows = document.querySelectorAll(ROW);
    for (let i = 0; i < rows.length; i++) {
      const el = rows[i];
      if (rowKeyword(el) !== keyword) continue;
      ensureStyles();
      let box = el.querySelector(":scope > .tm-kw-metrics");
      if (!box) {
        box = document.createElement("span");
        box.className = "tm-kw-metrics";
        el.appendChild(box);
      }
      box.classList.remove("tm-kw-loading");
      box.textContent = "";
      const span = document.createElement("span");
      span.style.color = "#fca5a5";
      span.textContent = String(message || "Limit reached");
      box.appendChild(span);
      el.setAttribute(MARK, "limit:" + keyword);
    }
  }

  function fetchStats(keyword, cb) {
    try {
      chrome.runtime.sendMessage(
        { type: "tm:keyword-stats", keyword: keyword, country: countryFromPage() },
        (resp) => {
          if (chrome.runtime.lastError) {
            cb({ ok: false, error: chrome.runtime.lastError.message });
            return;
          }
          if (resp && resp.ok && resp.stats) {
            cb({ ok: true, stats: resp.stats });
            return;
          }
          cb({
            ok: false,
            error: (resp && resp.error) || "fail",
            limit: !!(resp && (resp.limit || resp.error === "LimitReached")),
            message: (resp && resp.message) || "",
            retry_after_sec: (resp && resp.retry_after_sec) || 0,
            window: (resp && resp.window) || "",
          });
        }
      );
    } catch (e) {
      cb({ ok: false, error: e.message });
    }
  }

  function pump() {
    while (inflight < MAX_PARALLEL && queue.length) {
      const keyword = queue.shift();
      inflight++;
      fetchStats(keyword, (res) => {
        inflight--;
        pending.delete(keyword);
        if (res.ok) {
          failUntil.delete(keyword);
          cache.set(keyword, res.stats);
          paintAll(keyword, res.stats, false);
        } else {
          const waitMs = res.limit && res.retry_after_sec
            ? Math.min(Math.max(res.retry_after_sec * 1000, FAIL_BACKOFF_MS), 60 * 60 * 1000)
            : res.limit
              ? FAIL_BACKOFF_MS * 2
              : FAIL_BACKOFF_MS;
          failUntil.set(keyword, Date.now() + waitMs);
          if (res.limit) {
            lastLimitMsg = res.message || "Hourly limit reached. Try again later.";
            lastLimitUntil = Date.now() + waitMs;
            paintLimit(keyword, lastLimitMsg);
          } else {
            paintAll(keyword, null, true);
          }
        }
        pump();
      });
    }
  }

  function enqueue(keyword) {
    if (!keyword || keyword.length > 80) return;
    // Skip the "Not found" empty-state row
    if (keyword === "not found" || keyword === "no results") return;
    if (cache.has(keyword)) {
      paintAll(keyword, cache.get(keyword), false);
      return;
    }
    if ((failUntil.get(keyword) || 0) > Date.now()) {
      if (lastLimitMsg && lastLimitUntil > Date.now()) paintLimit(keyword, lastLimitMsg);
      else paintAll(keyword, null, true);
      return;
    }
    if (pending.has(keyword)) {
      paintAll(keyword, null, true);
      return;
    }
    pending.add(keyword);
    paintAll(keyword, null, true);
    queue.push(keyword);
    pump();
  }

  function scan() {
    if (!/[?&]q=/.test(location.search) && !/\/search/i.test(location.pathname)) return;
    const rows = document.querySelectorAll(ROW);
    for (let i = 0; i < rows.length; i++) {
      const el = rows[i];
      const kw = rowKeyword(el);
      if (!kw) continue;
      if (cache.has(kw)) {
        paint(el, cache.get(kw), false);
        el.setAttribute(MARK, "ok:" + kw);
        continue;
      }
      if (el.getAttribute(MARK) === "ok:" + kw && el.querySelector(".tm-kw-metrics") &&
          !el.querySelector(".tm-kw-loading")) {
        continue;
      }
      enqueue(kw);
    }
  }

  function schedule() {
    if (scheduled) return;
    scheduled = true;
    setTimeout(function () {
      scheduled = false;
      try {
        scan();
      } catch (_) {}
    }, 400);
  }

  function onQueryMaybeChanged() {
    const q = queryText();
    if (q === lastQ) {
      schedule();
      return;
    }
    lastQ = q;
    pending.clear();
    queue.length = 0;
    failUntil.clear();
    schedule();
    setTimeout(schedule, 600);
    setTimeout(schedule, 1500);
    setTimeout(schedule, 3000);
  }

  lastQ = queryText();
  schedule();
  onQueryMaybeChanged();

  try {
    new MutationObserver(schedule).observe(document.documentElement || document, {
      childList: true,
      subtree: true,
    });
  } catch (_) {}

  window.addEventListener("popstate", onQueryMaybeChanged, true);
  try {
    const wrap = function (fn) {
      return function () {
        const ret = fn.apply(this, arguments);
        try {
          onQueryMaybeChanged();
        } catch (_) {}
        return ret;
      };
    };
    // Only wrap once
    if (!history.pushState.__tmKw) {
      history.pushState = wrap(history.pushState);
      history.pushState.__tmKw = true;
      history.replaceState = wrap(history.replaceState);
      history.replaceState.__tmKw = true;
    }
  } catch (_) {}

  setInterval(onQueryMaybeChanged, 2000);

  try {
    chrome.runtime.sendMessage({ type: "tm:refresh" }, function () {
      schedule();
      setTimeout(schedule, 800);
    });
  } catch (_) {}
})();
