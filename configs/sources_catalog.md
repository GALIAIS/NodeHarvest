# 订阅源目录（2026-07-25 扩充）

> 公开免费节点源变动极快，本表仅作参考。以 `config.yaml` 中 `enabled` 为准。  
> 本次实测：`curl` HEAD/GET 200 + 内容为 URI / Base64 / Clash proxies 才入库。

## 规模

| 项 | 数量 |
|----|------|
| 总源 | **115** |
| 启用 | **113** |
| 关闭（重复镜像） | 2（`freefq-ghproxy`、`mahdibland-cdn`） |

## 新增批次（+66）

### itsyebekhe/PSG（TG 频道高频采集）

| 名称 | 类型 | 约体积 | URL 片段 |
|------|------|--------|----------|
| psg-xray-mix / mix-b64 | uri/b64 | 100–130KB | `.../subscriptions/xray/mix(.b64)` |
| psg-xray-vless/vmess/ss/hy2/reality | uri | 1–90KB | `.../subscriptions/xray/*` |
| psg-xray-cdn | uri | ~1.3MB | CDN 优选池 |
| psg-meta-mix | clash | ~140KB | Mihomo meta |
| psg-lite-* | uri/b64 | ~50–70KB | lite 子集 |
| psg-config-txt | uri | ~97KB | 根目录 config.txt |

### 社区自动爬取

| 名称 | 说明 |
|------|------|
| ovmvo-mihomo | FreeSub 永久 mihomo.yaml |
| wenxig-free-txt/yml | free-nodes 自动抓取 |
| wenxig-dongtai-txt/yml | Alvin9999 动态节点 |
| snakem-v2 / snakem-clash | proxypool 源 |
| kooker-jp / mihomo / byxiaoxi | FreeSubsCheck |
| ruking-v2 / clash | freeSub |
| firefox-vray / clash / mihomo | v2rayshare 订阅 |

### 聚合器分协议扩展

- mahdibland：Eternity、yaml 合并、vmess/ss/trojan/ssr 分片；**ss-aggregator 已启用**
- coldwater-10：merge b64/yaml + vmess/trojan/ssr
- barry-far / Epodonios / SoliSpirit / Surfboard TGParse：分协议
- mheidari98：vmess/vless/trojan/ss 分协议
- Leon406：hy2 / ssr
- Auto_proxy：Long_term_subscription**8**
- peasoft list_raw
- **ndsphonemy-all**（~34MB，节点极多，带宽紧可关）

### 各家 Clash 镜像

free18 / ermaozi / ripao / mfuu / chengaopan 的 clash/yml 输出。

## 体积警告（可按需 `enabled: false`）

| 源 | 约体积 |
|----|--------|
| ndsphonemy-all | **~34MB** |
| leon406-vless | ~20MB+ |
| mheidari98 / ms-vless / ms-vmess | 2–5MB 级 |
| psg-xray-cdn / mahdibland / coldwater-all | 1–2MB 级 |

## 当前失效（探测 404，勿加）

- `aiboboxx/v2rayfree`、旧 `yebekhe/TelegramV2rayCollector` 路径
- `Alvin9999/new-pac` 旧 raw 路径（改用 wenxig/dongtai-sub 转发）
- 大量臆造仓库名（freevless、shadowmere 等）

## 维护建议

1. 跑：`./bin/node-hunter -skip-test` 或 Web「仅采集」，看日志 `fetch failed`
2. 失败源设 `enabled: false`
3. 手机/沙箱带宽有限时优先关：`ndsphonemy-all`、`leon406-vless`、`psg-xray-cdn`、`mheidari98`/`ms-*`
4. 去重靠 cleaner（fingerprint），多源重叠是预期行为
