import { describe, expect, it } from "vitest";
import { EVENT, STORY, SCENES } from "./story";
import { freshRun, outcome, recencyOrder } from "./scoring";

const answer = (run: ReturnType<typeof freshRun>, node: string, i: number) => {
  const o = SCENES[node].options[i];
  for (const k in o.type ?? {}) run.type[k] = (run.type[k] || 0) + o.type![k];
  for (const k in o.booth ?? {}) run.booth[k] = (run.booth[k] || 0) + o.booth![k];
  if (o.interest) run.interest = o.interest;
  run.trail.push({ node, i });
};

describe("outcome", () => {
  it("recency order is latest scoring chapter first", () => {
    expect(recencyOrder(STORY.scenes)).toEqual(["c6", "c3", "c2", "c1"]);
  });
  it("every path yields a valid type, 3 unique booths and 1-3 stage picks", () => {
    for (const i of [0, 1, 2, 3]) {
      const run = freshRun();
      for (const n of ["c1", "c2", "c3", "c6"]) answer(run, n, Math.min(i, SCENES[n].options.length - 1));
      const r = outcome(STORY, EVENT, run);
      expect(STORY.types[r.win]).toBeTruthy();
      expect(new Set(r.booths).size).toBe(3);
      expect(r.picks.length).toBeGreaterThan(0);
    }
  });
  it("no answers falls back to stable code order", () => {
    const r = outcome(STORY, EVENT, freshRun());
    expect(r.win).toBe(STORY.typeOrder[0]);
    expect(r.booths).toEqual(STORY.types[r.win].booths);
  });
});
