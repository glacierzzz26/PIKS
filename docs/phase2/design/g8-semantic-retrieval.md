# G8 — /chat 语义检索设计文档(方案 B:LLM 同义扩展)

> 状态:**草案(待定稿门确认,2026-08-28)**。范围:仅 `/chat` 知识库问答的检索升级(web-app §4.1 中「语义/向量检索列为后续」的兑现)。
> **约束:只做 dev 环境,不升级生产(lab)**——不部署 lab、不改 prod compose、生产 DB 无新迁移;生产 /chat 维持关键词检索,待后续独立部署决策。
> 前置:迭代 5-3 `/chat` 已上线(关键词检索 + 引用 + 截图)。契约依据:`internal/store/search.go`(现状检索)、`internal/ai/openai_compat.go`(provider 客户端)。

---

## 1. 背景与现状

`/chat` 问答的 grounding 目前用**中文 n-gram 关键词检索**(`store/search.go`):问题拆 2/3 字 gram → `ILIKE` 命中 title/summary/facts。已知缺陷(进度总表 G8):

> 「中文同义词/近义(如『降准』vs『下调存准』)命中差」——关键词检索是**字面匹配**,同义改写即漏检。

目标:让「降准」这类问法也能召回「下调存款准备金率」相关事件。

## 2. 探针结论(G8 契约缺口,2026-08-28 实测)

**当前唯一 provider OpenCode Zen 不提供 embeddings 端点**:

| 探针 | 结果 |
|---|---|
| `GET {Zen}/go/v1/models` | 仅对话/视觉模型,无 `text-embedding-*` |
| `POST {Zen}/go/v1/embeddings` | **HTTP 404**(路由不存在) |
| `POST {Zen}/v1/embeddings` | **HTTP 404** |
| 对照 `POST {Zen}/go/v1/chat/completions` | HTTP 200(路由/key 正常) |

→ **无现成 embedding API 可用**。语义能力需自建。备选的「本地 Ollama + bge-m3」方案(**方案 A**)已评估:真语义向量但新增 dev 常驻服务 + ~1.2GB 模型下载(CPU 推理、网络可达性有风险)。
**决策:不引入新基建,选方案 B——用现有 extract 档模型做 query 端同义扩展**,直接修 G8 点名场景,成本 = 每次提问 +1 次便宜档 LLM 调用。

## 3. 方案 B 原理(忠实描述,数据诚实)

```
问题 → LLM(extract 档)同义扩展 → [原文 + 同义/近义改写词]
     → 并入现有 n-gram 检索(原文词高权重,扩展词低权重)
     → hybrid 命中召回 → 组装 grounding → 回答
```

- **本质**:query 端语义扩展,corpus 端不建索引(非向量检索)。改进的是「问题措辞」→「语料措辞」的鸿沟,不引入新数据模型/迁移/服务。
- **扩展失败自动降级**:LLM 调用失败或未配置 → 回落纯关键词检索(现行为),页面如实标注检索模式,不报错不编造。

## 4. 扩展 prompt(`internal/ai` 新增 `ExpandQuery`)

```text
系统:你是 A 股投资知识库的检索词扩展器。给定用户问题,输出与问题语义相关的检索词列表。
规则:
- 第 1 个词必须是问题原文;
- 输出同义词、近义表达、常见改写(如「降准」→「下调存款准备金率」「存款准备金下调」「法定存款准备金率」);
- 只输出名词/动词短语,不输出停用词;
- 5~15 个,中文为主,可含英文缩写/数字;
- 仅输出 JSON 数组,如 ["降准","下调存款准备金率","存款准备金下调"]。
```

实现:复用 `OpenAICompat.StructuredOutput`(JSON mode、temperature 0,确定性),extract 档模型。解析失败/空数组 → 视为降级(纯原文检索)。

## 5. 检索改造(`internal/store/search.go`)

**加权 gram** 引入,支撑「原文高权重 / 扩展低权重」:

```go
type weightedGram struct { term string; weight float64 }  // 原文 weight=1.0,扩展 weight=0.5
```

