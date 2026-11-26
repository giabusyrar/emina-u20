"use client";
import { STORY, asset, tr, type Lang } from "@/lib/story";

type Props = { lang: Lang; name: string; setName: (n: string) => void; onLang: () => void; onStart: () => void };

export default function Welcome({ lang, name, setName, onLang, onStart }: Props) {
  const w = STORY.welcome, T = (v: Parameters<typeof tr>[0]) => tr(v, lang);
  const title = T(STORY.theme).split(/(fearless\??)/i);
  return (
    <section id="welcome" className="screen on">
      <div className="w-top">
        <img className="w-emina" src={asset("art/kv/emina.png")} alt="Emina" />
        <button className="lang" type="button" onClick={onLang}>{lang === "id" ? "EN" : "ID"}</button>
      </div>
      <div className="w-hero"><img src={asset("art/kv/hero.jpg")} alt="" /></div>
      <div className="w-kicker">{w.kicker as string}</div>
      <h1>{title.map((p, i) => (i % 2 ? <em key={i}>{p}</em> : p))}</h1>
      <p className="w-intro">{T(STORY.intro)}</p>
      <p className="w-lead">{T(w.lead)}</p>
      <label className="w-name">
        <span>{T(w.namePrompt)}</span>
        <input maxLength={24} autoComplete="given-name" placeholder={T(w.namePh)} value={name} onChange={(e) => setName(e.target.value)} />
      </label>
      <button id="startBtn" type="button" onClick={onStart}>{T(w.cta)}</button>
      <p className="w-fine">{T(w.fine)}</p>
    </section>
  );
}
