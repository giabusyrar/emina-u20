# Shades for the brand-catalogue products (`vto_catalogue_shades.csv`)

Shades for the 21 products the seagull-web brand-catalogue seed added to
`ref_products` on 2026-10-01 (Wardah 14, Make Over 7). Those products were
created through reference-service, so their ids are not the seeder's
`clr-prd-…` ids. `cmd/seed-colour-shades` finds each one by brand and exact
name and attaches the shades to it, leaving the product row as it is.

Seed it like the demo catalogue (`docs/COLOUR-LOCAL-RUN.md`), with the same
`-brand-map`.

## Where the colours come from

Read from the brands' own product pages on 2026-10-01:

- Wardah: the `data-color` of each shade option on wardahbeauty.com.
- Make Over: the option `color` in the page's `__NEXT_DATA__` on
  makeoverforall.com.

Of the 178 rows, 146 are brand-site swatches (`[SWATCH]`) and 32 are
estimates (`[PERKIRAAN]`). Estimates reach the API as `colourSource:
"estimate"`. A row is an estimate when:

- **Make Over Hydrastay Lite Glow Cushion (all 25 rows).** Its swatch set is
  identical, name and hex, to the other Make Over cushions'. That set is one
  the site reuses, so it is not this product's own (same rule as the demo
  catalogue's cushions).
- **Make Over Perfect Cover Powder Foundation 06 Cappuccino and 07
  Espresso.** The page draws both as light peach, which contradicts the
  names. They use the same-named Two Way Cake shades instead.
- **Make Over Perfect Cover Two Way Cake 06 Cappuccino and 07 Espresso.** The
  page draws them in the wrong order, so they follow the refill page's
  order.
- **Wardah Colorfit Highlight Contour Palette, Light and Medium.** The page
  gives no colour.
- **Wardah EyeXpert Liftlast Waterproof Mascara.** The page lists no shade.
  It is one "Default" row, black, the same as the demo catalogue's mascaras.

Left out on purpose: Make Over Amplify Fluid Glass "500 Coat", a clear top
coat with no pigment. Any colour for it would be invented.

## Worth a second look

These are `[SWATCH]`, so they are the brand's own values, but they look
odd:

- **Make Over Perfect Cover Powder Foundation.** The other ten swatches are
  all light peach, including 05 Cinnamon and 12 Chai, so the range may render
  too light.
- **Wardah Lightening Liquid Foundation 6ml.** The swatches are not in tone
  order: 02 Beige is lighter than 01 Light Beige.
- **Wardah Refill Colorfit Perfect Glow Cushion.** It reuses the older
  "Colorfit Perfect Glow Cushion" page's swatches, which differ from the
  current product's for the same shade names.

`alpha`, `specular` and `mode` are render choices, copied from the closest
product in `vto_products_demo.csv`, the same as there. The lip balm, the
concealer and the highlight/contour palette have no close precedent.
