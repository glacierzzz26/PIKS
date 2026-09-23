// /m/upload 手机投递页冒烟：路由隔离(无侧栏) + 三态 + 卡片预览/编辑/确认(mock 后端)。
// 运行:先起 vite dev(:3100)→ node scripts/m_upload_check.mjs
// 依赖 playwright(未装时用 PLAYWRIGHT_PATH 指定)。
// 说明:识别链路用 page.route 打桩 —— 后端真实视觉调用受外部网关额度影响(429),
//       打桩能独立验证前端渲染/交互/布局,不依赖外部 AI 可用性。
import { createRequire } from "module";
import fs from "fs";
const require = createRequire(import.meta.url);
let chromium, devices;
try {
  ({ chromium, devices } = require("playwright"));
} catch {
  const dir = process.env.PLAYWRIGHT_PATH;
  if (!dir) throw new Error("缺少 playwright：npm i -D playwright 或设 PLAYWRIGHT_PATH");
  ({ chromium, devices } = require(dir + "/playwright"));
}

const BASE = "http://localhost:3100";
const TIMEOUT = 30000;
const FIXTURE = process.env.FIXTURE || "/tmp/tonghuashun-trade.png";

let pass = 0,
  fail = 0;
const report = (name, ok, detail = "") => {
  console.log(`${ok ? "✓" : "✗"} ${name}${detail ? " — " + detail : ""}`);
  ok ? pass++ : fail++;
};
const noHScroll = (page) =>
  page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1);

const tradePreview = {
  kind: "trade",
  attachment_id: "x",
  trades: [
    { include: true, exists: false, date: "2026-09-14", code: "600519", name: "贵州茅台", side: "buy", price: "1580.00", qty: "100", amount: "158000" },
    { include: true, exists: true, date: "2026-09-14", code: "300750", name: "宁德时代", side: "sell", price: "189.20", qty: "300", amount: "56760" },
  ],
  positions: [],
  watch: [],
};
const watchPreview = {
  kind: "watchlist",
  attachment_id: "x",
  trades: [],
  positions: [],
  watch: [
    { include: true, change: "add", code: "600519", name: "贵州茅台" },
    { include: true, change: "remove", code: "300750", name: "宁德时代" },
    { include: false, change: "keep", code: "600036", name: "招商银行" },
  ],
};

// P12 访问控制:/m/upload 也在鉴权门内(AuthGate),裸访问会跳 /login 致断言全灭。
// 用预共享 token(PIKS_AUTH_TOKEN)注入 Authorization 头 —— extraHTTPHeaders 对
// 页内 fetch 同样生效,故 AuthGate 的 GET /auth/me 会通过。
const browser = await chromium.launch();
const AUTH_TOKEN = process.env.PIKS_AUTH_TOKEN || "";
const ctx = await browser.newContext({
  ...devices["iPhone 13"],
  ...(AUTH_TOKEN ? { extraHTTPHeaders: { Authorization: `Bearer ${AUTH_TOKEN}` } } : {}),
});
const page = await ctx.newPage();
const errs = [];
page.on("pageerror", (e) => errs.push(String(e)));
page.on("console", (m) => m.type() === "error" && errs.push("console: " + m.text()));

// ---- 1) 路由隔离 / 布局 ----
await page.goto(BASE + "/m/upload", { waitUntil: "domcontentloaded", timeout: TIMEOUT });
await page.waitForSelector(".m-upload", { timeout: TIMEOUT });
report("路由隔离:无侧栏", (await page.locator(".side-nav").count()) === 0);
report("页面标题", (await page.locator(".m-upload h1").innerText()).includes("截图投递"));
const primary = page.locator(".m-primary").first();
report("初始态:主按钮禁用", await primary.isDisabled());
await page.getByRole("button", { name: "今日交易" }).click();
report("选类型后仍禁用(未选图)", await primary.isDisabled());
report("窄屏无横向滚动(选图态)", await noHScroll(page));

// ---- 2) 截图选择（相册入口，真实文件）----
if (!fs.existsSync(FIXTURE)) throw new Error(`夹具图不存在: ${FIXTURE}`);
const inputs = page.locator('.m-capture input[type="file"]');
report("拍照/相册双入口", (await inputs.count()) === 2);
await inputs.nth(1).setInputFiles(FIXTURE);
await page.waitForSelector(".m-thumb", { timeout: TIMEOUT });
report("缩略图预览", true);
report("选图后主按钮可用", !(await primary.isDisabled()));

// ---- 3) 识别 → 卡片预览（mock）----
await page.route("**/api/v1/trades/import", (r) =>
  r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(tradePreview) })
);
await primary.click();
await page.waitForSelector(".m-card", { timeout: TIMEOUT });
report("交易:识别出卡片", (await page.locator(".m-card").count()) === 2);
report("交易:已存在标注", (await page.locator(".m-card").first().locator(".m-card-head").innerText()).length > 0);
report("预览态无横向滚动", await noHScroll(page));

// 编辑：改价格 → 受控值更新
const priceInput = page.locator(".m-card").first().locator(".m-field", { hasText: "价格" }).locator("input");
await priceInput.fill("1600.00");
report("卡片行内可编辑", (await priceInput.inputValue()) === "1600.00");

// 确认入库（mock）→ 成功提示
await page.route("**/api/v1/trades/confirm", (r) =>
  r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ applied: 2 }) })
);
await page.locator(".m-actionbar .m-primary").click();
await page.waitForSelector(".m-note-ok", { timeout: TIMEOUT });
report("确认后成功提示", (await page.locator(".m-note-ok").innerText()).includes("已确认入库"));
report("成功后回到选图态", (await page.locator(".m-card").count()) === 0);

// ---- 4) 自选镜像：汇总 + 整组取消移出 ----
await page.route("**/api/v1/trades/import", (r) =>
  r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(watchPreview) })
);
await page.getByRole("button", { name: "自选股" }).click();
await inputs.nth(1).setInputFiles(FIXTURE);
await page.waitForSelector(".m-thumb", { timeout: TIMEOUT });
await page.locator(".m-primary").first().click();
await page.waitForSelector(".m-summary", { timeout: TIMEOUT });
const sum = await page.locator(".m-summary").innerText();
report("自选:将加入/将移出汇总", sum.includes("将加入") && sum.includes("将移出"), sum.replace(/\n/g, " "));
report("自选:keep 行可展示", (await page.locator(".m-card").count()) === 3);
const toggle = page.locator(".m-summary .m-link");
await toggle.click();
report("自选:整组取消移出可切换", (await toggle.innerText()).includes("恢复"));
report("预览态无横向滚动(自选)", await noHScroll(page));

report("无 console/page 错误", errs.length === 0, errs.slice(0, 3).join(" | "));

await browser.close();
console.log(`\n=== ${pass} 通过 / ${fail} 失败 ===`);
process.exit(fail > 0 ? 1 : 0);