- `SearchKnowledgeExpanded(ctx, q string, extra []string, eventLimit, entityLimit int)`:
  1. 原文 gram = `queryGrams(q)`(weight 1.0);
  2. 扩展词 = `queryGrams` 同法处理每个 extra 词(拆短词/2-3gram,剔停用词),weight 0.5;
  3. 同一 term 去重时取**较高**权重;
  4. ILIKE pattern 用全部 term(∪);打分按 term 权重加权(title 命中×2×weight,body ×1×weight);
  5. 排序仍按「加权分降序 → 时间新近」。
- 现有 `SearchKnowledge` 保留(降级路径 + 其他调用方不变)。

## 6. `/chat` 接入(`internal/web/chat.go`)

`answerChat` 检索段改为:

1. 读 app_config(每请求重读,现行为);取 extract 模型名;
2. `ExpandQuery`(extract 档)扩展问题;**失败 → extra=nil + note=「同义扩展不可用(模型调用失败),已用关键词检索」**;
3. `SearchKnowledgeExpanded(ctx, q, extra, 8, 8)` 检索;
4. 检索结果空 → 沿用现有「如实说明未检索到」逻辑;
5. `buildChatContext`/`extractRefs` 零改动;
6. **检索模式标注**:chat 页面(消息区 + 脚注)展示本次回答的检索模式(同义扩展 / 纯关键词)。

成本:每次提问 +1 次 extract 档短调用(~10 词输出),远小于回答生成本身;护栏(日预算)后续如需可接入,初版不做。

## 7. 配置与页面

- **无新 app_config 键**(复用现有 extract 模型配置;扩展走 extract 档,与抽取同源同档)。
- `/settings` 不加字段。
- 可选开关(列为可选,初版不做):`ai_retrieval_expansion` = on/off,默认 on。

## 8. 边界(明确不做)

- ❌ 不做真实向量/embedding(无 embedding API + 不引新基建,方案 A 已评估并否决)。
- ❌ 不做生产部署:不动 lab、不改 prod compose、生产 DB 无新迁移、生产 /chat 维持关键词检索(标注不受影响,生产不显示扩展标注)。
- ❌ 不做 embedding 索引/新鲜度管线(无索引,无此问题)。
- ❌ 不做多模型扩展配置(固定 extract 档)。

## 9. 验收清单(dev-only)

- [ ] 扩展:问「降准」→ 扩展词含「下调存款准备金率」类改写,召回原关键词检索漏掉的事件(需 dev 数据中构造或确认存在此类事件);
- [ ] 原文精确命中不退化:问已知实体名/事件标题,结果与改造前一致;
- [ ] 加权排序:原文命中项排在纯扩展命中项之前;
- [ ] 降级:强制扩展失败(如临时改错模型)→ /chat 回落关键词 + 如实标注,不报错不编造;
- [ ] 页面标注检索模式(同义扩展/纯关键词);
- [ ] **生产未动**:未部署 lab、prod 无新迁移/无新代码;
- [ ] 数据诚实:标注如实反映本次实际检索模式。

## 10. 涉及文件

- 新增:`internal/ai/expand.go`(`ExpandQuery` 扩展调用)。
- 修改:`internal/store/search.go`(加权 gram + `SearchKnowledgeExpanded`)、`internal/web/chat.go`(接入 + 降级 + 标注)、`internal/web/templates/chat.html` + 相关视图(检索模式标注)。
- 复用:`OpenAICompat.StructuredOutput`、`queryGrams`、`buildChatContext`/`extractRefs`、app_config extract 档配置。

## 11. 方案比选留痕

| | 方案 A:Ollama+bge-m3 真向量 | 方案 B:LLM 同义扩展(**选定**) |
|---|---|---|
| 语义能力 | 索引级,彻底 | query 端,覆盖同义表达 |
| 基建/运维 | +Ollama 常驻 + 模型下载 | 零 |
| 成本 | 免费离线,CPU 推理 | 每次提问 +1 次便宜档调用 |
| 降级 | Ollama 断 → 关键词 | 扩展失败 → 关键词 |
| 选因 | — | 零基建、复用现有 provider、直接修 G8 点名场景;生产可移植性最好 |
