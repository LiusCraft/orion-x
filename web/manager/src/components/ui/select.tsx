import * as React from "react";
import { Select as SelectPrimitive } from "@base-ui/react/select";

import { cn } from "@/lib/utils";
import { FloatingPortalContainerContext } from "@/components/ui/floating-portal-context";
import { ChevronDownIcon, CheckIcon, ChevronUpIcon } from "lucide-react";

const Select = SelectPrimitive.Root;

interface SimpleSelectOption {
  value: string;
  label: React.ReactNode;
  group?: string;
}

interface SimpleSelectProps {
  value: string;
  onValueChange: (value: string) => void;
  options: SimpleSelectOption[];
  placeholder?: string;
  className?: string;
  size?: "sm" | "default";
  disabled?: boolean;
}

function SimpleSelect({
  value,
  onValueChange,
  options,
  placeholder = "请选择",
  className,
  size = "default",
  disabled,
}: SimpleSelectProps) {
  const selected = options.find((option) => option.value === value);
  const groups = [
    ...new Set(options.map((option) => option.group).filter(Boolean)),
  ] as string[];
  const ungrouped = options.filter((option) => !option.group);

  return (
    <Select
      value={value || "__empty"}
      onValueChange={(next) =>
        onValueChange(next === "__empty" ? "" : (next ?? ""))
      }
      disabled={disabled}
    >
      <SelectTrigger className={cn("w-full", className)} size={size}>
        <span
          className={cn(
            "min-w-0 flex-1 truncate text-left",
            !selected && "text-[hsl(var(--muted-foreground))]",
          )}
        >
          {selected?.label ?? placeholder}
        </span>
      </SelectTrigger>
      <SelectContent>
        {!value && <SelectItem value="__empty">{placeholder}</SelectItem>}
        {ungrouped.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
        {groups.map((group) => (
          <SelectGroup key={group}>
            <SelectLabel>{group}</SelectLabel>
            {options
              .filter((option) => option.group === group)
              .map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  );
}

function SelectGroup({ className, ...props }: SelectPrimitive.Group.Props) {
  return (
    <SelectPrimitive.Group
      data-slot="select-group"
      className={cn("scroll-my-1 p-1", className)}
      {...props}
    />
  );
}

function SelectValue({ className, ...props }: SelectPrimitive.Value.Props) {
  return (
    <SelectPrimitive.Value
      data-slot="select-value"
      className={cn("min-w-0 flex-1 truncate text-left", className)}
      {...props}
    />
  );
}

function SelectTrigger({
  className,
  size = "default",
  children,
  ...props
}: SelectPrimitive.Trigger.Props & {
  size?: "sm" | "default";
}) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      data-size={size}
      className={cn(
        "flex min-w-0 max-w-full w-fit items-center justify-between gap-1.5 overflow-hidden rounded-md border py-2 pr-2 pl-3 text-sm whitespace-nowrap outline-none select-none",
        "!border-[hsl(var(--border))] !bg-[hsl(var(--card))] !text-[hsl(var(--foreground))] shadow-none transition-[border-color,box-shadow,background-color]",
        "hover:!border-[hsl(var(--muted-foreground)/.55)]",
        "focus-visible:!border-[hsl(var(--primary))] focus-visible:!ring-2 focus-visible:!ring-[hsl(var(--primary)/.14)]",
        "disabled:cursor-not-allowed disabled:!bg-[hsl(var(--muted))] disabled:opacity-60",
        "data-[size=default]:h-9 data-[size=sm]:h-7",
        "data-placeholder:text-zinc-500",
        "[&>span]:min-w-0 [&>span]:overflow-hidden [&>span]:text-ellipsis",
        "*:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-1.5",
        className,
      )}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon
        render={
          <ChevronDownIcon className="pointer-events-none size-4 text-[hsl(var(--muted-foreground))]" />
        }
      />
    </SelectPrimitive.Trigger>
  );
}

function SelectContent({
  className,
  children,
  side = "bottom",
  sideOffset = 4,
  align = "start",
  alignOffset = 0,
  alignItemWithTrigger = false,
  collisionAvoidance = { side: "none", align: "none" },
  disablePortal = false,
  positionMethod = "fixed",
  ...props
}: SelectPrimitive.Popup.Props &
  Pick<
    SelectPrimitive.Positioner.Props,
    | "align"
    | "alignOffset"
    | "side"
    | "sideOffset"
    | "alignItemWithTrigger"
    | "collisionAvoidance"
    | "positionMethod"
  > & { disablePortal?: boolean }) {
  const portalContainer = React.useContext(FloatingPortalContainerContext);
  const content = (
    <SelectPrimitive.Positioner
      side={side}
      sideOffset={sideOffset}
      align={align}
      alignOffset={alignOffset}
      alignItemWithTrigger={alignItemWithTrigger}
      collisionAvoidance={collisionAvoidance}
      positionMethod={positionMethod}
      className="isolate z-50"
    >
      <SelectPrimitive.Popup
        data-slot="select-content"
        data-align-trigger={alignItemWithTrigger}
        className={cn(
          "relative isolate z-50 max-h-(--available-height) w-[var(--anchor-width)] max-w-[var(--available-width)] origin-(--transform-origin) overflow-x-hidden overflow-y-auto rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--foreground))] shadow-lg p-1 duration-100 data-[align-trigger=true]:animate-none data-[side=bottom]:slide-in-from-top-2 data-[side=inline-end]:slide-in-from-left-2 data-[side=inline-start]:slide-in-from-right-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
          className,
        )}
        {...props}
      >
        <SelectScrollUpButton />
        <SelectPrimitive.List>{children}</SelectPrimitive.List>
        <SelectScrollDownButton />
      </SelectPrimitive.Popup>
    </SelectPrimitive.Positioner>
  );
  return disablePortal ? (
    content
  ) : (
    <SelectPrimitive.Portal container={portalContainer}>
      {content}
    </SelectPrimitive.Portal>
  );
}

function SelectLabel({
  className,
  ...props
}: SelectPrimitive.GroupLabel.Props) {
  return (
    <SelectPrimitive.GroupLabel
      data-slot="select-label"
      className={cn("px-1.5 py-1 text-xs text-muted-foreground", className)}
      {...props}
    />
  );
}

function SelectItem({
  className,
  children,
  ...props
}: SelectPrimitive.Item.Props) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        "relative flex w-full cursor-default items-center gap-1.5 overflow-hidden rounded-md py-1.5 pr-8 pl-2 text-sm outline-hidden select-none",
        "text-[hsl(var(--foreground))]",
        "data-highlighted:bg-[hsl(var(--accent))] data-highlighted:text-[hsl(var(--accent-foreground))]",
        "data-disabled:pointer-events-none data-disabled:opacity-40",
        "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2",
        className,
      )}
      title={typeof children === "string" ? children : undefined}
      {...props}
    >
      <SelectPrimitive.ItemText className="min-w-0 flex-1 truncate">
        {children}
      </SelectPrimitive.ItemText>
      <SelectPrimitive.ItemIndicator
        render={
          <span className="pointer-events-none absolute right-2 flex size-4 items-center justify-center" />
        }
      >
        <CheckIcon className="pointer-events-none" />
      </SelectPrimitive.ItemIndicator>
    </SelectPrimitive.Item>
  );
}

function SelectSeparator({
  className,
  ...props
}: SelectPrimitive.Separator.Props) {
  return (
    <SelectPrimitive.Separator
      data-slot="select-separator"
      className={cn("pointer-events-none -mx-1 my-1 h-px bg-border", className)}
      {...props}
    />
  );
}

function SelectScrollUpButton({
  className,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.ScrollUpArrow>) {
  return (
    <SelectPrimitive.ScrollUpArrow
      data-slot="select-scroll-up-button"
      className={cn(
        "top-0 z-10 flex w-full cursor-default items-center justify-center bg-popover py-1 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <ChevronUpIcon />
    </SelectPrimitive.ScrollUpArrow>
  );
}

function SelectScrollDownButton({
  className,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.ScrollDownArrow>) {
  return (
    <SelectPrimitive.ScrollDownArrow
      data-slot="select-scroll-down-button"
      className={cn(
        "bottom-0 z-10 flex w-full cursor-default items-center justify-center bg-popover py-1 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <ChevronDownIcon />
    </SelectPrimitive.ScrollDownArrow>
  );
}

export {
  Select,
  SimpleSelect,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
};
