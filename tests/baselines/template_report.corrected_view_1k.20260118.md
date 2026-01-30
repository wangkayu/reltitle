# Corrected-View Template Report (1K)

- **Source**
  - file: `/tmp/onnx_training_data_5000_batch.corrected_view.first1000.jsonl`
  - sampling: first 1000 exported lines
- **Corrected-view rule**
  - Apply `corrections.structure_corrections` onto `go_parser` (template/title/aliases/span only).
  - Do NOT apply `attribute_corrections/merge_suggestions` in this report.

## Top template_family

| template_family | count | pct |
|---|---:|---:|
| `[GROUP] TITLE / ALIAS - EP [TECH]` | 312 | 31.2% |
| `[GROUP] EP [SUB]` | 119 | 11.9% |
| `[GROUP] TITLE - EP [TECH]` | 94 | 9.4% |
| `TF01` | 58 | 5.8% |
| `GROUP+EP+SUB` | 55 | 5.5% |
| `GROUP+EP` | 47 | 4.7% |
| `[GROUP] title / alias - ep [tech]` | 45 | 4.5% |
| `[GROUP] EP` | 37 | 3.7% |
| `EP+TAILGROUP` | 36 | 3.6% |
| `TF02` | 33 | 3.3% |
| `EP+TAILGROUP (SUB)` | 24 | 2.4% |
| `[GROUP] TITLE EP [SUB]` | 19 | 1.9% |
| `GROUP+EP+SUB+ALIAS` | 9 | 0.9% |
| `TF03` | 8 | 0.8% |
| `EP` | 7 | 0.7% |
| `TF01: [group] title - ep [tech]` | 5 | 0.5% |
| `CURATION` | 4 | 0.4% |
| `[GROUP] [EP] [SUB]` | 4 | 0.4% |
| `TITLE` | 4 | 0.4% |
| `[GROUP] TITLE [EP] [SUB]` | 4 | 0.4% |

## Examples (top 10 template_family, up to 5 each)

### `[GROUP] TITLE / ALIAS - EP [TECH]`
- raw_title: `[ANi] Blue Miburo S02 / 青之壬生浪 第二季 芹泽暗杀篇 - 02 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `青之壬生浪 第二季 芹泽暗杀篇`
  - aliases: `Blue Miburo S02`
- raw_title: `[ANi] Blue Miburo S02 / 青之壬生浪 第二季 芹澤暗殺篇 - 02 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `青之壬生浪 第二季 芹澤暗殺篇`
  - aliases: `Blue Miburo S02`
- raw_title: `[ANi] Sentenced to Be a Hero / 判处勇者刑 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `Sentenced to Be a Hero / 判处勇者刑`
  - aliases: `Sentenced to Be a Hero`
- raw_title: `[ANi] Sentenced to Be a Hero / 判處勇者刑 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `Sentenced to Be a Hero / 判處勇者刑`
  - aliases: `Sentenced to Be a Hero`
- raw_title: `[LoliHouse] MF GHOST 3rd Season / 极速车魂 / 燃油車鬥魂 - 01 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]`
  - group: `LoliHouse` (head=`LoliHouse`, tail=``)
  - title: `MF GHOST 3rd Season`
  - aliases: `极速车魂 | 燃油車鬥魂`

### `[GROUP] EP [SUB]`
- raw_title: `[Prejudice-Studio] 我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜 - OVA03 [WebRip 1080P AVC 8bit AAC MP4][繁体内嵌]`
  - group: `Prejudice-Studio` (head=`Prejudice-Studio`, tail=``)
  - title: `我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜`
  - aliases: `Watanare 〜Next Sunshine〜`
- raw_title: `[Prejudice-Studio] 我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜 - OVA03 [WebRip 1080P HEVC 10bit EAC3&AAC MKV][简繁内封]`
  - group: `Prejudice-Studio` (head=`Prejudice-Studio`, tail=``)
  - title: `我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜`
  - aliases: `Watanare | Next Sunshine`
- raw_title: `[AI翻译] 贵族转生~得天眷顾一出生就获得最强力量~ - 01 [WEB-DL][1080p][AVC AAC][简繁外挂字幕]`
  - group: `AI翻译` (head=`AI翻译`, tail=``)
  - title: `贵族转生~得天眷顾一出生就获得最强力量~`
