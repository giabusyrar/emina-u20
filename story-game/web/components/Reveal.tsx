"use client";
import { useEffect, useState } from "react";
import { SFX } from "@/lib/sfx";
import { STORY, tr, type Lang } from "@/lib/story";

// Reveal ceremony steps: 1 flame, 2 colour flood, 3 kicker + icon + letters, 4 description, 5 tagline, 6 button.
export default function Reveal({ lang, win, onGo }: { lang: Lang; win: string; onGo: () => void }) {
  const a = STORY.types[win], T = (v: Parameters<typeof tr>[0]) => tr(v, lang);
  const [step, setStep] = useState(0);
  const [letters, setLetters] = useState(0);
  const total = a.name.replace(/ /g, "").length;

  useEffect(() => {
    let dead = false;
    const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));
    (async () => {
      await sleep(400); if (dead) return; setStep(1); SFX.lamp();
      await sleep(900); if (dead) return; setStep(2); SFX.chord();
      await sleep(520); if (dead) return; setStep(3);
      for (let i = 1; i <= total; i++) { if (dead) return; setLetters(i); await sleep(26); }
      setStep(4); await sleep(300); if (dead) return; setStep(5);
      await sleep(800); if (dead) return; setStep(6);
    })();
    return () => { dead = true; };
  }, [total]);

  const on = (n: number) => (step >= n ? " on" : "");
  let seen = 0;
  return (
    <div id="reveal" className="on" style={{ ["--c1" as string]: a.c1, ["--c2" as string]: a.c2 }}>
      <div id="flood" className={on(2).trim()} />
      <div id="flame" className={on(1).trim()} />
      <div id="rvKick" className={on(3).trim()}>{tr(STORY.ui.youAre, lang)}</div>
      <div id="rvIcon" className={on(3).trim()}>{a.icon}</div>
      <h2 id="rvType">
        {a.name.split(" ").map((wd, wi, arr) => (
          <span key={wi}>
            <span className="w">{[...wd].map((ch, ci) => <span key={ci} className={++seen <= letters ? "on" : ""}>{ch}</span>)}</span>
            {wi < arr.length - 1 && " "}
          </span>
        ))}
      </h2>
      <div id="rvEn" className={on(4).trim()}>{T(a.desc)}</div>
      <p id="rvLine" className={on(5).trim()}>{T(a.tag)}</p>
      <button id="rvGo" type="button" className={on(6).trim()} onClick={onGo}>{T(STORY.ui.seeCard)} ✨</button>
    </div>
  );
}
