import { createMatcher } from "./matcher.js";

const $ = (id) => document.getElementById(id);
const send = (m) => fetch("/msg", { method: "POST", body: JSON.stringify(m) }).catch(() => {});
let matcher = createMatcher("");
let model = null, ctx = null, stream = null, recognizer = null, node = null;

function setStatus(s) { $("status").textContent = s; }
function pushPos() { send({ type: "pos", tokenPos: matcher.tokenPos }); }
function pushStyle() {
  send({ type: "style", fontSize: +$("size").value, column: +$("column").value,
         guide: +$("guide").value / 100, mirror: $("mirror").checked });
  localStorage.setItem("style", JSON.stringify({ s: $("size").value, c: $("column").value, g: $("guide").value, m: $("mirror").checked }));
}

function loadScript() {
  const text = $("script").value;
  localStorage.setItem("script", text);
  matcher = createMatcher(text);
  send({ type: "script", text });
  pushPos();
}

function heard(words, final) {
  if (!words.length) return;
  $("heard").textContent = words.join(" ") + (final ? "" : " …");
  const before = matcher.pos;
  matcher.feed(words);
  if (matcher.pos !== before) pushPos();
}

async function listMics() {
  // Labels are blank until the page has mic permission, so ask once, then list.
  try { (await navigator.mediaDevices.getUserMedia({ audio: true })).getTracks().forEach((t) => t.stop()); } catch {}
  const devs = (await navigator.mediaDevices.enumerateDevices()).filter((d) => d.kind === "audioinput" && d.deviceId !== "communications");
  const saved = localStorage.getItem("mic");
  $("mic").innerHTML = "";
  devs.forEach((d, i) => $("mic").add(new Option(d.label || `Input ${i + 1}`, d.deviceId)));
  if (devs.some((d) => d.deviceId === saved)) $("mic").value = saved;
}
$("mic").onchange = async () => {
  localStorage.setItem("mic", $("mic").value);
  if (stream) { stopListening(); await startListening().catch((e) => setStatus(String(e))); } // switch live
};
navigator.mediaDevices.addEventListener?.("devicechange", listMics);

async function startListening() {
  if (!window.Vosk) { setStatus("Speech model missing: run scripts/fetch-assets.sh and rebuild"); return; }
  setStatus("Loading speech model…");
  model ??= await Vosk.createModel("vendor/model.tar.gz");
  stream = await navigator.mediaDevices.getUserMedia({
    audio: { deviceId: $("mic").value ? { exact: $("mic").value } : undefined,
             echoCancellation: true, noiseSuppression: true, channelCount: 1 },
  });
  ctx = new AudioContext();
  recognizer = new model.KaldiRecognizer(ctx.sampleRate);
  recognizer.on("partialresult", (m) => heard((m.result.partial || "").split(/\s+/).filter(Boolean), false));
  recognizer.on("result", (m) => heard((m.result.text || "").split(/\s+/).filter(Boolean), true));
  node = ctx.createScriptProcessor(4096, 1, 1);
  node.onaudioprocess = (e) => { try { recognizer.acceptWaveform(e.inputBuffer); } catch {} };
  const src = ctx.createMediaStreamSource(stream);
  src.connect(node); node.connect(ctx.destination);
  $("listen").textContent = "Stop listening";
  setStatus("Listening");
}

function stopListening() {
  node?.disconnect(); stream?.getTracks().forEach((t) => t.stop()); ctx?.close();
  recognizer?.remove?.(); node = stream = ctx = recognizer = null;
  $("listen").textContent = "Start listening";
  setStatus("Idle");
}

$("load").onclick = loadScript;
$("listen").onclick = () => (stream ? stopListening() : startListening().catch((e) => { setStatus(String(e)); stopListening(); }));
$("restart").onclick = () => { matcher.reset(0); pushPos(); };
$("back").onclick = () => { matcher.reset(matcher.pos - 5); pushPos(); };
$("fwd").onclick = () => { matcher.reset(matcher.pos + 5); pushPos(); };
["size", "column", "guide", "mirror"].forEach((id) => ($(id).oninput = pushStyle));

try {
  $("script").value = localStorage.getItem("script") || "";
  const s = JSON.parse(localStorage.getItem("style") || "null");
  if (s) { $("size").value = s.s; $("column").value = s.c; $("guide").value = s.g; $("mirror").checked = s.m; }
} catch {}
let screens = null;
async function loadScreens() {
  screens ??= await getScreenDetails();
  const list = screens.screens;
  $("screen").innerHTML = "";
  list.forEach((sc, i) => $("screen").add(new Option(`${sc.label || "Screen " + (i + 1)} (${sc.width}x${sc.height})`, i)));
  const idx = list.findIndex((sc) => sc.label === localStorage.getItem("screen"));
  if (idx >= 0) $("screen").value = idx;
  return list;
}
async function openPrompter() {
  if (!window.getScreenDetails) { setStatus("This browser has no window-management API; use Chrome"); return; }
  try {
    const list = await loadScreens(); // first click prompts for permission
    const sc = list[$("screen").value];
    localStorage.setItem("screen", sc.label);
    const w = window.open("/display.html", "prompter",
      `popup,left=${sc.availLeft},top=${sc.availTop},width=${sc.availWidth},height=${sc.availHeight}`);
    w?.addEventListener("load", () => w.document.documentElement.requestFullscreen({ screen: sc }).catch(() => {}));
    setStatus("Prompter opened. Click it or press F for fullscreen, Esc for the menu");
  } catch (e) { setStatus("Allow window management for this page, then press Open prompter again (" + e.name + ")"); }
}
$("open").onclick = openPrompter;
$("screen").onchange = () => { if (screens) localStorage.setItem("screen", screens.screens[$("screen").value].label); };
// Fill the screen list now if permission was already granted; otherwise wait for the button.
navigator.permissions?.query({ name: "window-management" }).then((p) => p.state === "granted" && loadScreens()).catch(() => {});

new EventSource("/events").addEventListener("message", (e) => {
  const m = JSON.parse(e.data);
  if (m.type !== "cmd") return;
  const bump = (id, d) => { const el = $(id); el.value = Math.max(+el.min, Math.min(+el.max, +el.value + d)); pushStyle(); };
  ({
    restart: () => $("restart").click(), back: () => $("back").click(), fwd: () => $("fwd").click(),
    smaller: () => bump("size", -8), bigger: () => bump("size", 8),
    mirror: () => { $("mirror").checked = !$("mirror").checked; pushStyle(); },
    listen: () => $("listen").click(),
  })[m.action]?.();
});
listMics();
pushStyle();
if ($("script").value) loadScript();
