// 前端冒烟(2026-09-14 重写:全站 React SPA,已无 Go HTML 交互页)。
// 运行:先起 vite dev(:3100,proxy 连真实后端 :8090)→ node scripts/e2e_check.mjs
//   P12 起后端需登录:导出 PIKS_AUTH_TOKEN=<预共享 token> 以跑全量数据断言(脚本会注入
//   Authorization 头);不设则只跑壳/导航档(后端已鉴权时数据断言自动跳过,不误报)。
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
  { path: "/board", kind: "spa" }, // issue #83 P-4：榜单（早/晚档事件榜，按时间序、不排名）
  { path: "/hot-topics", kind: "spa" }, // issue #68 D 层：热榜（独立数据源 + 独立页）
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
  { path: "/m/upload", kind: "spa" }, // 手机截图投递页(AppShell 之外,无侧栏)
];

let pass = 0;
let fail = 0;

function report(page, ok, msg) {
  console.log(`${ok ? "PASS" : "FAIL"}  ${page}  ${msg}`);
  ok ? pass++ : fail++;
}

const browser = await chromium.launch({ args: ["--no-sandbox"] });

// P12 访问控制:后端已需登录,裸请求会被 302 到 /login,于是全站页断言会集体误报
// 「路由回归」。用预共享 token(PIKS_AUTH_TOKEN)注入 Authorization 头绕过登录页。
// 未设 PIKS_AUTH_TOKEN 时回落到「探测到后端已鉴权则视作不可达」—— 宁可跳过数据断言,
// 也不把 401 误报成页面回归。
const AUTH_TOKEN = process.env.PIKS_AUTH_TOKEN || "";
const ctx = await browser.newContext(
  AUTH_TOKEN ? { extraHTTPHeaders: { Authorization: `Bearer ${AUTH_TOKEN}` } } : {}
);
const newPage = () => ctx.newPage();

// 后端可达性探测:决定断言档位(不可达时不把「加载失败」错报成路由回归)
let backendUp = false;
{
  const p = await newPage();
  try {
    const r = await p.goto("http://localhost:8090/api/v1/dashboard", { timeout: 3000 });
    backendUp = !!r && r.ok();
  } catch {
    backendUp = false;
  }
  await p.close();
}
if (!backendUp && !AUTH_TOKEN) {
  console.log("提示:后端可能已开鉴权(P12)。设 PIKS_AUTH_TOKEN 以跑全量数据断言。\n");
}
console.log(backendUp ? "后端 :8090 可达 —— 全量断言\n" : "后端 :8090 不可达 —— 仅壳/导航断言(数据断言跳过)\n");

