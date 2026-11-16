# Emina 20 Under 20 — Explorer Quiz ("What if you live with fearless?")

Interactive story quiz for Emina's **20 Under 20 · Made to Explore** campaign, built from the
concept deck (Nov 2026): 7 chapters, 6 questions, one connected story. Each answer scores an
Explorer type (+2/+1) and a passion booth (+2/+1); chapters 04 and 05 personalise the result
(reassurance message, stage picks). Output: two shareable pages, the Explorer identity card and
the pre-event Summit guide (3 booths + 3 stage picks).

**Live (production):** https://emina-20-under-20.vercel.app
**Claude preview:** https://claude.ai/artifact/44RHVwpwnryJ8B61TJVBPS
**Repo:** `story-game/` in https://gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/emina-u20 (the repo root is the seagull-core-engine Go service). Run everything below from `story-game/`.

`index.html` is **generated**: `python3 build/build4.py` (engine `build/template4.html`, story `story/v6-explorer.json`, event config `story/event.json`).

## Content model

- `story/v6-explorer.json` — bilingual (`{id, en}`) chapters, beats, prompts, options with `type` / `booth` scores, `doubt` (chapter 04) and `interest` (chapter 05) tags, `move` (mascot action), `x` (where she walks). Six types (CL CR AC CU IP ST), nine booths (MUS BEA ART SPO STEM MOV IMP FAS PSY), doubt messages, interest themes, UI strings.
  Deck codes SP and CH are mapped to CL (Culture) and IP (Impact).
- `story/event.json` — Summit name/date/location/time (placeholders), register link, stage session per booth with speaker, `interestStage` (chapter 05 interest → first stage pick). Fill this before launch.
- Scoring (`outcome()` in the template): sum types from chapters 01/02/03/06, highest wins; booths ranked independently, top 3 unique (padded from the type's starting booths); ties broken by recency 06 → 03 → 02 → 01, then stable code order. Stage picks = interest stage first, then the top booths.
- Language: Indonesian by default, EN toggle (remembered per device). Name asked on the welcome screen, used on page 2.

## Pictures

- `art/kv/` — crops of the official KV (hero, Emina wordmark, 20 under 20 lockup, Made to Explore).
- `art/explorer/c1..c7.jpg` — chapter scenes. Generate with `python3 build/gen_explorer.py --key-file ~/.openrouter.key` (one Gemini hit per chapter, each guided by the deck's reference image; c1 anchors the style). Until then the deck references themselves sit there as placeholders (`art/explorer/_placeholder/`).
- `art/char/` — mascot sprite poses (see below). `t_<CODE>.png` type variants are optional and picked up automatically for the identity card.

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

## The six Explorer types

| code | type | starting booths |
|---|---|---|
| CL | The Culture Explorer | Music & Fandom · Movie & Stories · Art & Craft |
| CR | The Creative Explorer | Art & Craft · Movie & Stories · Beauty & Expression |
| AC | The Active Explorer | Sport · Music & Fandom · Impact & Environment |
| CU | The Curious Explorer | STEM World · Psychology & Wellbeing · Movie & Stories |
| IP | The Impact Explorer | Impact & Environment · Psychology & Wellbeing · Art & Craft |
| ST | The Style Explorer | Fashion & Style · Beauty & Expression · Art & Craft |

Copy (descriptions, taglines, sparks, energy lines, besties) lives in `story/v6-explorer.json` under `types`.

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
