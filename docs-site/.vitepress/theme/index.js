import DefaultTheme from 'vitepress/theme'
import MermaidChart from './MermaidChart.vue'
import './style.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('MermaidChart', MermaidChart)
  }
}
