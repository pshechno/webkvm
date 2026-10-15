import { describe, it, expect } from 'vitest';
import en from './en.js';
import ru from './ru.js';

// Recursively collect the set of leaf (string) key paths of a dictionary.
function leafKeys(node, prefix = '') {
  const keys = [];
  for (const [k, v] of Object.entries(node)) {
    const path = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === 'object') {
      keys.push(...leafKeys(v, path));
    } else {
      keys.push(path);
    }
  }
  return keys;
}

describe('ru locale completeness', () => {
  const enLeaves = leafKeys(en);
  const ruLeaves = leafKeys(ru);
  const enSet = new Set(enLeaves);
  const ruSet = new Set(ruLeaves);

  it('ru has exactly the same leaf keys as en', () => {
    const missing = enLeaves.filter((k) => !ruSet.has(k));
    expect(missing, `keys missing in ru: ${missing.slice(0, 20).join(', ')}…`).toEqual([]);
  });

  it('ru has no keys that are absent in en', () => {
    const extra = ruLeaves.filter((k) => !enSet.has(k));
    expect(extra, `keys not present in en: ${extra.slice(0, 20).join(', ')}…`).toEqual([]);
  });

  it('ru preserves {placeholder} names in every translated string', () => {
    // Compare the multiset of {var} placeholders key by key.
    const placeholders = (s) => (s.match(/\{[A-Za-z_][\w.]*\}/g) || []).sort();
    const problems = [];
    for (const key of enLeaves) {
      let enNode = en;
      let ruNode = ru;
      for (const part of key.split('.')) {
        enNode = enNode[part];
        ruNode = ruNode[part];
      }
      if (typeof enNode !== 'string' || typeof ruNode !== 'string') continue;
      const a = placeholders(enNode).join(',');
      const b = placeholders(ruNode).join(',');
      if (a !== b) problems.push(`${key}: en[${a}] vs ru[${b}]`);
    }
    expect(problems, `placeholder mismatches: ${problems.slice(0, 10).join(' | ')}`).toEqual([]);
  });
});
