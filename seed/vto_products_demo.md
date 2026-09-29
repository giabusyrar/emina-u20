# Demo try-on catalog (`vto_products_demo.csv`)

Try-on products for all seven sections (brow, lip, complexion, blush,
eyeliner, eyeshadow, mascara) until the brands' RnD mapping arrives. Seeded
with `cmd/seed-colour-shades` (see `docs/COLOUR-LOCAL-RUN.md`).

## What is in it

| | rows | colour |
|---|---:|---|
| Make Over, from the WCPA notebook's `vto_products.csv` | 187 | `[SWATCH]`: the product's swatch hex |
| Wardah, Make Over and OMG demo products | 48 | `[PERKIRAAN]`: **estimated**, not from the brand |

Estimated rows reach the API as `colourSource: "estimate"`, and the web UI
shows "Warna perkiraan untuk demo, belum dari data brand." next to them. The
seeder refuses any other `sumber` value, so a colour of unknown origin cannot
look like a real one.

Products per section:

| Section | Products | Shades |
|---|---|---|
| Brow (black and brown only) | Make Over Powerstay Micro Brow Sculptor, Make Over Powerstay Brow Volumizer Mascara (swatch); Wardah EyeXpert Eyebrow Pencil, OMG Oh My Glam Precise Matic Eyebrow (estimate) | Light Brown → Black |
| Lip | 4 Make Over (swatch, 67 shades); Wardah Colorfit Velvet Matte Lip Mousse (estimate, 8 shades) | nude → deep |
| Complexion | 4 Make Over foundations and cushions (swatch, 95 shades) | very light → deep |
| Blush | 3 Make Over (swatch, 17 shades); Wardah Colorfit Cream Blush, Wardah Exclusive Blush On (estimate) | |
| Eyeliner (black and brown only) | Make Over Hyperblack Superstay Liner (swatch); Make Over Eye Liner Pencil (Black Jack, Brown Latte), Wardah EyeXpert Optimum Hi-Black Liner, Wardah EyeXpert Perfect Precision Liner (estimate) | |
| Eyeshadow | Make Over Powerstay Eye Palette Hall Of Stars and Futurist Gleam; Wardah Colorfit Quad Eye Palette 01 Brunette Dawn, 02 Rose Aurora, 03 Sunlit Blaze (all estimate) | light → dark per palette |
| Mascara (by type, all black) | Make Over Lash Impulse Waterproof (swatch, volumizing); OMG Oh My Glam Lashtention Waterproof (lengthening), Wardah EyeXpert Perfect Curl (natural), Wardah EyeXpert The Volume Expert (volumizing) | |

## How the estimates were made

- **Product and shade names** are the brands' own (sources below). No shade
  was invented for a real product; eyeshadow pans use the palette's pan
  names.
- **Wardah blush:** colour of the shade circle on Wardah's own try-on page
  (screenshot from the product owner), sampled where the circle is flat.
- **Eyeshadow pans:** from the brands' colour words ("light cream", "lilac
  grey", "medium cocoa", "brick orange", "turquoise", "pressed gold
  glitter"; Wardah's palette themes "earthy", "pink", "citrusy") where the
  brand gives them; otherwise from the pan name and palette theme. Shimmer
  and glitter (`specular` 0.12 and 0.2) only where a pan is described or
  named that way; the split of matte and shimmer inside the Wardah quads is
  not published and is a guess.
- **Brow, eyeliner, mascara:** usual pencil or liner colours for the named
  shade (black, brown, dark brown, grey brown).
- **Wardah lip mousse:** from the shade names and Wardah's own description
  of each shade.

## `mode` (render style)

`mode` picks the look the renderer draws; it is a demo choice, not a brand
claim, except for mascara, where it follows the product's claim: volume →
`volumizing`, "lash extension" effect → `lengthening`, curl → `natural`.
Eyeliner: liquid liner `classic`, felt-tip pen `cat_eye`, 0.01 mm pen `thin`,
pencil `smoky`. Eyeshadow: light shades `natural` (lid wash), mid and bright
shades `soft_glam`, the darkest `smoky`. Brow `mode` does not change the look.

## Replacing estimates

When a brand sends a shade's hex (hex only, per the product decision), put
it in `hex`, set `sumber` to `[SWATCH]` and re-run the seeder: the swatch row
replaces the estimate. Cube readings (`munsell_v`, `munsell_c`) outrank both.

## Sources

- Make Over Eye Liner Pencil shades: https://www.makeoverforall.com/product/make-over-eye-liner-pencil/96
- Make Over Lash Impulse Waterproof Mascara ("volume 10x"): https://www.makeoverforall.com/product/make-over-lash-impulse-waterproof-mascara/238
- Make Over Powerstay Eye Palette pans (Hall Of Stars, Futurist Gleam): https://www.makeoverforall.com/beauty-highlights/tutorial/tips-memilih-warna-eyeshadow-make-over-powerstay-eye-palette-284
- Make Over Powerstay Eye Palette Hall Of Stars: https://www.makeoverforall.com/product/make-over-powerstay-eye-palette-hall-of-stars/261
- Make Over Powerstay Eye Palette Futurist Gleam: https://www.makeoverforall.com/product/make-over-powerstay-eye-palette-futurist-gleam/262
- Wardah EyeXpert Eyebrow Pencil (Black, Brown): https://www.wardahbeauty.com/en/product/make-up/eyexpert-eyebrow-pencil
- Wardah EyeXpert Optimum Hi-Black Liner (pen): https://www.wardahbeauty.com/id/product/make-up/optimum-hi-black-eyeliner
- Wardah EyeXpert Perfect Precision Liner (0.01 mm pen, black): https://www.wardahbeauty.com/id/product/make-up/eyexpert-perfect-precision-liner
- Wardah EyeXpert The Volume Expert Mascara: https://www.wardahbeauty.com/en/product/make-up/eyexpert-the-volume-expert-mascara
- Wardah EyeXpert Perfect Curl Mascara: https://www.wardahbeauty.com/en/product/make-up/wardah-eyexpert-perfect-curl-mascara
- Wardah Colorfit Quad Eye Palette pan names: https://www.wardahbeauty.com/en/news/wardah-colorfit-quad-eye-palette-sempurnakan-tampilan-makeup-sehari-hari
- Wardah eyeshadow palette themes: https://www.wardahbeauty.com/en/news/rekomendasi-eyeshadow-palette-wardah
- Wardah Colorfit Velvet Matte Lip Mousse shades: https://www.wardahbeauty.com/en/news/cara-pilih-shade-wardah-velvet-matte-untuk-skin-tone-kamu
- OMG Oh My Glam Precise Matic Eyebrow (01 Dark Brown, 02 Grey Brown): https://makeupstorebn.com/product/omg-oh-my-glam-eyebrow-precise-matic/
- OMG Oh My Glam Lashtention Waterproof Mascara: https://www.watsons.co.id/en/omg-oh-my-glam-lashtention-waterproof-mascara-6.5g/p/BP_44981
