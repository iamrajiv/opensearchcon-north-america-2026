<script setup lang="ts">
import { computed } from 'vue'
import { useSlideContext } from '@slidev/client'

// The slide declares `clicks: 4`. Each click moves the story forward:
// 0 the static pipeline, 1 documents flow, 2 the cluster slows and the
// queue fills, 3 upstream stages block, 4 a 429 puts one worker to sleep.
const { $clicks } = useSlideContext()
const stage = computed(() => Math.min(4, Math.max(0, $clicks.value)))

// Geometry. Every connector starts on the right edge of one box and ends
// on the left edge of the next, so nothing floats or overshoots.
const laneY = 155
const workers = [0, 1, 2, 3].map(i => ({ i, y: 40 + i * 58, cy: 60 + i * 58 }))
const inBus = 775
const outBus = 925

// Documents ride the real connector path: lane, fan-in bus, a worker, fan-out
// bus, OpenSearch. Negative begin offsets spread them along the path at once.
const dots = Array.from({ length: 8 }, (_, i) => {
  const w = workers[i % workers.length]
  return {
    i,
    begin: `${-(i * 0.45).toFixed(2)}s`,
    path: `M150,${laneY} H${inBus} V${w.cy} H800 M900,${w.cy} H${outBus} V${laneY} H960`,
  }
})
// Flow speed is the story: brisk while healthy, crawling once the cluster slows.
const flowDur = computed(() => (stage.value === 1 ? '3.6s' : '9s'))
</script>

<template>
  <svg viewBox="0 0 1060 380" class="pipeline" :class="`s${stage}`" role="img"
    aria-label="Pipeline: generate, events channel, batcher, batches channel, worker pool, OpenSearch bulk API">
    <defs>
      <marker id="pl-head" markerWidth="9" markerHeight="9" refX="9" refY="4.5" orient="auto" markerUnits="userSpaceOnUse">
        <path d="M0,0 L9,4.5 L0,9 z" class="head" />
      </marker>
      <marker id="pl-head-error" markerWidth="9" markerHeight="9" refX="9" refY="4.5" orient="auto" markerUnits="userSpaceOnUse">
        <path d="M0,0 L9,4.5 L0,9 z" class="head error" />
      </marker>
    </defs>

    <!-- documents in flight, drawn first so the boxes sit on top. Removed once the pipeline blocks. -->
    <g v-if="stage === 1 || stage === 2" :key="`flow-${stage}`" class="flow" aria-hidden="true">
      <circle v-for="d in dots" :key="d.i" r="4.5" class="doc">
        <animateMotion :dur="flowDur" :begin="d.begin" repeatCount="indefinite" :path="d.path" />
      </circle>
    </g>

    <!-- main lane connectors: right edge to left edge -->
    <line x1="150" :y1="laneY" x2="220" :y2="laneY" class="lane" marker-end="url(#pl-head)" />
    <line x1="345" :y1="laneY" x2="415" :y2="laneY" class="lane" marker-end="url(#pl-head)" />
    <line x1="555" :y1="laneY" x2="625" :y2="laneY" class="lane" marker-end="url(#pl-head)" />
    <line x1="750" :y1="laneY" :x2="inBus" :y2="laneY" class="lane" />

    <!-- fan-in bus to the workers, fan-out bus to OpenSearch -->
    <line :x1="inBus" :y1="workers[0].cy" :x2="inBus" :y2="workers[3].cy" class="lane" />
    <line v-for="w in workers" :key="'in' + w.i" :x1="inBus" :y1="w.cy" x2="800" :y2="w.cy" class="lane thin" marker-end="url(#pl-head)" />
    <line v-for="w in workers" :key="'out' + w.i" x1="900" :y1="w.cy" :x2="outBus" :y2="w.cy" class="lane thin" />
    <line :x1="outBus" :y1="workers[0].cy" :x2="outBus" :y2="workers[3].cy" class="lane" />
    <line :x1="outBus" :y1="laneY" x2="960" :y2="laneY" class="lane" marker-end="url(#pl-head)" />

    <!-- generate -->
    <g class="stage gen" :class="{ blocked: stage >= 3 }">
      <rect x="20" y="120" width="130" height="70" rx="6" />
      <text x="85" y="150" class="label">generate</text>
      <text x="85" y="172" class="sub">1 goroutine</text>
      <text v-if="stage >= 3" x="85" y="215" class="state">blocked</text>
    </g>

    <!-- events channel -->
    <g class="chan">
      <rect x="220" y="130" width="125" height="50" rx="6" />
      <text x="282" y="151" class="label small">chan Event</text>
      <text x="282" y="169" class="sub">cap = batch</text>
      <g v-if="stage >= 3" class="fill" aria-hidden="true">
        <rect v-for="i in 5" :key="i" :x="226 + (i - 1) * 23" y="186" width="19" height="6" rx="1" />
      </g>
    </g>

    <!-- batcher -->
    <g class="stage batcher" :class="{ blocked: stage >= 3 }">
      <rect x="415" y="120" width="140" height="70" rx="6" />
      <text x="485" y="150" class="label">batcher</text>
      <text x="485" y="172" class="sub">2000 docs or 500 ms</text>
      <text v-if="stage >= 3" x="485" y="215" class="state">blocked</text>
    </g>

    <!-- batches channel -->
    <g class="chan">
      <rect x="625" y="130" width="125" height="50" rx="6" />
      <text x="687" y="151" class="label small">chan []Event</text>
      <text x="687" y="169" class="sub">cap = queue</text>
      <g v-if="stage >= 2" class="fill" aria-hidden="true">
        <rect v-for="i in 4" :key="i" :x="633 + (i - 1) * 28" y="186" width="24" height="6" rx="1" />
      </g>
    </g>

    <!-- workers -->
    <g class="workers">
      <g v-for="w in workers" :key="w.i" class="worker" :class="{ busy: stage >= 2, sleeping: stage >= 4 && w.i === 1 }">
        <rect x="800" :y="w.y" width="100" height="40" rx="6" />
        <text x="850" :y="w.cy + 5" class="label small">worker {{ w.i + 1 }}</text>
      </g>
      <text x="850" y="292" class="sub">N workers, one request in flight each</text>
      <text v-if="stage >= 2" x="850" y="312" class="state">all waiting on responses</text>
      <text v-if="stage >= 4" x="850" y="24" class="state">429 on worker 2: sleep 0.4 s, resend only what was rejected</text>
    </g>

    <!-- OpenSearch -->
    <g class="os" :class="{ slow: stage >= 2 }">
      <rect x="960" y="120" width="90" height="70" rx="6" />
      <text x="1005" y="150" class="label small on-dark">_bulk</text>
      <text x="1005" y="170" class="sub on-dark">OpenSearch</text>
      <text v-if="stage >= 2" x="1005" y="215" class="state">slow</text>
    </g>

    <!-- backpressure -->
    <g v-if="stage >= 3" class="backpressure">
      <line :x1="inBus" y1="330" x2="85" y2="330" marker-end="url(#pl-head-error)" />
      <text x="430" y="355" class="state">a full channel blocks the stage before it, all the way back to generate</text>
    </g>

    <text v-else x="530" y="355" class="caption">
      <template v-if="stage === 0">three knobs: batch size, number of workers, queue depth</template>
      <template v-else-if="stage === 1">documents flow left to right; N workers means N concurrent bulk requests</template>
      <template v-else>OpenSearch slows down, responses take longer, the batches channel fills to its capacity</template>
    </text>
  </svg>
