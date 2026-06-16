package pipeline

import (
	"context"
	"testing"
)

func TestTextFilterStage(t *testing.T) {
	stage := NewTextFilterStage()
	input := make(chan Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := stage.Process(ctx, input)

	// 发送带有 metadata 标签的文本
	input <- NewMessage(MessageTypeTextChunk, "Hello <metadata>world</metadata>!")

	msg := <-output
	if msg.Payload != "Hello !" {
		t.Errorf("Expected 'Hello !', got '%s'", msg.Payload)
	}
}

func TestTextFilterStage_NonTextMessage(t *testing.T) {
	stage := NewTextFilterStage()
	input := make(chan Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := stage.Process(ctx, input)

	// 发送非文本消息，应该原样透传
	input <- NewMessage(MessageTypeAudioData, []byte{1, 2, 3})

	msg := <-output
	if msg.Type != MessageTypeAudioData {
		t.Errorf("Expected MessageTypeAudioData, got %s", msg.Type)
	}
}

func TestEmotionExtractorStage(t *testing.T) {
	stage := NewEmotionExtractorStage()
	input := make(chan Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := stage.Process(ctx, input)

	// 发送带有 emotion 标签的文本
	input <- NewMessage(MessageTypeTextChunk, "I'm so <emotion>happy</emotion> today!")

	msg := <-output
	if msg.Metadata.Emotion != "happy" {
		t.Errorf("Expected emotion 'happy', got '%s'", msg.Metadata.Emotion)
	}
}

func TestEmotionExtractorStage_NoEmotion(t *testing.T) {
	stage := NewEmotionExtractorStage()
	input := make(chan Message, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := stage.Process(ctx, input)

	// 发送不带 emotion 标签的文本
	input <- NewMessage(MessageTypeTextChunk, "Just a normal message")

	msg := <-output
	if msg.Metadata.Emotion != "" {
		t.Errorf("Expected empty emotion, got '%s'", msg.Metadata.Emotion)
	}
}

func TestPipelineWithFiltersAndExtractors(t *testing.T) {
	// 组合多个 Stage
	pipeline := NewBuilder().
		AddStage(NewEmotionExtractorStage()). // 先提取情感
		AddStage(NewTextFilterStage()).       // 再过滤标签
		Build()

	ctx := context.Background()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = pipeline.Stop() }()

	go func() {
		pipeline.Input() <- NewMessage(MessageTypeTextChunk, "I'm <emotion>happy</emotion> <metadata>today</metadata>!")
	}()

	msg := <-pipeline.Output()

	// 验证情感被提取
	if msg.Metadata.Emotion != "happy" {
		t.Errorf("Expected emotion 'happy', got '%s'", msg.Metadata.Emotion)
	}

	// 验证标签被过滤（注意：emotion 标签不会被 TextFilterStage 过滤）
	expected := "I'm <emotion>happy</emotion> !"
	if msg.Payload != expected {
		t.Errorf("Expected '%s', got '%s'", expected, msg.Payload)
	}
}
