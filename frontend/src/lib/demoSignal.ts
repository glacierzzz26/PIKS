/**
 * 全局降级信号：任意 useData 落入演示数据时计数，TopNav 据此显示「演示数据」徽章。
 * 联调后数据来自真实后端，无降级则不显示徽章（诚实标注）。
 */

let count = 0;
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((fn) => fn());
}

export function incDemo() {
  count += 1;
  emit();
}

export function decDemo() {
  count = Math.max(0, count - 1);
  emit();
}

export function anyDemo(): boolean {
  return count > 0;
}

/** 订阅徽章可见性变化，返回取消订阅函数。 */
export function onDemoChange(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}
