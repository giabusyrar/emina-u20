// @ts-nocheck -- synth ported verbatim from the original template (shortcut: untyped, upgrade if it grows)
export const SFX:any={ctx:null,ambGain:null,
  unlock(){if(this.ctx)return;try{this.ctx=new (window.AudioContext||(window as any).webkitAudioContext)();this.ambient();}catch(e){}},
  ambient(){const c=this.ctx;if(!c)return;const g=c.createGain();g.gain.value=0;const lp=c.createBiquadFilter();lp.type='lowpass';lp.frequency.value=400;
    [110,110.6].forEach(f=>{const o=c.createOscillator();o.frequency.value=f;o.connect(lp);o.start();});
    lp.connect(g);g.connect(c.destination);g.gain.setTargetAtTime(.012,c.currentTime,1.2);this.ambGain=g;},
  noise(dur){const c=this.ctx,b=c.createBuffer(1,c.sampleRate*dur,c.sampleRate),d=b.getChannelData(0);for(let i=0;i<d.length;i++)d[i]=Math.random()*2-1;const s=c.createBufferSource();s.buffer=b;return s;},
  step(side){const c=this.ctx;if(!c)return;const s=this.noise(.04),bp=c.createBiquadFilter();bp.type='bandpass';bp.frequency.value=420;bp.Q.value=1.2;
    const g=c.createGain();g.gain.value=.05;const p=c.createStereoPanner?c.createStereoPanner():null;if(p)p.pan.value=side?.15:-.15;
    s.connect(bp);bp.connect(g);if(p){g.connect(p);p.connect(c.destination);}else g.connect(c.destination);s.start();},
  tick(){const c=this.ctx;if(!c)return;const o=c.createOscillator(),g=c.createGain();o.type='sine';o.frequency.value=1800;g.gain.setValueAtTime(.025,c.currentTime);
    g.gain.exponentialRampToValueAtTime(.0005,c.currentTime+.05);o.connect(g);g.connect(c.destination);o.start();o.stop(c.currentTime+.06);},
  pick(){const c=this.ctx;if(!c)return;[660,880].forEach((f,i)=>{const o=c.createOscillator(),g=c.createGain();o.type='triangle';o.frequency.value=f;const t=c.currentTime+i*.07;
    g.gain.setValueAtTime(0,t);g.gain.linearRampToValueAtTime(.06,t+.02);g.gain.exponentialRampToValueAtTime(.001,t+.3);o.connect(g);g.connect(c.destination);o.start(t);o.stop(t+.32);});},
  swoosh(){const c=this.ctx;if(!c)return;const n=this.noise(.5),lp=c.createBiquadFilter();lp.type='lowpass';lp.frequency.setValueAtTime(200,c.currentTime);lp.frequency.exponentialRampToValueAtTime(1600,c.currentTime+.45);
    const g=c.createGain();g.gain.setValueAtTime(.0001,c.currentTime);g.gain.exponentialRampToValueAtTime(.05,c.currentTime+.25);g.gain.exponentialRampToValueAtTime(.0001,c.currentTime+.5);n.connect(lp);lp.connect(g);g.connect(c.destination);n.start();},
  lamp(){const c=this.ctx;if(!c)return;const o=c.createOscillator(),g=c.createGain();o.type='triangle';o.frequency.setValueAtTime(660,c.currentTime);
    o.frequency.linearRampToValueAtTime(990,c.currentTime+.18);g.gain.setValueAtTime(0,c.currentTime);g.gain.linearRampToValueAtTime(.08,c.currentTime+.18);
    g.gain.exponentialRampToValueAtTime(.001,c.currentTime+1.6);o.connect(g);g.connect(c.destination);o.start();o.stop(c.currentTime+1.7);},
  chord(){const c=this.ctx;if(!c)return;const dl=c.createDelay();dl.delayTime.value=.12;const fb=c.createGain();fb.gain.value=.35;dl.connect(fb);fb.connect(dl);dl.connect(c.destination);
    [523,659,784].forEach((f,i)=>{const o=c.createOscillator(),g=c.createGain();o.type='triangle';o.frequency.value=f;const t=c.currentTime+i*.16;
      g.gain.setValueAtTime(0,t);g.gain.linearRampToValueAtTime(.07,t+.1);g.gain.exponentialRampToValueAtTime(.001,t+2.4);o.connect(g);g.connect(c.destination);g.connect(dl);o.start(t);o.stop(t+2.5);});}
};
export const haptic=(p:number)=>{try{navigator.vibrate&&navigator.vibrate(p);}catch(e){}};
