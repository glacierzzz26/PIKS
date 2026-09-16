// 前端冒烟(2026-09-14 重写:全站 React SPA,已无 Go HTML 交互页)。
// 运行:先起 vite dev(:3100,proxy 连真实后端 :8090)→ node scripts/e2e_check.mjs
// 断言分两档,取决于后端是否可达(脚本自动探测):
//   后端可达: 每页数据加载完成(无「加载中…」)、无降级徽章、无 console 错误
//   后端不可达: 仅断言 SPA 壳挂载(#root)、无 console 错误 —— 导航/路由回归网仍有效
// 导航断言恒执行: 侧栏各项可达且渲染 SPA 壳。
// 依赖 playwright(未装时用 PLAYWRIGHT_PATH=<playwright 包目录> 指定);
// 浏览器缓存缺失时 `npx playwright install chromium`;
// 缺系统库(libnspr4/libnss3/libasound)时可用 LD_LIBRARY_PATH 指向本地解包的 .deb。
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

// 全部 SPA 路由（含 P5 深链与 P6-2 保留路由 —— 防路由回归）
const PAGES = [
  { path: "/", kind: "watch" }, // 今天 = 引导式首页(Phase 3 落，现为自选)
  { path: "/market", kind: "spa" },
  { path: "/ladder", kind: "spa" },
  { path: "/events", kind: "spa" }, // 消息页主 tab
  { path: "/flashes", kind: "spa" }, // 旧深链 → 消息页快讯 tab
  { path: "/research", kind: "spa" },
  { path: "/reports", kind: "spa" }, // P9-2 研报列表(体裁独立入口)
  { path: "/trades", kind: "spa" },
  { path: "/reviews", kind: "spa" },
  { path: "/weekly", kind: "spa" },
  { path: "/notes", kind: "spa" },
  { path: "/chat", kind: "spa" },
  { path: "/settings", kind: "spa" },
  { path: "/help", kind: "spa" }, // P6-2 新增:使用指南 + 词汇表(静态)
  // P6-2 移出侧栏但保留的路由
  { path: "/entities", kind: "spa" },
  { path: "/graph", kind: "graph" },
  { path: "/recon", kind: "spa" },
];

let pass = 0;
let fail = 0;

function report(page, ok, msg) {
  console.log(`${ok ? "PASS" : "FAIL"}  ${page}  ${msg}`);
  ok ? pass++ : fail++;
}

const browser = await chromium.launch({ args: ["--no-sandbox"] });

// 后端可达性探测:决定断言档位(不可达时不把「加载失败」错报成路由回归)
let backendUp = false;
{
  const p = await browser.newPage();
  try {
    const r = await p.goto("http://localhost:8090/api/v1/dashboard", { timeout: 3000 });
    backendUp = !!r && r.ok();
  } catch {
    backendUp = false;
  }
  await p.close();
}
console.log(backendUp ? "后端 :8090 可达 —— 全量断言\n" : "后端 :8090 不可达 —— 仅壳/导航断言(数据断言跳过)\n");

for (const { path, kind } of PAGES) {
  const page = await browser.newPage();
  const consoleErrors = [];
  page.on("console", (m) => {
    if (m.type() === "error") consoleErrors.push(m.text());
  });
  page.on("pageerror", (e) => consoleErrors.push(String(e)));

  try {
    await page.goto(BASE + path, { waitUntil: "domcontentloaded", timeout: TIMEOUT });

    if (!backendUp) {
      // 无后端:只验 SPA 壳挂载 + 无 JS 异常
      const mounted = await page
        .waitForFunction(() => !!document.getElementById("root"), { timeout: TIMEOUT, polling: 500 })
        .then(() => true)
        .catch(() => false);
      const msgs = consoleErrors.filter((e) => !/favicon|manifest/i.test(e));
      report(path, mounted && msgs.length === 0, `shell=${mounted} err=${msgs.length} (backend down)`);
      await page.close();
      continue;
    }

    // 后端可达:等 React 挂载 + 数据加载完成(「加载中…」消失)
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
    // 图谱用自绘 SVG 力导（非 echarts canvas）
    const hasCanvas = kind === "graph" ? (await page.locator("svg").count()) > 0 : true;
    const empty = await page.locator("text=暂无").count();
    report(
      path,
      rendered && badge === 0 && msgs.length === 0 && hasCanvas,
      `render=${rendered} demo=${badge > 0} empty=${empty} canvas=${hasCanvas} err=${msgs.length}`
    );
  } catch (e) {
    report(path, false, `导航异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

// 侧栏导航 11 项：逐项点进，确认渲染 SPA 壳(防导航标签/分组重构断链)
{
  const page = await browser.newPage();
  try {
    await page.goto(BASE + "/", { waitUntil: "domcontentloaded", timeout: TIMEOUT });
    const links = await page.locator("nav a[href], aside a[href]").evaluateAll((els) =>
      [...new Set(els.map((e) => e.getAttribute("href")).filter((h) => h && h.startsWith("/")))]
    );
    report("nav", links.length >= 10, `侧栏可点 ${links.length} 项`);
    for (const href of links.slice(0, 14)) {
      const p = await browser.newPage();
      const errs = [];
      p.on("pageerror", (e) => errs.push(String(e)));
      await p.goto(BASE + href, { waitUntil: "domcontentloaded", timeout: TIMEOUT });
      const ok = await p
        .waitForFunction(() => !!document.getElementById("root"), { timeout: TIMEOUT })
        .then(() => true)
        .catch(() => false);
      report(`nav${href}`, ok && errs.length === 0, `spa=${ok} err=${errs.length}`);
      await p.close();
    }
  } catch (e) {
    report("nav", false, `导航遍历异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

await browser.close();
console.log(`\n=== ${pass} 通过 / ${fail} 失败 ===`);
process.exit(fail > 0 ? 1 : 0);
