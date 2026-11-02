<template>
  <div class="root" :data-theme="theme">
    <!-- ── 顶栏 ── -->
    <header class="appbar">
      <div class="inner">
        <div class="brand">
          <div class="mk">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="2" y="3" width="20" height="14" rx="2.5"/>
              <path d="M8 21h8M12 17v4"/>
            </svg>
          </div>
          <div>
            <h1>屏幕控制台</h1>
            <div class="sub">FRONTSCREEN CONSOLE</div>
          </div>
        </div>

        <div class="acts">
          <span class="pill" :class="{ bad: offline }">
            <i></i>{{ offline ? '连接中断' : '设备在线' }}
          </span>
          <button class="btn ghost" @click="toggleTheme">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <path v-if="theme === 'light'" d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>
              <g v-else><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></g>
            </svg>
            {{ theme === 'light' ? '深色' : '浅色' }}
          </button>
          <button class="btn ghost" @click="refreshAll" :class="{ busy: loading }">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 12a9 9 0 1 1-2.6-6.4M21 3v6h-6"/>
            </svg>
            刷新
          </button>
        </div>
      </div>
    </header>

    <div class="wrap">
      <!-- ── 状态带 ── -->
      <div class="banner" :class="banner.tone">
        <span class="dot"></span>
        <span v-html="banner.text"></span>
      </div>

      <!-- ── 指标 ── -->
      <div class="kpis">
        <div class="kpi">
          <div class="chip cpu">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/>
              <path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>
            </svg>
          </div>
          <div class="txt">
            <div class="lb">CPU 占用</div>
            <div class="vl">{{ fmt(status.cpu, 1, '%') }}</div>
            <div class="sb">{{ cores }} 核 · 每 2 秒采样</div>
          </div>
        </div>

        <div class="kpi">
          <div class="chip mem">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <rect x="3" y="7" width="18" height="10" rx="2"/><path d="M7 7v10M12 7v10M17 7v10"/>
            </svg>
          </div>
          <div class="txt">
            <div class="lb">内存占用</div>
            <div class="vl">{{ fmt(status.memory, 1, '%') }}</div>
            <div class="sb">当前运行态</div>
          </div>
        </div>

        <div class="kpi">
          <div class="chip cput">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <path d="M14 14.8V5a2 2 0 1 0-4 0v9.8a4 4 0 1 0 4 0z"/><path d="M12 9v6"/>
            </svg>
          </div>
          <div class="txt">
            <div class="lb">CPU 温度</div>
            <div class="vl" :class="tempClass(status.cpu_temp)">{{ fmt(status.cpu_temp, 0, '°C') }}</div>
            <div class="sb">风扇按此调速</div>
          </div>
        </div>

        <div class="kpi">
          <div class="chip hddt">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 9h10M7 13h6"/>
            </svg>
          </div>
          <div class="txt">
            <div class="lb">机箱温度</div>
            <div class="vl" :class="tempClass(status.hdd_temp)">{{ fmt(status.hdd_temp, 0, '°C') }}</div>
            <div class="sb">取最高盘温</div>
          </div>
        </div>
      </div>

      <div class="cols">
        <div class="col">
          <!-- ── 实时预览 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#0d9488"></span>
              <h2>实时预览</h2>
              <span class="en">LIVE PREVIEW</span>
              <label class="auto"><input type="checkbox" v-model="previewAuto" /> 15 秒自动刷新</label>
            </div>
            <div class="bd pv">
              <div class="pv-side">
                <button v-for="n in display.pages" :key="n" class="btn ghost pv-btn"
                        :class="{ on: previewPage === n }" @click="loadPreview(n)">{{ pageLabels[n] }}</button>
                <button class="btn ghost sm" @click="refreshPreview" :class="{ busy: previewBusy }">刷新</button>
              </div>
              <div class="pv-shot">
                <img v-if="previewSrc" :src="previewSrc" alt="屏幕预览" @load="previewBusy = false" />
                <div v-else class="empty">选择左侧页面开始预览</div>
              </div>
              <div class="pv-note">
                <b>这是离屏重绘，不是截屏</b>：与屏幕共用同一份采样与设置，改动立即反映；
                但天气图标动画相位、入场编排、夜间背光不会体现在预览里。
              </div>
            </div>
          </section>

          <!-- ── 告警 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#dc2626"></span>
              <h2>告警</h2>
              <span class="en">ALERTS</span>
              <span class="hd-right">{{ alerts.active.length ? alerts.active.length + ' 项异常' : '一切正常' }}</span>
            </div>
            <div class="bd">
              <div v-if="alerts.active.length" class="alist">
                <div class="arow" v-for="a in alerts.active" :key="a.key">
                  <span class="asev" :class="a.sev">{{ a.sev === 'crit' ? '故障' : '预警' }}</span>
                  <span class="atxt">{{ a.text }}</span>
                </div>
              </div>
              <div v-else class="empty">当前没有活跃告警 —— 屏幕顶部也不会有横幅。</div>

              <div class="row top mt">
                <div class="field">
                  <label>温度预警 / 故障（°C）</label>
                  <div class="inline">
                    <input class="num xs" type="number" v-model.number="alertForm.temp_warn" />
                    <span class="unit">/</span>
                    <input class="num xs" type="number" v-model.number="alertForm.temp_crit" />
                    <button class="btn ghost sm" @click="saveAlerts()">应用</button>
                  </div>
                </div>
                <div class="field">
                  <label>磁盘容量预警（%）</label>
                  <div class="inline">
                    <input class="num xs" type="number" v-model.number="alertForm.disk_warn" />
                    <button class="btn ghost sm" @click="saveAlerts()">应用</button>
                  </div>
                </div>
              </div>

              <div class="row top mt">
                <div class="field grow">
                  <label>Bark 推送 {{ alerts.bark_set ? '（已设置，留空保存则不修改）' : '（未设置）' }}</label>
                  <div class="inline">
                    <input class="txt" type="password" v-model="alertForm.bark_url"
                           placeholder="https://api.day.app/你的key" autocomplete="off" />
                    <button class="btn ghost sm" @click="saveBark()">保存</button>
                    <button class="btn primary sm" @click="testBark()">发送测试</button>
                  </div>
                  <div class="hint">仅支持 https；key 只写入设备配置文件，接口不回显。</div>
                </div>
              </div>

              <div v-if="alerts.events.length" class="mt">
                <label class="lab">最近记录
                  <span class="lnk" @click="eventsOpen = !eventsOpen">
                    {{ eventsOpen ? '收起' : `展开全部 ${alerts.events.length} 条` }}</span>
                </label>
                <div class="hwrow" v-for="(e, i) in (eventsOpen ? alerts.events : alerts.events.slice(0, 5))" :key="i">
                  <span class="hwkey">{{ fmtTime(e.at) }}</span>
                  <span class="asev mini" :class="e.kind === 'trigger' ? e.sev : 'rec'">{{ e.kind === 'trigger' ? '触发' : '恢复' }}</span>
                  <span class="hwval">{{ e.text }}</span>
                </div>
              </div>
            </div>
          </section>

          <!-- ── 历史 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#0ea5e9"></span>
              <h2>历史</h2>
              <span class="en">24 HOURS</span>
              <span class="hd-right">每分钟一行 · 均值与峰值</span>
            </div>
            <div class="bd">
              <div class="hseg">
                <span v-for="m in histMetrics" :key="m.v" :class="{ on: history.metric === m.v }"
                      @click="history.metric = m.v">{{ m.t }}</span>
              </div>

              <div v-if="histView" class="hchart">
                <svg :viewBox="`0 0 ${histView.W} ${histView.H}`" preserveAspectRatio="none">
                  <line v-for="(g, i) in histView.grid" :key="'g' + i"
                        :x1="0" :x2="histView.W" :y1="g.y" :y2="g.y" class="hgrid"
                        vector-effect="non-scaling-stroke" />
                  <path v-if="histView.band" :d="histView.band" :fill="histView.color" opacity="0.16" />
                  <path :d="histView.line" fill="none" :stroke="histView.color" stroke-width="1.8"
                        stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
                  <path v-if="histView.line2" :d="histView.line2" fill="none" stroke="#64748b" stroke-width="1.4"
                        stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
                </svg>
                <div class="haxis">
                  <span>{{ histView.axis[0] }}</span><span>{{ histView.axis[1] }}</span><span>{{ histView.axis[2] }}</span>
                </div>
              </div>
              <div v-else class="empty">历史正在积累（每分钟一行），稍后回来就有曲线了。</div>

              <div v-if="histView" class="hstat">
                <span class="hcur" :style="{ color: histView.color }">{{ histView.now }}</span>
                <span class="hsub">{{ histView.nowLabel }}</span>
                <span class="hsub">·</span>
                <span class="hsub">24h 峰值 {{ histView.peak }}</span>
                <span v-if="history.points.length < 60" class="hsub">· 已积累 {{ history.points.length }} 分钟</span>
                <span v-if="history.metric === 'net'" class="hsub">· 深色线为发送</span>
              </div>
            </div>
          </section>

          <!-- ── 硬件 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#64748b"></span>
              <h2>硬件与映射</h2>
              <span class="en">HARDWARE</span>
              <button class="btn ghost sm hd-right" @click="scanHardware" :class="{ busy: scanning }">扫描硬件</button>
            </div>
            <div class="bd">
              <div class="maps">
                <div class="map" v-for="f in fanRows" :key="f.key">
                  <div class="map-name">{{ f.name }}</div>
                  <div class="inline">
                    <label>PWM</label>
                    <input class="num xs" type="number" min="1" max="5" v-model.number="f.pwm" />
                    <label>温度源</label>
                    <input class="txt" type="text" v-model="f.sensor" placeholder="芯片:标签" />
                    <button class="btn ghost sm" @click="saveMapping(f)">应用</button>
                  </div>
                </div>
              </div>

              <div v-if="hw.fans.length || hw.temps.length" class="hwlist">
                <div class="hwgroup" v-if="hw.fans.length">
                  <div class="hwtitle">风扇</div>
                  <div class="hwrow" v-for="f in hw.fans" :key="f.Label">
                    <span class="hwkey">pwm{{ pwmNum(f.Label) }}</span>
                    <span class="hwval">{{ f.Label }}</span>
                    <span class="hwext">{{ f.RPM }} RPM</span>
                  </div>
                </div>
                <div class="hwgroup" v-if="hw.temps.length">
                  <div class="hwtitle">温度传感器</div>
                  <div class="hwrow" v-for="t in hw.temps" :key="t.Chip + t.Label">
                    <span class="hwkey">{{ t.Label }}</span>
                    <span class="hwval">{{ t.Chip }}</span>
                    <span class="hwext">{{ t.Temp.toFixed(1) }}°C</span>
                  </div>
                </div>
              </div>
              <div v-else class="empty">点「扫描硬件」列出本机风扇与温度传感器，换机器时用它确定 PWM 通道与温度源。</div>

              <div class="hint mt">PWM 映射改动需重启 r1-toolbox 服务后生效。</div>
            </div>
          </section>

          <!-- ── 存储 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#7c3aed"></span>
              <h2>存储</h2>
              <span class="en">STORAGE</span>
              <span class="hd-right" v-if="disk.scan">耗时 {{ (disk.scan.took_ms/1000).toFixed(1) }}s</span>
            </div>
            <div class="bd">
              <div class="hseg">
                <span v-for="v in disk.volumes" :key="v.mount"
                      :class="{ on: disk.cur === v.mount }" @click="disk.cur = v.mount">
                  {{ v.name }} {{ v.used_pct.toFixed(0) }}%
                </span>
              </div>

              <div v-if="curVol" class="duse">
                <div class="duse-bar"><i :style="{ width: Math.min(100, curVol.used_pct) + '%' }"></i></div>
                <div class="duse-txt">
                  已用 {{ curVol.used_gb.toFixed(0) }} / {{ curVol.total_gb.toFixed(0) }} GB
                  · {{ curVol.device_type }}
                </div>
              </div>

              <div class="inline mt">
                <button class="btn ghost sm" @click="scanDisk" :class="{ busy: disk.scanning }">
                  {{ disk.scanning ? '扫描中…' : (disk.scan ? '重新扫描' : '扫描大目录与大文件') }}
                </button>
                <span v-if="disk.scan && disk.scan.truncated" class="hsub">{{ disk.scan.note }}</span>
              </div>

              <div v-if="disk.scan && disk.scan.dirs.length" class="mt">
                <label class="lab">一级目录（Top {{ disk.scan.dirs.length }}）</label>
                <div class="drow2" v-for="d in disk.scan.dirs" :key="d.name">
                  <span class="dname">{{ d.name }}</span>
                  <div class="dbar"><i :style="{ width: dirPct(d) + '%' }"></i></div>
                  <span class="dsz">{{ fmtSize(d.bytes) }}</span>
                </div>
              </div>

              <div v-if="disk.scan && disk.scan.files.length" class="mt">
                <label class="lab">大文件（&gt; 500 MB，Top {{ disk.scan.files.length }}）</label>
                <div class="drow2" v-for="f in disk.scan.files" :key="f.path">
                  <span class="dpath" :title="f.path">{{ f.path }}</span>
                  <span class="dsz">{{ fmtSize(f.bytes) }}</span>
                </div>
              </div>

              <div v-else-if="!disk.scan" class="hint mt">
                扫描会给这块卷跑一遍 du，几百 GB 的机械盘约 5 秒；结果不落盘，刷新页面后需重新扫描。
              </div>
            </div>
          </section>

          <section class="card">
          <div class="hd">
          <span class="sq" style="background:#7c3aed"></span>
          <h2>灯效</h2>
          <span class="en">RGB</span>
          </div>
          <div class="bd fill">
          <div class="field">
          <label>模式</label>
          <div class="seg wide">
          <span v-for="m in rgbModes" :key="m.v" :class="{ on: rgbMode === m.v }"
          @click="setRGB(m.v)">{{ m.t }}</span>
          </div>
          </div>
          <div class="field mt">
          <label>颜色</label>
          <div class="swatches">
          <span v-for="c in rgbColors" :key="c.v" class="sw"
          :class="{ on: rgbColor === c.v }" :style="{ background: c.hex }"
          :title="c.t" @click="setRGBColor(c.v)"></span>
          </div>
          </div>
          <div class="hint mt">R1 前屏硬件没有系统级灯效控制通路，这里写入可能无可见效果。</div>
          </div>
          </section>

        </div>
        <div class="col">
          <!-- ── 屏幕 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq"></span>
              <h2>屏幕</h2>
              <span class="en">DISPLAY</span>
              <span class="hd-right">{{ screenOff ? '屏幕已关闭' : '屏幕点亮中' }}</span>
            </div>
            <div class="bd">
              <div class="row">
                <div class="field grow">
                  <label>屏幕亮度</label>
                  <div class="slider-wrap">
                    <input class="range" type="range" min="0" :max="brightMax" step="200"
                           v-model.number="brightness" @input="onBrightness" />
                    <span class="val">{{ brightnessPct }}%</span>
                  </div>
                </div>
                <div class="field">
                  <label>开关屏</label>
                  <button class="btn" :class="offArmed ? 'danger' : 'ghost'" @click="toggleScreen">
                    {{ offArmed ? '确认关闭？' : (screenOff ? '点亮屏幕' : '关闭屏幕') }}
                  </button>
                </div>
              </div>

              <div class="row top">
                <div class="field">
                  <label>自动关屏</label>
                  <div class="inline">
                    <input class="num" type="number" min="0" max="3600" v-model.number="timeout" />
                    <span class="unit">{{ timeout ? '秒' : '常亮' }}</span>
                    <button class="btn ghost sm" @click="saveTimeout">应用</button>
                  </div>
                  <div class="hint">0 = 常亮，不自动息屏</div>
                </div>
                <div class="field">
                  <label>夜间亮度</label>
                  <div class="inline">
                    <span class="ro">{{ nightPct }}%</span>
                    <span class="unit">18:00–06:00</span>
                  </div>
                  <div class="hint">随昼夜切换自动应用，不覆盖手动调节</div>
                </div>
              </div>
            </div>
          </section>


          <!-- ── 显示 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#0891b2"></span>
              <h2>显示</h2>
              <span class="en">APPEARANCE</span>
              <span class="hd-right">改完即生效</span>
            </div>
            <div class="bd">
              <div class="row">
                <div class="field">
                  <label>主题</label>
                  <div class="seg">
                    <span v-for="t in themes" :key="t.v" :class="{ on: display.theme === t.v }"
                          @click="setDisplay('theme', t.v)">{{ t.t }}</span>
                  </div>
                </div>
                <div class="field">
                  <label>时钟</label>
                  <div class="seg">
                    <span :class="{ on: display.clock_24h }" @click="setDisplay('clock_24h', true)">24 小时</span>
                    <span :class="{ on: !display.clock_24h }" @click="setDisplay('clock_24h', false)">12 小时</span>
                  </div>
                </div>
                <div class="field">
                  <label>天气</label>
                  <div class="seg">
                    <span :class="{ on: display.weather }" @click="setDisplay('weather', true)">显示</span>
                    <span :class="{ on: !display.weather }" @click="setDisplay('weather', false)">隐藏</span>
                  </div>
                </div>

                <div class="field">
                  <label>刷新节奏</label>
                  <div class="seg">
                    <span v-for="n in [2, 3, 5]" :key="n" :class="{ on: display.sample_seconds === n }"
                          @click="setDisplay('sample_seconds', n)">{{ n }} 秒</span>
                  </div>
                </div>
              </div>

              <div class="row top">
                <div class="field grow">
                  <label>页面与顺序</label>
                  <div class="prow" v-for="(n, i) in display.page_order" :key="n">
                    <span class="pname" :class="{ off: !display.pages.includes(n) }">{{ pageLabels[n] }}</span>
                    <div class="seg">
                      <span :class="{ on: display.pages.includes(n) }" @click="togglePage(n)">显示</span>
                      <span :class="{ on: !display.pages.includes(n) }" @click="togglePage(n)">隐藏</span>
                    </div>
                    <button class="btn ghost sm" @click="movePage(i, -1)" :disabled="i === 0">↑</button>
                    <button class="btn ghost sm" @click="movePage(i, 1)" :disabled="i === display.page_order.length - 1">↓</button>
                  </div>
                  <div class="hint">翻页只在「显示」的页之间循环，页签也只画启用的；隐藏的页保留在列表里，随时可以再打开。</div>
                </div>
              </div>

              <div class="hint mt">主题与时钟立刻生效；刷新节奏改动后，新周期从下一次采样开始算。</div>
            </div>
          </section>

          <!-- ── 自检 ── -->
          <section class="card">
            <div class="hd">
              <span class="sq" style="background:#64748b"></span>
              <h2>自检</h2>
              <span class="en">SELF MONITOR</span>
              <span class="hd-right">运行 {{ selfmon.uptime }}</span>
            </div>
            <div class="bd">
              <div class="grid4">
                <div class="k4">
                  <div class="k4l">进程 CPU（单核）</div>
                  <div class="k4v" :class="cpuCls">{{ selfmon.cpu_percent != null ? selfmon.cpu_percent.toFixed(2) : '—' }}%</div>
                  <div class="k4s">基线 2.1% · 上限 5%</div>
                </div>
                <div class="k4">
                  <div class="k4l">进程内存</div>
                  <div class="k4v">{{ selfmon.rss_mb != null ? selfmon.rss_mb.toFixed(1) : '—' }} MB</div>
                  <div class="k4s">goroutine {{ selfmon.goroutines ?? '—' }}</div>
                </div>
                <div class="k4">
                  <div class="k4l">平均帧耗时</div>
                  <div class="k4v" :class="selfmon.frame_max_ms > 33 ? 'warn' : ''">{{ selfmon.frame_avg_ms != null ? selfmon.frame_avg_ms.toFixed(1) : '—' }} ms</div>
                  <div class="k4s">最大 {{ selfmon.frame_max_ms != null ? selfmon.frame_max_ms.toFixed(1) : '—' }} ms · 慢帧 {{ selfmon.frame_slow ?? 0 }}</div>
                </div>
                <div class="k4">
                  <div class="k4l">采样</div>
                  <div class="k4v" :class="selfmon.last_sample_ago > 10 ? 'warn' : ''">
                    {{ selfmon.last_sample_ago == null || selfmon.last_sample_ago < 0 ? '—' : selfmon.last_sample_ago.toFixed(0) + 's 前' }}</div>
                  <div class="k4s">{{ selfmon.sample_count ?? 0 }} 次 · 失败 {{ selfmon.sample_errors ?? 0 }}</div>
                </div>
              </div>
              <div class="hint mt">看门狗：{{ selfmon.watchdog_on ? `systemd 托管（${selfmon.watchdog_sec}s 心跳，主循环卡死自动重启）` : '未启用（非 systemd 环境或单元未配 WatchdogSec）' }}</div>
            </div>
          </section>

            <section class="card">
              <div class="hd">
                <span class="sq" style="background:#d97706"></span>
                <h2>风扇</h2>
                <span class="en">FANS</span>
              </div>
              <div class="bd">
                <div class="fan" v-for="f in fanRows" :key="f.key">
                  <div class="fan-top">
                    <div class="fan-name">{{ f.name }}<span class="fan-sub">{{ f.sub }}</span></div>
                    <div class="seg">
                      <span v-for="m in fanModes" :key="m.v" :class="{ on: f.mode === m.v }"
                            @click="setFanMode(f.key, m.v)">{{ m.t }}</span>
                    </div>
                  </div>
                  <div class="fan-curve">
                    <label>启动温度</label>
                    <input class="num sm" type="number" v-model.number="f.min" />
                    <span class="unit">°C</span>
                    <label>全速温度</label>
                    <input class="num sm" type="number" v-model.number="f.max" />
                    <span class="unit">°C</span>
                    <button class="btn ghost sm" @click="saveCurve(f)">应用</button>
                  </div>
                </div>
                <div class="hint mt">曲线只在「自动」模式生效：低于启动温度按最低转速，达到全速温度即满转。</div>
              </div>
            </section>

            <!-- ── 容器（只读） ── -->
            <section class="card">
              <div class="hd">
                <span class="sq" style="background:#0e7490"></span>
                <h2>容器</h2>
                <span class="en">DOCKER</span>
                <span class="hd-right">{{ dockerRunning }} / {{ docker.containers.length }} 运行</span>
                <button class="btn ghost sm" @click="loadDocker" :class="{ busy: dockerLoading }">刷新</button>
              </div>
              <div class="bd">
                <div v-if="docker.err" class="empty">{{ docker.err }}</div>
                <div v-else-if="!docker.containers.length" class="empty">读取中…</div>
                <div v-else class="dlist">
                  <div class="drow" v-for="c in docker.containers" :key="c.id">
                    <span class="ddot" :class="c.state"></span>
                    <span class="dname" :title="c.image">{{ c.name }}</span>
                    <span class="dmeta">{{ c.status }}</span>
                    <button class="btn ghost sm" @click="showLogs(c)">日志</button>
                  </div>
                </div>

                <div v-if="docker.logName" class="dlog">
                  <div class="dlog-hd">
                    <span><b>{{ docker.logName }}</b> · 最近 200 行</span>
                    <span class="lnk" @click="closeLogs">关闭</span>
                  </div>
                  <pre>{{ docker.logLoading ? '读取中…' : (docker.logText || '（无输出）') }}</pre>
                </div>
              </div>
            </section>

        </div>
      </div>

      <div class="foot">
        面板令牌决定这台设备的控制权 · 令牌在 <code>/etc/r1-toolbox/config.yaml</code> 的 <code>web.token</code>
      </div>
    </div>

    <div class="toast" :class="{ on: toast.on }">{{ toast.text }}</div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'

