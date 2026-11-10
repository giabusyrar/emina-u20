# Emina 20 Under 20 — "H-1" (satu hari sebelum pensi)

An interactive story for Emina's **20 Under 20** ("Made to Explore") campaign, built like a
text-based adventure / visual novel for phones. Tomorrow is the school pensi. Nay, your best
friend and the committee chair, is about to melt down. You spend one day helping her, and the
way you help reveals your energy type. Six endings, an Instagram Story card, a push to register.

**Live (production):** https://emina-20-under-20.vercel.app (H-1, with AI scene art)
**Claude preview of the current build:** https://claude.ai/artifact/44RHVwpwnryJ8B61TJVBPS
**Deploy:** `python3 deploy.py` (preview) · `python3 deploy.py --prod` (production). SHA-upload, no Node needed.
**Repo:** this game lives under `story-game/` in https://gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/emina-u20 (the repo root is the seagull-core-engine Go service). Run the commands below from `story-game/`.

`index.html` is **generated**. Edit the sources, then run `python3 build/build3.py`.

---

## How it plays

1. **Welcome screen** (Emina pink): a plain game intro (what it is, the situation, what you do, what you get), one button.
   The first scene (06.50, gerbang sekolah) is a diegetic tutorial: the narration stays in the story
   (you arrive, Nay spams caps lock, you pick how to walk in); the mechanics are taught by UI only
   (a pulsing "ketuk buat lanjut" pill, a TUTORIAL badge, a tip box above the first choice). It flows
   straight into 07.10.
2. **Picture on top, text below.** Every scene is a place and a time (07.10 Aula sekolah → 18.45 malam pensi).
   The text box types out narration and dialogue with a speaker tag; tap to continue.
3. **Actions, not answers.** After the beats, a short list of first-person actions appears
   (`› Duduk sebelah Kinan, bantu ngelem`). Pick one. Your decision echoes in the box as *Kamu*,
   the character walks to that spot in the picture, and the scene reacts to what you did.
4. **Five acts, one day.** Four fires at 07.10 → four rooms → six lanes → the blackout at 17.15
   (Nay hands you her HT) → the night itself. 576 complete paths, 6 endings.
5. **Reveal ceremony** then a result card: mascot, type, "Besok paginya" closing, the pictures you
   walked through, a 1080×1920 Story card, replay, "Daftar 20U20".

## The story (`story/v5-story.json`)

Written after reading ten references teens actually read (see `story/REFERENCES.md`): Dilan,
Mariposa, NKCTHI, Rintik Sedu, Perahu Kertas, Hujan, Antares, Dear Nathan, Garis Waktu,
Laskar Pelangi, plus Oxenfree / Life is Strange for how choices should feel.

- Cast: **Nay** (ketua panitia, caps lock), **Rara** (publikasi), **Kinan** (dekor, wants to be seen),
  **Dimas** (perlengkapan, calls you Bos), **Sasa** (kelas sepuluh, gitar, never played for anyone), Kak Alya (alumni, ghosting).
- Motif: rain (BMKG screenshot at 07.10 → blackout at 17.15 → lights on at 18.45). Object: the HT.
- Scoring: each action carries `scores` like `deep+2,artisan+1`. Highest wins; ties resolve by the
  latest action favouring a tied type, so the last choice of the night decides.
- Voice: `story/style-sheet.md`. aku/kamu throughout, slang only in dialogue, no apostrophes
  (the copy ships inside JS strings), beats under 26 words.

## Character (`build/template3.html`)

The player is the white Emina mascot. Two render modes, picked automatically at build time:

- **Sprite mode** (current): 16 illustrated poses generated with Gemini 3 Pro Image in ONE hit as a
  4x4 character sheet on a green key (`python3 build/gen_char.py --key-file ...`, references:
  `art/char/_ref_mascot.png` for the design and `art/a1_aula.jpg` for the style), then chroma-keyed and
  sliced into `art/char/p00..p15.png` + `poses.json` by `python3 build/slice_char.py`. The engine swaps
  the pose per action, alternates two frames while walking, and keeps the procedural motion on top
  (anticipation crouch, waddle, bob, arrival squash, emote bubble above the head).
- **Vector rig** (fallback when `art/char/poses.json` is missing): a jointed SVG mascot with the same
  16 actions driven by CSS.

Pose order on the sheet: idle, walk A, walk B, run, sit, listen, phone, call, write, tidy, wave, mic,
look, talk, torch, paint. Every story option carries `move` with one of those verbs.

## The six types

| key | nick | one-liner |
|---|---|---|
| creator | Si Main Character | Main Character Energy |
| boss | Si Ambis Santuy | Big dreams, low pressure |
| artisan | Si Soft Life | Slow hands, full heart |
| deep | Si Deep Talk | Feels it first, says it best |
| change | Si Penjaga Circle | The one who holds everyone together |
| explorer | Si Serba Penasaran | Curious about everything, sorry about nothing |

Copy and colours in `build/_arch.js`, mascots in `build/_mascots.js`.

## Pictures (`art/*.jpg`)

One illustration per scene plus `hero.jpg` for the welcome screen, generated once each:

```bash
python3 build/gen_art.py --key-file /path/to/openrouter.key              # all missing scenes
python3 build/gen_art.py --key-file ... --only a1_aula                    # one scene
python3 build/gen_art.py --key-file ... --list-models                     # image-capable models
```

Prompts live in the story file (`style` + each scene's `imagePrompt`, `heroPrompt`). The first
picture becomes the style reference for the rest. Existing files are skipped, failures are not
retried, the key is read in place and never printed. Missing pictures fall back to a flat colour.

## Build and deploy

```bash
python3 build/build3.py            # story + template + types → index.html
python3 build/build3.py --out test.html
python3 deploy.py                  # preview URL (Vercel login required to open)
python3 deploy.py --prod           # production
```

## Files

```
index.html             generated bundle
art/                   scene pictures (shipped next to index.html)
deploy.py              Vercel SHA-upload deploy (index.html + art/)
build/template3.html   visual-novel engine, CSS, markup
build/build3.py        assembler
build/gen_art.py       scene picture generator (OpenRouter, one hit per scene)
build/gen_char.py      mascot character sheet (one hit), build/slice_char.py slices it
art/char/              16 mascot poses + poses.json (sheet.png and _ref_* stay local)
build/_arch.js         six types
build/_mascots.js      mascot generators
story/v5-story.json    the script
story/REFERENCES.md    reading notes behind the script
story/style-sheet.md   voice rules
build/template2.html, build/build2.py, story/v4-story.json, story/art2/   previous point-and-click build, kept for reference
prototype.html, story-map.html                                            earlier explorations
```

## Before launch

- Generate the 14 pictures, review, then `python3 deploy.py --prod`.
- Confirm placeholders: `bit.ly/Emina20Under20`, `@eminacosmetics`, `#Emina20Under20`.
