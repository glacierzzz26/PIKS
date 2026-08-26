# 生产化 P3:部署实现与验收归档

> 子阶段:生产化实现与验收(2026-08-27)
> 前置:`docs/phase3/design/prod-deploy.md` 已定稿(D-P1~D-P12,D-P10=方案 B)
> 状态:✅ 已落地(除 2 项外部依赖待办,见 §6)

## 1. 部署模型(实现期修正后)

- **dev 本地编译 → 镜像传输 lab**:`docker build --build-arg GIT_SHORT=<hash>` → `docker save | ssh lab docker load`。lab **不保留代码仓库、不 clone GitHub**;镜像 = 唯一交付物。
- lab 只持有:`docker-compose.yml`(模板实例化)、`.env`(0600 真实密钥)、`vault/`(Generated 工作仓,基线从 dev rsync)、`scripts/`(pipeline/backup/health 3 个 lab 侧脚本)、`backups/`、`logs/`。
- 编排全部在 dev 侧:`scripts/setup.sh`(一次性初始化,10 步)+ `scripts/deploy.sh`(更新,dev 侧)。
- 密钥只存 lab `/home/rguo/piks/.env`(0600);代码仓/vault 仓零密钥;聊天零密钥。

## 2. 已交付物

| 交付物 | 位置 | 说明 |
|---|---|---|
| `Dockerfile` | 仓库根 | vendored 多阶段构建;CGO=0 静态;`PIKS_GIT_SHORT` 烘焙血缘;运行镜像 ~50MB,含 11 bin + migrations/ + prompts/ |
| `configs/docker-compose.prod.yml` | 仓库 | 生产 compose 模板:postgres 常驻 + tools(profile=run) |
| `configs/.env.prod.example` | 仓库 | 键名模板,4×CHANGE_ME 占位,零真实值 |
| `scripts/deploy.sh` / `setup.sh` | 仓库 | dev 侧编排(§1 模型) |
| `scripts/pipeline.sh` / `backup.sh` / `health.sh` | 仓库→lab | lab 侧日管线 / 备份 / 健康自检 |
| `vendor/` | 仓库 | 自包含构建依赖(vendored,免构建期网络) |

## 3. 部署执行记录

`setup.sh`(dev 侧)逐步骤结果:

1. 目录 + 传输 compose/脚本 → ✅
2. `.env` 校验(存在、无 CHANGE_ME、600)→ ✅(真实值生成/提取均在 shell 内,零打印)
3. compose 插件:dev `/usr/libexec/docker/cli-plugins/docker-compose`(2.40.3)→ lab `~/.docker/cli-plugins/` → ✅
4. 镜像构建 + `docker save|load` 传输 + up postgres + migrate → ✅(build 2 轮修复后成功,见 §5)
5. postgres 就绪(pg_isready 轮询)→ ✅
6. migrate → ✅(4 个迁移;幂等,重跑 0)
7. vault 基线:dev `PIKS-Vault/Generated` rsync → lab(排除 09-Personal/.obsidian);设 origin=GitHub 私有仓 + credential.helper 读 `PIKS_VAULT_GIT_TOKEN` 环境变量 → ✅
8. crontab:pipeline 每 15min + backup 23:59 → ✅(cron daemon active 已验证)
9. 时间核对:容器内 `date` = Asia/Shanghai → ✅(`Thu Aug 27 00:56:04 CST 2026`)
10. 初始数据同步(单独):`pg_dump dev → lab` restore → 计数一致 → ✅

## 4. 验收结果(设计 §15)

