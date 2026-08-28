import React, { createContext, useCallback, useContext, useState } from "react";

export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}

// ─── Buttons ──────────────────────────────────────────────

type BtnVariant = "primary" | "ghost" | "danger" | "subtle";
const BTN: Record<BtnVariant, string> = {
  primary: "bg-[var(--signal)] hover:bg-[var(--signal-hover)] text-zinc-900 border-[var(--signal)] shadow-md shadow-amber-500/20",
  danger: "bg-red-600 hover:bg-red-500 text-white border-red-500 shadow-md shadow-red-600/20",
  ghost: "bg-transparent hover:bg-white/[0.06] text-zinc-300 border-white/10",
  subtle: "bg-white/[0.06] hover:bg-white/[0.10] text-zinc-200 border-white/10",
};

export function Button({
  variant = "subtle",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BtnVariant }) {
  return (
    <button
      {...props}
      className={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-lg border px-3.5 py-2 text-[13px] font-medium leading-none tracking-wide",
        "shadow-sm transition-all duration-150 active:scale-[0.98] disabled:opacity-40 disabled:cursor-not-allowed disabled:active:scale-100",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--signal)]/40 focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950",
        "min-h-[36px]",
        BTN[variant],
        className,
      )}
    />
  );
}

// ─── Cards / layout ───────────────────────────────────────

export function Card({
  className,
  style,
  children,
}: {
  className?: string;
  style?: React.CSSProperties;
  children: React.ReactNode;
}) {
  return (
    <div style={style} className={cx("rounded-xl border border-[var(--line)] bg-[var(--panel)]/70 p-4 shadow-sm backdrop-blur", className)}>
      {children}
    </div>
  );
}

export function SectionTitle({ children, hint }: { children: React.ReactNode; hint?: string }) {
  return (
    <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
      <h2 className="font-[var(--font-display)] text-[15px] font-semibold tracking-tight text-zinc-100">{children}</h2>
      <h2 className="text-[15px] font-semibold tracking-tight text-zinc-100">{children}</h2>
      {hint && <span className="text-[12px] font-normal tracking-wide text-zinc-500">{hint}</span>}
    </div>
  );
}

// ─── Form controls ────────────────────────────────────────

export function Label({ children }: { children: React.ReactNode }) {
  return <label className="mb-1.5 block text-[11px] font-semibold tracking-widest uppercase text-zinc-400">{children}</label>;
}

const CTRL =
  "w-full rounded-lg border border-[var(--line)] bg-zinc-950/70 px-3.5 py-2 text-[13px] leading-5 text-zinc-100 " +
  "outline-none placeholder:text-zinc-600 focus:border-[var(--signal)]/50 focus:ring-2 focus:ring-[var(--signal)]/20";
  "w-full rounded-lg border border-white/10 bg-zinc-950/70 px-3.5 py-2 text-[13px] leading-5 text-zinc-100 " +
  "outline-none placeholder:text-zinc-600 focus:border-indigo-500/60 focus:ring-2 focus:ring-indigo-500/20";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={cx(CTRL, props.className)} />;
}
export function Textarea(props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className={cx(CTRL, "font-mono", props.className)} />;
}
export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={cx(CTRL, props.className)} />;
}

export function Switch({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={onChange}
      className={cx(
        "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-150",
        "focus:outline-none focus:ring-2 focus:ring-[var(--signal)]/40 focus:ring-offset-2 focus:ring-offset-zinc-950",
        checked ? "bg-[var(--signal)]" : "bg-zinc-700",
      )}
    >
      <span
        className={cx(
          "pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-150",
          checked ? "translate-x-5" : "translate-x-0",
        )}
      />
    </button>
  );
}

export function Field({
  label,
  children,
}: {
  label: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-3">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

export function Badge({
  children,
  tone = "zinc",
}: {
  children: React.ReactNode;
  tone?: "zinc" | "green" | "red" | "indigo" | "amber";
}) {
  const tones: Record<string, string> = {
    zinc: "bg-white/[0.06] text-zinc-300 border-white/10",
    green: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
    red: "bg-red-500/15 text-red-300 border-red-500/30",
    indigo: "bg-indigo-500/15 text-indigo-300 border-indigo-500/30",
    amber: "bg-amber-500/15 text-amber-300 border-amber-500/30",
  };
  return (
    <span
      className={cx(
        "inline-flex items-center rounded-full border px-2.5 py-0.5 text-[11px] font-medium tracking-wide",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}

// ─── Modal ────────────────────────────────────────────────

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-2 py-4 sm:p-4 sm:py-10"
      onMouseDown={onClose}
    >
      <div
        className={cx(
          "my-auto w-full rounded-xl border border-white/10 bg-zinc-900 p-4 shadow-2xl sm:rounded-2xl sm:p-5",
          "max-h-[92vh] overflow-y-auto overscroll-contain",
          wide ? "max-w-3xl" : "max-w-xl",
        )}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between gap-3">
          <h3 className="min-w-0 text-base font-semibold text-zinc-100">{title}</h3>
          <button onClick={onClose} className="shrink-0 text-zinc-500 hover:text-zinc-200" aria-label="Close">
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// ─── Toasts ───────────────────────────────────────────────

type Toast = { id: number; kind: "ok" | "err"; msg: string };
const ToastCtx = createContext<(kind: "ok" | "err", msg: string) => void>(() => {});
export const useToast = () => useContext(ToastCtx);

let toastSeq = 1;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((kind: "ok" | "err", msg: string) => {
    const id = toastSeq++;
    setToasts((t) => [...t, { id, kind, msg }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4200);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-4 left-2 right-2 z-[60] flex flex-col gap-2 sm:left-auto sm:right-4 sm:w-80">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cx(
              "rounded-lg border px-3 py-2 text-sm shadow-lg",
              t.kind === "ok"
                ? "border-emerald-500/40 bg-emerald-950/80 text-emerald-200"
                : "border-red-500/40 bg-red-950/80 text-red-200",
            )}
          >
            {t.msg}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

/** Wrap an async action with toast feedback + optional refresh. */
export function useAction() {
  const toast = useToast();
  return useCallback(
    async (fn: () => Promise<unknown>, okMsg?: string, after?: () => void) => {
      try {
        await fn();
        if (okMsg) toast("ok", okMsg);
        after?.();
      } catch (e) {
        toast("err", e instanceof Error ? e.message : String(e));
      }
    },
    [toast],
  );
}