/* ── 主题 ── */
const theme = ref(localStorage.getItem('r1tb_theme') || 'light')
const toggleTheme = () => {
  theme.value = theme.value === 'light' ? 'dark' : 'light'
  localStorage.setItem('r1tb_theme', theme.value)
}

/* ── 状态 ── */
const status = reactive({ cpu: 0, memory: 0, cpu_temp: null, hdd_temp: null })
const offline = ref(false)
const loading = ref(false)
const scanning = ref(false)
const screenOff = ref(false)
const cores = ref(4)

/* ── 屏幕 ── */
const brightness = ref(36000)
const brightMax = ref(96000)
const brightnessPct = computed(() => Math.round((brightness.value / (brightMax.value || 96000)) * 100))
const nightPct = computed(() => Math.round((nightBrightness.value / (brightMax.value || 96000)) * 100))
const timeout = ref(300)
const nightBrightness = ref(0)
const offArmed = ref(false)
let offTimer = null

/* ── 风扇 ── */
const fanModes = [
  { v: 'auto', t: '自动' }, { v: 'low', t: '低速' },
  { v: 'medium', t: '中速' }, { v: 'high', t: '高速' },
]
const fanRows = reactive([
  { key: 'cpu', name: 'CPU 风扇', sub: '跟随 CPU 温度', mode: 'auto', min: 35, max: 75, pwm: 3, sensor: '' },
  { key: 'hdd', name: '机箱风扇', sub: '跟随盘温', mode: 'auto', min: 30, max: 45, pwm: 2, sensor: '' },
])

