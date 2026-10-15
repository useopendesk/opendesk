/*
 * Tiny timeline for the product scenes, so they act out a moment instead of
 * sitting still: a ticket arrives, a teammate starts typing, a draft streams in.
 *
 *   <div data-seq data-hold="3200">           a timeline; loops while on screen
 *     <div data-step="1" data-wait="900">    gets .on at step 1, 900ms after step 0
 *     <p data-step="2" data-stream>          gets .on, then its words stream in
 *     <i data-step="1" data-until="3">       shows from step 1, leaves at step 3
 *
 * Each round eases back to the first frame and plays again, without a blank.
 * Without JS, or under reduced motion, every step shows in its final state.
 */
const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function splitWords(el: HTMLElement) {
  if (el.dataset.split) return;
  el.dataset.split = '1';
  const walk = (node: Node) => {
    for (const child of [...node.childNodes]) {
      if (child.nodeType === Node.TEXT_NODE) {
        const frag = document.createDocumentFragment();
        for (const part of (child.textContent ?? '').split(/(\s+)/)) {
          if (!part) continue;
          if (/^\s+$/.test(part)) frag.append(part);
          else {
            const s = document.createElement('span');
            s.className = 'w';
            s.textContent = part;
            frag.append(s);
          }
        }
        child.replaceWith(frag);
      } else if (child.nodeType === Node.ELEMENT_NODE) walk(child);
    }
  };
  walk(el);
}

async function stream(el: HTMLElement, isLive: () => boolean, token: () => number, run: number) {
  const words = [...el.querySelectorAll<HTMLElement>('.w')];
  const speed = Number(el.dataset.stream) || 38;
  for (const w of words) {
    while (!isLive()) await sleep(250);
    if (token() !== run) return;
    w.classList.add('on');
    await sleep(speed + Math.random() * speed);
  }
}

export function initScenes() {
  const seqs = [...document.querySelectorAll<HTMLElement>('[data-seq]')];
  if (!seqs.length) return;
  if (reduced()) return; // final state is the default state

  for (const seq of seqs) {
    const box = seq.closest<HTMLElement>('.stage-box, [data-live]') ?? seq;
    const isLive = () => box.classList.contains('is-live');
    const steps = [...seq.querySelectorAll<HTMLElement>('[data-step]')];
    const max = Math.max(...steps.map((s) => Number(s.dataset.step)));
    seq.querySelectorAll<HTMLElement>('[data-stream]').forEach(splitWords);
    seq.classList.add('seq-js');
    let run = 0;
    const token = () => run;

    const loop = async () => {
      for (;;) {
        run++;
        const mine = run;
        steps.forEach((s) => s.classList.remove('on', 'off'));
        seq.querySelectorAll('.w.on').forEach((w) => w.classList.remove('on'));
        for (let i = 0; i <= max; i++) {
          const here = steps.filter((s) => Number(s.dataset.step) === i);
          const wait = Math.max(0, ...here.map((s) => Number(s.dataset.wait ?? 1100)));
          let waited = 0;
          while (waited < wait) {
            await sleep(100);
            if (isLive()) waited += 100;
          }
          steps.filter((s) => s.dataset.until && Number(s.dataset.until) <= i).forEach((s) => s.classList.add('off'));
          const streams: Promise<void>[] = [];
          for (const s of here) {
            s.classList.add('on');
            if (s.hasAttribute('data-stream')) streams.push(stream(s, isLive, token, mine));
            s.querySelectorAll<HTMLElement>('[data-stream]:not([data-step])').forEach((x) => streams.push(stream(x, isLive, token, mine)));
          }
          await Promise.all(streams);
        }
        const hold = Number(seq.dataset.hold ?? 3500);
        let held = 0;
        while (held < hold) {
          await sleep(100);
          if (isLive()) held += 100;
        }
        if (seq.dataset.once !== undefined) return;
        // Settle back: every step eases to its first state, the words last, so
        // the next round starts from a calm frame instead of a blank one.
        steps.forEach((s) => s.classList.remove('on', 'off'));
        await sleep(650);
        seq.querySelectorAll('.w.on').forEach((w) => w.classList.remove('on'));
        await sleep(450);
      }
    };
    loop();
  }
}
