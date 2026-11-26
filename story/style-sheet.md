# Style Sheet — Emina "Empat Pintu, Satu Hari"

Format follows the `voice-style` skill from danjdewhurst/story-skills.
Every copy pass on this project must be checked against this sheet.

---

## Language

- **Primary:** `id-ID` — Bahasa Indonesia, Jakarta Gen Z register (Jaksel).
- **Secondary:** `en` — woven in, not translated alongside.
- **Mode:** natural code-switching, the way an Indonesian girl aged 15–22 actually texts. English enters as **vocabulary she already uses**, never as a parallel translation and never as a quotation.

### Where English is allowed

| Layer | Language | Why |
|---|---|---|
| Narration (the story body) | **Indonesian** | This is where the feeling lives. Heavy switching here breaks immersion. |
| Nouns she genuinely says in English | **English** | ring light, flatlay, rundown, stand mic, booth, pitch, sponsor, playlist, scrunchie, headphone, tripod |
| Act titles / section labels | **Bilingual pair** | Matches Emina's own lockups: a short English line over an Indonesian one |
| Character type names | **Bilingual pair** | Already locked: "Si Anak FYP" + "The Spotlight Creator" |
| Taglines, buttons, share card | **English-forward** | Emina's brand layer: "Born to be Loved", "Made to Explore", "Let your joy lead the way" |

### Where English is banned

- Inside an emotional beat. `Dia cerita sambil nunduk` stays Indonesian. Do not write `She looked down` next to it.
- As a gloss. Never `pintu (door)`. Never a parenthetical translation.
- Formal/corporate English: *empower, unleash, journey, discover your true self, embrace*. These read as ad copy, not as a girl talking.

---

## Voice Levers

- **POV:** second person, `kamu`. Never `anda`, never `lo/gue` (too hard-edged for Emina).
- **Tense:** present, immediate.
- **Register:** warm, close, a little funny. She is a friend who is already inside the building with you.
- **Brand guardrails:** positive, inclusive, never mean, never body-focused, never about being prettier than someone else.

---

## Hard Rules (from stop-slop)

1. No em dashes. Use a comma or a full stop.
2. No adverbs. Cut `banget` when it is filler; keep it when it is how she talks.
3. No filler openers: `Ternyata`, `Yang pasti`, `Intinya`.
4. No rhetorical setups: `Gimana kalau`, `Coba bayangin`.
5. No binary contrast: `Bukan X, tapi Y`. State Y.
6. Active voice, named actor. Never `Lampu itu nemu kamu`. Say who aimed it.
7. Vary sentence length. Never three in a row at the same length.
8. No pull-quote lines. If it sounds screenshot-ready as a standalone caption, rewrite it.
9. **No apostrophe characters anywhere** — the copy ships inside single-quoted JS strings. Use double quotes for quoted speech.

### Opener quota

- At most **3 of 13** scene narrations may open with the demonstrative `X itu`.
- At most **1 in 4** transitions may begin with `Kamu`.
- The six endings must not all open the same way. No more than two may start with a time-of-day phrase.

---

## Continuity Anchors (do not break)

Checked by `revision-continuity` logic. These threads must survive any copy pass:

1. **Nay** — appears in all 13 scenes and all 6 endings. She opens the door, vanishes, leaves traces, drops the box in Babak 4, and closes every ending.
2. **Lampu kecil** — enters in the fallen box (Babak 4), travels to the roof (Babak 5), becomes the final choice, and reappears in every ending.
3. **The carried object** — whatever she picked up in her Babak 3 lane rides with her into the yard and onto the roof.
4. **Transitions** — every option's transition must plant a physical detail that the next scene's opening line picks up. This is the anti-`patah` device. Never write a transition that just orders her to the next location.

---

## Path Rules (from interactive-fiction)

- **`unreachable-chapter`:** every scene must be reachable from `a1_pintu`. Verified: 0 unreachable.
- **`state-differs-by-path`:** an ending's closing line must be true for **every** path that reaches it. An ending may not reference a prop or action the reader may not have chosen. Verified across 576 paths.
- **Tiebreak:** when scores tie, the Babak 5 lamp choice decides, then Babak 4, then earlier. Narratively: the last thing she does with the light is who she is.

---

## Lexicon

**Use:** gercep, nagih, nyelip, mampir, kepo, nyeletuk, ngegas, melting, nyambung, anget, rame, kebuka, nyangkut, ketelungkup

**Avoid:** sobat, guys, bestie-bestie (forced), literally (overused), slay, periodt, "it's giving", vibes (allowed once per 10 scenes max)

**Brand terms (fixed spelling):** Emina, Rumah Emina, 20 Under 20, Made to Explore, @eminacosmetics, #Emina20Under20
