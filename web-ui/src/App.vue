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
          <div class="hint mt">主题与时钟立刻生效；刷新节奏改动后，新周期从下一次采样开始算。</div>
        </div>
      </section>

      <!-- ── 风扇 + 灯效 ── -->
      <div class="grid2">
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
const display = reactive({ theme: 'ink', clock_24h: true, weather: true, sample_seconds: 2 })

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

const loadDisplay = () => api('/api/display').then((d) => Object.assign(display, d)).catch(() => {})

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
  Promise.all([loadConfig(), loadDisplay(), loadStatus()])
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
    .then(() => tip('映射已保存，重启服务后生效'))
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
  api('/api/display', { ...display })
    .then((d) => { if (d.display) Object.assign(display, d.display); tip('显示设置已应用') })
    .catch(() => { display[key] = prev; tip('显示设置失败') })
}

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
})
onUnmounted(() => clearInterval(pollTimer))
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

.grid2 { display: grid; grid-template-columns: 1fr 360px; gap: 18px; align-items: stretch; }
.grid2 .card { display: flex; flex-direction: column; }
.grid2 .card .bd { flex: 1 1 auto; }
.grid2 .card { margin-bottom: 18px; }

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
  .grid2 { grid-template-columns: 1fr; }
  .hwlist { grid-template-columns: 1fr; }
  .appbar .inner, .wrap { padding-left: 16px; padding-right: 16px; }
  .brand .sub { display: none; }
}
@media (max-width: 560px) {
  .kpis { grid-template-columns: 1fr; }
  .acts .pill { display: none; }
}
</style>
