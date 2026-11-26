import { EVENT, POSES, STORY, asset, tr, type Lang } from "./story";
import type { Outcome } from "./scoring";

const W = 1080, H = 1920;
const SANS = '"Plus Jakarta Sans",sans-serif', SERIF = "Fraunces,serif";

function rr(c: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  c.beginPath();
  c.roundRect ? c.roundRect(x, y, w, h, r) : (c.moveTo(x + r, y), c.arcTo(x + w, y, x + w, y + h, r), c.arcTo(x + w, y + h, x, y + h, r), c.arcTo(x, y + h, x, y, r), c.arcTo(x, y, x + w, y, r), c.closePath());
}
const loadImg = (src: string) => new Promise<HTMLImageElement | null>((res) => {
  if (!src) return res(null);
  const i = new Image(); i.onload = () => res(i); i.onerror = () => res(null); i.src = src;
});

/** Draws a 1080x1920 share card (page 1 = identity, 2 = Summit guide) and returns it as a PNG data URL. */
export async function drawCard(canvas: HTMLCanvasElement, page: 1 | 2, lang: Lang, R: Outcome, name: string): Promise<void> {
  const cx = canvas.getContext("2d")!;
  const T = (v: Parameters<typeof tr>[0]) => tr(v, lang);
  const UI = STORY.ui, a = STORY.types[R.win], BOOTHS = STORY.booths;
  function wrapT(t: string, x: number, y: number, mw: number, lh: number, align: CanvasTextAlign = "center") {
    cx.textAlign = align; let l = "", yy = y;
    t.split(" ").forEach((q) => { const s = l + q + " "; if (cx.measureText(s).width > mw && l) { cx.fillText(l.trim(), x, yy); l = q + " "; yy += lh; } else l = s; });
    cx.fillText(l.trim(), x, yy); return yy;
  }
  const label = (t: string, x: number, y: number, col: string) => {
    cx.textAlign = "left"; cx.fillStyle = col; cx.font = `800 24px ${SANS}`;
    cx.fillText(t.toUpperCase().split("").join(String.fromCharCode(8202)), x, y);
  };
  const uiT = (k: string) => T(UI[k]);
  try {
    await document.fonts.ready;
    await Promise.all(["800 60px Fraunces", "italic 500 40px Fraunces", "400 36px Fraunces", '800 30px "Plus Jakarta Sans"', '600 30px "Plus Jakarta Sans"'].map((f) => document.fonts.load(f)));
  } catch { /* fonts fall back to system serif/sans */ }
  const hi = `${uiT("hi")}, ${name ? name + "! " : ""}${a.name} 👋`;
  if (page === 1) {
    const g = cx.createLinearGradient(0, 0, W * 0.3, H); g.addColorStop(0, "#FFFFFF"); g.addColorStop(0.35, a.soft); g.addColorStop(1, a.c2);
    cx.fillStyle = g; cx.fillRect(0, 0, W, H);
    cx.textAlign = "center"; cx.fillStyle = a.c1; cx.font = `800 26px ${SANS}`; cx.fillText(uiT("cardQuiz"), W / 2, 120);
    cx.fillStyle = "#4A4A4A"; cx.font = "italic 500 40px Fraunces,serif"; cx.fillText(uiT("youAre"), W / 2, 330);
    let ns = 120; cx.font = `800 ${ns}px ${SERIF}`;
    while (cx.measureText(a.name).width > W - 140 && ns > 70) { ns -= 6; cx.font = `800 ${ns}px ${SERIF}`; }
    cx.fillStyle = a.c1; cx.fillText(a.name, W / 2, 330 + ns + 20);
    cx.fillStyle = "#2B1E2A"; cx.font = `400 38px ${SERIF}`;
    const yT = wrapT(T(a.tag), W / 2, 330 + ns + 100, W - 220, 52);
    const m = await loadImg(asset(POSES[0].src));
    if (m) { const h = 560, w = (h * m.width) / m.height; cx.drawImage(m, W / 2 - w / 2, yT + 50, w, h); }
    let y = yT + 50 + 600;
    label(uiT("sparks"), 120, y, a.c1); y += 70; cx.textAlign = "left";
    let x = 120; cx.font = `700 30px ${SANS}`;
    a.sparks.forEach((s) => { const w = cx.measureText(s).width + 48; if (x + w > W - 120) { x = 120; y += 66; } cx.fillStyle = "rgba(255,255,255,.85)"; rr(cx, x, y - 40, w, 58, 29); cx.fill(); cx.fillStyle = "#2B1E2A"; cx.fillText(s, x + 24, y); x += w + 14; });
    y += 90; label(uiT("universe"), 120, y, a.c1); y += 56; cx.fillStyle = "#2B1E2A"; cx.font = `600 34px ${SANS}`;
    y = wrapT(R.booths.map((b) => BOOTHS[b].name).join("  ·  "), 120, y, W - 240, 46, "left");
    y += 90; label(uiT("energy"), 120, y, a.c1); y += 56; cx.font = `600 34px ${SANS}`; cx.fillStyle = "#2B1E2A";
    y = wrapT(T(a.energy), 120, y, W - 240, 46, "left");
    y += 90; label(uiT("besties"), 120, y, a.c1); y += 56; cx.font = `600 32px ${SANS}`; cx.fillStyle = "#2B1E2A";
    cx.fillText(a.besties.map((b) => STORY.types[b].name).join("   ·   "), 120, y);
    cx.fillStyle = "rgba(255,255,255,.95)"; rr(cx, 90, 1690, W - 180, 150, 36); cx.fill();
    cx.textAlign = "center"; cx.fillStyle = a.c1; cx.font = `800 40px ${SERIF}`; cx.fillText(uiT("askCta") + " ✨", W / 2, 1750);
    cx.fillStyle = "#4A4A4A"; cx.font = `600 26px ${SANS}`; cx.fillText(EVENT.shareTags, W / 2, 1800);
  } else {
    const g = cx.createLinearGradient(0, 0, 0, H); g.addColorStop(0, "#FFFFFF"); g.addColorStop(0.5, "#FBE7F0"); g.addColorStop(1, "#F1BCD5");
    cx.fillStyle = g; cx.fillRect(0, 0, W, H);
    cx.textAlign = "center"; cx.fillStyle = "#DA1884"; cx.font = `800 26px ${SANS}`; cx.fillText(uiT("cardSummit"), W / 2, 120);
    cx.font = `800 72px ${SERIF}`; let y = wrapT(hi, W / 2, 300, W - 160, 84);
    cx.fillStyle = "#B5126C"; cx.font = "italic 500 40px Fraunces,serif"; cx.fillText(uiT("starts"), W / 2, y + 80); y += 150;
    cx.fillStyle = "rgba(255,255,255,.95)"; rr(cx, 90, y, W - 180, 250, 32); cx.fill();
    cx.textAlign = "left"; cx.fillStyle = "#DA1884"; cx.font = `700 40px ${SERIF}`; wrapT(EVENT.name, 130, y + 70, W - 300, 48, "left");
    cx.fillStyle = "#4A4A4A"; cx.font = `600 28px ${SANS}`;
    cx.fillText(`${uiT("date")}:  ${EVENT.date}`, 130, y + 150); cx.fillText(`${uiT("loc")}:  ${EVENT.location}`, 130, y + 190); cx.fillText(`${uiT("time")}:  ${EVENT.time}`, 130, y + 230);
    y += 330; cx.fillStyle = "#2B1E2A"; cx.font = `500 32px ${SANS}`;
    y = wrapT(uiT("based").replace(/<[^>]+>/g, ""), 120, y, W - 240, 44, "left");
    const list = (title: string, rows: { t: string; s: string }[]) => {
      label(title, 120, y, "#DA1884"); y += 66;
      rows.forEach((r, i) => { cx.fillStyle = "#2B1E2A"; cx.font = `800 36px ${SANS}`; cx.textAlign = "left"; cx.fillText(`${i + 1}.  ${r.t}`, 120, y); cx.fillStyle = "#4A4A4A"; cx.font = `500 28px ${SANS}`; cx.fillText(r.s, 172, y + 40); y += 100; });
    };
    y += 80; list(uiT("booths"), R.booths.map((b) => ({ t: BOOTHS[b].name, s: T(BOOTHS[b].tag) })));
    y += 30; list(uiT("stages"), R.picks.map((p) => ({ t: EVENT.stages[p].title, s: EVENT.stages[p].speaker })));
    cx.fillStyle = "rgba(255,255,255,.95)"; rr(cx, 90, 1700, W - 180, 140, 32); cx.fill(); cx.textAlign = "center"; cx.fillStyle = "#DA1884"; cx.font = "italic 500 34px Fraunces,serif";
    wrapT(uiT("free") + " ✨", W / 2, 1760, W - 260, 42);
  }
}