</template>

<style scoped>
.pipeline { width: 100%; height: auto; font-family: inherit; color: var(--osc-text); }
.lane { stroke: var(--osc-text); stroke-width: 2; fill: none; }
.lane.thin { stroke-width: 1.5; }
.head { fill: var(--osc-text); }
.head.error { fill: var(--osc-error); }
.stage rect { fill: var(--osc-bg); stroke: var(--osc-text); stroke-width: 1.5; transition: stroke 0.3s; }
.chan rect { fill: var(--osc-bg); stroke: var(--osc-text-secondary); stroke-width: 1.5; stroke-dasharray: 5 4; }
.worker rect { fill: var(--osc-bg); stroke: var(--osc-text); stroke-width: 1.5; transition: fill 0.3s, stroke 0.3s; }
.worker.busy rect { fill: var(--osc-bg-secondary); }
.worker.sleeping rect { stroke: var(--osc-error); stroke-width: 2; }
.os rect { fill: var(--osc-text); stroke: var(--osc-text); stroke-width: 1.5; transition: fill 0.3s; }
.os.slow rect { fill: var(--osc-text-secondary); stroke: var(--osc-text-secondary); }
.stage.blocked rect { stroke: var(--osc-error); stroke-width: 2; }
.fill rect { fill: var(--osc-text-secondary); }
.label { font-size: 20px; font-weight: 500; fill: var(--osc-text); text-anchor: middle; }
.label.small { font-size: 16px; }
.sub { font-size: 13px; fill: var(--osc-text-secondary); text-anchor: middle; }
.on-dark { fill: var(--osc-bg); }
.sub.on-dark { fill: var(--osc-bg); opacity: 0.8; }
.state { font-size: 14px; font-weight: 500; fill: var(--osc-error); text-anchor: middle; }
.caption { font-size: 16px; fill: var(--osc-text-secondary); text-anchor: middle; }
.backpressure line { stroke: var(--osc-error); stroke-width: 1.5; stroke-dasharray: 8 6; }
.doc { fill: var(--osc-text-secondary); }
@media (prefers-reduced-motion: reduce) { .flow { display: none; } }
</style>
