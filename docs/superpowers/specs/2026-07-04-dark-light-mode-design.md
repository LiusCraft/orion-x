# Dark/Light Mode + Top Navigation Bar

## Motivation

The web manager at `web/manager/` currently uses hardcoded dark-mode Tailwind classes across all components. The CSS infrastructure for both themes (CSS variables in `index.css` for `:root` and `.dark`) already exists but is not wired up. This spec introduces a proper theme system and a top navigation bar.

## Architecture

```
ThemeProvider (React Context)
  └─ Layout
       ├─ Top Bar
       │   ├─ Left: Notification icon (placeholder)
       │   └─ Right: Theme toggle (Sun/Moon)
       ├─ Sidebar (unchanged)
       └─ Content (<Outlet />)
```

ThemeProvider reads theme preference from localStorage, falling back to `prefers-color-scheme`, and toggles `.dark` class on `<html>`.

## Files

### New file: `src/components/ThemeProvider.tsx`
- React Context: `{ theme: 'light' | 'dark', setTheme }`
- On mount: `localStorage.getItem('theme')` → `matchMedia('(prefers-color-scheme: dark)')` → default `'dark'`
- On change: toggle `document.documentElement.classList`, persist to localStorage
- Uses Zustand or plain React state (prefer plain React for simplicity)

### Modified: `src/main.tsx`
- Wrap `<App />` with `<ThemeProvider>`

### Modified: `src/components/Layout.tsx`
- Restructure: `flex flex-col h-screen` → top bar + `flex flex-1` (sidebar + content)
- Top bar: fixed height `h-14`, `border-b`, `flex items-center justify-between`, `px-6`
- Left: `<Bell />` icon (lucide-react), `text-muted-foreground hover:text-foreground`
- Right: `<Sun />` / `<Moon />` toggle button, calls `setTheme`
- Sidebar nav groups remain unchanged

### Modified: `src/index.css`
- No structural changes — `:root` and `.dark` blocks already exist
- Verify all CSS variable values produce acceptable light-mode appearance
- Add `@media (prefers-color-scheme: dark)` as override for initial load flash (optional)

### Modified: all page components (25 files)
Replace hardcoded dark-mode Tailwind classes with shadcn CSS variable classes:

| Current | Replacement |
|---|---|
| `bg-zinc-950` | `bg-background` |
| `bg-zinc-900` | `bg-card` |
| `bg-zinc-800` | `bg-muted` |
| `bg-zinc-700` | `bg-accent` |
| `text-white` | `text-foreground` or `text-card-foreground` |
| `text-zinc-200/300/400` | `text-muted-foreground` |
| `text-zinc-500/600` | `text-muted-foreground/60` |
| `border-zinc-800/700` | `border-border` |
| `border-zinc-600` | `border-border/80` |
| `placeholder:text-zinc-500/600` | `placeholder:text-muted-foreground` |
| `hover:bg-zinc-800` | `hover:bg-accent` |

## Theme Behavior
1. First visit: detect `prefers-color-scheme`, apply `.dark` class if needed
2. Manual toggle: save to `localStorage.theme`, override system preference
3. Class-based: the `.dark` class on `<html>` activates the `.dark` CSS variable block

## Notification Icon
- Lucide `<Bell />` icon, no click handler or notification badge for now
- Pure placeholder for future notification system

## Non-goals
- No notification system / polling / WebSocket
- No sidebar restructuring
- No change to shadcn/ui component files (they already use CSS variables)
- No Tailwind config changes (Tailwind v4 uses CSS-based config)