- raw_title: `[ANi] DDDD 恶魔的破坏 - 04 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `恶魔的破坏`
  - aliases: `DDDD`
- raw_title: `[ANi] DDDD 恶魔的破坏 - 08 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `恶魔的破坏`
  - aliases: `DDDD`

### `[GROUP] TITLE - EP [TECH]`
- raw_title: `[Prejudice-Studio] 我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜 - OVA03 [WebRip 1080P HEVC 10bit EAC3&AAC MKV][简繁内封]`
  - group: `Prejudice-Studio` (head=`Prejudice-Studio`, tail=``)
  - title: `我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜`
- raw_title: `[Prejudice-Studio] 我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜 - OVA03 [WebRip 1080P AVC 8bit AAC MP4][简体内嵌]`
  - group: `Prejudice-Studio` (head=`Prejudice-Studio`, tail=``)
  - title: `我们不可能成为恋人！绝对不行。〜再次闪耀！〜 Watanare 〜Next Sunshine〜`
  - aliases: `Watanare 〜Next Sunshine〜`
- raw_title: `[ANi] FAIRY TAIL 魔导少年 百年任务 - 02 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `FAIRY TAIL 魔导少年 百年任务`
- raw_title: `[ANi] FAIRY TAIL 魔导少年 百年任务 - 11 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `FAIRY TAIL 魔导少年 百年任务`
- raw_title: `[ANi] FAIRY TAIL 魔导少年 百年任务 - 15 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `FAIRY TAIL 魔导少年 百年任务`

### `TF01`
- raw_title: `[樱桃花字幕组]不擅长吸血的吸血鬼 Chanto Suenai Kyuuketsuki-chan - 9 [1080p][简日内嵌]`
  - group: `樱桃花字幕组` (head=`樱桃花字幕组`, tail=``)
  - title: `不擅长吸血的吸血鬼`
  - aliases: `Chanto Suenai Kyuuketsuki-chan`
- raw_title: `[New-raws] Fatestrange Fake - TVSP [1080p] [AMZN].mkv`
  - group: `New-raws` (head=`New-raws`, tail=``)
  - title: `Fatestrange Fake`
  - aliases: `TVSP`
- raw_title: `Maria.Sama.Ga.Miteru.S03.MULTISub[FRE][ENG][CHI].1080p.BDRip.CUSTOM.x264.PCM-KuroNeko - Maria Watches Over Us 3rd VOSTFR VOSTEN マリア様がみてる〜３rd〜`
  - group: `` (head=``, tail=``)
  - title: `Maria Sama Ga Miteru S03 - Maria Watches Over Us 3rd マリア様がみてる〜３rd〜`
  - aliases: `Maria Watches Over Us 3rd | マリア様がみてる〜３rd〜`
- raw_title: `[AI-Raws] Anne of Green Gables 赤毛のアン TV+Movie BDRip 1080p MKV`
  - group: `AI-Raws` (head=`AI-Raws`, tail=``)
  - title: `Anne of Green Gables 赤毛のアン`
  - aliases: `Anne of Green Gables`
- raw_title: `[AMF]Ring ni Kakero 1 Nichibei Kessen Hen 01 a 12 - Completa`
  - group: `AMF` (head=`AMF`, tail=``)
  - title: `Ring ni Kakero 1 Nichibei Kessen Hen`

### `GROUP+EP+SUB`
- raw_title: `Detective Conan S01E1187 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (ADN) (Meitantei Conan,Case Closed)`
  - group: `` (head=``, tail=``)
  - title: `Detective Conan`
  - aliases: `Meitantei Conan | Case Closed`
- raw_title: `Detective Conan S01E1187 VOSTFR 720p WEB x264 AAC -Tsundere-Raws (ADN) (Meitantei Conan,Case Closed)`
  - group: `` (head=``, tail=``)
  - title: `Detective Conan`
  - aliases: `Meitantei Conan | Case Closed`
- raw_title: `Sentenced to Be a Hero S01E01 Sentence Support Retreat From Couveunge Forest 1080p CR WEB-DL DUAL AAC2.0 H 264-VARYG (Yuusha Kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku, …`
  - group: `VARYG` (head=``, tail=`VARYG`)
  - title: `Sentenced to Be a Hero`
  - aliases: `Yuusha Kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku`
- raw_title: `Sentenced to Be a Hero S01E01 VOSTFR 720p WEB x264 AAC -Tsundere-Raws (CR) (Yuusha-kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku)`
  - group: `Tsundere-Raws` (head=``, tail=`Tsundere-Raws`)
  - title: `Sentenced to Be a Hero`
  - aliases: `Yuusha-kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku`
- raw_title: `TOUGEN ANKI S01 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (CR)`
  - group: `Tsundere-Raws` (head=``, tail=`Tsundere-Raws`)
  - title: `TOUGEN ANKI`

### `GROUP+EP`
- raw_title: `Detective Conan S01E1187 Episode 1080p CR WEB-DL AAC2.0 H 264-VARYG (Meitantei Conan, Multi-Subs)`
  - group: `VARYG` (head=``, tail=`VARYG`)
  - title: `Detective Conan`
  - aliases: `Meitantei Conan`
- raw_title: `Sentenced to Be a Hero S01E01 Sentence Support Retreat From Couveunge Forest 1080p CR WEB-DL MULTi AAC2.0 H 264-VARYG (Yuusha Kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku,…`
  - group: `VARYG` (head=``, tail=`VARYG`)
  - title: `Sentenced to Be a Hero`
  - aliases: `Yuusha Kei ni Shosu: Choubatsu Yuusha 9004-tai Keimu Kiroku`
- raw_title: `Mobile Suit Gundam GQuuuuuuX S01 1080p WEB H.264 AAC -Tsundere-Raws (DSNP) (Kidou Senshi Gundam: GQuuuuuuX)`
  - group: `Tsundere-Raws` (head=``, tail=`Tsundere-Raws`)
  - title: `Mobile Suit Gundam GQuuuuuuX`
  - aliases: `Kidou Senshi Gundam: GQuuuuuuX`
- raw_title: `Fate strange Fake S00E01 720p WEB x264 AAC -Tsundere-Raws (ABEMA) (Fate/strange Fake: Whispers of Dawn)`
  - group: `Tsundere-Raws` (head=``, tail=`Tsundere-Raws`)
  - title: `Fate strange Fake`
  - aliases: `Fate/strange Fake: Whispers of Dawn`
- raw_title: `Fate strange Fake S01E01 720p WEB x264 AAC -Tsundere-Raws (ABEMA)`
  - group: `Tsundere-Raws` (head=``, tail=`Tsundere-Raws`)
  - title: `Fate strange Fake`

### `[GROUP] title / alias - ep [tech]`
- raw_title: `[Ironclad] Fate strange Fake - S01E01 [1080p.AV1] | Fate/Strange Fake (2024) (Dual-Audio, Multi-Subs) (Weekly)`
  - group: `Ironclad` (head=`Ironclad`, tail=``)
  - title: `Fate strange Fake`
  - aliases: `Fate/Strange Fake`
- raw_title: `[Yameii] Sentenced to Be a Hero - S01E01 [English Dub] [CR WEB-DL 720p] [07F6F0F2] (Yuusha-kei ni Shosu: Choubatsu Yuusha 9004 Tai Keimu Kiroku)`
  - group: `Yameii` (head=`Yameii`, tail=``)
  - title: `Sentenced to Be a Hero`
  - aliases: `Yuusha-kei ni Shosu: Choubatsu Yuusha 9004 Tai Keimu Kiroku`
- raw_title: `[ANi] Acro Trip / Acro Trip 顶尖恶路 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `Acro Trip`
  - aliases: `Acro Trip 顶尖恶路`
- raw_title: `[ANi] Ao no Orchestra S02 / 蓝色管弦乐 第二季 - 07 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `Ao no Orchestra S02 / 蓝色管弦乐 第二季`
  - aliases: `Ao no Orchestra`
- raw_title: `[ANi] Ao no Orchestra S02 / 蓝色管弦乐 第二季 - 08 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]`
  - group: `ANi` (head=`ANi`, tail=``)
  - title: `Ao no Orchestra S02 / 蓝色管弦乐 第二季`
  - aliases: `Ao no Orchestra S02`

### `[GROUP] EP`
- raw_title: `[ToonsHub] Ao no Miburo S02E02 1080p AMZN WEB-DL DDP2.0 H.264 (Blue Miburo)`
  - group: `ToonsHub` (head=`ToonsHub`, tail=``)
  - title: `Ao no Miburo`
  - aliases: `Blue Miburo`
- raw_title: `[T3KASHi] Pokemon Indigo League S01 TRUEFRENCH 1080p WEB-DL H.264 (VF)`
  - group: `T3KASHi` (head=`T3KASHi`, tail=``)
  - title: `Pokemon Indigo League`
  - aliases: `Pokemon Indigo League S01 TRUEFRENCH`
- raw_title: `[Erai-raws] Detective Conan - 1187 [480p CR WEB-DL AVC AAC][E842FA2D]`
  - group: `Erai-raws` (head=`Erai-raws`, tail=``)
  - title: `Detective Conan`
- raw_title: `[SubsPlease] Detective Conan - 1187 (1080p) [28225DAC].mkv`
  - group: `SubsPlease` (head=`SubsPlease`, tail=``)
  - title: `Detective Conan`
- raw_title: `[SubsPlease] Detective Conan - 1187 (720p) [8CFEBD60].mkv`
  - group: `SubsPlease` (head=`SubsPlease`, tail=``)
  - title: `Detective Conan`

### `EP+TAILGROUP`
- raw_title: `Blue Miburo S02E02 1080p Baha WEB-DL H264 AAC 2.0-ANi`
  - group: `ANi` (head=``, tail=`ANi`)
  - title: `Blue Miburo`
- raw_title: `Sentenced to Be a Hero S01E01 1080p Baha WEB-DL H264 AAC 2.0-ANi`
  - group: `ANi` (head=``, tail=`ANi`)
  - title: `Sentenced to Be a Hero`
- raw_title: `Sentenced.to.Be.a.Hero.S01E01.Sentence.Support.Retreat.From.Couveunge.Forest.1080p.CR.WEB-DL.AAC2.0.H.264-ENK`
  - group: `ENK` (head=``, tail=`ENK`)
  - title: `Sentenced to Be a Hero`
  - aliases: `Sentence Support Retreat From Couveunge Forest`
- raw_title: `Sentenced.to.Be.a.Hero.S01E01.MULTi.1080p.WEBRiP.x265-KAF`
  - group: `KAF` (head=``, tail=`KAF`)
  - title: `Sentenced to Be a Hero`
- raw_title: `Mizuzokusei.no.Mahoutsukai.E10.El.Festival.del.Port.WEBRip.tRR.x264-H3AsO3`
  - group: `H3AsO3` (head=``, tail=`H3AsO3`)
  - title: `Mizuzokusei no Mahoutsukai`

### `TF02`
- raw_title: `[黒ネズミたち] 名侦探柯南 / Detective Conan - 1187 (CR 1920x1080 AVC AAC MKV)`
  - group: `黒ネズミたち` (head=`黒ネズミたち`, tail=``)
  - title: `名侦探柯南`
  - aliases: `Detective Conan`
- raw_title: `[黒ネズミたち] 青之壬生浪 第二季 芹泽暗杀篇 / Ao no Miburo: Serizawa Ansatsu-hen - 02 (Baha 1920x1080 AVC AAC MP4)`
  - group: `黒ネズミたち` (head=`黒ネズミたち`, tail=``)
  - title: `青之壬生浪 第二季 芹泽暗杀篇`
  - aliases: `Ao no Miburo: Serizawa Ansatsu-hen`
- raw_title: `[黒ネズミたち] 勇者处刑：惩罚勇者9004队服刑记录 / Yuusha-kei ni Shosu - 01 (B-Global 1920x1080 HEVC AAC MKV)`
  - group: `黒ネズミたち` (head=`黒ネズミたち`, tail=``)
  - title: `勇者处刑：惩罚勇者9004队服刑记录`
  - aliases: `Yuusha-kei ni Shosu`
- raw_title: `[黒ネズミたち] 判处勇者刑 / Yuusha-kei ni Shosu - 01 (Baha 1920x1080 AVC AAC MP4)`
  - group: `黒ネズミたち` (head=`黒ネズミたち`, tail=``)
  - title: `判处勇者刑`
  - aliases: `Yuusha-kei ni Shosu`
- raw_title: `[黒ネズミたち] 判处勇者刑 / Yuusha-kei ni Shosu - 01 (CR 1920x1080 AVC AAC MKV)`
  - group: `黒ネズミたち` (head=`黒ネズミたち`, tail=``)
  - title: `判处勇者刑`
  - aliases: `Yuusha-kei ni Shosu`

## Reproduce

```bash
python3 training/scripts/export_corrected_view_jsonl_v1.py \
  --input training/artifacts/onnx_training_data_5000_batch.jsonl \
  --output /tmp/onnx_training_data_5000_batch.corrected_view.first1000.jsonl \
  --limit 1000
```