/* ── 灯效 ── */
const rgbModes = [
  { v: 'off', t: '关闭' }, { v: 'rainbow', t: '流光' },
  { v: 'breathing', t: '呼吸' }, { v: 'solid', t: '常亮' },
]
const rgbColors = [
  { v: 'red', t: '红', hex: '#ef4444' }, { v: 'orange', t: '橙', hex: '#f97316' },
  { v: 'yellow', t: '黄', hex: '#eab308' }, { v: 'green', t: '绿', hex: '#22c55e' },
  { v: 'cyan', t: '青', hex: '#06b6d4' }, { v: 'blue', t: '蓝', hex: '#3b82f6' },
  { v: 'purple', t: '紫', hex: '#a855f7' }, { v: 'white', t: '白', hex: '#f8fafc' },
]
const rgbMode = ref('solid')
const rgbColor = ref('green')

/* ── 显示 ── */
const themes = [
  { v: 'ink', t: '墨' }, { v: 'mono', t: '素' }, { v: 'sand', t: '沙' }, { v: 'jade', t: '青' },
]
const display = reactive({
  theme: 'ink', clock_24h: true, weather: true, sample_seconds: 2,
  page_order: ['env', 'detail', 'system'], pages: ['env', 'detail', 'system'],
})
const pageLabels = { env: '环境', detail: '详情', system: '系统' }

