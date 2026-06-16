<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useData } from 'vitepress'

let chartCounter = 0

const props = defineProps({
  code: {
    type: String,
    required: true
  }
})

const { isDark } = useData()
const chartRef = ref(null)
const error = ref('')
const rendered = ref(false)
const instanceId = chartCounter

chartCounter += 1

const chartId = computed(() => {
  let hash = 0
  for (let i = 0; i < props.code.length; i += 1) {
    hash = (hash << 5) - hash + props.code.charCodeAt(i)
    hash |= 0
  }
  return `mermaid-${instanceId}-${Math.abs(hash)}-${isDark.value ? 'dark' : 'light'}`
})

async function renderChart() {
  if (!chartRef.value) {
    return
  }

  error.value = ''
  rendered.value = false

  try {
    const { default: mermaid } = await import('mermaid')

    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: isDark.value ? 'dark' : 'default'
    })

    const { svg } = await mermaid.render(chartId.value, props.code)
    chartRef.value.innerHTML = svg
    rendered.value = true
  } catch (err) {
    chartRef.value.innerHTML = ''
    error.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(() => {
  renderChart()
})

watch(
  () => [props.code, isDark.value],
  async () => {
    await nextTick()
    renderChart()
  }
)
</script>

<template>
  <figure class="mermaid-chart">
    <div ref="chartRef" class="mermaid-chart__canvas" />
    <pre v-if="!rendered && !error" class="mermaid-chart__source">{{ code }}</pre>
    <p v-if="error" class="mermaid-chart__error">Mermaid render failed: {{ error }}</p>
  </figure>
</template>
