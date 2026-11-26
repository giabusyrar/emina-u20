"use client";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { SFX } from "@/lib/sfx";
import { POSES, asset } from "@/lib/story";

/** Pose sheet order (art/char/poses.json): idle, walk A, walk B, run, sit, listen, phone, call, write, tidy, wave, mic, look, talk, torch, paint. */
const POSE: Record<string, number> = { idle: 0, walkA: 1, walkB: 2, run: 3, sit: 4, listen: 5, craft: 4, phone: 6, call: 7, write: 8, tidy: 9, wave: 10, mic: 11, look: 12, talk: 13, torch: 14, paint: 15, breathe: 0, climb: 0 };
const SPR_H = 62; // sprite height in stage units (viewBox is 360 wide)
const SCALE = 1.55;
const STAGE_W = 360;
const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export type ActorHandle = {
  place(xf: number, yf: number, instant?: boolean): void;
  walkTo(xf: number, yf: number, manner?: string): Promise<void>;
  act(manner?: string): Promise<void>;
};

// Static, trusted markup: the emote bubble glyphs (CSS in globals.css switches one on per action).
const EMOTES = `<rect x="-14" y="-12" width="28" height="24" rx="9" fill="#fff" stroke="#FFB0D4" stroke-width="1.3"/><path d="M-8 11 L-10 17 L-3 11.5" fill="#fff" stroke="#FFB0D4" stroke-width="1.3" stroke-linejoin="round"/>
<g id="emTalk" class="em"><circle class="d1" cx="-6" cy="0" r="2.1" fill="#43283A"/><circle class="d2" cx="0" cy="0" r="2.1" fill="#43283A"/><circle class="d3" cx="6" cy="0" r="2.1" fill="#43283A"/></g>
<g id="emNote" class="em"><path d="M-2 5 V-6 L6 -8 V3" stroke="#6B7FD7" stroke-width="2.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/><circle cx="-4" cy="5" r="2.8" fill="#6B7FD7"/><circle cx="4" cy="3" r="2.8" fill="#6B7FD7"/></g>
<g id="emHeart" class="em"><path d="M0 6 C-2 3 -8 0 -8 -4 C-8 -7 -5 -9 -2 -7 L0 -5 L2 -7 C5 -9 8 -7 8 -4 C8 0 2 3 0 6Z" fill="#FF4D97"/></g>
<g id="emBang" class="em"><rect x="-1.8" y="-8" width="3.6" height="10" rx="1.8" fill="#FF4D97"/><circle cx="0" cy="6" r="2.1" fill="#FF4D97"/></g>
<g id="emQ" class="em"><path d="M-4 -4 Q-4 -8 0 -8 Q4 -8 4 -4 Q4 -1 0 0 V2" stroke="#8A4FD6" stroke-width="2.3" fill="none" stroke-linecap="round"/><circle cx="0" cy="6.5" r="1.8" fill="#8A4FD6"/></g>
<g id="emSpark" class="em"><path d="M-5 -2 l1.6 3.6 3.6 1.6 -3.6 1.6 -1.6 3.6 -1.6 -3.6 -3.6 -1.6 3.6 -1.6Z" fill="#FFCE57"/><path d="M5 -7 l1.2 2.6 2.6 1.2 -2.6 1.2 -1.2 2.6 -1.2 -2.6 -2.6 -1.2 2.6 -1.2Z" fill="#FF8FC0"/></g>
<g id="emRing" class="em"><path d="M-6 -1 Q-6 -8 0 -8" stroke="#17B6A6" stroke-width="2.2" fill="none" stroke-linecap="round"/><path d="M-2 2 Q-2 -4 3 -4" stroke="#17B6A6" stroke-width="2.2" fill="none" stroke-linecap="round"/><circle cx="4" cy="4" r="2.6" fill="#17B6A6"/></g>
<g id="emPen" class="em"><path d="M-8 3 L-4 -4 L0 3 L4 -4 L8 3" stroke="#43283A" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></g>
<g id="emFlower" class="em"><g fill="#FF9DB4"><circle cx="0" cy="-5" r="3"/><circle cx="4.5" cy="-1.5" r="3"/><circle cx="3" cy="4" r="3"/><circle cx="-3" cy="4" r="3"/><circle cx="-4.5" cy="-1.5" r="3"/></g><circle r="2.4" fill="#FFD86B"/></g>
<g id="emZ" class="em"><circle cx="-5" cy="4" r="2" fill="#8FD0FF"/><circle cx="0" cy="0" r="2.6" fill="#8FD0FF"/><circle cx="6" cy="-5" r="3.2" fill="#8FD0FF"/></g>
<g id="emWave" class="em"><path d="M-7 -4 Q-3 0 -7 4M-1 -7 Q4 0 -1 7M5 -9 Q11 0 5 9" stroke="#FF4D97" stroke-width="2.2" fill="none" stroke-linecap="round"/></g>`;

