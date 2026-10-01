# Demo try-on catalog (`vto_products_demo.csv`)

Try-on products for all seven sections (brow, lip, complexion, blush,
eyeliner, eyeshadow, mascara) until the brands' RnD mapping arrives. Seeded
with `cmd/seed-colour-shades` (see `docs/COLOUR-LOCAL-RUN.md`).

## What is in it

Every product and shade name is the brand's own, checked against the brand
sites on 2026-09-30 (sources below). An earlier version carried the WCPA
notebook's rows, where most shades had lost their names (`02`, `03`, …); the
names were restored from the brand sites and each hex re-checked against them.

| | rows | colour |
|---|---:|---|
| Brand-site swatch | 157 | `[SWATCH]`: the swatch the brand's product page shows for that shade (makeoverforall.com `__NEXT_DATA__` option `color`; wardahbeauty.com `data-color`) |
| Estimate | 86 | `[PERKIRAAN]`: **estimated**, not from the brand |

A brand-site swatch is the colour the brand's website draws for the shade, not
a colour measured from the product. Rows are estimates where the brand
publishes no usable colour:

- both Make Over cushions: their site swatches are one colour set the site
  reuses on other products (a contour kit, Hydrastay Lite Glow Cushion), so
  they are not these products' own;
- Make Over Powerstay 24H Matte Powder Foundation W12 Warm Marble: its site
  swatch is the same dark colour as C41 Cool Sand, which a "Marble" shade is not;
- black mascara and liner: the site's `#000000` is a generic placeholder;
- eyeshadow pans (only a palette-level swatch is published), OMG (no colours
  published).

Left out on purpose: shades named only by resellers or reviews with no brand
page (Glazed Lock D19–D24, Wardah Cream Blush 04–06, Wardah lip mousse
"Color Therapy"), and palette pans beyond the ones below.

Estimated rows reach the API as `colourSource: "estimate"`, and the web UI
shows "Warna perkiraan untuk demo, belum dari data brand." next to them. The
seeder refuses any other `sumber` value, so a colour of unknown origin cannot
look like a real one.

Products per section:

| Section | Products | Shades |
|---|---|---|
| Brow (black, brown, grey) | Make Over Powerstay Micro Brow Sculptor, Make Over Powerstay Brow Volumizer Mascara (1.0 Light Brown, 2.0 Dark Brown, 3.5 Deep Gray; swatch), Wardah EyeXpert Eyebrow Pencil (swatch); OMG Oh My Glam Precise Matic Eyebrow (estimate) | Light Brown → Black |
| Lip | Make Over Glassy Lip Lacquer x Hian Tjen (401–411), Powerstay Glazed Lock Lip Pigment (D01–D18), Hydrastay Smooth Lip Whip (C01–C18), Powerstay Transferproof Matte Lip Cream (20); Wardah Colorfit Velvet Matte Lip Mousse (01–14) — all swatch | nude → deep |
| Complexion | Make Over Powerstay 24H Matte Powder Foundation (25), Weightless Liquid Foundation (20) (swatch); Hydrastay Prismatic Glass Cushion, Powerstay Sync Matte Cushion (25 each, estimate) | N00 Porcelain → N70 Ebony |
| Blush | Make Over Powerstay Suede Blush, Hydrastay Liquid Tint Blush, Blush On Single (01–10); Wardah Colorfit Cream Blush (01–03), Wardah Exclusive Blush On — all swatch | |
| Eyeliner (black and brown only) | Make Over Eye Liner Pencil (Black Jack, Brown Latte; swatch); Make Over Hyperblack Superstay Liner, Wardah EyeXpert Optimum Hi-Black Liner, Wardah EyeXpert Perfect Precision Liner (estimate) | |
| Eyeshadow | Make Over Powerstay Eye Palette Hall Of Stars and Futurist Gleam (7 of 12 pans each); Wardah Colorfit Quad Eye Palette 01 Brunette Dawn, 02 Rose Aurora, 03 Sunlit Blaze (all estimate) | light → dark per palette |
| Mascara (by type, all black) | Make Over Lash Impulse Waterproof (volumizing); OMG Oh My Glam Lashtention Waterproof (lengthening), Wardah EyeXpert Perfect Curl (natural), Wardah EyeXpert The Volume Expert (volumizing) — all estimate | |

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
  and glitter (`specular` 0.12 and 0.2): Make Over pans follow the finish
  the brand publishes per pan (Matte 0, Shimmer 0.12, Foiled Metallic and
  Glitter 0.2); the split of matte and shimmer inside the Wardah quads is
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

Make Over product pages are `https://www.makeoverforall.com/product/<slug>/<id>`;
the shade list and swatches are in the page's `__NEXT_DATA__` JSON.

- Powerstay 24H Matte Powder Foundation /263; Hydrastay Prismatic Glass Cushion /270; Powerstay Sync Matte Cushion /271; Powerstay 24H Weightless Liquid Foundation /150
- Powerstay Suede Blush /256; Hydrastay Liquid Tint Blush /236; Blush On Single /210
- Glassy Lip Lacquer x Hian Tjen /273; Powerstay Glazed Lock Lip Pigment /264; Hydrastay Smooth Lip Whip /235; Powerstay Transferproof Matte Lip Cream /145
- Powerstay Micro Brow Sculptor /267; Powerstay Brow Volumizer Mascara /265; Hyperblack Superstay Liner /107
- Wardah Colorfit Velvet Matte Lip Mousse (01–14, swatches): https://www.wardahbeauty.com/id/product/make-up/colorfit-velvet-matte-lip-mousse
- Wardah Colorfit Cream Blush (01–03): https://www.wardahbeauty.com/id/product/make-up/colorfit-cream-blush
- Wardah Exclusive Blush On (01 Rosy Pink, 02 Coral Peach): https://www.wardahbeauty.com/en/product/make-up/exclusive-blush-on

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
