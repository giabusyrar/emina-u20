import type { EventCfg, Scene, Story } from "./story";

export type Trail = { node: string; i: number }[];
export type Run = {
  type: Record<string, number>; booth: Record<string, number>;
  doubt: string | null; interest: string | null; trail: Trail;
};
export type Outcome = { win: string; booths: string[]; picks: string[] };

export const freshRun = (): Run => ({ type: {}, booth: {}, doubt: null, interest: null, trail: [] });

/** Chapters that score, latest first: a tie goes to the answer given most recently (deck rule). */
export const recencyOrder = (scenes: Scene[]) =>
  scenes.filter((s) => s.options.some((o) => o.type || o.booth)).map((s) => s.id).reverse();

function rank(kind: "type" | "booth", order: string[], run: Run, scenes: Scene[]) {
  const byId = Object.fromEntries(scenes.map((s) => [s.id, s]));
  const contrib = (code: string, node: string) => {
    const t = run.trail.find((x) => x.node === node);
    return t ? byId[node].options[t.i][kind]?.[code] ?? 0 : 0;
  };
  const rec = recencyOrder(scenes);
  return order.slice().sort((a, b) => {
    const d = (run[kind][b] || 0) - (run[kind][a] || 0);
    if (d) return d;
    for (const n of rec) {
      const e = contrib(b, n) - contrib(a, n);
      if (e) return e;
    }
    return order.indexOf(a) - order.indexOf(b);
  });
}

export function outcome(story: Story, event: EventCfg, run: Run): Outcome {
  const win = rank("type", story.typeOrder, run, story.scenes)[0];
  const booths = rank("booth", story.boothOrder, run, story.scenes).filter((b) => (run.booth[b] || 0) > 0);
  story.types[win].booths.forEach((b) => { if (!booths.includes(b)) booths.push(b); });
  const top = booths.slice(0, 3);
  const picks: string[] = [];
  const first = run.interest ? event.interestStage[run.interest] : undefined;
  if (first && event.stages[first]) picks.push(first);
  top.forEach((b) => { if (picks.length < 3 && !picks.includes(b) && event.stages[b]) picks.push(b); });
  return { win, booths: top, picks };
}
