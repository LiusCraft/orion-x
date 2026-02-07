import { useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import MessageItem from './MessageItem.jsx';

const BOTTOM_THRESHOLD = 100; // 距离底部多少像素内认为是"在底部"

export default function MessageList({ messages }) {
  const scrollRef = useRef(null);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const prevLengthRef = useRef(0);

  // 检测是否在底部
  const checkIsAtBottom = () => {
    const el = scrollRef.current;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_THRESHOLD;
  };

  // 滚动到底部
  const scrollToBottom = (smooth = true) => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({
      top: el.scrollHeight,
      behavior: smooth ? 'smooth' : 'auto',
    });
  };

  // 监听滚动事件
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const handleScroll = () => {
      setIsAtBottom(checkIsAtBottom());
    };

    el.addEventListener('scroll', handleScroll);
    return () => el.removeEventListener('scroll', handleScroll);
  }, []);

  // 当消息数量变化时，如果在底部则滚动到最新
  useEffect(() => {
    const currentLength = messages.length;
    const prevLength = prevLengthRef.current;

    // 只有在新增消息时才自动滚动
    if (currentLength > prevLength && isAtBottom) {
      scrollToBottom();
    }

    prevLengthRef.current = currentLength;
  }, [messages.length, isAtBottom]);

  // 初始加载时滚动到底部
  useEffect(() => {
    if (messages.length > 0) {
      scrollToBottom(false);
    }
  }, []);

  if (!messages.length) {
    return (
      <Box sx={{ flex: 1, display: 'flex', alignItems: 'center' }}>
        <Stack spacing={1}>
          <Box sx={{ color: 'text.secondary' }}>还没有消息</Box>
          <Box sx={{ color: 'text.disabled', fontSize: 12 }}>
            发送第一条消息开始对话
          </Box>
        </Stack>
      </Box>
    );
  }

  return (
    <Box ref={scrollRef} sx={{ flex: 1, overflowY: 'auto', pr: 2 }}>
      <Stack sx={{ py: 2 }}>
        {messages.map((msg) => (
          <MessageItem key={msg.id} message={msg} />
        ))}
      </Stack>
    </Box>
  );
}