/* ── 告警 ── */
const alerts = reactive({ active: [], events: [], bark_set: false, debounce: 3, silence_min: 30 })
const eventsOpen = ref(false) // 记录默认只显示最近 5 条（测试与真实告警会攒很多条）
const alertForm = reactive({ temp_warn: 70, temp_crit: 80, disk_warn: 90, bark_url: '' })
const loadAlerts = () => api('/api/alerts').then((d) => {
  alerts.active = d.active || []
  alerts.events = d.events || []
  alerts.bark_set = !!d.bark_set
  alerts.debounce = d.debounce
  alerts.silence_min = d.silence_min
  alertForm.temp_warn = d.temp_warn
  alertForm.temp_crit = d.temp_crit
  alertForm.disk_warn = d.disk_warn
}).catch(() => {})
const saveAlerts = (barkURL) => {
  const payload = {
    temp_warn: +alertForm.temp_warn, temp_crit: +alertForm.temp_crit,
    disk_warn: +alertForm.disk_warn,
    debounce: alerts.debounce, silence_min: alerts.silence_min,
    bark_url: barkURL === undefined ? null : barkURL,
  }
  return api('/api/alerts/config', payload)
    .then(() => { tip('告警设置已保存'); return loadAlerts() })
    .catch(() => tip('告警设置保存失败'))
}
const saveBark = () => {
  const v = alertForm.bark_url.trim()
  if (v && !v.startsWith('https://')) return tip('Bark 地址必须以 https:// 开头')
  return saveAlerts(v).then(() => { alertForm.bark_url = '' })
}
const testBark = () => api('/api/alerts/test', {})
  .then((d) => { if (d.ok) tip('测试已发送，查收手机通知') })
  .catch(() => tip('发送失败 —— 检查地址与网络'))
const fmtTime = (t) => new Date(t).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

/* ── 存储 ── */
const disk = reactive({ volumes: [], cur: '', scan: null, scanning: false })
const curVol = computed(() => disk.volumes.find((v) => v.mount === disk.cur) || null)
const loadDisk = () => api('/api/disk/volumes').then((d) => {
  disk.volumes = d.volumes || []
  // 默认选占用率最高的卷 —— 那才是"谁占的"这个问题最要紧的一块
  if (!disk.cur && disk.volumes.length) {
    disk.cur = disk.volumes.reduce((a, b) => (b.used_pct > a.used_pct ? b : a)).mount
  }
}).catch(() => {})
const scanDisk = () => {
  if (!disk.cur) return
  disk.scanning = true
  api('/api/disk/scan', { mount: disk.cur })
    .then((d) => { disk.scan = d })
    .catch(() => tip('扫描失败'))
    .finally(() => { disk.scanning = false })
}
const fmtSize = (b) => (b >= 1 << 30 ? (b / (1 << 30)).toFixed(1) + ' GB' : (b / (1 << 20)).toFixed(0) + ' MB')
const dirPct = (d) => {
  const max = disk.scan && disk.scan.dirs.length ? disk.scan.dirs[0].bytes : 1
  return Math.max(2, Math.round((d.bytes / max) * 100))
}

/* ── 容器（只读） ── */
const docker = reactive({ containers: [], err: '', logName: '', logText: '', logLoading: false })
const dockerLoading = ref(false)
const dockerRunning = computed(() => docker.containers.filter((c) => c.state === 'running').length)
const loadDocker = () => {
  dockerLoading.value = true
  return api('/api/docker/containers')
    .then((d) => { docker.containers = d.containers || []; docker.err = '' })
    .catch(() => { docker.err = '未接入 Docker（读不到 /var/run/docker.sock）' })
    .finally(() => { dockerLoading.value = false })
}
const showLogs = (c) => {
  docker.logName = c.name
  docker.logText = ''
  docker.logLoading = true
  api(`/api/docker/logs?id=${encodeURIComponent(c.id)}&tail=200`)
    .then((d) => { docker.logText = d.logs || '' })
    .catch(() => { docker.logText = '读取失败（容器可能已停止或日志驱动不支持）' })
    .finally(() => { docker.logLoading = false })
}
const closeLogs = () => { docker.logName = ''; docker.logText = '' }

/* ── 历史（24h） ── */
const histMetrics = [
  { v: 'cpu', t: 'CPU' }, { v: 'mem', t: '内存' }, { v: 'temp', t: '温度' },
  { v: 'disk', t: '磁盘' }, { v: 'net', t: '网络' },
]
const history = reactive({ points: [], metric: 'cpu' })
const loadHistory = () => api('/api/history?hours=24')
  .then((d) => { history.points = d.points || [] })
  .catch(() => {})

