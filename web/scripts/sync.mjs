// Copies the single source of truth (../story, ../art) into web/data and web/public/art. Runs before dev/build/test.
import { cpSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const web = fileURLToPath(new URL("..", import.meta.url));
const game = join(web, "..");
rmSync(join(web, "data"), { recursive: true, force: true });
rmSync(join(web, "public/art"), { recursive: true, force: true });
mkdirSync(join(web, "data"), { recursive: true });
cpSync(join(game, "story/v6-explorer.json"), join(web, "data/story.json"));
cpSync(join(game, "story/event.json"), join(web, "data/event.json"));
cpSync(join(game, "art/char/poses.json"), join(web, "data/poses.json"));
const art = (dir, keep) => {
  mkdirSync(join(web, "public/art", dir), { recursive: true });
  for (const f of readdirSync(join(game, "art", dir))) if (keep(f)) cpSync(join(game, "art", dir, f), join(web, "public/art", dir, f));
};
art("kv", () => true);
art("explorer", (f) => /^c\d\.jpg$/.test(f));
art("char", (f) => /^p\d+\.png$/.test(f));
