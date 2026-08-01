const { execSync } = require('child_process');
const fs = require('fs');

const files = ['agent/prompt.md','ask/prompt.md','plan/prompt.md','debug/prompt.md','common_prefix.md','multitask/prompt.md','subagent/prompt.md'];
const cjk = /[\u4e00-\u9fff]/;

for (const f of files) {
  const orig = execSync(`git show HEAD:prompt/${f}`, { encoding: 'utf8' }).split('\n');
  // strip trailing \r
  const origLines = orig.map(l => l.replace(/\r$/, ''));
  const newLines = fs.readFileSync(`prompt/${f}`, 'utf8').split('\n').map(l => l.replace(/\r$/, ''));
  // drop trailing empty line artifacts if lengths differ by exactly the trailing \n
  const n = Math.min(origLines.length, newLines.length);
  let mism = 0;
  let untranslated = 0;
  const samples = [];
  for (let i = 0; i < n; i++) {
    const o = origLines[i];
    const m = newLines[i];
    if (!cjk.test(o)) {
      if (o !== m) {
        mism++;
        if (samples.length < 8) samples.push(`L${i+1} ORIG: ${JSON.stringify(o)}\nL${i+1} NEW : ${JSON.stringify(m)}`);
      }
    } else {
      if (cjk.test(m)) untranslated++;
    }
  }
  console.log(`=== ${f} ===`);
  console.log(`orig=${origLines.length} new=${newLines.length} structuralMismatch=${mism} untranslatedCJK=${untranslated}`);
  if (samples.length) console.log(samples.join('\n'));
}