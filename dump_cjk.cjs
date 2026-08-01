const { execSync } = require('child_process');

const files = ['agent/prompt.md','ask/prompt.md','plan/prompt.md','debug/prompt.md'];
const cjk = /[\u4e00-\u9fff]/;

for (const f of files) {
  const orig = execSync(`git show HEAD:prompt/${f}`, { encoding: 'utf8' }).split('\n').map(l => l.replace(/\r$/, ''));
  console.log(`===== ${f} (${orig.length} lines) =====`);
  orig.forEach((l, i) => {
    if (cjk.test(l)) {
      console.log(`${String(i+1).padStart(4)}|${l}`);
    }
  });
}