const SAMPLE_RATE = 16000;
const CHANNELS = 1;
const FRAME_SIZE = 960;
const MIN_AUDIO_DURATION = 0.1;

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function createAudioEngine({ onStatus, onError } = {}) {
  let audioContext = null;
  let analyser = null;
  let audioSource = null;
  let audioProcessor = null;
  let audioProcessorType = null;
  let opusEncoder = null;
  let opusDecoder = null;
  let isRecording = false;
  let pcmDataBuffer = new Int16Array(0);
  let currentWs = null;

  const stats = {
    receivedFrames: 0,
    receivedBytes: 0,
    lastFrameBytes: 0,
    decodedSamples: 0,
    lastDecodedSamples: 0,
    playedChunks: 0,
    audioContextState: 'none',
    bufferQueueLength: 0,
    isAudioBuffering: false,
    isAudioPlaying: false,
    opusEncoderReady: false,
    opusDecoderReady: false,
    lastError: ''
  };

  let audioBufferQueue = [];
  let isAudioBuffering = false;
  let isAudioPlaying = false;
  let streamingContext = null;

  const audioProcessorCode = `
    class AudioRecorderProcessor extends AudioWorkletProcessor {
      constructor() {
        super();
        this.frameSize = ${FRAME_SIZE};
        this.buffer = new Int16Array(this.frameSize);
        this.bufferIndex = 0;
        this.isRecording = false;
        this.port.onmessage = (event) => {
          if (event.data.command === 'start') {
            this.isRecording = true;
            this.port.postMessage({ type: 'status', status: 'started' });
          } else if (event.data.command === 'stop') {
            this.isRecording = false;
            if (this.bufferIndex > 0) {
              const finalBuffer = this.buffer.slice(0, this.bufferIndex);
              this.port.postMessage({ type: 'buffer', buffer: finalBuffer });
              this.bufferIndex = 0;
            }
            this.port.postMessage({ type: 'status', status: 'stopped' });
          }
        };
      }

      process(inputs) {
        if (!this.isRecording) return true;
        const input = inputs[0][0];
        if (!input) return true;

        for (let i = 0; i < input.length; i++) {
          if (this.bufferIndex >= this.frameSize) {
            this.port.postMessage({ type: 'buffer', buffer: this.buffer.slice(0) });
            this.bufferIndex = 0;
          }
          this.buffer[this.bufferIndex++] = Math.max(
            -32768,
            Math.min(32767, Math.floor(input[i] * 32767))
          );
        }
        return true;
      }
    }

    registerProcessor('audio-recorder-processor', AudioRecorderProcessor);
  `;

  const emitStatus = (msg) => {
    if (onStatus) onStatus(msg);
  };

  const emitError = (msg) => {
    stats.lastError = msg;
    if (onError) onError(msg);
  };

  async function waitForOpusModule(timeoutMs = 8000) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      if (typeof window.ModuleInstance !== 'undefined') return true;
      if (typeof window.Module !== 'undefined') {
        if (window.Module.instance) {
          window.ModuleInstance = window.Module.instance;
          return true;
        }
        if (typeof window.Module._opus_decoder_get_size === 'function') {
          window.ModuleInstance = window.Module;
          return true;
        }
      }
      await sleep(100);
    }
    return false;
  }

  async function ensureModule() {
    const ok = await waitForOpusModule();
    if (!ok) {
      throw new Error('Opus库未加载，请确认libopus.js已引入');
    }
    return window.ModuleInstance;
  }

  async function ensureAudioContext() {
    if (!audioContext) {
      audioContext = new (window.AudioContext || window.webkitAudioContext)({
        sampleRate: SAMPLE_RATE,
        latencyHint: 'interactive'
      });
    }
    if (audioContext.state === 'suspended') {
      try {
        await audioContext.resume();
      } catch (error) {
        emitError(`音频上下文恢复失败: ${error.message}`);
      }
    }
    stats.audioContextState = audioContext.state;
    return audioContext;
  }

  async function initOpusEncoder() {
    if (opusEncoder) return opusEncoder;
    const mod = await ensureModule();
    const application = 2048;
    opusEncoder = {
      channels: CHANNELS,
      sampleRate: SAMPLE_RATE,
      frameSize: FRAME_SIZE,
      maxPacketSize: 4000,
      module: mod,
      encoderPtr: null,
      init: function () {
        if (this.encoderPtr) return true;
        const encoderSize = mod._opus_encoder_get_size(this.channels);
        this.encoderPtr = mod._malloc(encoderSize);
        if (!this.encoderPtr) {
          throw new Error('无法分配编码器内存');
        }
        const err = mod._opus_encoder_init(
          this.encoderPtr,
          this.sampleRate,
          this.channels,
          application
        );
        if (err < 0) {
          throw new Error(`Opus编码器初始化失败: ${err}`);
        }
        mod._opus_encoder_ctl(this.encoderPtr, 4002, 16000);
        mod._opus_encoder_ctl(this.encoderPtr, 4010, 5);
        mod._opus_encoder_ctl(this.encoderPtr, 4016, 1);
        return true;
      },
      encode: function (pcmData) {
        if (!this.encoderPtr && !this.init()) return null;
        const mod = this.module;
        const pcmPtr = mod._malloc(pcmData.length * 2);
        for (let i = 0; i < pcmData.length; i++) {
          mod.HEAP16[(pcmPtr >> 1) + i] = pcmData[i];
        }
        const outputPtr = mod._malloc(this.maxPacketSize);
        const encodedBytes = mod._opus_encode(
          this.encoderPtr,
          pcmPtr,
          this.frameSize,
          outputPtr,
          this.maxPacketSize
        );
        const result =
          encodedBytes > 0
            ? new Uint8Array(mod.HEAPU8.buffer, outputPtr, encodedBytes).slice()
            : null;
        mod._free(pcmPtr);
        mod._free(outputPtr);
        return result;
      }
    };
    opusEncoder.init();
    stats.opusEncoderReady = true;
    return opusEncoder;
  }

  async function initOpusDecoder() {
    if (opusDecoder) return opusDecoder;
    const mod = await ensureModule();
    opusDecoder = {
      channels: CHANNELS,
      rate: SAMPLE_RATE,
      frameSize: FRAME_SIZE,
      module: mod,
      decoderPtr: null,
      init: function () {
        if (this.decoderPtr) return true;
        const decoderSize = mod._opus_decoder_get_size(this.channels);
        this.decoderPtr = mod._malloc(decoderSize);
        if (!this.decoderPtr) {
          throw new Error('无法分配解码器内存');
        }
        const err = mod._opus_decoder_init(
          this.decoderPtr,
          this.rate,
          this.channels
        );
        if (err < 0) {
          throw new Error(`Opus解码器初始化失败: ${err}`);
        }
        return true;
      },
      decode: function (opusData) {
        if (!this.decoderPtr && !this.init()) return new Int16Array(0);
        const mod = this.module;
        const opusPtr = mod._malloc(opusData.length);
        mod.HEAPU8.set(opusData, opusPtr);
        const pcmPtr = mod._malloc(this.frameSize * 2);
        const decodedSamples = mod._opus_decode(
          this.decoderPtr,
          opusPtr,
          opusData.length,
          pcmPtr,
          this.frameSize,
          0
        );
        if (decodedSamples < 0) {
          mod._free(opusPtr);
          mod._free(pcmPtr);
          return new Int16Array(0);
        }
        const decodedData = new Int16Array(decodedSamples);
        for (let i = 0; i < decodedSamples; i++) {
          decodedData[i] = mod.HEAP16[(pcmPtr >> 1) + i];
        }
        mod._free(opusPtr);
        mod._free(pcmPtr);
        return decodedData;
      }
    };
    opusDecoder.init();
    stats.opusDecoderReady = true;
    return opusDecoder;
  }

  function convertInt16ToFloat32(int16Data) {
    const float32Data = new Float32Array(int16Data.length);
    for (let i = 0; i < int16Data.length; i++) {
      float32Data[i] = int16Data[i] / (int16Data[i] < 0 ? 0x8000 : 0x7fff);
    }
    return float32Data;
  }

  async function createAudioProcessor() {
    await ensureAudioContext();

    if (audioContext.audioWorklet) {
      const blob = new Blob([audioProcessorCode], {
        type: 'application/javascript'
      });
      const url = URL.createObjectURL(blob);
      await audioContext.audioWorklet.addModule(url);
      URL.revokeObjectURL(url);
      const node = new AudioWorkletNode(
        audioContext,
        'audio-recorder-processor'
      );
      node.port.onmessage = (event) => {
        if (event.data.type === 'buffer') {
          processPCMBuffer(event.data.buffer);
        }
      };
      audioProcessorType = 'worklet';
      return node;
    }

    const frameSize = 4096;
    const scriptProcessor = audioContext.createScriptProcessor(frameSize, 1, 1);
    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording) return;
      const input = event.inputBuffer.getChannelData(0);
      const buffer = new Int16Array(input.length);
      for (let i = 0; i < input.length; i++) {
        buffer[i] = Math.max(-32768, Math.min(32767, Math.floor(input[i] * 32767)));
      }
      processPCMBuffer(buffer);
    };
    const silent = audioContext.createGain();
    silent.gain.value = 0;
    scriptProcessor.connect(silent);
    silent.connect(audioContext.destination);
    audioProcessorType = 'processor';
    return scriptProcessor;
  }

  function processPCMBuffer(buffer) {
    if (!isRecording) return;
    const newBuffer = new Int16Array(pcmDataBuffer.length + buffer.length);
    newBuffer.set(pcmDataBuffer);
    newBuffer.set(buffer, pcmDataBuffer.length);
    pcmDataBuffer = newBuffer;

    while (pcmDataBuffer.length >= FRAME_SIZE) {
      const frameData = pcmDataBuffer.slice(0, FRAME_SIZE);
      pcmDataBuffer = pcmDataBuffer.slice(FRAME_SIZE);
      encodeAndSendOpus(frameData);
    }
  }

  function encodeAndSendOpus(pcmData, ws) {
    if (!opusEncoder) return;
    const targetWs = ws || currentWs;
    if (pcmData) {
      const opusData = opusEncoder.encode(pcmData);
      if (opusData && opusData.length > 0 && targetWs && targetWs.readyState === WebSocket.OPEN) {
        targetWs.send(opusData.buffer);
      }
    } else if (pcmDataBuffer.length > 0) {
      const padded = new Int16Array(FRAME_SIZE);
      padded.set(pcmDataBuffer.slice(0, FRAME_SIZE));
      pcmDataBuffer = new Int16Array(0);
      encodeAndSendOpus(padded, targetWs);
    }
  }

  async function startRecording(ws) {
    if (isRecording) return true;
    try {
      currentWs = ws || null;
      await initOpusEncoder();
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          sampleRate: SAMPLE_RATE,
          channelCount: 1
        }
      });
      await ensureAudioContext();
      audioSource = audioContext.createMediaStreamSource(stream);
      analyser = audioContext.createAnalyser();
      analyser.fftSize = 2048;
      audioSource.connect(analyser);

      audioProcessor = await createAudioProcessor();
      audioSource.connect(audioProcessor);

      pcmDataBuffer = new Int16Array(0);
      isRecording = true;

      if (audioProcessorType === 'worklet' && audioProcessor.port) {
        audioProcessor.port.postMessage({ command: 'start' });
      }

      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'listen',
            mode: 'manual',
            state: 'start'
          })
        );
      }
      emitStatus('录音已开始');
      return true;
    } catch (error) {
      emitError(`录音启动失败: ${error.message}`);
      isRecording = false;
      return false;
    }
  }

  function stopRecording(ws) {
    if (!isRecording) return;
    isRecording = false;
    currentWs = ws || currentWs;

    if (audioProcessor) {
      if (audioProcessorType === 'worklet' && audioProcessor.port) {
        audioProcessor.port.postMessage({ command: 'stop' });
      }
      audioProcessor.disconnect();
      audioProcessor = null;
    }
    if (audioSource) {
      audioSource.disconnect();
      audioSource = null;
    }
    encodeAndSendOpus(null, ws);
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new Uint8Array(0));
      ws.send(
        JSON.stringify({
          type: 'listen',
          mode: 'manual',
          state: 'stop'
        })
      );
    }
    emitStatus('录音已停止');
  }

  function startAudioBuffering() {
    if (isAudioBuffering) return;
    isAudioBuffering = true;
    stats.isAudioBuffering = true;
    setTimeout(() => {
      if (isAudioBuffering && audioBufferQueue.length > 0) {
        playBufferedAudio();
      }
    }, 300);
    const interval = setInterval(() => {
      if (!isAudioBuffering) {
        clearInterval(interval);
        return;
      }
      if (audioBufferQueue.length >= 3) {
        clearInterval(interval);
        playBufferedAudio();
      }
    }, 50);
  }

  async function playBufferedAudio() {
    if (isAudioPlaying || audioBufferQueue.length === 0) return;
    isAudioPlaying = true;
    isAudioBuffering = false;
    stats.isAudioPlaying = true;
    stats.isAudioBuffering = false;

    await ensureAudioContext();
    if (!opusDecoder) {
      await initOpusDecoder();
    }

    if (!streamingContext) {
      streamingContext = {
        queue: [],
        playing: false,
        endOfStream: false,
        source: null,
        decodeOpusFrames: async function (frames) {
          if (!opusDecoder) return;
          const decodedSamples = [];
          for (const frame of frames) {
            const frameData = opusDecoder.decode(frame);
            if (frameData && frameData.length > 0) {
              const floatData = convertInt16ToFloat32(frameData);
              for (let i = 0; i < floatData.length; i++) {
                decodedSamples.push(floatData[i]);
              }
            }
          }
          stats.lastDecodedSamples = decodedSamples.length;
          stats.decodedSamples += decodedSamples.length;
          if (decodedSamples.length > 0) {
            for (let i = 0; i < decodedSamples.length; i++) {
              this.queue.push(decodedSamples[i]);
            }
          const minSamples = SAMPLE_RATE * MIN_AUDIO_DURATION;
            if (!this.playing && this.queue.length >= minSamples) {
              this.startPlaying();
            }
          }
        },
        startPlaying: function () {
          if (this.playing || this.queue.length === 0) return;
          this.playing = true;
          stats.playedChunks += 1;
          const playSamples = Math.min(this.queue.length, SAMPLE_RATE);
          const currentSamples = this.queue.splice(0, playSamples);
          const audioBuffer = audioContext.createBuffer(
            CHANNELS,
            currentSamples.length,
            SAMPLE_RATE
          );
          audioBuffer.copyToChannel(new Float32Array(currentSamples), 0);
          this.source = audioContext.createBufferSource();
          this.source.buffer = audioBuffer;
          const gainNode = audioContext.createGain();
          const fade = 0.02;
          gainNode.gain.setValueAtTime(0, audioContext.currentTime);
          gainNode.gain.linearRampToValueAtTime(1, audioContext.currentTime + fade);
          const duration = audioBuffer.duration;
          if (duration > fade * 2) {
            gainNode.gain.setValueAtTime(1, audioContext.currentTime + duration - fade);
            gainNode.gain.linearRampToValueAtTime(0, audioContext.currentTime + duration);
          }
          this.source.connect(gainNode);
          gainNode.connect(audioContext.destination);
          this.source.onended = () => {
            this.source = null;
            this.playing = false;
            setTimeout(() => {
              if (this.queue.length > 0) {
                this.startPlaying();
              } else if (audioBufferQueue.length > 0) {
                const frames = [...audioBufferQueue];
                audioBufferQueue = [];
                this.decodeOpusFrames(frames);
              } else if (this.endOfStream) {
                isAudioPlaying = false;
                this.endOfStream = false;
                streamingContext = null;
                stats.isAudioPlaying = false;
              } else {
                setTimeout(() => {
                  if (this.queue.length === 0 && audioBufferQueue.length === 0) {
                    isAudioPlaying = false;
                    streamingContext = null;
                    stats.isAudioPlaying = false;
                  }
                }, 500);
              }
            }, 10);
          };
          this.source.start();
        }
      };
    }

    const frames = [...audioBufferQueue];
    audioBufferQueue = [];
    await streamingContext.decodeOpusFrames(frames);
  }

  async function handleIncomingAudio(data) {
    let arrayBuffer;
    if (data instanceof ArrayBuffer) {
      arrayBuffer = data;
    } else if (data instanceof Blob) {
      arrayBuffer = await data.arrayBuffer();
    } else {
      return;
    }
    const opusData = new Uint8Array(arrayBuffer);
    if (opusData.length > 0) {
      audioBufferQueue.push(opusData);
      stats.receivedFrames += 1;
      stats.receivedBytes += opusData.length;
      stats.lastFrameBytes = opusData.length;
      if (audioBufferQueue.length === 1 && !isAudioBuffering && !isAudioPlaying) {
        startAudioBuffering();
      }
    } else {
      if (audioBufferQueue.length > 0 && !isAudioPlaying) {
        playBufferedAudio();
      }
      if (isAudioPlaying && streamingContext) {
        streamingContext.endOfStream = true;
      }
    }
  }

  function stopAll(ws) {
    if (isRecording) stopRecording(ws);
    isAudioBuffering = false;
    audioBufferQueue = [];
    streamingContext = null;
    stats.isAudioBuffering = false;
    stats.isAudioPlaying = false;
  }

  return {
    startRecording,
    stopRecording,
    handleIncomingAudio,
    stopAll,
    resume: ensureAudioContext,
    getStats: () => {
      stats.bufferQueueLength = audioBufferQueue.length;
      stats.isAudioBuffering = isAudioBuffering;
      stats.isAudioPlaying = isAudioPlaying;
      stats.audioContextState = audioContext ? audioContext.state : 'none';
      return { ...stats };
    }
  };
}
