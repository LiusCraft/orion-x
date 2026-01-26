import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Orion-X',
  description: '智能语音机器人系统 - 基于 Go 的实时语音交互平台',
  base: '/orion-x/',

  themeConfig: {
    nav: [
      { text: '首页', link: '/' },
      { text: '快速开始', link: '/guide/getting-started' },
      { text: '架构设计', link: '/architecture/overview' },
      { text: 'GitHub', link: 'https://github.com/LiusCraft/orion-x' }
    ],

    sidebar: {
      '/guide/': [
        {
          text: '指南',
          items: [
            { text: '快速开始', link: '/guide/getting-started' },
            { text: '配置说明', link: '/guide/configuration' },
            { text: '工具开发', link: '/guide/development' }
          ]
        },
        {
          text: '模块文档',
          items: [
            { text: 'ASR 语音识别', link: '/guide/asr' },
            { text: 'TTS 语音合成', link: '/guide/tts' },
            { text: '音频输入管道', link: '/guide/audio-in-pipe' },
            { text: '日志系统', link: '/guide/logging' }
          ]
        }
      ],
      '/architecture/': [
        {
          text: '架构设计',
          items: [
            { text: '系统架构', link: '/architecture/overview' },
            { text: '模块详细设计', link: '/architecture/modules' },
            { text: '开发任务', link: '/architecture/todo' }
          ]
        }
      ]
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/LiusCraft/orion-x' }
    ],

    search: {
      provider: 'local'
    }
  }
})
