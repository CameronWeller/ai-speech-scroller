import test from "node:test";
import assert from "node:assert/strict";
import { createMatcher, wordsMatch } from "../web/matcher.js";

const SCRIPT =
  "Good evening everyone. Tonight we are talking about the history of the telescope, " +
  "and how a simple lens changed astronomy forever. Galileo built his first instrument in 1609.";

test("advances on exact words", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening", "everyone"]);
  assert.equal(m.pos, 3);
});

test("repeated and revised partials are idempotent", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening"]);
  m.feed(["good", "evening"]);
  m.feed(["good", "evening", "everyone"]);
  assert.equal(m.pos, 3);
});

test("skips one or two missed words and catches up", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening", "everyone"]);
  // "tonight we are" missed; recogniser hears "talking about"
  m.feed(["talking", "about"]);
  assert.equal(m.words[m.pos - 1], "about");
});

test("tolerates misrecognised words via fuzzy match", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening", "everyone", "tonight", "we", "are", "talking", "about", "the", "history", "of", "the"]);
  m.feed(["telescopes"]);
  assert.equal(m.words[m.pos - 1], "telescope");
});

test("prefix of a word still being spoken counts", () => {
  assert.ok(wordsMatch("astron", "astronomy"));
  assert.ok(!wordsMatch("as", "astronomy"));
});

test("never moves backward", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening", "everyone", "tonight", "we", "are"]);
  const p = m.pos;
  m.feed(["good", "evening"]);
  assert.equal(m.pos, p);
});

test("a lone stopword far ahead does not cause a jump", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good"]);
  m.feed(["the"]);
  assert.equal(m.pos, 1);
});

test("a single unrelated far word is ignored; two in a row jump", () => {
  const m = createMatcher(SCRIPT);
  m.feed(["good", "evening"]);
  m.feed(["galileo"]); // far ahead, no context
  assert.equal(m.pos, 2);
  const m2 = createMatcher("one two three four five six seven eight nine ten eleven twelve alpha bravo charlie");
  m2.feed(["twelve", "alpha"]);
  assert.equal(m2.pos, 13);
});

test("punctuation-only tokens are not spoken words", () => {
  const m = createMatcher("Hello — world");
  assert.deepEqual(m.words, ["hello", "world"]);
  m.feed(["hello"]);
  assert.equal(m.tokenPos, 2);
});

test("stage directions are not spoken", () => {
  const m = createMatcher("[A-cam] CLOSE\n\nAnd it costs less.\n\n→ GFX 9 · Price\n\nTry it now.");
  assert.deepEqual(m.words, ["and", "it", "costs", "less", "try", "it", "now"]);
  m.feed(["less", "try"]);
  assert.equal(m.words[m.pos - 1], "try");
});
