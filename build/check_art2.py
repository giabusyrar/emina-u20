#!/usr/bin/env python3
"""Validate story/art2/*.svg against ART-CONTRACT.md (hotspots, stands, palette, forbidden)."""
import json, os, re, glob, sys
ROOT=os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ART=os.path.join(ROOT,"story","art2")
sp=os.path.join(ROOT,"story","v4-story.json")
if not os.path.exists(sp): sp=os.path.join(ROOT,"story","v3-story.json")
S={s["id"]:s for s in json.load(open(sp))["scenes"]}
PAL=set("""#FF4D97 #FF5BA3 #FF8FC0 #FF9DC0 #FFB0D4 #FFD7E9 #FFE4F0 #E0357D #8A4FD6 #B14BD1 #C9A7FF #CDBEF2 #DCCBFB #EFE6FF #9E8FD6 #B79BE0
#FFCE57 #FFD86B #FFE08A #FFF3D6 #FFEAD9 #FBD9B0 #F6A24A #FFB199 #E8A95A #17B6A6 #4FCF96 #86E3C6 #8FE3C8 #C7EBD0 #DFF6EC #B8E6C0 #2EA27E
#4FA6E8 #8FD0FF #BFE2FF #E3F1FF #CFE2FB #3A8FD0 #FFFFFF #FFF4F9 #9B9BB5 #43283A #6B7FD7 #C9D4FF #E8ECFF""".split())
bad=0
for sid,sc in S.items():
    p=os.path.join(ART,sid+".svg")
    if not os.path.exists(p): print("  ✗ %-12s MISSING"%sid); bad+=1; continue
    s=open(p).read(); probs=[]
    n=len(sc["options"])
    hots=re.findall(r'<g[^>]*class="hot"[^>]*>',s)
    if len(hots)!=n: probs.append("hotspots %d (need %d)"%(len(hots),n))
    opts=sorted(int(x) for x in re.findall(r'data-opt="(\d+)"',s))
    if opts!=list(range(n)): probs.append("data-opt %s"%opts)
    stands=re.findall(r'data-stand="(\d+(?:\.\d+)?),(\d+(?:\.\d+)?)"',s)
    if len(stands)!=n: probs.append("data-stand %d"%len(stands))
    if not re.search(r'<rect[^>]*width="360"[^>]*height="640"',s) and not re.search(r'<rect[^>]*height="640"[^>]*width="360"',s): probs.append("no full-bleed bg")
    for bad_el in ("<svg","<text","<image","<linearGradient","<radialGradient","<filter","```"):
        if bad_el in s: probs.append("forbidden "+bad_el)
    if "'" in s: probs.append("apostrophe")
    if re.search(r'\bid="',s): probs.append("id attr")
    off=sorted({h for h in re.findall(r'#[0-9a-fA-F]{3,6}',s) if h.upper() not in PAL})
    if off: probs.append("off-palette "+",".join(off[:4]))
    kb=len(s)/1024
    print(("  ✓" if not probs else "  ✗")+" %-12s %5.1fKB  %s"%(sid,kb,"; ".join(probs) if probs else "stands="+",".join(a for a,_ in stands)))
    bad+=bool(probs)
print("\n%d scene(s) need attention"%bad); sys.exit(1 if bad else 0)
