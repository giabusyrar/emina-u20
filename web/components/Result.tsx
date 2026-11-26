"use client";
import { useRef, useState } from "react";
import { drawCard } from "@/lib/card";
import { EVENT, POSES, STORY, asset, fmt, tr, type Lang } from "@/lib/story";
import type { Outcome, Run } from "@/lib/scoring";

type Props = { lang: Lang; name: string; R: Outcome; run: Run; onLang: () => void; onAgain: () => void };

export default function Result({ lang, name, R, run, onLang, onAgain }: Props) {
  const T = (v: Parameters<typeof tr>[0]) => tr(v, lang);
  const UI = STORY.ui, a = STORY.types[R.win], B = STORY.booths;
  const [page, setPage] = useState<1 | 2>(1);
  const [modal, setModal] = useState(false);
  const [img, setImg] = useState("");
  const [ready, setReady] = useState(false); // card fully drawn: Save is safe
  const canvas = useRef<HTMLCanvasElement>(null);
  const L = (k: string) => T(UI[k]);

  const openShare = async () => {
    setImg(""); setReady(false); setModal(true);
    if (!canvas.current) return;
    await drawCard(canvas.current, page, lang, R, name);
    setReady(true);
    try { setImg(canvas.current.toDataURL("image/png")); } catch { /* tainted canvas: card still saves via blob */ }
  };
  const save = () => {
    const file = `emina-explorer-${R.win.toLowerCase()}-p${page}.png`;
    const text = `${fmt(L("shareText"), { type: a.name })} ${EVENT.hashtag}`;
    canvas.current?.toBlob(async (b) => {
      if (!b) return;
      try {
        const f = new File([b], file, { type: "image/png" });
        if (navigator.canShare?.({ files: [f] })) { await navigator.share({ files: [f], title: L("shareTitle"), text }); return; }
      } catch (e) { if ((e as Error)?.name === "AbortError") return; }
      const u = URL.createObjectURL(b), l = document.createElement("a");
      l.href = u; l.download = file; document.body.appendChild(l); l.click(); l.remove(); setTimeout(() => URL.revokeObjectURL(u), 1500);
    }, "image/png");
  };
  const hi = `${L("hi")}, ${name ? name + "! " : ""}${a.name} 👋`;

  return (
    <>
    <div id="result" className="on">
      <div className="r-top">
        <img className="w-emina" src={asset("art/kv/emina.png")} alt="Emina" />
        <span className="camp">{L("campaign")}</span>
        <button className="lang" type="button" onClick={onLang}>{lang === "id" ? "EN" : "ID"}</button>
      </div>
      <div className="tabs">
        <button className={"tab" + (page === 1 ? " on" : "")} type="button" onClick={() => setPage(1)}>{L("page1")}</button>
        <button className={"tab" + (page === 2 ? " on" : "")} type="button" onClick={() => setPage(2)}>{L("page2")}</button>
      </div>

      <section className={"pg" + (page === 1 ? " on" : "")}>
        <div className="card" style={{ background: `linear-gradient(160deg,#fff 0%,${a.soft} 40%,${a.c2} 100%)` }}>
          <p className="lbl">{L("identity")}</p>
          <div className="c-head">
            <div><p className="c-kick">{L("youAre")}</p><h1 className="c-name">{a.name} {a.icon}</h1></div>
            <div className="c-m"><img src={asset(POSES[0].src)} alt="" /></div>
          </div>
          <p className="c-tag">{T(a.tag)}</p>
          <div className="hr" />
          <p className="lbl">{L("sparks")}</p><div className="chips">{a.sparks.map((s) => <span key={s} className="chip">{s}</span>)}</div>
          <p className="lbl" style={{ marginTop: 12 }}>{L("universe")}</p><p className="c-line">{R.booths.map((b) => B[b].name).join(" · ")}</p>
          <div className="hr" />
          <p className="lbl">{L("energy")}</p><p className="c-line">{T(a.energy)}</p>
          <p className="lbl" style={{ marginTop: 12 }}>{L("besties")}</p><div className="chips">{a.besties.map((b) => <span key={b} className="chip">{STORY.types[b].name}</span>)}</div>
          <div className="hr" />
          <p className="lbl">{L("note")}</p><p className="c-note">{run.doubt ? T(STORY.doubts[run.doubt]) : ""}</p>
        </div>
      </section>

      <section className={"pg" + (page === 2 ? " on" : "")}>
        <div className="card" style={{ background: "linear-gradient(160deg,#fff 0%,var(--pinkSoft) 45%,var(--pink) 100%)" }}>
          <h2 className="g-hi">{hi}</h2>
          <span className="g-sub">{L("starts")}</span>
          <div className="ev"><b>{EVENT.name}</b>
            <div className="meta">
              <div><span>{L("date")}</span>{EVENT.date}</div><div><span>{L("loc")}</span>{EVENT.location}</div><div><span>{L("time")}</span>{EVENT.time}</div>
            </div>
          </div>
          <p className="g-based" dangerouslySetInnerHTML={{ __html: L("based") }} />
          <p className="lbl" style={{ marginTop: 14 }}>{L("booths")}</p>
          <ol className="ol">{R.booths.map((b) => <li key={b}><b>{B[b].name}</b><small>{T(B[b].tag)}</small></li>)}</ol>
          <p className="lbl" style={{ marginTop: 14 }}>{L("stages")}</p>
          <ol className="ol">{R.picks.map((p) => <li key={p}><b>{EVENT.stages[p].title}</b><small>{EVENT.stages[p].speaker}</small></li>)}</ol>
          <p className="g-free">{L("free")}</p>
        </div>
      </section>

      <div className="btns">
        <button className="btn" type="button" onClick={openShare}>{L("save")} 📲</button>
        <div className="row">
          <button className="btn ghost" type="button" onClick={onAgain}>{L("again")} ↩</button>
          <button className="btn ghost" type="button" onClick={() => window.open(EVENT.register, "_blank", "noopener")}>{L("reg")} ✦</button>
        </div>
      </div>
      <p className="fine">{fmt(L("ofTypes"), { n: String(STORY.typeOrder.length) })} · {run.interest ? STORY.interests[run.interest] : ""}</p>
    </div>

      <div id="modal" className={modal ? "on" : ""} onClick={(e) => e.target === e.currentTarget && setModal(false)}>
        <div className="in">
          <button className="mx" type="button" aria-label={L("close")} onClick={() => setModal(false)}>&#10005;</button>
          <img id="cardImg" alt="Story card" src={img || undefined} />
          <div className="btns" style={{ marginTop: 12 }}><button className="btn" type="button" disabled={!ready} onClick={save}>{L("save")} 💾</button></div>
          <p className="fine">{lang === "id" ? "Tekan & tahan gambarnya buat simpan" : "Press and hold the image to save"}</p>
        </div>
      </div>
      <canvas ref={canvas} width={1080} height={1920} className="sr" aria-hidden />
    </>
  );
}
