const spark=(x,y,s,f)=>`<path transform="translate(${x},${y})" d="M0 ${-s} Q${s*0.2} ${-s*0.2} ${s} 0 Q${s*0.2} ${s*0.2} 0 ${s} Q${-s*0.2} ${s*0.2} ${-s} 0 Q${-s*0.2} ${-s*0.2} 0 ${-s} Z" fill="${f}"/>`;
const heart=(x,y,s)=>{const u=s/16;return `M${x} ${y+6*u} C ${x-2*u} ${y+2*u} ${x-10*u} ${y-2*u} ${x-10*u} ${y-9*u} C ${x-10*u} ${y-14*u} ${x-4*u} ${y-16*u} ${x} ${y-11*u} C ${x+4*u} ${y-16*u} ${x+10*u} ${y-14*u} ${x+10*u} ${y-9*u} C ${x+10*u} ${y-2*u} ${x+2*u} ${y+2*u} ${x} ${y+6*u} Z`;};

function mascotBase(halo,topper,prop){
  const haloC=halo?`<circle cx="110" cy="116" r="106" fill="${halo}" opacity=".18"/>`:'';
  return `<svg class="mascot" width="220" height="236" viewBox="0 0 220 236" xmlns="http://www.w3.org/2000/svg" role="img" aria-hidden="true">
    ${haloC}
    <ellipse cx="110" cy="214" rx="56" ry="13" fill="#43283a" opacity=".06"/>
    <ellipse cx="60" cy="158" rx="13" ry="20" fill="#fff"/>
    <ellipse cx="160" cy="158" rx="13" ry="20" fill="#fff"/>
    <rect x="63" y="126" width="94" height="84" rx="45" fill="#fff"/>
    <ellipse cx="92" cy="208" rx="15" ry="10" fill="#fff"/>
    <ellipse cx="128" cy="208" rx="15" ry="10" fill="#fff"/>
    <circle cx="110" cy="94" r="57" fill="#fff"/>
    ${topper||''}
    <ellipse cx="91" cy="98" rx="6.5" ry="8.6" fill="#43283a"/>
    <ellipse cx="129" cy="98" rx="6.5" ry="8.6" fill="#43283a"/>
    <circle cx="93.4" cy="94.6" r="2.3" fill="#fff"/>
    <circle cx="131.4" cy="94.6" r="2.3" fill="#fff"/>
    <ellipse cx="72" cy="113" rx="9.5" ry="6" fill="#FF9DC0" opacity=".75"/>
    <ellipse cx="148" cy="113" rx="9.5" ry="6" fill="#FF9DC0" opacity=".75"/>
    <path d="M101 114 Q110 122 119 114" stroke="#43283a" stroke-width="3.3" fill="none" stroke-linecap="round"/>
    ${prop||''}
  </svg>`;
}
const MASCOT={
  creator:{halo:'#FF8FC0',
    topper:`<g transform="translate(110,41)"><path d="M0 0 L-27 -14 L-27 16 Z" fill="#FF5BA3"/><path d="M0 0 L27 -14 L27 16 Z" fill="#FF5BA3"/><circle r="8.5" fill="#E0357D"/></g>`,
    prop:`<g transform="translate(160,150) rotate(10)"><rect x="-13" y="-22" width="26" height="42" rx="7" fill="#FF5BA3"/><rect x="-9" y="-16" width="18" height="28" rx="4" fill="#fff"/><path d="${heart(0,-1,9)}" fill="#FF5BA3"/></g>`},
  boss:{halo:'#C9A7FF',
    topper:`<g transform="translate(110,30)"><path d="M-28 12 L-28 -9 L-14 3 L0 -15 L14 3 L28 -9 L28 12 Z" fill="#FFC24B" stroke="#EDA62F" stroke-width="2.5" stroke-linejoin="round"/><circle cx="0" cy="-3" r="3.6" fill="#fff"/><circle cx="-28" cy="-9" r="3" fill="#FFE39A"/><circle cx="28" cy="-9" r="3" fill="#FFE39A"/></g>`,
    prop:`<g transform="translate(161,150)"><rect x="-2.6" y="-6" width="5.2" height="42" rx="2.6" fill="#C9A7FF"/>${spark(0,-12,13,'#FFC24B')}</g>`},
  artisan:{halo:'#FFC2A8',
    topper:`<g transform="translate(110,33)"><g fill="#FF9DB4"><circle cx="0" cy="-11" r="8.4"/><circle cx="10.5" cy="-4" r="8.4"/><circle cx="6.5" cy="8.5" r="8.4"/><circle cx="-6.5" cy="8.5" r="8.4"/><circle cx="-10.5" cy="-4" r="8.4"/></g><circle r="7" fill="#FFD86B"/></g>`,
    prop:`<g transform="translate(160,150)"><path d="M0 27 L-7 2 M0 27 L0 -2 M0 27 L7 2" stroke="#4FCF96" stroke-width="3.2" fill="none" stroke-linecap="round"/><circle cx="-7" cy="-1" r="6.6" fill="#FF8FB0"/><circle cx="0" cy="-5" r="6.6" fill="#FFB3C6"/><circle cx="7" cy="-1" r="6.6" fill="#FFD86B"/></g>`},
  deep:{halo:'#C9D4FF',
    topper:`<path d="M56 96 Q110 -30 164 96" stroke="#6B7FD7" stroke-width="9" fill="none" stroke-linecap="round"/><circle cx="56" cy="98" r="13" fill="#6B7FD7"/><circle cx="164" cy="98" r="13" fill="#6B7FD7"/><circle cx="56" cy="98" r="7" fill="#C9D4FF"/><circle cx="164" cy="98" r="7" fill="#C9D4FF"/>`,
    prop:`<g transform="translate(160,152)"><rect x="-13" y="-10" width="24" height="22" rx="6" fill="#C9D4FF"/><path d="M11 -4 Q20 -4 20 4 Q20 12 11 12" stroke="#6B7FD7" stroke-width="3.5" fill="none"/><path d="M-6 -16 Q-3 -22 -6 -28" stroke="#FFFFFF" stroke-width="2.5" fill="none" stroke-linecap="round" opacity=".8"/><path d="M2 -16 Q5 -22 2 -28" stroke="#FFFFFF" stroke-width="2.5" fill="none" stroke-linecap="round" opacity=".8"/></g>`},
  change:{halo:'#86E3C6',
    topper:`<g transform="translate(110,34)"><rect x="-2.5" y="-5" width="5" height="13" rx="2.5" fill="#2EA27E"/><ellipse cx="-11" cy="-7" rx="11" ry="6.5" fill="#4FCF96" transform="rotate(-28 -11 -7)"/><ellipse cx="11" cy="-7" rx="11" ry="6.5" fill="#4FCF96" transform="rotate(28 11 -7)"/></g>`,
    prop:`<g transform="translate(160,150)"><path d="${heart(0,0,17)}" fill="#17B6A6"/></g>`},
  explorer:{halo:'#8FD0FF',
    topper:`<g transform="translate(110,36)"><path d="M-32 5 Q0 -34 32 5 Z" fill="#4FA6E8"/><rect x="-37" y="3" width="74" height="9" rx="4.5" fill="#3A8FD0"/><circle cx="0" cy="-20" r="4" fill="#BFE2FF"/></g>`,
    prop:`<g transform="translate(160,150)"><circle r="14" fill="#4FA6E8"/><circle r="9.5" fill="#EAF4FF"/><path d="M0 -8 L3.5 0 L0 8 L-3.5 0 Z" fill="#FF5B6E"/><circle r="2" fill="#3A8FD0"/></g>`},
};
const mascotSVG=k=>mascotBase(MASCOT[k].halo,MASCOT[k].topper,MASCOT[k].prop);
const blankMascot=()=>mascotBase('#FFD1E6','',`<g transform="translate(150,60)">${spark(0,0,11,'#FFD36E')}</g><g transform="translate(66,78)">${spark(0,0,8,'#C9A7FF')}</g>`);
