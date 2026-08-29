// 前端冒烟(2026-08-29 起:SPA 只读 8 页 + Go 交互页 5 页,nginx/vite proxy 分流)。
// 运行:先起 vite dev(:3100,proxy 连真实后端 :8090)→ node scripts/e2e_check.mjs
// 断言:
//   SPA 页: 出现 #root(React 壳)、加载完成(无「加载中…」)、真实数据无「演示数据」徽章、无 console 错误
//   Go 页:  无 #root(Go HTML 渲染,经 proxy)→ 编辑页可达、无 console 错误
// 依赖 playwright(未装时用 PLAYWRIGHT_PATH=<playwright 包目录> 指定)
import { createRequire } from "module";
const require = createRequire(import.meta.url);
let chromium;
try {
  ({ chromium } = require("playwright"));
} catch {
  const dir = process.env.PLAYWRIGHT_PATH;
  if (!dir) throw new Error("缺少 playwright：npm i -D playwright 或设 PLAYWRIGHT_PATH");
  ({ chromium } = require(dir + "/playwright"));
}

const BASE = "http://localhost:3100";
const TIMEOUT = 20000;

const PAGES = [
  // SPA 只读页(React Router 客户端渲染)
  { path: "/", kind: "spa" },
  { path: "/events", kind: "spa" },
  { path: "/entities", kind: "spa" },
  { path: "/graph", kind: "graph" },
  { path: "/ladder", kind: "spa" },
  { path: "/flashes", kind: "spa" },
  { path: "/recon", kind: "spa" },
  { path: "/reviews", kind: "spa" },
  // Go 交互页(经 nginx/vite proxy → Go HTML 编辑页)
  { path: "/notes", kind: "go" },
  { path: "/trades", kind: "go" },
  { path: "/chat", kind: "go" },
  { path: "/settings", kind: "go" },
  { path: "/weekly", kind: "go" },
];

let pass = 0;
let fail = 0;

function report(page, ok, msg) {
  console.log(`${ok ? "PASS" : "FAIL"}  ${page}  ${msg}`);
  ok ? pass++ : fail++;
}

const browser = await chromium.launch({ args: ["--no-sandbox"] });

for (const { path, kind } of PAGES) {
  const page = await browser.newPage();
  const consoleErrors = [];
  page.on("console", (m) => {
    if (m.type() === "error") consoleErrors.push(m.text());
  });
  page.on("pageerror", (e) => consoleErrors.push(String(e)));

  try {
    await page.goto(BASE + path, { waitUntil: "domcontentloaded", timeout: TIMEOUT });

    if (kind === "go") {
      // Go HTML 页:无 #root(不是 SPA 壳),body 有真实内容
      const isSPA = await page.locator("#root").count();
      const textLen = (await page.evaluate(() => document.body.innerText.length)) || 0;
      const msgs = consoleErrors.filter((e) => !/favicon|manifest/i.test(e));
      report(path, isSPA === 0 && textLen > 50 && msgs.length === 0, `goHtml=ok text=${textLen} err=${msgs.length}`);
    } else {
      // SPA 页:等 React 挂载 + 数据加载完成(「加载中…」消失),断言无降级徽章
      const rendered = await page
        .waitForFunction(
          () =>
            !!document.getElementById("root") &&
            !document.body.innerText.includes("加载中…"),
          { timeout: TIMEOUT, polling: 500 }
        )
        .then(() => true)
        .catch(() => false);
      const badge = await page.locator("text=演示数据").count();
      const msgs = consoleErrors.filter((e) => !/favicon|manifest/i.test(e));
      const hasCanvas = kind === "graph" ? (await page.locator("canvas").count()) > 0 : true;
      const empty = await page.locator("text=暂无").count();
      report(path, rendered && badge === 0 && msgs.length === 0 && hasCanvas, `render=${rendered} demo=${badge > 0} empty=${empty} canvas=${hasCanvas} err=${msgs.length}`);
    }
  } catch (e) {
    report(path, false, `导航异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

await browser.close();
console.log(`\n=== ${pass} 通过 / ${fail} 失败 ===`);
process.exit(fail > 0 ? 1 : 0);
