import * as React from "react";
import { cn } from "@/lib/utils";

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  variant?: "default" | "unstyled";
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, variant = "default", ...props }, ref) => (
    <input
      type={type}
      className={cn(
        variant === "default" &&
          "flex h-9 w-full rounded-md border !border-[hsl(var(--border))] !bg-[hsl(var(--card))] px-3 py-1 text-sm !text-[hsl(var(--foreground))] shadow-none transition-[border-color,box-shadow,background-color] file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:!text-[hsl(var(--muted-foreground))] hover:!border-[hsl(var(--muted-foreground)/.55)] focus-visible:!border-[hsl(var(--primary))] focus-visible:outline-none focus-visible:!ring-2 focus-visible:!ring-[hsl(var(--primary)/.14)] disabled:cursor-not-allowed disabled:!bg-[hsl(var(--muted))] disabled:opacity-60",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";

export { Input };
