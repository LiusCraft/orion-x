package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
)

var (
	ttsFile      = flag.String("tts", "", "TTS 音频文件路径（WAV 格式）")
	resourceFile = flag.String("resource", "", "Resource 音频文件路径（WAV 格式）")
	duration     = flag.Float64("duration", 2.0, "生成音频的持续时间（秒）")
	help         = flag.Bool("h", false, "显示帮助信息")
)

func main() {
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	fmt.Println("=== AudioMixer 验证工具 ===")
	fmt.Println()

	mixer, err := audio.NewMixer(audio.DefaultMixerConfig())
	if err != nil {
		fmt.Printf("创建 Mixer 失败: %v\n", err)
		return
	}
	defer mixer.Stop()

	var ttsReader, resourceReader io.Reader
	var ttsSource, resourceSource string

	if *ttsFile != "" {
		fmt.Println("1. 读取 TTS 音频文件...")
		ttsData, err := readWAVFile(*ttsFile)
		if err != nil {
			fmt.Printf("读取 TTS 文件失败: %v\n", err)
			return
		}
		ttsReader = &loopReader{data: ttsData}
		ttsSource = fmt.Sprintf("文件: %s", *ttsFile)
	} else {
		fmt.Println("1. 生成 TTS 测试音频...")
		ttsSignal := generateSineWave(440, *duration)
		ttsReader = &loopReader{data: ttsSignal}
		ttsSource = fmt.Sprintf("440Hz 正弦波, %.1f秒", *duration)
	}

	if *resourceFile != "" {
		fmt.Println("2. 读取 Resource 音频文件...")
		resourceData, err := readWAVFile(*resourceFile)
		if err != nil {
			fmt.Printf("读取 Resource 文件失败: %v\n", err)
			return
		}
		resourceReader = &loopReader{data: resourceData}
		resourceSource = fmt.Sprintf("文件: %s", *resourceFile)
	} else {
		fmt.Println("2. 生成 Resource 测试音频...")
		resourceSignal := generateSineWave(880, *duration)
		resourceReader = &loopReader{data: resourceSignal}
		resourceSource = fmt.Sprintf("880Hz 正弦波, %.1f秒", *duration)
	}

	fmt.Println()
	fmt.Println("3. 添加音频流到 Mixer...")
	mixer.AddTTSStream(ttsReader)
	mixer.AddResourceStream(resourceReader)
	fmt.Printf("   - TTS: %s\n", ttsSource)
	fmt.Printf("   - Resource: %s\n", resourceSource)
	fmt.Println()

	fmt.Println("4. 开始播放...")
	mixer.Start()
	time.Sleep(time.Duration(*duration) * time.Second)

	fmt.Println("5. TTS 开始，Resource 音量降为 50%...")
	mixer.OnTTSStarted()
	time.Sleep(time.Duration(*duration) * time.Second)

	fmt.Println("6. TTS 结束，Resource 音量恢复 100%...")
	mixer.OnTTSFinished()
	time.Sleep(time.Duration(*duration) * time.Second)

	fmt.Println("7. 停止播放")
	time.Sleep(500 * time.Millisecond)
	mixer.Stop()

	fmt.Println()
	fmt.Println("=== 验证完成 ===")
	fmt.Println()
	fmt.Println("预期效果:")
	fmt.Println("  - 阶段1: 两个通道正常播放")
	fmt.Println("  - 阶段2: Resource 音量明显降低（约 50%）")
	fmt.Println("  - 阶段3: Resource 音量恢复原大小")
}

func printHelp() {
	fmt.Println("AudioMixer 验证工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  go run ./cmd/mixer [选项]")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -tts string")
	fmt.Println("        TTS 音频文件路径（WAV 格式，默认使用 440Hz 正弦波）")
	fmt.Println("  -resource string")
	fmt.Println("        Resource 音频文件路径（WAV 格式，默认使用 880Hz 正弦波）")
	fmt.Println("  -duration float")
	fmt.Println("        生成音频的持续时间，单位秒（默认 2.0）")
	fmt.Println("  -h")
	fmt.Println("        显示此帮助信息")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  go run ./cmd/mixer")
	fmt.Println("    - 使用默认的正弦波测试音频")
	fmt.Println()
	fmt.Println("  go run ./cmd/mixer -tts=voice.wav -resource=music.wav")
	fmt.Println("    - 使用指定的音频文件")
	fmt.Println()
	fmt.Println("  go run ./cmd/mixer -duration=5.0")
	fmt.Println("    - 生成 5 秒的测试音频")
}

type wavHeader struct {
	ChunkID       [4]byte
	FileSize      uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

func readWAVFile(filename string) ([]byte, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if len(data) < 44 {
		return nil, fmt.Errorf("文件太小，不是有效的 WAV 文件")
	}

	header := wavHeader{}
	reader := bytes.NewReader(data)
	binary.Read(reader, binary.LittleEndian, &header)

	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" {
		return nil, fmt.Errorf("不是有效的 WAV 文件")
	}

	if string(header.Subchunk1ID[:]) != "fmt " {
		return nil, fmt.Errorf("找不到 fmt chunk")
	}

	pcmData := data[44:]

	if header.NumChannels == 1 && header.BitsPerSample == 16 {
		pcmData = monoToStereo(pcmData)
	}

	return pcmData, nil
}

func monoToStereo(data []byte) []byte {
	if len(data)%2 != 0 {
		return data
	}

	stereoData := make([]byte, len(data)*2)
	for i := 0; i < len(data); i += 2 {
		stereoData[i*2] = data[i]
		stereoData[i*2+1] = data[i+1]
		stereoData[i*2+2] = data[i]
		stereoData[i*2+3] = data[i+1]
	}
	return stereoData
}

func generateSineWave(freq, duration float64) []byte {
	sampleRate := 16000
	samples := int(duration * float64(sampleRate))
	data := make([]byte, samples*4)

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(32767 * math.Sin(2*math.Pi*freq*t))

		data[i*4] = byte(sample)
		data[i*4+1] = byte(sample >> 8)
		data[i*4+2] = byte(sample)
		data[i*4+3] = byte(sample >> 8)
	}
	return data
}

type loopReader struct {
	data []byte
	pos  int
}

func (lr *loopReader) Read(p []byte) (n int, err error) {
	if lr.data == nil {
		return 0, io.EOF
	}
	if len(lr.data) == 0 {
		return 0, io.EOF
	}
	for len(p) > 0 {
		if lr.pos >= len(lr.data) {
			lr.pos = 0
		}
		copied := copy(p, lr.data[lr.pos:])
		lr.pos += copied
		p = p[copied:]
		n += copied
	}
	return n, nil
}
