# Art Contract v2 — portrait game scenes with in-scene hotspots

Every scene is a PLACE the player stands in, drawn to fill a phone. The choices are
OBJECTS inside the picture that she walks to and touches. There are no buttons.

## Canvas
- viewBox `0 0 360 640` (portrait, 9:16). Draw edge to edge. Anything outside is clipped.
- Output ONLY inner SVG markup (no <svg> wrapper, no xmlns, no doctype, no markdown).
- FIRST element: `<rect width="360" height="640" fill="SKY"/>` full-bleed background.

## Layers, in this order
1. `<g data-layer="bg">` sky, walls, far objects, light wash.
2. `<g data-layer="floor">` the ground plane. The floor line (where feet touch) is at **y=520**
   across the full width unless the scene is a rooftop (y=500) or a corridor (y=520).
   Draw the floor from the floor line down to 640 so she has somewhere to stand.
3. `<g data-layer="props">` furniture and decoration that is NOT a choice.
4. One `<g class="hot" data-opt="N" data-stand="X,Y">` per choice, N = 0..3 in story order.
   - Contains the drawn object itself (the ring light, the microphone, the cable coil).
   - The object must be big enough to tap: at least 70x70 units, ideally 90-130.
   - `data-stand` = the point on the floor line where she stands when she reaches it.
     X is roughly below/beside the object, Y is the floor line (520 or 500).
   - Spread the 3-4 hotspots across the width so no two overlap and each is reachable.
5. `<g data-layer="fg">` optional foreground framing (a doorframe edge, a plant), kept to the
   edges so it never covers a hotspot.

## The girl
DO NOT draw her. The engine places one shared actor (back view, ~62 units tall) on the floor
line at the scene's entry point. Leave the centre of the floor line clear at x=180 for her to
appear, and leave a clear path along the floor line to each data-stand point.

## Scale
Human scale is fixed: a door is ~150 tall, a table ~90 tall, a chair ~70, a phone on a
tripod ~110, a microphone stand ~120. Keep objects to this scale so she reads as a person
in a room.

## Style (unchanged from v1, strictly enforced)
- Flat vector only. Solid fills. No gradients, filters, images, text, emoji, ids, classes
  (except `hot` on hotspot groups), CSS, external refs.
- Rounded and plump. rx on every rect. Soft, cute, kawaii-adjacent. Depth by layering flat
  shapes and pale low-opacity washes, never shading.
- Strokes only with stroke-linecap="round".
- No apostrophes in the output.
- 2-3 tiny 4-point sparkles max, white or #FFE08A, low opacity.

## Palette (ONLY these)
pink    #FF4D97 #FF5BA3 #FF8FC0 #FF9DC0 #FFB0D4 #FFD7E9 #FFE4F0 #E0357D
purple  #8A4FD6 #B14BD1 #C9A7FF #CDBEF2 #DCCBFB #EFE6FF #9E8FD6 #B79BE0
warm    #FFCE57 #FFD86B #FFE08A #FFF3D6 #FFEAD9 #FBD9B0 #F6A24A #FFB199 #E8A95A
green   #17B6A6 #4FCF96 #86E3C6 #8FE3C8 #C7EBD0 #DFF6EC #B8E6C0 #2EA27E
blue    #4FA6E8 #8FD0FF #BFE2FF #E3F1FF #CFE2FB #3A8FD0
neutral #FFFFFF #FFF4F9 #9B9BB5 #43283A (ink, tiny details only)
deep    #6B7FD7 #C9D4FF #E8ECFF (the Ruang Dengar lane only)

## Time of day (sky per scene) — the day walks from pre-dawn to dusk
a1_pintu #CDBEF2 corridor, violet, warm light leaking under four arched doors
a2_suara #FFEAD9 early morning, warm gold indoors
a2_tangan #FFF3D6 early morning, honey lamp light
a2_papan #E3F1FF morning, cool white daylight through glass
a2_teras #CFE2FB morning after rain, bright grey-blue
a3_creator #FFE4F0 midday, pink ring light
a3_boss #EFE6FF midday, cool indoor light
a3_artisan #FFF3D6 afternoon, last dry light through a window
a3_deep #E8ECFF listening booth behind the stage, dim periwinkle, one round window
a3_change #DFF6EC afternoon, soft green quiet corner
a3_explorer #BFE2FF afternoon, open landing high in the building
a4_halaman #FFD86B late afternoon golden hour, back yard
a5_atap #FFB199 dusk rooftop, peach sky turning violet at the top

## Composition for a phone
- Horizon / eye level around y=300-360. Sky and far wall above, room and floor below.
- The important objects live in the band y=330..520 so they sit at hand height.
- Keep the top 90 units quiet (a caption band may overlay there) and the bottom 120 units
  quiet except floor (the caption/lamp UI may overlay there).

## Special scenes
- a1_pintu: FOUR arched doors across the far wall, each spilling a different coloured light
  down the floor toward the viewer. Each door is a hotspot (data-opt 0..3, left to right).
  Doors ~150 tall, arch tops around y=250, bases on a raised step at y=400, then the floor
  slopes to 520. Light pools from each door reach the floor line. This is the hero image.
- a4_halaman: a spilled cardboard box and scattered objects in a golden back yard, Nay
  nearby (a small second figure is allowed here, side view, same scale), a ladder on the right.
- a5_atap: rooftop at dusk, the city soft below the parapet, four places to put a small lamp.
