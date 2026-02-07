import { useRef } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import MicIcon from "@mui/icons-material/Mic";
import PhoneIcon from "@mui/icons-material/Phone";

const defaultSizes = {
    container: 88,
    button: 68,
};

const compactSizes = {
    container: 44,
    button: 36,
};

const LONG_PRESS_THRESHOLD = 200; // 毫秒，超过此时间为长按

export default function VoiceOrb({
    active,
    calling,
    onPressStart,
    onPressEnd,
    onClick,
    disabled,
    size = "default",
}) {
    const sizes = size === "compact" ? compactSizes : defaultSizes;
    const pressTimerRef = useRef(null);
    const isLongPressRef = useRef(false);

    const handleStart = () => {
        if (disabled) return;
        isLongPressRef.current = false;
        pressTimerRef.current = setTimeout(() => {
            isLongPressRef.current = true;
            onPressStart?.();
        }, LONG_PRESS_THRESHOLD);
    };

    const handleEnd = () => {
        if (disabled) return;
        clearTimeout(pressTimerRef.current);
        if (isLongPressRef.current) {
            onPressEnd?.();
        } else {
            onClick?.();
        }
    };

    const handleLeave = () => {
        if (disabled) return;
        clearTimeout(pressTimerRef.current);
        if (isLongPressRef.current) {
            onPressEnd?.();
        }
    };

    return (
        <Box
            sx={{
                position: "relative",
                width: sizes.container,
                height: sizes.container,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                "@keyframes pulse": {
                    "0%": { transform: "scale(1)", opacity: 0.6 },
                    "70%": { transform: "scale(1.35)", opacity: 0 },
                    "100%": { transform: "scale(1.35)", opacity: 0 },
                },
            }}
        >
            {(active || calling) && (
                <Box
                    sx={{
                        position: "absolute",
                        width: sizes.container,
                        height: sizes.container,
                        borderRadius: "50%",
                        border: calling
                            ? "1px solid rgba(76,175,80,0.6)"
                            : "1px solid rgba(77,163,255,0.6)",
                        animation: "pulse 1.6s ease-out infinite",
                    }}
                />
            )}
            <IconButton
                disabled={disabled}
                onMouseDown={handleStart}
                onMouseUp={handleEnd}
                onMouseLeave={handleLeave}
                onTouchStart={handleStart}
                onTouchEnd={handleEnd}
                sx={{
                    width: sizes.button,
                    height: sizes.button,
                    bgcolor: calling
                        ? "#4CAF50"
                        : active
                        ? "primary.main"
                        : "rgba(255,255,255,0.08)",
                    color: calling || active ? "#0E1116" : "text.primary",
                    boxShadow: calling
                        ? "0 0 20px rgba(76,175,80,0.5)"
                        : active
                        ? "0 0 20px rgba(77,163,255,0.5)"
                        : "inset 0 0 0 1px rgba(255,255,255,0.08)",
                    "&:hover": {
                        bgcolor: calling
                            ? "#4CAF50"
                            : active
                            ? "primary.main"
                            : "rgba(255,255,255,0.14)",
                    },
                }}
            >
                {calling ? <PhoneIcon /> : <MicIcon />}
            </IconButton>
        </Box>
    );
}
