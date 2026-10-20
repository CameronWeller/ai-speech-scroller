import { tokenize } from "./matcher.js";

const $text = document.getElementById("text");
const $view = document.getElementById("viewport");
let spans = [];
let tokenPos = 0;
let offset = 0;      // current scroll offset in px (eased)
let target = 0;
let guide = 0.38;    // reading line as a fraction of viewport height

function setScript(text) {
  $text.textContent = "";
  spans = tokenize(text).map((t, i) => {
    if (i > 0) for (let k = 0; k < Math.min(t.br, 2); k++) $text.appendChild(document.createElement("br"));
    const s = document.createElement("span");
    s.className = "w" + (t.cue ? " cue" : "");
    s.textContent = t.raw + " ";
    $text.appendChild(s);
    return s;
  });
  tokenPos = 0;
  paint();
}

function paint() {
  spans.forEach((s, i) => {
    s.classList.toggle("done", i < tokenPos);
    s.classList.toggle("now", i === tokenPos);
  });
  const s = spans[Math.min(tokenPos, spans.length - 1)];
  if (!s) return;
  const lineTop = s.offsetTop;
  target = lineTop - $view.clientHeight * guide;
}

function tick() {
  offset += (target - offset) * 0.07;
  $text.style.transform = `translate(-50%, ${-offset}px)`;
  requestAnimationFrame(tick);
}

function applyStyle(m) {
  const root = document.documentElement.style;
  if (m.fontSize) root.setProperty("--size", m.fontSize + "px");
  if (m.column) root.setProperty("--col", m.column + "%");
  if (m.guide != null) { guide = m.guide; root.setProperty("--guide", guide * 100 + "%"); }
  document.body.classList.toggle("mirror", !!m.mirror);
  paint();
}

const es = new EventSource("/events");
es.onmessage = (e) => {
  const m = JSON.parse(e.data);
  if (m.type === "script") setScript(m.text);
  else if (m.type === "pos") { tokenPos = m.tokenPos; paint(); }
  else if (m.type === "style") applyStyle(m);
};
window.addEventListener("resize", paint);
tick();

// Fullscreen on whichever screen this window is on. In fullscreen, lock Esc so it
// opens the menu instead of leaving fullscreen.
const $menu = document.getElementById("menu");
const send = (m) => fetch("/msg", { method: "POST", body: JSON.stringify(m) }).catch(() => {});
const toggleFull = () =>
  document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen().catch(() => {});
document.addEventListener("fullscreenchange", () => {
  if (document.fullscreenElement) navigator.keyboard?.lock?.(["Escape"]).catch(() => {});
  else navigator.keyboard?.unlock?.();
});
window.addEventListener("click", (e) => { if (!$menu.contains(e.target) && !document.fullscreenElement) toggleFull(); });
window.addEventListener("keydown", (e) => {
  if (e.key === "Escape") { $menu.hidden = !$menu.hidden; e.preventDefault(); }
  else if (e.key === "f") toggleFull();
});
$menu.addEventListener("click", (e) => {
  const a = e.target.dataset?.a;
  if (!a) return;
  if (a === "fullscreen") toggleFull();
  else send({ type: "cmd", action: a, n: Date.now() });
});
