package llm

import (
	"context"
	"io"
)

type legacyGenerationClient struct{ client Client }

func AdaptLegacyClient(client Client) GenerationClient {
	if modern, ok := client.(GenerationClient); ok {
		return modern
	}
	return &legacyGenerationClient{client: client}
}

func (c *legacyGenerationClient) Generate(ctx context.Context, req Request) (Response, error) {
	msg, err := c.client.ChatSync(ctx, req)
	if err != nil {
		return Response{}, err
	}
	msg = msg.Normalize()
	stop := StopReasonStop
	if len(msg.Calls()) > 0 {
		stop = StopReasonToolCalls
	}
	return Response{Message: msg, StopReason: stop}, nil
}

func (c *legacyGenerationClient) Stream(ctx context.Context, req Request) (Stream, error) {
	legacy, err := c.client.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	out := NewEventStream(func() { legacy.Close() })
	go func() {
		defer out.Finish()
		out.Send(Event{Type: EventResponseStart})
		message := Message{Role: string(RoleAssistant)}
		for {
			msg, recvErr := legacy.Recv()
			if recvErr != nil {
				if recvErr != io.EOF {
					out.SendError(recvErr)
				}
				break
			}
			if msg.Content != "" {
				message.Content += msg.Content
				message.Blocks = append(message.Blocks, Block{Type: BlockTypeText, Text: msg.Content})
				if !out.Send(Event{Type: EventTextDelta, TextDelta: msg.Content}) {
					return
				}
			}
			if len(msg.ToolCalls) > 0 {
				message.ToolCalls = msg.ToolCalls
				for _, call := range msg.ToolCalls {
					callCopy := call
					message.Blocks = append(message.Blocks, Block{Type: BlockTypeToolCall, ToolCall: &callCopy})
				}
			}
		}
		stop := StopReasonStop
		if len(message.ToolCalls) > 0 {
			stop = StopReasonToolCalls
		}
		out.Send(Event{Type: EventResponseDone, Response: &Response{Message: message, StopReason: stop}})
	}()
	return out, nil
}
