package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法: go run ./cmd/make_test_audio.go <输出文件> <频率> <时长>")
		fmt.Println("示例: go run ./cmd/make_test_audio.go test.wav 440 2")
		return
	}

	filename := os.Args[1]
	var freq, duration float64
	fmt.Sscanf(os.Args[2], "%f", &freq)
	fmt.Sscanf(os.Args[3], "%f", &duration)

	fmt.Printf("生成音频文件: %s (频率: %.0fHz, 时长: %.1f秒)\n", filename, freq, duration)

	if err := generateWAV(filename, freq, duration); err != nil {
		fmt.Printf("生成失败: %v\n", err)
		return
	}

	fmt.Println("完成!")
}

func generateWAV(filename string, freq, duration float64) error {
	sampleRate := 16000
	bitsPerSample := 16
	numChannels := 2
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8

	samples := int(duration * float64(sampleRate))
	dataSize := samples * numChannels * bitsPerSample / 8
	fileSize := 36 + dataSize

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	binary.Write(file, binary.LittleEndian, []byte("RIFF"))
	binary.Write(file, binary.LittleEndian, uint32(fileSize))
	binary.Write(file, binary.LittleEndian, []byte("WAVE"))
	binary.Write(file, binary.LittleEndian, []byte("fmt "))
	binary.Write(file, binary.LittleEndian, uint32(16))
	binary.Write(file, binary.LittleEndian, uint16(1))
	binary.Write(file, binary.LittleEndian, uint16(numChannels))
	binary.Write(file, binary.LittleEndian, uint32(sampleRate))
	binary.Write(file, binary.LittleEndian, uint32(byteRate))
	binary.Write(file, binary.LittleEndian, uint16(blockAlign))
	binary.Write(file, binary.LittleEndian, uint16(bitsPerSample))
	binary.Write(file, binary.LittleEndian, []byte("data"))
	binary.Write(file, binary.LittleEndian, uint32(dataSize))

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(32767 * 0.5 * math.Sin(2*math.Pi*freq*t))

		binary.Write(file, binary.LittleEndian, sample)
		binary.Write(file, binary.LittleEndian, sample)
	}

	return nil
}
