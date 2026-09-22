-- 清理 events.affected / facts 的**非数组**值(issue #71)。
--
-- 成因:internal/extract 曾用裸 json.Marshal 序列化 []string,nil 切片 → 字面量 `null`
--   (err 仍为 nil,回退分支不触发),events.affected 因此落成 JSON null。
--   消费方 jsonb_array_elements_text(e.affected) 遇标量报 SQLSTATE 22023
--   「cannot extract elements from a scalar」→ entity-build 每轮必崩。
--
-- 写入侧已修(mustJSONArray / emptyToArrayIfScalar);本脚本只清历史脏行。
-- ⚠️ 幂等:只命中 jsonb_typeof <> 'array' 的行,重复执行第二次即 0 行。
-- ⚠️ 语义:标量/null 一律归一为**空数组** —— 与写入侧「宁可存空数组,不可存标量」一致。
--     不猜测标量的原意(如把字符串包装成单元素数组),避免伪造出上游并未给出的数据。

UPDATE events SET affected = '[]'::jsonb
 WHERE jsonb_typeof(affected) <> 'array';

UPDATE events SET facts = '[]'::jsonb
 WHERE jsonb_typeof(facts) <> 'array';