| # | 验收项 | 结果 | 证据 |
|---|---|---|---|
| §15.1 | postgres 运行、5433 仅回环 | ✅ | `ss -ltn`:仅 `127.0.0.1:5433`;容器 healthy |
| §15.2 | 初始数据计数一致 + reconcile 异常=0 | ✅ | dev/lab 均为 events=9/entities=90/snapshots=1/sources=1;reconcile 结论=通过 |
| §15.3 | 首次全链:新闻→抽取→发布→push 私有仓;工作机 pull 无冲突 | ✅ | dongcai 采集 50 条→worker 抽取 55 events→publisher 55 卡+提交推 GitHub;工作机 fetch 后 fast-forward(behind N),零冲突 |
| §15.4 | 幂等:重跑零重复处理、零脏写 | ✅ | 重跑:collector new=2 dup=48(去重生效);仅新增 2 条真新新闻→4 events→4 卡,一次提交 |
| §15.5 | 非交易日:周六触发零快照、collector 不报错 | ✅ | pipeline.sh 周末门实测退出零动作;quote-collector qdate 不匹配即跳过的非交易日路径实测触发 |
| §15.6 | 备份可恢复 | ✅ | `backup.sh` 产 75K dump;恢复临时库计数与线上一致(68/142/60);临时库已删 |
| §15.7 | 密钥:lab .env 0600;代码/vault 仓无真实 key;example 无真实值;vault config 无 token | ✅ | .env 600;4×CHANGE_ME;git 追踪文件零 key/token;credential.helper 仅引用环境变量名 |
| §15.8 | 容器时间 Beijing;复盘/快照 trade_date 为北京日 | ✅ | 容器 `CST 2026-08-27 00:56`;快照 trade_date=2026-08-26;recon-2026-08-27.md 已生成并入 GitHub |
| §15.9 | 调度:16:10 后触发;已有 stamp 零动作 | ✅(门逻辑实测) | 周末门/1610 门/stamp 门三态均实测 exit 0 零动作;16:10 后放行路径由当日多次全链实跑覆盖;首个真实 cron 触发待次日确认(见 §6) |
| §15.10 | 安全面:无对外 5433;仅 postgres 常驻 | ✅ | `ss -ltn` 仅回环;`docker ps` 仅 piks-postgres |

## 5. 实现期发现与修复(2026-08-27)

- **Dockerfile bin 布局**:`/usr/local/bin` → `/app/bin`,与 dev `bin/` 布局一致,脚本统一 `./bin/<cmd>`(commit `46170f6`)。
- **legacy builder 不认行内 `#` 注释**:COPY 指令后内联注释导致构建失败,拆到独立注释行(commit `a04ee7d`)。
- **collector 生产驱动 = dongcai**:`file` 驱动需 `-input` 路径,仅迭代 0 保底;`pipeline.sh` 显式 `-driver dongcai`(东方财富 7x24 快讯)(commit `d1ad41b`)。
- **pipeline.sh run() 加 `-T`**:cron 非交互,防吞 stdin(commit `d74551c`)。
- **publisher push 自愈**(commit `fadae92`):原实现 push 失败被 `_ = run(...)` 静默丢弃,且无变更路径提前 return 永不补推 → 曾出现 lab `ahead 1`(5551ff0 未推)。修复:每次发布都 push(含无变更路径,up-to-date 为 no-op),失败写 stderr 进管线日志下次重试。实测补推成功,`master...origin/master` 归零。
- **lab 实测约束落地**:`/srv/piks`→`/home/rguo/piks`(无 sudo);镜像传输替代 lab clone;vault 基线 rsync 替代 lab 拉 GitHub 私有仓(HTTP2 framing 不稳)。

## 6. 待办(外部依赖)

- [ ] **真实 16:10 cron 触发确认**:首个交易日实跑须由次日日志确认(管线 16:10 后触发、stamp 落盘、复盘页生成、市场快照 trade_date=当日)。命令:`cat /home/rguo/piks/logs/cron.log` 与 `ls /home/rguo/piks/logs/pipeline-$(date +%F).done`。
- [ ] **系统 NTP/chrony 同步**(需 sudo / lab 管理员):时间戳是核心数据;当前 timesyncd inactive,进程级 TZ 只解决显示、不解决漂移。
- [ ] 可选:ssh 禁密码登录(加固,需 sudo)。
- [ ] 可选:备份 offsite rsync 到工作机。

## 7. 运维速查

```bash
# 更新(dev 侧):build → 传镜像 → migrate
./scripts/deploy.sh
# 回滚:数据=备份 pg_restore --clean;代码=dev checkout 旧 commit + 重跑 deploy
# 日管线(log 在 lab)
ssh lab 'tail -50 /home/rguo/piks/logs/pipeline-$(date +%F).log'
# 健康对账
ssh lab 'docker compose -f /home/rguo/piks/docker-compose.yml run --rm -T tools ./bin/reconcile'
```
