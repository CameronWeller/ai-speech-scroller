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
  spans = tokenize(text).map((t) => {
    const s = document.createElement("span");
    s.className = "w";
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
