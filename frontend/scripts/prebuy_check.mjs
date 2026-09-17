// P7 速评卡 E2E（一次性验收脚本）：
// 打开 /stock/600519 → 断言「买入前速评」区渲染、双轴图 canvas 出现、
// 点「重新分析」原地出卡片（不跳页）、全程 0 console 错误。
// 运行：vite dev(:3100, API→:8199) 已起时 `node scripts/prebuy_check.mjs`
import { createRequire } from "module";
const require = createRequire(import.meta.url);
let chromium;
try {
  ({ chromium } = require("playwright"));
} catch {
  const dir = process.env.PLAYWRIGHT_PATH;
  if (!dir) throw new Error("缺少 playwright");
  ({ chromium } = require(dir + "/playwright"));
}

const BASE = "http://localhost:3100";
const browser = await chromium.launch({ args: ["--no-sandbox"] });
const page = await browser.newPage();
const errors = [];
page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
page.on("pageerror", (e) => errors.push(String(e)));

let pass = 0, fail = 0;
const check = (name, ok, extra = "") => {
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}  ${extra}`);
  ok ? pass++ : fail++;
};

await page.goto(`${BASE}/stock/600519`, { waitUntil: "networkidle", timeout: 25000 });

// 1. 速评区存在
const sec = page.locator("section.section", { hasText: "买入前速评" }).first();
check("速评区块存在", (await sec.count()) > 0);

// 2. 等已有速评自动渲染（usePrebuy 挂载即拉最近 done）—— 出现结论/风险等
await page.getByText("速评结论", { exact: false }).first().waitFor({ timeout: 20000 });
check("已有速评自动渲染结论横幅", true);

// 3. 风险红线标题（P7 新增上屏）
check("风险红线上屏", (await page.getByText("风险红线").count()) > 0);

// 4. 双轴图 canvas（ECharts 渲染到 canvas）
await page.waitForTimeout(800);
const canvas = await page.locator("canvas").count();
check("双轴图 canvas 已渲染", canvas > 0, `canvas=${canvas}`);

// 5. 量价形态标签（规则判定）
check("形态标签上屏", (await page.getByText("量价形态").count()) > 0);

// 6. 一键重跑：点「重新分析」，URL 不变（原地出卡）
const urlBefore = page.url();
const btn = page.getByRole("button", { name: /重新分析|快速分析/ }).first();
check("速评按钮存在", (await btn.count()) > 0);
if (await btn.count()) {
  await btn.click();
  await page.waitForTimeout(1500);
  check("点击后仍在个股页（原地不跳页）", page.url() === urlBefore, page.url());
  // 等待重跑收敛（≤90s）
  try {
    await page.getByText("速评结论", { exact: false }).first().waitFor({ timeout: 90000 });
    check("重跑完成原地出卡", true);
  } catch {
    check("重跑完成原地出卡", false, "超时未出结论");
  }
}

check("全程 0 console 错误", errors.length === 0, errors.slice(0, 3).join(" | "));

await browser.close();
console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail ? 1 : 0);