async function oneShot(el: Element | null, frames: Keyframe[], ms: number, opts?: KeyframeAnimationOptions) {
  if (!el || !el.animate) return;
  try {
    const an = el.animate(frames, { duration: ms, easing: "cubic-bezier(.34,1.56,.64,1)", ...opts });
    await Promise.race([an.finished.catch(() => {}), wait(ms * (opts?.iterations ?? 1) + 150)]);
  } catch { /* animation unsupported: skip the flourish */ }
}

const Actor = forwardRef<ActorHandle, { vbH: number }>(function Actor({ vbH }, ref) {
  const [pose, setPose] = useState(0);
  const [action, setAction] = useState<string | null>(null);
  const actorEl = useRef<SVGGElement>(null);
  const squashEl = useRef<SVGGElement>(null);
  const x = useRef(-0.14);
  const y = useRef(0);
  const vb = useRef(vbH); vb.current = vbH;
  const [state, setState] = useState({ walking: false, run: false });

  const place = useCallback((xf: number, yf: number, instant?: boolean) => {
    const a = actorEl.current; if (!a) return;
    if (instant) a.style.transition = "none";
    if (instant) { setAction(null); setPose(0); } // a new scene starts from idle, not the last scene's action
    a.style.transform = `translate(${xf * STAGE_W}px,${yf * vb.current}px) scale(${SCALE})`; x.current = xf; y.current = yf;
    if (instant) { void a.getBoundingClientRect(); a.style.transition = ""; }
  }, []);

  useEffect(() => { if (y.current) place(x.current, y.current, true); }, [vbH, place]); // keep her on the floor line when the stage resizes

  useImperativeHandle(ref, () => ({
    place,
    async walkTo(xf, yf, manner) {
      const a = actorEl.current; if (!a) return;
      const dist = Math.abs(xf - x.current) * STAGE_W, isRun = manner === "run";
      setAction(null); setPose(0);
      if (dist > 6) await oneShot(squashEl.current, [{ transform: "scaleY(1) scaleX(1)" }, { transform: "scaleY(.93) scaleX(1.04)" }, { transform: "scaleY(1.015) scaleX(.99)" }, { transform: "none" }], 160, { easing: "ease-out" });
      const dur = Math.max(isRun ? 240 : 380, Math.min(1500, dist * (isRun ? 3.4 : 6)));
      a.style.transitionDuration = dur + "ms"; setState({ walking: true, run: isRun });
      let i = 0; setPose(isRun ? 3 : 1);
      const iv = setInterval(() => { SFX.step(i++ % 2); if (!isRun) setPose(i % 2 ? 2 : 1); }, isRun ? 120 : 210);
      place(xf, yf); await wait(dur); clearInterval(iv);
      setState({ walking: false, run: false }); a.style.transitionDuration = ""; setPose(0);
      if (dist > 6) await oneShot(squashEl.current, [{ transform: "scaleY(.9) scaleX(1.08)" }, { transform: "scaleY(1.04) scaleX(.97)" }, { transform: "none" }], 300);
    },
    async act(manner) {
      if (!manner || manner === "walk" || manner === "run") return;
      if (manner === "climb") return oneShot(squashEl.current, [{ transform: "translateY(0)" }, { transform: "translateY(-16px) scaleY(1.06)" }, { transform: "translateY(0) scaleY(.9) scaleX(1.08)" }, { transform: "none" }], 520, { iterations: 2, easing: "ease-in-out" });
      setAction(manner); setPose(POSE[manner] ?? 0); SFX.step(1);
      void oneShot(squashEl.current, [{ transform: "scaleY(.9) scaleX(1.06)" }, { transform: "scaleY(1.03) scaleX(.98)" }, { transform: "none" }], 260);
      await wait(manner === "breathe" ? 1000 : 380);
    },
  }), [place]);

  const p = POSES[pose] ?? POSES[0], k = SPR_H / POSES[0].h, w = p.w * k, h = p.h * k;
  const cls = [state.walking && "walking", state.run && "run"].filter(Boolean).join(" ");
  return (
    <svg id="actorLayer" className={cls} viewBox={`0 0 ${STAGE_W} ${vbH}`} xmlns="http://www.w3.org/2000/svg">
      <g id="actor" ref={actorEl} className={"sprite" + (action === "breathe" ? " act-breathe" : "")}>
        <ellipse id="shadow" cx="0" cy="2" rx="21" ry="5.5" fill="#43283A" opacity=".16" />
        <g id="lean"><g id="squash" ref={squashEl}><g id="actorBob">
          <image id="sprite" href={asset(p.src)} x={-w / 2} y={-h} width={w} height={h} preserveAspectRatio="xMidYMax meet" />
          <g transform={`translate(20,${-(SPR_H + 16)})`}>
            <g id="emoteWrap" className={action ? "act-" + action : ""}>
              <g id="emote" dangerouslySetInnerHTML={{ __html: EMOTES }} />
            </g>
          </g>
        </g></g></g>
      </g>
    </svg>
  );
});
export default Actor;
