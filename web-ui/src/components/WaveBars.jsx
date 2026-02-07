import Box from '@mui/material/Box';

const barCount = 12;

export default function WaveBars({ active }) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'flex-end',
        gap: 0.5,
        height: 32,
        opacity: active ? 1 : 0.4,
        transition: 'opacity 0.3s ease',
        '@keyframes wave': {
          '0%, 100%': { height: 6 },
          '50%': { height: 22 }
        }
      }}
    >
      {Array.from({ length: barCount }).map((_, idx) => (
        <Box
          key={idx}
          sx={{
            width: 4,
            height: 6,
            borderRadius: 999,
            bgcolor: 'primary.main',
            animation: active ? 'wave 1.1s ease-in-out infinite' : 'none',
            animationDelay: String(idx * 0.08) + 's'
          }}
        />
      ))}
    </Box>
  );
}
