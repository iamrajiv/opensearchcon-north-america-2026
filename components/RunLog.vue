<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { onSlideEnter, useIsSlideActive, useNav } from '@slidev/client'

// Replays a recorded run at the speed it actually happened. The delay between
// lines comes from the timestamps in the log itself, so a pause on the slide is
// a pause the cluster really took. Nothing is simulated: the text is the same
// file the repository ships.
const props = withDefaults(defineProps<{
  /** Basename of a file in examples/ingestor/runs, without the .log */
  src: string
  /** 1 is real time. Raise it to get through a long run faster. */
  speed?: number
}>(), { speed: 1 })

const logs = import.meta.glob('../examples/ingestor/runs/*.log', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

const lines = computed(() => {
  const key = Object.keys(logs).find(k => k.endsWith(`/${props.src}.log`))
  return key ? logs[key].trimEnd().split('\n') : [`RunLog: no such run "${props.src}"`]
})

// Seconds between consecutive lines, taken from the HH:MM:SS in each line.
const delays = computed(() => {
  const at = (l: string) => {
    const m = l.match(/\b(\d{2}):(\d{2}):(\d{2})\b/)
    return m ? (+m[1] * 3600 + +m[2] * 60 + +m[3]) : null
  }
  return lines.value.map((l, i) => {
    if (i === 0) return 0.35
    const a = at(lines.value[i - 1]); const b = at(l)
    return a !== null && b !== null ? Math.max(0, b - a) : 1
  })
})

const reduced = typeof matchMedia !== 'undefined'
  && matchMedia('(prefers-reduced-motion: reduce)').matches

const { isPrintMode } = useNav()
const isActive = useIsSlideActive()
const shown = ref(0)
let timer: ReturnType<typeof setTimeout> | undefined

function stop() {
  if (timer) clearTimeout(timer)
  timer = undefined
}

function showAll() {
  stop()
  shown.value = lines.value.length
}

function play() {
  stop()
  // Exported to PDF, shown in the overview, or the viewer asked for less
  // motion: the finished run is the honest still frame.
  if (isPrintMode.value || reduced) return showAll()
  shown.value = 0
  const step = () => {
    if (shown.value >= lines.value.length) return
    shown.value++
    const next = delays.value[shown.value]
    if (next === undefined) return
    timer = setTimeout(step, (next * 1000) / props.speed)
  }
  timer = setTimeout(step, (delays.value[0] * 1000) / props.speed)
}

onSlideEnter(play)
watch(isActive, a => a ? play() : showAll(), { immediate: true })
onUnmounted(stop)

const done = computed(() => shown.value >= lines.value.length)

// Split a line into its timestamp, its words and its numbers, so the numbers
// that change during the run are the ones that carry weight.
function parts(line: string) {
  const m = line.match(/^(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2} )?([\s\S]*)$/)
  const stamp = m?.[1] ?? ''
  const rest = m?.[2] ?? line
  const chunks = rest.split(/(\d[\d.]*)/)
  return {
    stamp,
    chunks: chunks.map((text, i) => ({
      text,
      isNumber: i % 2 === 1,
      // A non-zero failure count is the one thing on this slide that would be bad news.
      isBadNews: i % 2 === 1 && /failed=$/.test(chunks[i - 1] ?? '') && Number(text) > 0,
    })),
  }
}
</script>

<template>
  <div
    class="runlog"
    :class="{ 'is-done': done }"
    :style="{ '--lines': lines.length }"
    role="log"
    aria-live="off"
    :aria-label="`Recorded output of the ${src} run`"
  >
    <pre><code><template v-for="(line, i) in lines.slice(0, shown)" :key="i"><span
      class="line"
      :class="{ summary: /\b(done|throughput):/.test(line) }"
    ><span class="stamp">{{ parts(line).stamp }}</span><template
      v-for="(c, j) in parts(line).chunks"
      :key="j"
    ><span v-if="c.isNumber" class="num" :class="{ bad: c.isBadNews }">{{ c.text }}</span><template v-else>{{ c.text }}</template></template>
</span></template></code></pre>
  </div>
</template>

<style scoped>
/* Hold the full height from the first frame so the slide never reflows while
   the run plays and the text below it never moves. */
.runlog {
  min-height: calc(var(--lines) * 18px + 28px);
  border: 1px solid var(--osc-border-subtle);
  border-radius: var(--osc-radius-small);
  padding: 14px 16px;
}

.runlog pre {
  margin: 0;
  font-family: 'Geist Mono', ui-monospace, monospace;
  font-size: 13px;
  line-height: 18px;
  color: var(--osc-text-secondary);
}

.line { display: block; }
.stamp { opacity: 0.55; }
.num { color: var(--osc-text); font-weight: var(--osc-weight-medium); }
.num.bad { color: var(--osc-error); }
.summary { color: var(--osc-text); }
</style>