// histView 折线几何：均值线 + 峰值带（均值到峰值之间的半透明区域）。
// 降采样已在后端做（≤240 点），这里只管画。
const histView = computed(() => {
  const pts = history.points
  if (!pts || pts.length < 2) return null
  const m = history.metric
  const conf = {
    cpu: { mean: 'cpu', max: 'cpu_max', cap: 100, unit: '%', color: '#0d9488' },
    mem: { mean: 'mem', max: 'mem_max', cap: 100, unit: '%', color: '#2563eb' },
    temp: { mean: 'temp', max: 'temp_max', cap: 0, unit: '°C', color: '#d97706' },
    disk: { mean: 'disk', max: 'disk_max', cap: 100, unit: '%', color: '#7c3aed' },
    net: { mean: 'net_rx', max: 'net_rx_max', cap: 0, unit: 'MB/s', color: '#0ea5e9' },
  }[m]
  const W = 520, H = 130
  let ymax = conf.cap
  if (!ymax) {
    const mx = Math.max(1, ...pts.map((p) => p[conf.max] || 0))
    ymax = m === 'temp' ? Math.max(50, Math.ceil(mx / 10) * 10)
                        : Math.max(0.5, Math.round(mx * 10) / 10)
  }
  // X 轴按**真实时间戳**映射在 24h 窗口上。不能按点索引均匀铺开：
  // 只有 5 分钟数据时会画满整个宽度，而坐标轴写着「24 小时前」——那是在骗人。
  //
  // 数据不足 24 小时时**自适应窗口**：窗口收到实际数据跨度（至少 10 分钟），
  // 坐标轴标签跟着变（"8 分钟前" / "4 分钟前" / "现在"）。否则刚装好的机器
  // 会看到一张几乎全空的图，以为图坏了。攒满一天后自然过渡回 24h。
  const now = Math.floor(Date.now() / 1000)
  const span = Math.min(24 * 3600, Math.max(now - pts[0].ts, 600))
  const t0 = now - span
  const X = (ts) => Math.min(W, Math.max(0, ((ts - t0) / span) * W))
  const Y = (v) => H - (Math.min(Math.max(v || 0, 0), ymax) / ymax) * H
  const path = (key) => pts.map((p, i) => `${i ? 'L' : 'M'}${X(p.ts).toFixed(1)},${Y(p[key]).toFixed(1)}`).join('')
  const band = path(conf.max) +
    pts.slice().reverse().map((p) => `L${X(p.ts).toFixed(1)},${Y(p[conf.mean]).toFixed(1)}`).join('') + 'Z'
  const ago = (sec) => (sec >= 7200 ? `${Math.round(sec / 3600)} 小时前`
                                    : `${Math.max(1, Math.round(sec / 60))} 分钟前`)
  const last = pts[pts.length - 1]
  const peak = Math.max(...pts.map((p) => p[conf.max] || 0))
  const fmt = (v) => (m === 'net' ? v.toFixed(2) : v.toFixed(m === 'temp' ? 0 : 1))
  return {
    W, H, color: conf.color,
    line: path(conf.mean),
    line2: m === 'net' ? path('net_tx') : '',
    band,
    grid: [1, 2, 3].map((i) => ({ y: ((H * i) / 4).toFixed(1) })),
    axis: [ago(span), ago(span / 2), '现在'],
    now: fmt(last[conf.mean]) + ' ' + conf.unit,
    nowLabel: m === 'net' ? '接收' : '',
    peak: fmt(peak) + ' ' + conf.unit,
  }
})

/* ── 自检 ── */
const selfmon = reactive({})
const cpuCls = computed(() => {
  const v = selfmon.cpu_percent || 0
  return v <= 2.5 ? 'ok' : v <= 5 ? 'warn' : 'bad'
})
const loadSelfmon = () => api('/api/selfmon').then((d) => Object.assign(selfmon, d)).catch(() => {})

/* ── 实时预览 ── */
const previewPage = ref('')
const previewSrc = ref('')
const previewAuto = ref(true)
const previewBusy = ref(false)
let previewTimer = null
const loadPreview = async (name) => {
  if (!name) return
  previewBusy.value = true
  previewPage.value = name
  try {
    // 必须走 fetch 拿 blob：<img src> 不经过全局 fetch 拦截器，
    // 请求头上不了 X-Token，会被 401 拒掉（表现为一张纯黑的破图）
    const r = await fetch(`/api/preview?page=${name}&t=${Date.now()}`)
    if (!r.ok) throw new Error(String(r.status))
    const blob = await r.blob()
    if (previewSrc.value.startsWith('blob:')) URL.revokeObjectURL(previewSrc.value)
    previewSrc.value = URL.createObjectURL(blob)
  } catch {
    tip('预览加载失败')
  } finally {
    previewBusy.value = false
  }
}
const refreshPreview = () => loadPreview(previewPage.value || display.pages[0])
const watchAutoPreview = () => {
  clearInterval(previewTimer)
  if (previewAuto.value) previewTimer = setInterval(refreshPreview, 15000)
}
const togglePage = (name) => {
  const on = display.pages.includes(name)
  if (on && display.pages.length <= 1) return tip('至少保留一页 —— 屏幕总得显示点什么')
  display.pages = on ? display.pages.filter((n) => n !== name) : [...display.pages, name]
  saveDisplay().then(() => {
    if (!display.pages.includes(previewPage.value)) loadPreview(display.pages[0])
  })
}
const movePage = (i, dir) => {
  const order = [...display.page_order]
  const j = i + dir
  if (j < 0 || j >= order.length) return
  ;[order[i], order[j]] = [order[j], order[i]]
  display.page_order = order
  saveDisplay()
}

/* ── 硬件 ── */
const hw = reactive({ fans: [], temps: [] })

/* ── 提示 ── */
const toast = reactive({ on: false, text: '' })
let toastTimer = null
const tip = (text) => {
  toast.text = text
  toast.on = true
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toast.on = false), 2200)
}

/* ── 请求 ── */
const api = (url, data) => fetch(url, {
  method: data ? 'POST' : 'GET',
  headers: data ? { 'Content-Type': 'application/json' } : {},
  body: data ? JSON.stringify(data) : undefined,
}).then((r) => (r.ok ? r.json() : Promise.reject(r)))

const fmt = (v, digits, unit) =>
  v === null || v === undefined || Number.isNaN(v) ? '—' : v.toFixed(digits) + unit

const tempClass = (t) => (t === null || t === undefined ? '' : t >= 70 ? 'bad' : t >= 55 ? 'warn' : '')

const banner = computed(() => {
  if (offline.value) return { tone: 'bad', text: '连不上设备，下面的数值是最后一次读取的结果，不可信。' }
  if (screenOff.value) return { tone: 'warn', text: '屏幕已关闭，正在等点亮。数据采样仍在继续。' }
  return {
    tone: '',
    text: `屏幕正常工作 · 亮度 <b>${brightnessPct.value}%</b> · 自动关屏 <b>${timeout.value ? timeout.value + ' 秒' : '常亮'}</b> · CPU <b>${fmt(status.cpu, 0, '%')}</b>`,
  }
})

/* ── 加载 ── */
const loadConfig = () =>
  api('/api/config').then((d) => {
    brightness.value = d.screen.brightness
    brightMax.value = d.screen.brightness_max || 96000
    timeout.value = d.screen.timeout
    nightBrightness.value = d.screen.night_brightness
    fanRows[0].mode = d.fan.cpu.mode || 'auto'
    fanRows[0].min = d.fan.cpu.min_temp
    fanRows[0].max = d.fan.cpu.max_temp
    fanRows[0].pwm = d.fan.cpu.pwm_index
    fanRows[0].sensor = d.fan.cpu.temp_sensor
    fanRows[1].mode = d.fan.hdd.mode || 'auto'
    fanRows[1].min = d.fan.hdd.min_temp
    fanRows[1].max = d.fan.hdd.max_temp
    fanRows[1].pwm = d.fan.hdd.pwm_index
    fanRows[1].sensor = d.fan.hdd.temp_sensor
    rgbMode.value = d.rgb.mode || 'solid'
    rgbColor.value = d.rgb.color || 'green'
  })

const loadDisplay = () => api('/api/display').then((d) => {
  Object.assign(display, d)
  if (!previewPage.value || !display.pages.includes(previewPage.value)) loadPreview(display.pages[0])
}).catch(() => {})

const loadStatus = () =>
  api('/api/status').then((d) => {
    status.cpu = d.cpu
    status.memory = d.memory
    status.cpu_temp = d.cpu_temp ?? null
    status.hdd_temp = d.hdd_temp ?? null
    screenOff.value = !!d.screen_off
    if (typeof d.brightness === 'number' && !dragging) brightness.value = d.brightness
    offline.value = false
  }).catch(() => { offline.value = true })

