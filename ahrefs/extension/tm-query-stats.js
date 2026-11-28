/**
 * tm-query-stats.js — KD / Volume / GV / TP / Clicks / CPC stuck under the search box.
 *
 * Must sit directly under Google's search pill (.RNNXgb), NOT under #appbar / tabs.
 */
(function tmQueryStats() {
  if (!/google\./i.test(location.hostname)) return;

  const ID = "tm-query-stats";
  const STYLE_ID = "tm-query-stats-css";
  let lastQ = "";
  let inflight = "";
  let scheduled = false;

  function isSearchPage() {
    try {
      return /[?&]q=/.test(location.search) || /\/search/i.test(location.pathname);
    } catch (_) {
      return false;
    }
  }

  function queryText() {
    try {
      return (new URLSearchParams(location.search).get("q") || "").trim();
    } catch (_) {
      return "";
    }
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

  function fmt(n) {
    if (n == null || n === "") return "—";
    const x = Number(n);
    if (!Number.isFinite(x)) return "—";
    if (x >= 1e6) return (x / 1e6).toFixed(1).replace(/\.0$/, "") + "M";
    if (x >= 1e3) return (x / 1e3).toFixed(1).replace(/\.0$/, "") + "K";
    return String(Math.round(x));
  }

  function fmtMoney(n) {
    if (n == null || n === "") return "—";
    const x = Number(n);
    if (!Number.isFinite(x)) return "—";
    return "$" + x.toFixed(2);
  }

  function kdClass(kd) {
    if (kd == null || !Number.isFinite(Number(kd))) return "tm-qs-kd-na";
    const v = Number(kd);
    if (v <= 10) return "tm-qs-kd-0";
    if (v <= 20) return "tm-qs-kd-1";
    if (v <= 30) return "tm-qs-kd-2";
    if (v <= 40) return "tm-qs-kd-3";
    if (v <= 50) return "tm-qs-kd-4";
    if (v <= 60) return "tm-qs-kd-5";
    if (v <= 70) return "tm-qs-kd-6";
    if (v <= 80) return "tm-qs-kd-7";
    if (v <= 90) return "tm-qs-kd-8";
    return "tm-qs-kd-9";
  }

  function ensureCss() {
    if (document.getElementById(STYLE_ID)) return;
    const s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = [
      /* Keep official stats out of the way if Google absolute-positioned it badly */
      "#ah-tb-serp-stats{position:relative!important;left:auto!important;top:auto!important;}",
      "#" + ID + "{",
      "display:flex!important;align-items:center!important;flex-wrap:wrap!important;",
      "gap:8px 12px!important;",
      "box-sizing:border-box!important;",
      "width:100%!important;max-width:100%!important;",
      "margin:8px 0 0!important;padding:5px 10px!important;",
      "border-radius:16px!important;",
      "background:rgba(48,49,52,.95)!important;",
      "border:1px solid rgba(255,255,255,.10)!important;",
      "font:12px/1.2 Arial,Helvetica,sans-serif!important;color:#e8eaed!important;",
      "position:relative!important;z-index:5!important;",
      "}",
      "#" + ID + ".tm-qs-hidden{display:none!important;}",
      "#" + ID + " .tm-qs-item{display:inline-flex!important;align-items:center!important;gap:4px!important;white-space:nowrap!important;}",
      "#" + ID + " .tm-qs-label{color:#9aa0a6!important;font-size:12px!important;}",
      "#" + ID + " .tm-qs-val{font-weight:700!important;color:#e8eaed!important;font-size:12px!important;}",
      "#" + ID + " .tm-qs-kd{",
      "display:inline-flex!important;align-items:center!important;padding:1px 5px!important;",
      "border-radius:3px!important;font-weight:600!important;font-size:11px!important;",
      "}",
      "#" + ID + " .tm-qs-kd-0,#" + ID + " .tm-qs-kd-1,#" + ID + " .tm-qs-kd-2{background:#1e3a2f!important;color:#8ad0ab!important;}",
      "#" + ID + " .tm-qs-kd-3,#" + ID + " .tm-qs-kd-4,#" + ID + " .tm-qs-kd-5{background:#3a3418!important;color:#fce892!important;}",
      "#" + ID + " .tm-qs-kd-6,#" + ID + " .tm-qs-kd-7{background:#3a2410!important;color:#ffda8a!important;}",
      "#" + ID + " .tm-qs-kd-8,#" + ID + " .tm-qs-kd-9{background:#3a1515!important;color:#ffb4b4!important;}",
      "#" + ID + " .tm-qs-kd-na{background:#2a2a2a!important;color:#9aa0a6!important;}",
      "#" + ID + " .tm-qs-loading{opacity:.6!important;}",
    ].join("");
    (document.head || document.documentElement).appendChild(s);
  }

  function officialHasMetrics() {
    const el = document.getElementById("ah-tb-serp-stats");
    if (!el) return false;
    const wrap = el.querySelector(".ah-tb-serp-stats__wrapper");
    if (!wrap) return false;
    const fields = wrap.querySelectorAll(".ah-tb-serp-stats__field");
    if (fields.length < 3) return false;
    const r = wrap.getBoundingClientRect();
    // Must be visibly under the search area (not off-screen / zero-size)
    return r.width > 80 && r.height > 10 && r.top > 40 && r.top < 220;
  }

  /** The white/dark search pill — bar must stick right under this. */
  function searchPill() {
    return (
      document.querySelector(".RNNXgb") ||
      document.querySelector(".SDkEP") ||
      document.querySelector("#searchform .A8SBwf") ||
      document.querySelector("form#tsf .A8SBwf") ||
      null
    );
  }

  function searchForm() {
    return (
      document.querySelector("#searchform") ||
      document.querySelector("form#tsf") ||
      document.querySelector("form[role=search]") ||
      null
    );
  }

  function ensureBar() {
    ensureCss();
    let bar = document.getElementById(ID);
    if (!bar) {
      bar = document.createElement("div");
      bar.id = ID;
      bar.innerHTML = '<span class="tm-qs-loading">Loading keyword metrics…</span>';
    }

    const pill = searchPill();
    const form = searchForm();

    // Prefer: insert as next sibling of the search pill (directly under it).
    if (pill && pill.parentElement) {
      if (bar.previousElementSibling !== pill || bar.parentElement !== pill.parentElement) {
        if (pill.nextSibling) pill.parentElement.insertBefore(bar, pill.nextSibling);
        else pill.parentElement.appendChild(bar);
      }
      return bar;
    }

    // Fallback: inside/after search form
    if (form) {
      if (bar.parentElement !== form) form.appendChild(bar);
      return bar;
    }

    return bar;
  }

  function render(bar, stats, loading, limitMsg) {
    if (!bar) return;
    if (limitMsg) {
      bar.textContent = "";
      const span = document.createElement("span");
      span.className = "tm-qs-item";
      span.style.color = "#fca5a5";
      span.style.fontWeight = "600";
      span.textContent = String(limitMsg);
      bar.appendChild(span);
      return;
    }
    if (loading && !stats) {
      bar.innerHTML = '<span class="tm-qs-loading">Loading keyword metrics…</span>';
      return;
    }
    if (!stats) {
      bar.innerHTML = '<span class="tm-qs-loading">Keyword metrics unavailable</span>';
      return;
    }
    const vol = stats.volume && stats.volume.value;
    const clicks = stats.clicks && stats.clicks.value;
    const cpc = stats.cpc && stats.cpc.value;
    const kd = stats.difficulty && stats.difficulty.value;
    const gv = stats.global_volume && stats.global_volume.value;
    const tp = stats.traffic_potential && stats.traffic_potential.value;
    bar.innerHTML =
      '<span class="tm-qs-item"><span class="tm-qs-label">KD</span>' +
      '<span class="tm-qs-kd ' +
      kdClass(kd) +
      '">' +
      fmt(kd) +
      "</span></span>" +
      '<span class="tm-qs-item"><span class="tm-qs-label">Volume</span><span class="tm-qs-val">' +
      fmt(vol) +
      "</span></span>" +
      '<span class="tm-qs-item"><span class="tm-qs-label">GV</span><span class="tm-qs-val">' +
      fmt(gv) +
      "</span></span>" +
      '<span class="tm-qs-item"><span class="tm-qs-label">TP</span><span class="tm-qs-val">' +
      fmt(tp) +
      "</span></span>" +
      '<span class="tm-qs-item"><span class="tm-qs-label">Clicks</span><span class="tm-qs-val">' +
      fmt(clicks) +
      "</span></span>" +
      '<span class="tm-qs-item"><span class="tm-qs-label">CPC</span><span class="tm-qs-val">' +
      fmtMoney(cpc) +
      "</span></span>";
  }

  function fetchStats(keyword) {
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendMessage(
          { type: "tm:keyword-stats", keyword: keyword, country: countryFromPage() },
          (resp) => {
            if (chrome.runtime.lastError) {
              resolve({ stats: null, limitMsg: "" });
              return;
            }
            if (resp && resp.ok && resp.stats) {
              resolve({ stats: resp.stats, limitMsg: "" });
              return;
            }
            const limit = !!(resp && (resp.limit || resp.error === "LimitReached"));
            resolve({
              stats: null,
              limitMsg: limit
                ? (resp.message || "Hourly limit reached. Try again later.")
                : "",
            });
          }
        );
      } catch (_) {
        resolve({ stats: null, limitMsg: "" });
      }
    });
  }

  async function refresh() {
    ensureCss();
    if (!isSearchPage()) {
      const b = document.getElementById(ID);
      if (b) b.classList.add("tm-qs-hidden");
      return;
    }

    if (officialHasMetrics()) {
      const b = document.getElementById(ID);
      if (b) b.classList.add("tm-qs-hidden");
      return;
    }

    const q = queryText();
    if (!q) return;
    const bar = ensureBar();
    if (!bar) return;
    bar.classList.remove("tm-qs-hidden");

    if (q === lastQ && bar.dataset.ready === "1") return;
    if (inflight === q) return;
    inflight = q;
    lastQ = q;
    bar.dataset.ready = "0";
    render(bar, null, true, "");
    const res = await fetchStats(q);
    if (queryText() !== q) {
      inflight = "";
      return;
    }
    render(bar, res.stats, false, res.limitMsg);
    if (res.stats) bar.dataset.ready = "1";
    inflight = "";
  }

  function schedule() {
    if (scheduled) return;
    scheduled = true;
    setTimeout(function () {
      scheduled = false;
      refresh().catch(function () {});
    }, 250);
  }

  schedule();
  setTimeout(schedule, 700);
  setTimeout(schedule, 1800);
  setInterval(schedule, 2500);

  try {
    new MutationObserver(schedule).observe(document.documentElement || document, {
      childList: true,
      subtree: true,
    });
  } catch (_) {}

  window.addEventListener("popstate", schedule, true);
  window.addEventListener("resize", schedule, true);
  try {
    if (!history.pushState.__tmQs) {
      const wrap = function (fn) {
        return function () {
          const ret = fn.apply(this, arguments);
          schedule();
          return ret;
        };
      };
      history.pushState = wrap(history.pushState);
      history.pushState.__tmQs = true;
      history.replaceState = wrap(history.replaceState);
      history.replaceState.__tmQs = true;
    }
  } catch (_) {}
})();
