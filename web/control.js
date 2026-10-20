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
  try { (await navigator.mediaDevices.getUserMedia({ audio: true })).getTracks().forEach((t) => t.stop()); } catch {}
  const devs = (await navigator.mediaDevices.enumerateDevices()).filter((d) => d.kind === "audioinput");
  $("mic").innerHTML = "";
  devs.forEach((d) => $("mic").add(new Option(d.label || d.deviceId, d.deviceId)));
}

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
listMics();
pushStyle();
if ($("script").value) loadScript();
