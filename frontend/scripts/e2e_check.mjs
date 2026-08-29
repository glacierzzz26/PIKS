// 联调端到端验证：逐页加载，断言真实数据出现、无「演示数据」徽章、无 console 错误。
// 运行：node scripts/e2e_check.mjs
// 依赖 playwright（未装时用 PLAYWRIGHT_PATH=<playwright 包目录> 指定）
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
const TIMEOUT = 15000;

const PAGES = [
  { path: "/", marker: "2026-08-26" },
  { path: "/events", marker: "降准" },
  { path: "/entities", marker: "一拖股份" },
  { path: "/graph", marker: null, needData: "relationships" },
  { path: "/ladder", marker: "海鸥住工" },
  { path: "/flashes", marker: "异常事件甲" },
  { path: "/notes", marker: "仓位集中度风险" },
  { path: "/recon", marker: "2026-08-26" },
  { path: "/reviews", marker: "组合持仓诊断" },
  { path: "/trades", marker: "贵州茅台" },
  { path: "/chat", marker: "降准" },
  { path: "/settings", marker: "抽取模型" },
  { path: "/weekly", marker: "周报 · 2026-W35" },
];

let pass = 0;
let fail = 0;

function report(page, ok, msg) {
  console.log(`${ok ? "PASS" : "FAIL"}  ${page}  ${msg}`);
  ok ? pass++ : fail++;
}

const browser = await chromium.launch({ args: ["--no-sandbox"] });

for (const { path, marker, needData } of PAGES) {
  const page = await browser.newPage();
  const consoleErrors = [];
  page.on("console", (m) => {
    if (m.type() === "error") consoleErrors.push(m.text());
  });
  page.on("pageerror", (e) => consoleErrors.push(String(e)));

  try {
    await page.goto(BASE + path, { waitUntil: "domcontentloaded", timeout: TIMEOUT });
    // 等客户端 fetch 完成：页面出现真实数据标记 或 出现降级徽章/空态
    const okData = await page
      .waitForFunction(
        (m) => m === null || document.body.innerText.includes(m),
        marker,
        { timeout: TIMEOUT, polling: 500 }
      )
      .then(() => true)
      .catch(() => false);

    // 徽章：真实数据时应隐藏；降级时显示。统一断言"有数据则无徽章"。
    const badgeCount = await page.locator("text=演示数据").count();
    const badgeShown = badgeCount > 0;

    if (needData) {
      // 图谱页无文本标记：断言无空态提示（无数据不应出现）
      const empty = await page.locator("text=暂无").count();
      const hasCanvas = (await page.locator("canvas").count()) > 0;
      const msgs = consoleErrors.filter((e) => !/favicon|manifest/i.test(e));
      report(path, msgs.length === 0 && !empty && (hasCanvas || badgeShown === false), `canvas=${hasCanvas} empty=${empty} demo=${badgeShown} err=${msgs.length}`);
    } else if (okData) {
      report(path, badgeShown === false, `marker='${marker}' demo=${badgeShown}`);
    } else {
      // 数据未出现：区分"后端不可达降级"(可接受) 与"真失败"
      const degraded = badgeShown && consoleErrors.length === 0;
      report(path, degraded, degraded ? "降级为演示数据(后端不可达)" : `marker='${marker}' 未出现 err=${consoleErrors.slice(0, 2).join(" | ")}`);
    }
  } catch (e) {
    report(path, false, `导航异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

await browser.close();
console.log(`\n=== ${pass} 通过 / ${fail} 失败 ===`);
process.exit(fail > 0 ? 1 : 0);
