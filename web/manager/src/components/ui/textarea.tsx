import * as React from "react";
import { cn } from "@/lib/utils";

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea
    className={cn(
      "flex min-h-[60px] w-full rounded-md border !border-[hsl(var(--border))] !bg-[hsl(var(--card))] px-3 py-2 text-sm !text-[hsl(var(--foreground))] shadow-none transition-[border-color,box-shadow,background-color] placeholder:!text-[hsl(var(--muted-foreground))] hover:!border-[hsl(var(--muted-foreground)/.55)] focus-visible:!border-[hsl(var(--primary))] focus-visible:outline-none focus-visible:!ring-2 focus-visible:!ring-[hsl(var(--primary)/.14)] disabled:cursor-not-allowed disabled:!bg-[hsl(var(--muted))] disabled:opacity-60",
      className,
    )}
    ref={ref}
    {...props}
  />
));
Textarea.displayName = "Textarea";

export { Textarea };
