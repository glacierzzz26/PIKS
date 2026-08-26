# sample 测试夹具

- `sample-news.json`:采集/抽取链路的**合成测试数据**(非真实新闻)。
  - 公司名"星河新能源"为虚构主体,避免把真实世界的未发生事件当作事实入库。
  - 用于迭代0 端到端验收:采集 → 去重 → AI 抽取 → 发布 → Obsidian 可见。
- `sample-real.json`:**真实 provider 验证 fixture**(#17,2026-08-26)。
  - 内容为真实措辞(央行降准 ×2 不同表述 + 宁德时代),用于去 mock 跑真实 deepseek-v4-flash 全链。
  - 注意:内容为示例事实,非真实新闻源采集。
- 真实数据源接入见 `docs/phase1/design/iter0-min-loop.md` §6(东财快讯为 G1 缺口,待验证)。
