"use client";
import { useEffect, useState } from "react";
import { SFX, haptic } from "@/lib/sfx";
import { EVENT, STORY, type Lang } from "@/lib/story";
import { outcome, type Outcome, type Run } from "@/lib/scoring";
import Welcome from "./Welcome";
import Game from "./Game";
import Reveal from "./Reveal";
import Result from "./Result";

type Screen = "welcome" | "game" | "reveal" | "result";
const LANG_KEY = "eq_lang";

export default function Quiz() {
  const [screen, setScreen] = useState<Screen>("welcome");
  const [lang, setLang] = useState<Lang>("id");
  const [name, setName] = useState("");
  const [run, setRun] = useState<Run | null>(null);
  const [result, setResult] = useState<Outcome | null>(null);
  const [round, setRound] = useState(0); // remounts <Game> on replay

  useEffect(() => {
    try { const l = localStorage.getItem(LANG_KEY); if (l === "en" || l === "id") setLang(l); } catch { /* storage blocked */ }
  }, []);
  const toggleLang = () => {
    const l: Lang = lang === "id" ? "en" : "id"; setLang(l);
    try { localStorage.setItem(LANG_KEY, l); } catch { /* storage blocked */ }
  };
  const start = () => { SFX.unlock(); haptic(10); setRound((r) => r + 1); setScreen("game"); };
  const finish = (r: Run) => { setRun(r); setResult(outcome(STORY, EVENT, r)); setScreen("reveal"); };

  return (
    <div id="app">
      {screen === "welcome" && <Welcome lang={lang} name={name} setName={setName} onLang={toggleLang} onStart={start} />}
      {screen === "game" && <Game key={round} lang={lang} onEnding={finish} />}
      {screen === "reveal" && result && <Reveal lang={lang} win={result.win} onGo={() => setScreen("result")} />}
      {screen === "result" && result && run && (
        <Result lang={lang} name={name.trim().slice(0, 24)} R={result} run={run} onLang={toggleLang} onAgain={start} />
      )}
    </div>
  );
}
