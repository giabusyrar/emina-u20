import storyJson from "@/data/story.json";
import eventJson from "@/data/event.json";
import posesJson from "@/data/poses.json";

export type Lang = "id" | "en";
export type L10n = string | { id?: string; en?: string };
export type Beat = { who: string } & { id?: string; en?: string };
export type Option = {
  label: L10n; x?: number; move?: string; react?: Beat[];
  type?: Record<string, number>; booth?: Record<string, number>;
  doubt?: string; interest?: string; leadsTo: string;
};
export type Scene = {
  id: string; chapter: string; place: L10n; img: string; floor?: number; tutorial?: boolean;
  beats: Beat[]; prompt: L10n; options: Option[];
};
export type ExplorerType = {
  name: string; icon: string; c1: string; c2: string; soft: string; desc: L10n; tag: L10n; energy: L10n;
  sparks: string[]; booths: string[]; besties: string[];
};
export type Story = {
  theme: L10n; intro: L10n; welcome: Record<string, L10n | string>; ui: Record<string, L10n>;
  typeOrder: string[]; boothOrder: string[]; types: Record<string, ExplorerType>;
  booths: Record<string, { name: string; tag: L10n }>; doubts: Record<string, L10n>;
  interests: Record<string, string>; scenes: Scene[];
};
export type EventCfg = {
  name: string; date: string; location: string; time: string; register: string; shareTags: string; hashtag: string;
  stages: Record<string, { title: string; speaker: string }>; interestStage: Record<string, string>;
};
export type Pose = { src: string; w: number; h: number };

export const STORY = storyJson as unknown as Story;
export const EVENT = eventJson as EventCfg;
export const POSES = posesJson as Pose[];
export const SCENES: Record<string, Scene> = Object.fromEntries(STORY.scenes.map((s) => [s.id, s]));
export const ORDER = STORY.scenes.map((s) => s.id);

/** Stage backdrop shown until a scene picture loads. Source: build/build4.py `FB`. */
export const FALLBACK_BG: Record<string, string> = {
  c1: "#FFE9D6", c2: "#DCEFFF", c3: "#FFF1E6", c4: "#2E3A2E", c5: "#3A2535", c6: "#E6E0FF", c7: "#D8E4FF",
};

export const tr = (v: L10n | Beat | undefined | null, lang: Lang): string =>
  v == null ? "" : typeof v === "string" ? v : v[lang] || v.id || v.en || "";
export const fmt = (s: string, o: Record<string, string>) => s.replace(/\{(\w+)\}/g, (m, k) => (o[k] == null ? m : o[k]));
export const asset = (p: string) => "/" + p.replace(/^\//, "");
