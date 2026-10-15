/**
 * Lightweight i18n for webkvm — no dependencies.
 *
 * Usage (in any component/route):
 *   import { t, getLocale, setLocale, LOCALES } from '$lib/i18n.svelte.js';
 *   t('nav.storage')            -> current locale string (falls back to key)
 *   t('common.confirm')         -> ...
 *   t('vms.created', { name })  -> interpolates {name}
 *
 * The locale is reactive ($state): any component that calls t() during
 * render re-renders automatically when the language changes.
 * The selection is persisted in localStorage.
 */

export const LOCALES = [
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Español' },
  { code: 'ca', label: 'Català' },
];

// NOTE: `current` is initialized AFTER the messages dictionary (at the
// bottom of this file). Referencing `messages` during module init while
// it is still in the Temporal Dead Zone throws, which previously made
// load() fall back to the browser language and silently ignore the
// user's saved locale on every reload.

export function getLocale() {
  return current;
}

export function setLocale(code) {
  if (!messages[code]) return;
  current = code;
  try {
    localStorage.setItem('webkvm.locale', code);
  } catch {
    /* ignore */
  }
}

// escapeHtml prevents an interpolated value from being able to inject
// markup when the resulting string is later rendered with {@html} (a
// handful of translation keys need literal HTML in their interpolated
// slot, e.g. a <code> snippet — those pass { html: '...' } explicitly
// via htmlVar() rather than a plain string, opting out on a per-value
// basis instead of the whole call).
function escapeHtml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// htmlVar marks a t() interpolation value as pre-formatted HTML that
// must NOT be escaped. Use ONLY for a developer-controlled literal
// (e.g. a fixed <code> example) — never for anything derived from a
// VM name, username, or other value another user can influence.
export function htmlVar(html) {
  return { __html: html };
}

export function t(key, vars) {
  // Defensive: a caller passing an undefined/non-string key (e.g. a
  // lookup table miss like LABEL_MAP[unknownState]) must not crash the
  // whole page — that one bad translation is worth more broken than
  // the entire app going white with an uncaught TypeError. Render the
  // key itself (or an empty string) so the mistake is visible/debuggable
  // instead of fatal.
  if (typeof key !== 'string' || !key) return key || '';
  let node = messages[current] || messages.en;
  for (const part of key.split('.')) {
    if (node == null) break;
    node = node[part];
  }
  let out = typeof node === 'string' ? node : key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      const value =
        v && typeof v === 'object' && '__html' in v ? String(v.__html) : escapeHtml(String(v));
      out = out.replace(new RegExp(`\\{${k}\\}`, 'g'), value);
    }
  }
  return out;
}

function load() {
  try {
    const l = localStorage.getItem('webkvm.locale');
    if (messages[l]) return l;
  } catch {
    /* ignore */
  }
  // Default to the browser language when it's one we support.
  try {
    const nav = (navigator.language || 'en').toLowerCase().slice(0, 2);
    if (messages[nav]) return nav;
  } catch {
    /* ignore */
  }
  return 'en';
}

import en from './locales/en.js';
import es from './locales/es.js';
import ca from './locales/ca.js';

export const messages = { en, es, ca };

// Initialize the reactive locale AFTER messages so load() can actually
// read the dictionary (fixes the Temporal Dead Zone bug that made the
// saved locale ignored on reload).
let current = $state(load());
