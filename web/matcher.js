// Misread-tolerant script follower. Pure logic, no DOM: runs in browsers and Node.
//
// feed() takes the tail of the latest recognition hypothesis (partial or final).
// The position only moves forward, and feeding the same words twice is a no-op,
// so partial results that repeat or get revised are safe.

const STOPWORDS = new Set(
  "a an and are as at be but by for from had has have he her his i if in is it its me my of on or our she so that the their them then there they this to was we were what when which who will with you your".split(
    " "
  )
);

export function normalize(word) {
  return word
    .toLowerCase()
    .replace(/[’']/g, "")
    .replace(/[^\p{L}\p{N}]+/gu, "");
}

/** Split script text into display tokens; `spoken` tokens have a normalized form. */
export function tokenize(text) {
  const out = [];
  for (const raw of text.split(/\s+/)) {
    if (!raw) continue;
    const norm = normalize(raw);
    out.push({ raw, norm, spoken: norm.length > 0 });
  }
  return out;
}

function levenshtein(a, b) {
  if (a === b) return 0;
  const prev = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    let diag = prev[0];
    prev[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const tmp = prev[j];
      prev[j] = Math.min(prev[j] + 1, prev[j - 1] + 1, diag + (a[i - 1] === b[j - 1] ? 0 : 1));
      diag = tmp;
    }
  }
  return prev[b.length];
}

export function wordsMatch(heard, expected) {
  if (heard === expected) return true;
  const len = Math.max(heard.length, expected.length);
  if (len < 4) return false;
  // Partial results often cut a word short ("reconcil" for "reconciliation").
  if (heard.length >= 4 && expected.startsWith(heard)) return true;
  return 1 - levenshtein(heard, expected) / len >= 0.72;
}

export function createMatcher(scriptText, options = {}) {
  const { lookahead = 12, nearSkip = 3, tail = 4 } = options;
  const tokens = tokenize(scriptText);
  // Indices into `tokens` of spoken words; matching works in this word space.
  const spokenIdx = [];
  tokens.forEach((t, i) => t.spoken && spokenIdx.push(i));
  const words = spokenIdx.map((i) => tokens[i].norm);
  let pos = 0; // next spoken word we expect to hear

  function candidates(heard, k) {
    // Score each script position where heard[k] could be the word just spoken.
    const w = heard[k];
    const found = [];
    const end = Math.min(words.length, pos + lookahead + 1);
    for (let i = pos; i < end; i++) {
      if (!wordsMatch(w, words[i])) continue;
      const dist = i - pos;
      const stop = STOPWORDS.has(words[i]);
      // Context: preceding heard words that also line up with preceding script words.
      let ctx = 0;
      for (let b = 1; b <= k && b <= i; b++) {
        if (wordsMatch(heard[k - b], words[i - b])) ctx++;
        else break;
      }
      if (stop && dist > 0 && ctx === 0) continue; // "the" must not cause a jump
      if (dist > nearSkip && ctx === 0) continue; // far jumps need two words in a row
      found.push({ i, dist, ctx, score: ctx * 3 - dist - (stop ? 2 : 0) });
    }
    return found;
  }

  function feed(heardRaw) {
    const heard = heardRaw.map(normalize).filter(Boolean).slice(-tail);
    // Walk the tail left to right so a catch-up after a missed word lands on the last word.
    for (let k = 0; k < heard.length; k++) {
      const found = candidates(heard, k);
      if (!found.length) continue;
      found.sort((a, b) => b.score - a.score || a.dist - b.dist);
      pos = Math.max(pos, found[0].i + 1);
    }
    return pos;
  }

  return {
    tokens,
    words,
    feed,
    get pos() {
      return pos;
    },
    /** Display-token index of the next word to be spoken (for highlighting). */
    get tokenPos() {
      return pos < spokenIdx.length ? spokenIdx[pos] : tokens.length;
    },
    reset(p = 0) {
      pos = Math.max(0, Math.min(words.length, p));
    },
    /** Move to the spoken-word position nearest a display-token index (manual jump). */
    resetToToken(t) {
      const n = spokenIdx.findIndex((i) => i >= t);
      pos = n === -1 ? words.length : n;
    },
  };
}
