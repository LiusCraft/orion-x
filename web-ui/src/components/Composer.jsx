import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import SendIcon from "@mui/icons-material/Send";
import VoiceOrb from "./VoiceOrb.jsx";
import WaveBars from "./WaveBars.jsx";

export default function Composer({
    inputValue,
    onInputChange,
    onSend,
    onPressStart,
    onPressEnd,
    onToggleCall,
    listening,
    calling,
    disabled,
}) {
    return (
        <Box
            sx={{
                p: 2,
                borderRadius: 3,
                bgcolor: "rgba(255,255,255,0.04)",
                border: "1px solid rgba(255,255,255,0.08)",
            }}
        >
            <Stack
                direction="row"
                spacing={1.5}
                alignItems="center"
                sx={{ flexWrap: "wrap" }}
            >
                <TextField
                    sx={{ flex: "1 1 240px", minWidth: 0 }}
                    placeholder="输入消息..."
                    size="small"
                    value={inputValue}
                    onChange={(e) => onInputChange(e.target.value)}
                    disabled={disabled}
                    onKeyDown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) {
                            e.preventDefault();
                            onSend();
                        }
                    }}
                />
                <Stack
                    direction="row"
                    spacing={1}
                    alignItems="center"
                    sx={{ flex: "0 0 auto" }}
                >
                    {inputValue.trim() === "" ? (
                        <VoiceOrb
                            size="compact"
                            active={listening}
                            calling={calling}
                            onPressStart={onPressStart}
                            onPressEnd={onPressEnd}
                            onClick={onToggleCall}
                            disabled={disabled}
                        />
                    ) : (
                        <Box
                            sx={{
                                width: 44,
                                height: 44,
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                            }}
                        >
                            <IconButton
                                disabled={disabled}
                                onClick={onSend}
                                sx={{
                                    width: 36,
                                    height: 36,
                                    bgcolor: "primary.main",
                                    color: "#0E1116",
                                    boxShadow: "0 0 20px rgba(77,163,255,0.5)",
                                    "&:hover": {
                                        bgcolor: "primary.main",
                                    },
                                }}
                            >
                                <SendIcon sx={{ fontSize: 18 }} />
                            </IconButton>
                        </Box>
                    )}
                </Stack>
            </Stack>

            <Stack
                direction="row"
                spacing={1.5}
                alignItems="center"
                sx={{ mt: 1.5 }}
            >
                <Typography variant="caption" color="text.secondary">
                    {calling
                        ? "通话中持续语音对话"
                        : "点击拨打，长按说话"}
                </Typography>
                <Box sx={{ flex: 1 }} />
                <WaveBars active={listening || calling} />
            </Stack>
        </Box>
    );
}
