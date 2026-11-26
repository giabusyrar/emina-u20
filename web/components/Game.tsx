"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { SFX, haptic } from "@/lib/sfx";
import { FALLBACK_BG, ORDER, SCENES, STORY, asset, tr, type Beat, type Lang, type Scene } from "@/lib/story";
import { freshRun, type Run } from "@/lib/scoring";
import Actor, { type ActorHandle } from "./Actor";

const CANCEL = Symbol("cancel");
const TYPE_MS = 15; // typewriter speed per character
const MIN_VB = 360; // stage viewBox is 360 wide; height follows the stage aspect ratio

type Choices = { scene: Scene; picked: number | null; shown: boolean };
type Props = { lang: Lang; onEnding: (run: Run) => void };

export default function Game({ lang, onEnding }: Props) {
  const langRef = useRef(lang); langRef.current = lang;
  const T = (v: Parameters<typeof tr>[0]) => tr(v, langRef.current);
  const UI = STORY.ui;

  const [vbH, setVbH] = useState(440);
  const [bgs, setBgs] = useState([{ src: "", on: false }, { src: "", on: false }]);
  const [fb, setFb] = useState("#F1BCD5");
  const [chip, setChip] = useState<{ t: string; p: string } | null>(null);
  const [fade, setFade] = useState(false);
  const [who, setWho] = useState<{ name: string; color: string } | null>(null);
  const [text, setText] = useState("");
  const [textCls, setTextCls] = useState("");
  const [next, setNext] = useState(false);
  const [hint, setHint] = useState<{ on: boolean; tut: boolean }>({ on: false, tut: false });
  const [choices, setChoices] = useState<Choices | null>(null);

  const stageEl = useRef<HTMLDivElement>(null);
  const actor = useRef<ActorHandle>(null);
  const run = useRef<Run>(freshRun());
  const tok = useRef(0);
  const cur = useRef(0);
  const hinted = useRef(false);
  const typing = useRef<{ finish: () => void } | null>(null);
  const tapWait = useRef<(() => void) | null>(null);
  const pickWait = useRef<((i: number) => void) | null>(null);
  const vb = useRef(440);

  const sleep = useCallback((ms: number, t: number) => new Promise<void>((res, rej) => setTimeout(() => (tok.current === t ? res() : rej(CANCEL)), ms)), []);

  const showWho = (k: string) => {
    if (!k) return setWho(null);
    setWho(k === "kamu" ? { name: T(UI.kamu), color: "#DA1884" } : { name: k, color: "#9B9BB5" });
  };

  const type = (full: string, cls: string, t: number) => new Promise<void>((res, rej) => {
    setTextCls(cls); setText(""); setNext(false);
    let i = 0, timer = 0;
    const done = () => { typing.current = null; res(); };
    typing.current = { finish() { clearTimeout(timer); setText(full); done(); } };
    const step = () => {
      if (tok.current !== t) return rej(CANCEL);
      i += full.charCodeAt(i) === 32 ? 2 : 1; setText(full.slice(0, i)); if (i % 6 === 0) SFX.tick();
      if (i < full.length) timer = window.setTimeout(step, TYPE_MS); else done();
    };
    step();
  });

  const waitTap = (t: number) => new Promise<void>((res, rej) => {
    tapWait.current = () => { tapWait.current = null; tok.current === t ? res() : rej(CANCEL); };
  });

  async function playBeats(beats: Beat[], scene: Scene | null, t: number) {
    for (const b of beats) {
      showWho(b.who); await type(T(b), b.who ? "" : "narr", t);
      setNext(true);
      if (!hinted.current) setHint({ on: true, tut: !!scene?.tutorial });
      await waitTap(t); setNext(false);
    }
  }

  const tap = useCallback((e: React.SyntheticEvent) => {
    if ((e.target as HTMLElement).closest("button")) return;
    SFX.unlock();
    if (!hinted.current) { hinted.current = true; setHint((h) => ({ ...h, on: false })); }
    if (typing.current) { typing.current.finish(); setNext(true); return; }
    tapWait.current?.();
  }, []);

  // crossfade between the two stacked background images
  const setImage = (id: string) => {
    const nxt = 1 - cur.current;
    setFb(FALLBACK_BG[id] || "#CDBEF2");
    setBgs((b) => b.map((x, i) => (i === nxt ? { src: asset(SCENES[id].img || `art/${id}.jpg`), on: false } : x)));
  };
  const bgLoaded = (i: number) => { if (i !== 1 - cur.current) return; cur.current = i; setBgs((b) => b.map((x, j) => ({ ...x, on: j === i }))); };
  const bgFailed = (i: number) => { if (i !== 1 - cur.current) return; setBgs((b) => b.map((x, j) => (j === i ? { src: "", on: false } : { ...x, on: false }))); };
  const groundHidden = bgs.some((b) => b.on);

  async function enter(id: string, first: boolean, t: number) {
    const s = SCENES[id], floor = s.floor || 0.86;
    setChoices(null); showWho(""); setText(""); setNext(false);
    if (!first) { setFade(true); SFX.swoosh(); await sleep(480, t); }
    setImage(id); setChip({ t: s.chapter, p: T(s.place) });
    actor.current?.place(-0.14, floor, true);
    setFade(false); await sleep(first ? 500 : 380, t);
    await actor.current?.walkTo(0.42, floor);
    if (tok.current !== t) throw CANCEL;
    await playBeats(s.beats, s, t);
    await showChoices(s, t);
  }

  async function showChoices(s: Scene, t: number) {
    showWho(""); setText("");
    setChoices({ scene: s, picked: null, shown: false });
    await sleep(30, t); setChoices({ scene: s, picked: null, shown: true });
    const i = await new Promise<number>((res) => { pickWait.current = res; });
    if (tok.current !== t) throw CANCEL;
    await choose(s, i, t);
  }

  async function choose(s: Scene, i: number, t: number) {
    const o = s.options[i], floor = s.floor || 0.86;
    haptic(12); SFX.pick(); setChoices({ scene: s, picked: i, shown: true });
    await sleep(380, t); setChoices(null);
    if (s.options.length > 1) { showWho("kamu"); setTextCls("echo"); setText("› " + T(o.label)); }
    await actor.current?.walkTo(o.x == null ? 0.5 : o.x, floor, o.move); await actor.current?.act(o.move);
    if (tok.current !== t) throw CANCEL;
    const r = run.current;
    for (const k in o.type ?? {}) r.type[k] = (r.type[k] || 0) + o.type![k];
    for (const k in o.booth ?? {}) r.booth[k] = (r.booth[k] || 0) + o.booth![k];
    if (o.doubt) r.doubt = o.doubt;
    if (o.interest) r.interest = o.interest;
    r.trail.push({ node: s.id, i });
    if (o.react?.length) await playBeats(o.react, null, t);
    if (o.leadsTo === "ending") { setFade(true); await sleep(900, t); onEnding(r); return; }
    await enter(o.leadsTo, false, t);
  }

  // stage size → viewBox height
  useEffect(() => {
    const el = stageEl.current; if (!el) return;
    const fit = () => { const r = el.getBoundingClientRect(); if (r.width) { vb.current = Math.round((MIN_VB * r.height) / r.width); setVbH(vb.current); } };
    fit(); const ro = new ResizeObserver(fit); ro.observe(el); return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const t = ++tok.current; run.current = freshRun();
    enter(ORDER[0], true, t).catch((e) => { if (e !== CANCEL) throw e; });
    return () => { tok.current++; typing.current = null; tapWait.current = null; pickWait.current = null; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === " " || e.key === "Enter") { e.preventDefault(); if (!pickWait.current) tap({ target: document.body } as unknown as React.SyntheticEvent); }
      if (pickWait.current && /^[1-4]$/.test(e.key)) { const i = +e.key - 1; const n = choices?.scene.options.length ?? 0; if (i < n) { const p = pickWait.current; pickWait.current = null; p(i); } }
    };
    document.addEventListener("keydown", onKey); return () => document.removeEventListener("keydown", onKey);
  }, [tap, choices]);

  const pick = (i: number) => { const p = pickWait.current; if (!p) return; pickWait.current = null; p(i); };
  const s = choices?.scene, single = s?.options.length === 1;

  return (
    <section id="game" className="screen on">
      <div id="stage" ref={stageEl} style={{ ["--fb" as string]: fb }} onClick={tap}>
        <div id="ground" style={{ display: groundHidden ? "none" : "" }} />
        {bgs.map((b, i) => (
          // eslint-disable-next-line @next/next/no-img-element
          <img key={i} className={"bg" + (b.on ? " on" : "")} alt="" src={b.src || undefined} onLoad={() => bgLoaded(i)} onError={() => bgFailed(i)} />
        ))}
        <Actor ref={actor} vbH={vbH} />
        <div id="chip" className={chip ? "on" : ""}><b>{chip?.t}</b><span>{chip?.p}</span></div>
        <div id="fade" className={fade ? "on" : ""} />
      </div>
      <div id="panel" onClick={tap}>
        <div id="box">
          <span id="who" className={who ? "on" : ""} style={{ ["--who" as string]: who?.color }}>{who?.name}</span>
          <p id="txt" className={textCls}>{text}</p>
          <p id="hint" className={(hint.on ? "on " : "") + (hint.tut ? "tut" : "")}>{T(UI.tap)}</p>
          <svg id="next" className={next ? "on" : ""} viewBox="0 0 12 12"><path d="M1 3l5 6 5-6" fill="none" stroke="#DA1884" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" /></svg>
        </div>
        <div id="choices" className={(choices?.shown ? "on " : "") + (s?.tutorial ? "tut" : "")}>
          <p id="tip" className={s?.tutorial ? "on" : ""} dangerouslySetInnerHTML={{ __html: T(UI.tip) }} />
          <p id="ask">{s && T(s.prompt)}</p>
          <div id="optList">
            {s?.options.map((o, i) => (
              <button key={s.id + i} type="button" className={"opt" + (single ? " single" : "") + (choices!.shown ? " in" : "") + (choices!.picked == null ? "" : choices!.picked === i ? " pick" : " dim")}
                style={{ transitionDelay: choices!.shown && choices!.picked == null ? `${80 + i * 70}ms` : undefined }} onClick={() => pick(i)}>
                {T(o.label)}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
