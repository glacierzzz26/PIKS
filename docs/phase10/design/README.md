# phase10 定稿设计索引

> 阶段:容器按功能拆分(部署形状)。已定稿设计在此登记,此后按它执行,改动需走变更。

## P10 容器拆分:单镜像 → 四镜像 ✅ 已定稿并实施(2026-09-20)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [container-split.md](./container-split.md) | ✅ **已定稿并实施** | 2026-09-20 | issue #47。把 `piks-tools`(934MB,同背 nginx + Go web + 12 CLI + Python 深研)拆为 `piks-gateway`(纯 nginx)/ `piks-web`(纯 Go API)/ `piks-tools`(9 管线命令)/ `piks-research`(Python + 深研队列 worker)。单 Dockerfile 多 `--target`。**唯一运行时行为改动**:UI 深研从「web 进程内 `os/exec python3`」改为 **DB 队列**(migration 0015 + `FOR UPDATE SKIP LOCKED` + `LISTEN/NOTIFY` + 租约回收)+ `cmd/research-worker` 常驻。**取代 D-2 单镜像**。已上生产(栈 `v0.0.0-4048f5a`)。 |

## 前序阶段

- **P4**(`docs/phase4/design/research-merge.md`):research 深研并入,D-11 独立迭代。本阶段把
  「深研在 web 进程内跑」改为队列驱动,并收窄 D-11 的一处隔离(见设计 §8)。
- **P3**(`docs/phase3/design/prod-deploy.md`):生产化部署形态。本阶段在其部署编排上重写
  `deploy.sh`(按镜像版本派生 + 顺序硬约束 + 栈清单)。
