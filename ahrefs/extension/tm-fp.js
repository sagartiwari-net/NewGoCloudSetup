/*
 * tm-fp.js — stable install fingerprint (session bind)
 * One random id per extension profile in chrome.storage.local.
 * Stolen tb.* tokens without this id are rejected by the API.
 */
(() => {
  const KEY = 'tm_bar_fp';

  function randomId() {
    try {
      if (self.crypto && typeof self.crypto.randomUUID === 'function') {
        return 'tmfp.' + self.crypto.randomUUID();
      }
    } catch (_) {}
    const bytes = new Uint8Array(16);
    if (self.crypto && self.crypto.getRandomValues) {
      self.crypto.getRandomValues(bytes);
    } else {
      for (let i = 0; i < bytes.length; i++) bytes[i] = (Math.random() * 256) | 0;
    }
    return 'tmfp.' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  }

  async function getFingerprint() {
    const cur = await new Promise((r) => chrome.storage.local.get([KEY], r));
    const existing = cur && cur[KEY] ? String(cur[KEY]).trim() : '';
    if (existing.length >= 16) return existing;
    const id = randomId();
    await new Promise((r) => chrome.storage.local.set({ [KEY]: id }, r));
    return id;
  }

  self.tmFp = { getFingerprint, KEY };
})();