for (const { path, kind } of PAGES) {
  const page = await newPage();
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

// 消息页三 tab（issue #73）：路由正确性只认**渲染后的 DOM** ——
// SPA 的 try_files 对任何路径都返 index.html，故 HTTP 200 与 bundle grep 均无鉴别力；
// 必须断言 tab 条存在 + 点「公告」真的发出 /api/v1/announcements 请求。
// 曾因 /events 误绑纯列表组件(events.tsx)致 tab 条消失、公告 tab 不可达。
{
  const page = await newPage();
  const reqs = [];
  page.on("request", (r) => reqs.push(r.url()));
  try {
    await page.goto(BASE + "/events", { waitUntil: "domcontentloaded", timeout: TIMEOUT });
    await page.waitForSelector(".chip-btn", { timeout: TIMEOUT }).catch(() => {});
    // ⚠️ 不能按 .chip-btn 总数断言：重要消息 tab 内还有状态筛选 chip（全部/已抽取/已被合并），
    // 故取 tab 条(filter-bar)内前 3 个 chip 的标签判定。
    const tabs = (await page.locator(".filter-bar").first().locator(".chip-btn").allInnerTexts())
      .slice(0, 3).map((t) => t.trim());
    const tabBar = tabs.join(",") === "重要消息,快讯,公告";
    // 公告 tab 可达性：点击后必须触发公告接口（backendUp 时才断言接口，否则只断言 DOM）
    let annReachable = true;
    if (backendUp) {
      await page.locator(".chip-btn", { hasText: "公告" }).first().click();
      await page.waitForTimeout(1500);
      annReachable = reqs.some((u) => u.includes("/api/v1/announcements"));
    }
    report("/events tab条", tabBar, `tabs=[${tabs.join(", ")}]`);
    if (backendUp) report("/events 公告tab可达", annReachable, `announcements 请求=${annReachable}`);
  } catch (e) {
    report("/events tab条", false, `异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

// 消息页筛选回归（issue #80）：断言「点筛选 → URL 真的带上该参数 **且** 列表真的变了」。
//
// ⚠️ 为何必须两条一起断：旧版只断言「进某页渲染出 tab 条」，照不到筛选 ——
// bug 本体是「改动摇篮里就丢了」，URL 静默回退到旧值、UI 无任何反馈。
// 只查 URL 也不够：筛选可能写进了 URL 但后端没认（如 strSub 不搜 affected），
// 故必须同时断言**可见列表确实变化**（分页条声称的总数变小）。
//
// 挂后端 + 有数据才跑；空库/无后端时如实跳过（不把「没样本」误报成回归）。
if (backendUp) {
  const page = await newPage();
  try {
    await page.goto(BASE + "/events", { waitUntil: "domcontentloaded", timeout: TIMEOUT });
    const hasRows = await page
      .waitForSelector(".table tbody tr", { timeout: TIMEOUT })
      .then(() => true)
      .catch(() => false);

    // 行数用「分页条声称的总数」而非 DOM 行数 —— 后者被 20/页上限截断，
    // 全量>20 时筛选前后都恒为 20，无鉴别力。
    const totalOf = async () => {
      const t = await page.locator(".pager .num").first().innerText().catch(() => "");
      const m = t.match(/共\s*(\d+)\s*条/);
      return m ? Number(m[1]) : null;
    };
    const rowsOf = async () => page.locator(".table tbody tr").count();

    if (!hasRows) {
      report("/events 筛选URL", true, "无事件数据，跳过筛选断言");
    } else {
      const totalAll = await totalOf();
      const rowsAll = await rowsOf();

      // ① 状态 chip：点「已抽取」→ URL 带 status=extracted（旧 bug：此参数被静默丢弃）
      const before1 = page.url();
      await page.locator(".filter-bar .chip-btn", { hasText: "已抽取" }).first().click();
      await page.waitForTimeout(1200);
      const urlOk = /[?&]status=extracted\b/.test(page.url());
      report(
        "/events 筛选URL",
        urlOk && page.url() !== before1,
        `点击前=${before1.split("?")[1] ?? "(无)"} 点击后=${page.url().split("?")[1] ?? "(无)"}`
      );

      // ② 列表确实变短（若后端/前端没把该筛选接上，总数不会动）
      const totalExtracted = await totalOf();
      report(
        "/events 筛选生效",
        totalAll !== null && totalExtracted !== null && totalExtracted < totalAll,
        `全部=${totalAll} → 已抽取=${totalExtracted}`
      );

      // ③ 改页大小：setSize 曾连调两次 setParam（size 被 page 覆盖），断言 size 落到 URL
      await page.locator(".pager select").selectOption("100");
      await page.waitForTimeout(1200);
      report(
        "/events 页大小URL",
        /[?&]size=100\b/.test(page.url()),
        `URL=${page.url().split("?")[1] ?? "(无)"}`
      );
      report(
        "/events 页大小生效",
        totalAll !== null && (await rowsOf()) >= Math.min(100, totalAll),
        `全部=${totalAll} 行数=${await rowsOf()}`
      );

      // ④ 搜索口径：占位符承诺搜「影响实体」，取首行实体名做关键词，断言确实搜得到
      if (rowsAll > 0) {
        const ent = (await page.locator(".table tbody tr").first().locator("td").nth(2).innerText())
          .trim()
          .split(/[\s,、]+/)[0];
        if (ent && totalAll !== null && totalAll > 1) {
          await page.locator(".f-search input").first().fill(ent);
          await page.locator(".f-search input").first().press("Enter");
          await page.waitForTimeout(1200);
          const totalQ = await totalOf();
          report(
            "/events 搜影响实体",
            /[?&]q=/.test(page.url()) && totalQ !== null && totalQ < totalAll,
            `实体「${ent}」 全部=${totalAll} → 搜索结果=${totalQ}`
          );
        } else {
          report("/events 搜影响实体", true, `样本不足(实体=${ent} 总数=${totalAll})，跳过`);
        }
      }
    }
  } catch (e) {
    report("/events 筛选URL", false, `异常 ${String(e).slice(0, 120)}`);
  }
  await page.close();
}

// 侧栏导航 11 项：逐项点进，确认渲染 SPA 壳(防导航标签/分组重构断链)
{
  const page = await newPage();
  try {
    await page.goto(BASE + "/", { waitUntil: "domcontentloaded", timeout: TIMEOUT });
    const links = await page.locator("nav a[href], aside a[href]").evaluateAll((els) =>
      [...new Set(els.map((e) => e.getAttribute("href")).filter((h) => h && h.startsWith("/")))]
    );
    report("nav", links.length >= 10, `侧栏可点 ${links.length} 项`);
    for (const href of links.slice(0, 14)) {
      const p = await newPage();
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