const refreshAll = () => {
  loading.value = true
  Promise.all([loadConfig(), loadDisplay(), loadAlerts(), loadSelfmon(), loadHistory(), loadDisk(), loadDocker(), loadStatus()])
    .catch(() => tip('刷新失败，检查网络'))
    .finally(() => { loading.value = false })
}

/* ── 屏幕动作 ── */
let dragging = false
let brightTimer = null
const onBrightness = () => {
  dragging = true
  clearTimeout(brightTimer)
  brightTimer = setTimeout(() => {
    api('/api/brightness', { value: brightness.value })
      .then(() => {
        dragging = false
        // 夜间调节会改写夜间值，回读一次让「夜间亮度」不再显示旧数字
        api('/api/config').then((d) => { nightBrightness.value = d.screen.night_brightness }).catch(() => {})
      })
      .catch(() => { dragging = false; tip('亮度设置失败') })
  }, 220)
}

const saveTimeout = () =>
  api('/api/screen/timeout', { timeout: timeout.value })
    .then(() => tip('自动关屏已更新'))
    .catch(() => tip('设置失败'))

const toggleScreen = () => {
  if (screenOff.value) {
    api('/api/screen/on', {}).then(loadStatus).catch(() => tip('点亮失败'))
    return
  }
  if (!offArmed.value) {
    offArmed.value = true
    clearTimeout(offTimer)
    offTimer = setTimeout(() => (offArmed.value = false), 3500)
    return
  }
  offArmed.value = false
  api('/api/screen/off', {}).then(loadStatus).catch(() => tip('关闭失败'))
}

/* ── 风扇动作 ── */
const setFanMode = (type, mode) => {
  const row = fanRows.find((f) => f.key === type)
  const prev = row.mode
  row.mode = mode
  api('/api/fan', { type, mode }).catch(() => { row.mode = prev; tip('风扇模式设置失败') })
}

const saveCurve = (f) => {
  if (f.min >= f.max) return tip('启动温度必须小于全速温度')
  api('/api/fan/curve', { type: f.key, min_temp: f.min, max_temp: f.max })
    .then(() => tip(f.name + '曲线已更新'))
    .catch(() => tip('曲线设置失败'))
}

const saveMapping = (f) =>
  api('/api/fan/mapping', { type: f.key, pwm_index: f.pwm, temp_sensor: f.sensor })
    .then(() => tip('映射已应用（立即生效）'))
    .catch(() => tip('映射保存失败'))

/* ── 灯效动作 ── */
const setRGB = (mode) => {
  rgbMode.value = mode
  api('/api/rgb', { mode, color: rgbColor.value }).catch(() => tip('灯效设置失败'))
}
const setRGBColor = (color) => {
  rgbColor.value = color
  api('/api/rgb', { mode: rgbMode.value, color }).catch(() => tip('灯效设置失败'))
}

/* ── 硬件 ── */
const setDisplay = (key, val) => {
  const prev = display[key]
  display[key] = val
  return saveDisplay().catch(() => { display[key] = prev; tip('显示设置失败') })
}
const saveDisplay = () =>
  api('/api/display', { ...display }).then((d) => {
    if (d.display) Object.assign(display, d.display)
    tip('显示设置已应用')
    return d
  }).catch(() => { tip('显示设置失败'); throw new Error('save failed') })

const scanHardware = () => {
  scanning.value = true
  api('/api/hwmon/scan')
    .then((d) => { hw.fans = d.fans || []; hw.temps = d.temps || [] })
    .catch(() => tip('扫描失败'))
    .finally(() => { scanning.value = false })
}
const pwmNum = (label) => (label.match(/fan(\d+)/) || [, '?'])[1]

let pollTimer = null
onMounted(() => {
  refreshAll()
  pollTimer = setInterval(loadStatus, 2000)
  setInterval(loadSelfmon, 5000)
  setInterval(loadHistory, 60000)
  watchAutoPreview()
})
onUnmounted(() => { clearInterval(pollTimer); clearInterval(previewTimer) })
</script>

<style>
:root {
  --bg: #f7f8fa; --card: #ffffff;
  --line: #e9edf2; --line2: #f0f3f7; --hover: #f8fafc;
  --ink: #111827; --ink2: #6b7280; --ink3: #9ca3af;
  --accent: #0d9488; --accent-deep: #0f766e; --accent-l: #e6f4f2;
  --ok: #16a34a; --bad: #dc2626; --warn: #d97706;
  color-scheme: light;
}
.root[data-theme='dark'], .root[data-theme='dark'] * {
  --bg: #0f1417; --card: #151b20;
  --line: #262f36; --line2: #1e262c; --hover: #1a2228;
  --ink: #e9edf1; --ink2: #9aa7b0; --ink3: #5f6d76;
  --accent: #2dd4bf; --accent-deep: #14b8a6; --accent-l: #0e2a28;
  --ok: #4ade80; --bad: #f87171; --warn: #fbbf24;
  color-scheme: dark;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  font-size: 14px; line-height: 1.75;
}
.root {
  background: var(--bg); color: var(--ink);
  min-height: 100vh; padding-bottom: 64px;
  transition: background .25s ease, color .25s ease;
}
b { font-weight: 600; color: var(--ink); }
code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px; background: var(--line2); padding: 1px 6px; border-radius: 5px;
}

/* ── 顶栏 ── */
.appbar {
  position: sticky; top: 0; z-index: 40;
  background: color-mix(in srgb, var(--card) 88%, transparent);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--line);
}
.appbar .inner {
  max-width: 1180px; margin: 0 auto; padding: 13px 26px;
  display: flex; align-items: center; justify-content: space-between; gap: 16px;
}
.brand { display: flex; align-items: center; gap: 12px; }
.brand .mk {
  width: 38px; height: 38px; border-radius: 11px; background: var(--accent);
  color: #fff; display: flex; align-items: center; justify-content: center; flex: 0 0 auto;
}
.root[data-theme='dark'] .brand .mk { color: #052e2a; }
.brand .mk svg { width: 20px; height: 20px; }
.brand h1 { font-size: 17px; font-weight: 650; letter-spacing: .02em; line-height: 1.3; }
.brand .sub { font-size: 10.5px; color: var(--ink3); letter-spacing: .14em; line-height: 1.2; }
.acts { display: flex; align-items: center; gap: 10px; }

.pill {
  display: inline-flex; align-items: center; gap: 7px; font-size: 12.5px;
  color: var(--ink2); background: var(--card); border: 1px solid var(--line);
  border-radius: 99px; padding: 5px 13px 5px 11px; white-space: nowrap;
}
.pill i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); display: inline-block; }
.pill.bad i { background: var(--bad); }

.btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  font: inherit; font-size: 12.5px; font-weight: 500;
  padding: 7px 14px; border-radius: 8px; cursor: pointer;
  border: 1px solid transparent; transition: all .14s ease; white-space: nowrap;
}
.btn svg { width: 14px; height: 14px; }
.btn.primary { background: var(--accent); color: #fff; box-shadow: 0 1px 2px rgba(13, 148, 136, .28); }
.btn.primary:hover { background: var(--accent-deep); }
.btn.ghost { background: transparent; color: var(--ink2); border-color: var(--line); }
.btn.ghost:hover { color: var(--ink); border-color: var(--ink3); }
.btn.danger { background: var(--bad); color: #fff; }
.btn.sm { padding: 5px 11px; font-size: 12px; }
.btn.busy { opacity: .55; pointer-events: none; }
.btn.busy svg { animation: rot .8s linear infinite; }
@keyframes rot { to { transform: rotate(360deg); } }

/* ── 容器 ── */
.wrap { max-width: 1180px; margin: 0 auto; padding: 18px 26px 0; }

/* ── 状态带 ── */
.banner {
  display: flex; align-items: center; gap: 9px;
  background: var(--accent-l); border: 1px solid color-mix(in srgb, var(--accent) 22%, transparent);
  color: var(--accent-deep); border-radius: 10px; padding: 10px 15px;
  font-size: 12.5px; margin-bottom: 18px; line-height: 1.6;
}
.root[data-theme='dark'] .banner { color: var(--accent); }
.banner .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; flex: 0 0 auto; }
.banner b { color: inherit; font-weight: 650; }
.banner.warn { background: #fdf6e7; border-color: #f2e2c0; color: #92600b; }
.banner.bad { background: #fdeded; border-color: #f4cccc; color: #b02a2a; }

/* ── 指标卡 ── */
.kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 18px; }
.kpi {
  background: var(--card); border: 1px solid var(--line); border-radius: 12px;
  padding: 17px 18px; display: flex; gap: 14px; align-items: flex-start; min-width: 0;
  box-shadow: 0 1px 2px rgba(16, 24, 40, .03);
}
.kpi .chip {
  width: 42px; height: 42px; border-radius: 12px; flex: 0 0 auto;
  display: flex; align-items: center; justify-content: center;
}
.kpi .chip svg { width: 21px; height: 21px; }
.kpi .chip.cpu { background: #e6f4f2; color: #0d9488; }
.kpi .chip.mem { background: #e8effd; color: #2563eb; }
.kpi .chip.cput { background: #fdf0e3; color: #d97706; }
.kpi .chip.hddt { background: #f1ecfd; color: #7c3aed; }
.root[data-theme='dark'] .kpi .chip.cpu { background: rgba(45, 212, 191, .14); color: #2dd4bf; }
.root[data-theme='dark'] .kpi .chip.mem { background: rgba(96, 165, 250, .14); color: #60a5fa; }
.root[data-theme='dark'] .kpi .chip.cput { background: rgba(251, 191, 36, .14); color: #fbbf24; }
.root[data-theme='dark'] .kpi .chip.hddt { background: rgba(167, 139, 250, .14); color: #a78bfa; }
.kpi .txt { min-width: 0; }
.kpi .lb { font-size: 11.5px; color: var(--ink2); letter-spacing: .06em; line-height: 1.4; white-space: nowrap; }
.kpi .vl {
  font-size: 25px; font-weight: 660; letter-spacing: -.015em; line-height: 1.25;
  margin-top: 1px; font-variant-numeric: tabular-nums;
}
.kpi .vl.warn { color: var(--warn); }
.kpi .vl.bad { color: var(--bad); }
.kpi .sb {
  font-size: 11.5px; color: var(--ink3); margin-top: 1px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}

/* ── 卡片 ── */
.card {
  background: var(--card); border: 1px solid var(--line); border-radius: 12px;
  margin-bottom: 18px; box-shadow: 0 1px 2px rgba(16, 24, 40, .03);
}
.card .hd {
  display: flex; align-items: center; gap: 9px;
  padding: 14px 18px; border-bottom: 1px solid var(--line2);
}
.card .hd .sq { width: 8px; height: 8px; border-radius: 2.5px; background: var(--accent); flex: 0 0 auto; }
.card .hd h2 { font-size: 14px; font-weight: 640; letter-spacing: .01em; }
.card .hd .en { font-size: 10.5px; color: var(--ink3); letter-spacing: .13em; }
.card .hd .hd-right { margin-left: auto; font-size: 12px; color: var(--ink3); }
.card .bd { padding: 16px 18px; }
.card .bd.fill { display: flex; flex-direction: column; }
.card .bd.fill .hint { margin-top: auto; }

/* ── 两列主网格 ──
   2026-11-02 洋哥：单列堆叠时每张卡都 1180px 宽、内容只有 400~600，
   右侧大片空白。改成两列：左列放内容型（预览/告警/硬件），
   右列放控件型（屏幕/显示/自检/风扇/灯效），每列 ~560px。 */
.cols { display: grid; grid-template-columns: 1.14fr 1fr; gap: 18px; align-items: start; }
.col { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
.col .card { margin-bottom: 0; }

/* ── 表单 ── */
.row { display: flex; gap: 26px; align-items: flex-end; flex-wrap: wrap; }
.row + .row { margin-top: 18px; }
.row.top { align-items: flex-start; }
.field { min-width: 0; }
.field.grow { flex: 1 1 320px; }
.field > label {
  display: block; font-size: 11.5px; color: var(--ink2);
  letter-spacing: .06em; margin-bottom: 7px;
}
.inline { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.inline label { font-size: 12px; color: var(--ink2); }
.unit { font-size: 12px; color: var(--ink3); }
.ro {
  font-size: 13px; font-weight: 600; color: var(--ink2);
  padding: 7px 14px; background: var(--line2); border-radius: 8px;
  font-variant-numeric: tabular-nums;
}
.hint { font-size: 11.5px; color: var(--ink3); margin-top: 7px; line-height: 1.6; }
.hint.mt { margin-top: 14px; }

.slider-wrap { display: flex; align-items: center; gap: 14px; }
.slider-wrap .val {
  font-size: 13px; font-weight: 600; color: var(--accent);
  font-variant-numeric: tabular-nums; min-width: 42px; text-align: right;
}
.range {
  -webkit-appearance: none; appearance: none; flex: 1;
  height: 6px; border-radius: 99px; background: var(--line); outline: none;
}
.range::-webkit-slider-thumb {
  -webkit-appearance: none; width: 20px; height: 20px; border-radius: 50%;
  background: var(--card); border: 3px solid var(--accent);
  box-shadow: 0 1px 4px rgba(16, 24, 40, .22); cursor: pointer;
}
.range::-moz-range-thumb {
  width: 20px; height: 20px; border: 3px solid var(--accent);
  border-radius: 50%; background: var(--card); cursor: pointer;
}

.num, .txt {
  font: inherit; font-size: 13px; padding: 7px 11px;
  border: 1px solid var(--line); border-radius: 8px;
  background: var(--card); color: var(--ink); outline: none;
  transition: border-color .14s, box-shadow .14s;
}
.num { width: 92px; text-align: center; font-variant-numeric: tabular-nums; }
.num.sm { width: 74px; }
.num.xs { width: 62px; }
.txt { width: 200px; }
.num:focus, .txt:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 16%, transparent);
}
.num:disabled { background: var(--line2); color: var(--ink3); cursor: not-allowed; }

/* ── 分段控件 ── */
.seg {
  display: inline-flex; border: 1px solid var(--line);
  border-radius: 8px; overflow: hidden; background: var(--card);
}
.seg span {
  font-size: 12px; color: var(--ink2); padding: 5px 14px;
  cursor: pointer; transition: background .12s, color .12s; user-select: none;
}
.seg span + span { border-left: 1px solid var(--line); }
.seg span.on { background: var(--accent); color: #fff; }
.root[data-theme='dark'] .seg span.on { color: #052e2a; }
.seg span:not(.on):hover { background: var(--hover); }
.seg.wide { display: flex; width: 100%; }
.seg.wide span { flex: 1; text-align: center; }

/* ── 预览 ── */
.auto {
  margin-left: auto; font-size: 12px; color: var(--ink2);
  display: inline-flex; align-items: center; gap: 6px; cursor: pointer;
}
.auto input { accent-color: var(--accent); }
.pv { display: flex; gap: 18px; align-items: flex-start; flex-wrap: wrap; }
.pv-side { display: flex; flex-direction: column; gap: 8px; min-width: 108px; }
.pv-btn.on {
  border-color: var(--accent); color: var(--accent);
  background: var(--accent-l); font-weight: 560;
}
.pv-shot { flex: 0 0 auto; }
.pv-shot img {
  height: 420px; width: 165px; /* 376:960 等比，写死避免 flex 收缩压成窄条 */
  display: block; border-radius: 10px; border: 1px solid var(--line);
  background: #000; box-shadow: 0 1px 3px rgba(16, 24, 40, .12);
  object-fit: cover;
}
.pv-note { flex: 1 1 200px; font-size: 12px; color: var(--ink3); line-height: 1.9; min-width: 180px; }
.pv-note b { display: block; color: var(--ink2); font-weight: 600; margin-bottom: 4px; }

.mt { margin-top: 16px; }
.lab {
  font-size: 11.5px; color: var(--ink2); letter-spacing: .06em;
  display: block; margin-bottom: 6px;
}
.lnk {
  cursor: pointer; color: var(--accent); margin-left: 8px;
  font-size: 11.5px; letter-spacing: 0;
}
.lnk:hover { text-decoration: underline; }
.alist { display: flex; flex-direction: column; }
.arow { display: flex; gap: 10px; align-items: center; padding: 5px 0; }
.atxt { font-size: 13px; }
.asev {
  font-size: 11.5px; font-weight: 600; padding: 2px 9px;
  border-radius: 6px; white-space: nowrap;
}
.asev.crit { background: #fdeded; color: #b02a2a; }
.asev.warn { background: #fdf6e7; color: #92600b; }
.asev.rec  { background: var(--accent-l); color: var(--accent-deep); }
.asev.mini { padding: 1px 7px; }

/* ── 存储 ── */
.duse { margin-bottom: 4px; }
.duse-bar { height: 8px; border-radius: 4px; background: var(--line2); overflow: hidden; }
.duse-bar i { display: block; height: 100%; background: var(--accent); border-radius: 4px; }
.duse-txt { font-size: 11.5px; color: var(--ink3); margin-top: 6px; font-variant-numeric: tabular-nums; }
.drow2 { display: flex; align-items: center; gap: 10px; padding: 4px 0; }
.drow2 .dname {
  font-size: 12px; max-width: 120px; white-space: nowrap;
  overflow: hidden; text-overflow: ellipsis; flex: 0 0 auto;
}
.dbar { flex: 1; height: 6px; border-radius: 3px; background: var(--line2); overflow: hidden; }
.dbar i { display: block; height: 100%; background: #a78bfa; border-radius: 3px; }
.dsz {
  font-size: 11.5px; color: var(--ink2); flex: 0 0 auto;
  font-variant-numeric: tabular-nums; min-width: 62px; text-align: right;
}
.dpath {
  font-size: 11px; color: var(--ink2); flex: 1; min-width: 0;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}

/* ── 容器（只读） ── */
.dlist { max-height: 300px; overflow-y: auto; }
.drow {
  display: flex; align-items: center; gap: 9px; padding: 6px 0;
  border-bottom: 1px solid var(--line2);
}
.drow:last-child { border-bottom: 0; }
.ddot { width: 7px; height: 7px; border-radius: 50%; flex: 0 0 auto; background: var(--ink3); }
.ddot.running { background: var(--accent); }
.ddot.restarting { background: #d97706; }
.ddot.paused { background: #94a3b8; }
.dname {
  font-size: 12.5px; font-weight: 560; max-width: 150px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.dmeta {
  font-size: 11px; color: var(--ink3); flex: 1; min-width: 0;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.dlog { margin-top: 12px; }
.dlog-hd {
  display: flex; justify-content: space-between; align-items: baseline;
  font-size: 11.5px; color: var(--ink2); margin-bottom: 6px;
}
.dlog pre {
  margin: 0; background: #14171c; color: #d6dae2;
  font-size: 11px; line-height: 1.6; padding: 10px 12px; border-radius: 8px;
  max-height: 240px; overflow: auto; white-space: pre-wrap; word-break: break-all;
  font-family: ui-monospace, Menlo, Consolas, monospace;
}

/* ── 历史 ── */
.hseg {
  display: inline-flex; gap: 2px; padding: 3px;
  background: var(--line2); border-radius: 9px; margin-bottom: 14px;
}
.hseg span {
  font-size: 12.5px; padding: 5px 13px; border-radius: 7px;
  color: var(--ink2); cursor: pointer; white-space: nowrap;
}
.hseg span.on {
  background: var(--card); color: var(--ink); font-weight: 560;
  box-shadow: 0 1px 2px rgba(16, 24, 40, .08);
}
.hchart svg { width: 100%; height: 130px; display: block; }
.hgrid { stroke: var(--line); stroke-width: 1; }
.haxis {
  display: flex; justify-content: space-between;
  font-size: 10.5px; color: var(--ink3); margin-top: 6px;
  font-variant-numeric: tabular-nums;
}
.hstat { margin-top: 10px; display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.hcur { font-size: 18px; font-weight: 650; font-variant-numeric: tabular-nums; }
.hsub { font-size: 11.5px; color: var(--ink3); }

/* ── 自检 ── */
.grid4 {
  display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 14px;
}
.k4l { font-size: 11.5px; color: var(--ink3); margin-bottom: 5px; }
.k4v { font-size: 22px; font-weight: 660; font-variant-numeric: tabular-nums; }
.k4v.ok { color: var(--accent-deep); }
.k4v.warn { color: #d97706; }
.k4v.bad { color: #dc2626; }
.k4s { font-size: 11.5px; color: var(--ink3); margin-top: 4px; }

/* ── 页面与顺序 ── */
.prow { display: flex; align-items: center; gap: 12px; padding: 5px 0; }
.pname { width: 46px; font-size: 13px; font-weight: 560; }
.pname.off { color: var(--ink3); font-weight: 400; }
.btn:disabled { opacity: .35; pointer-events: none; }

/* ── 风扇 ── */
.fan + .fan { border-top: 1px solid var(--line2); margin-top: 16px; padding-top: 16px; }
.fan-top { display: flex; align-items: center; justify-content: space-between; gap: 14px; flex-wrap: wrap; }
.fan-name { font-size: 13px; font-weight: 560; }
.fan-sub { font-size: 11.5px; color: var(--ink3); font-weight: 400; margin-left: 8px; }
.fan-curve { display: flex; align-items: center; gap: 8px; margin-top: 11px; flex-wrap: wrap; }
.fan-curve label { font-size: 12px; color: var(--ink2); }

/* ── 色块 ── */
.swatches { display: flex; gap: 9px; flex-wrap: wrap; }
.sw {
  width: 26px; height: 26px; border-radius: 8px; cursor: pointer;
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, .08);
  transition: transform .12s, box-shadow .12s;
}
.sw:hover { transform: translateY(-1px); }
.sw.on { box-shadow: 0 0 0 2px var(--card), 0 0 0 4px var(--accent); }

/* ── 硬件 ── */
.maps { display: grid; gap: 10px; }
.map {
  display: flex; align-items: center; gap: 16px; flex-wrap: wrap;
  background: var(--line2); border-radius: 10px; padding: 10px 14px;
}
.map-name { font-size: 13px; font-weight: 560; min-width: 82px; }

.hwlist { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px; }
.hwgroup { border: 1px solid var(--line); border-radius: 10px; padding: 12px 14px; }
.hwtitle { font-size: 11.5px; color: var(--ink2); letter-spacing: .06em; margin-bottom: 8px; }
.hwrow { display: flex; align-items: baseline; gap: 10px; font-size: 12.5px; padding: 3px 0; }
.hwkey { color: var(--ink3); min-width: 56px; font-family: ui-monospace, Menlo, monospace; font-size: 11.5px; }
.hwval { color: var(--ink); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hwext { color: var(--ink2); font-variant-numeric: tabular-nums; }
.empty {
  margin-top: 14px; padding: 18px; border: 1px dashed var(--line);
  border-radius: 10px; font-size: 12.5px; color: var(--ink3); text-align: center;
}

.foot { font-size: 11.5px; color: var(--ink3); text-align: center; padding: 10px 0 26px; line-height: 2; }

/* ── 轻提示 ── */
.toast {
  position: fixed; left: 50%; bottom: 28px; transform: translateX(-50%) translateY(14px);
  background: var(--ink); color: var(--card); font-size: 13px;
  padding: 9px 18px; border-radius: 10px; opacity: 0; pointer-events: none;
  transition: opacity .2s, transform .2s; z-index: 90; max-width: 80vw; text-align: center;
}
.toast.on { opacity: 1; transform: translateX(-50%) translateY(0); }

@media (max-width: 960px) {
  .kpis { grid-template-columns: repeat(2, 1fr); }
  .cols { grid-template-columns: 1fr; }
  .hwlist { grid-template-columns: 1fr; }
  .appbar .inner, .wrap { padding-left: 16px; padding-right: 16px; }
  .brand .sub { display: none; }
}
@media (max-width: 560px) {
  .kpis { grid-template-columns: 1fr; }
  .acts .pill { display: none; }
}
</style>
