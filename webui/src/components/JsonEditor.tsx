import { useEffect, useRef } from "react";

export function JsonEditor({
  value,
  onChange,
  label,
  className = "h-80",
  errorLine = null,
}: {
  value: string;
  onChange: (v: string) => void;
  label: string;
  className?: string;
  errorLine?: number | null;
}) {
  const lines = value.split("\n");
  const lineCount = lines.length;
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);
  const highlightRef = useRef<HTMLDivElement>(null);

  // Sync scroll between textarea and line numbers + highlight layer
  useEffect(() => {
    const ta = textareaRef.current;
    const ln = lineNumbersRef.current;
    const hl = highlightRef.current;
    if (!ta) return;
    const onScroll = () => {
      if (ln) ln.scrollTop = ta.scrollTop;
      if (hl) hl.scrollTop = ta.scrollTop;
    };
    ta.addEventListener("scroll", onScroll);
    return () => ta.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <div className={`relative flex overflow-hidden rounded-lg border border-white/10 bg-zinc-950 ${className}`}>
      {/* Line numbers */}
      <div
        ref={lineNumbersRef}
        className="select-none overflow-hidden bg-zinc-900/50 px-3 py-3 text-right font-mono text-xs leading-6 text-zinc-500"
        style={{ minWidth: "3rem" }}
        aria-hidden="true"
      >
        {Array.from({ length: lineCount }, (_, i) => {
          const isErr = errorLine != null && i + 1 === errorLine;
          return (
            <div key={i} className={`leading-6 ${isErr ? "bg-red-500/10 text-red-400" : ""}`}>
              {i + 1}
            </div>
          );
        })}
      </div>
      {/* Editor */}
      <div className="relative flex-1 overflow-hidden">
        {/* Highlight layer behind textarea */}
        <div
          ref={highlightRef}
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 overflow-hidden p-3 font-mono text-xs leading-6"
          style={{ lineHeight: "1.5rem" }}
        >
          {Array.from({ length: lineCount }, (_, i) => {
            const isErr = errorLine != null && i + 1 === errorLine;
            return (
              <div key={i} className={`leading-6 whitespace-pre ${isErr ? "bg-red-500/10" : "text-transparent"}`} style={{ lineHeight: "1.5rem" }}>
                {"\u00A0"}
              </div>
            );
          })}
        </div>
        <textarea
          ref={textareaRef}
          aria-label={label}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          spellCheck={false}
          autoComplete="off"
          autoCorrect="off"
          data-gramm="false"
          data-gramm_editor="false"
          data-enable-grammarly="false"
          className="absolute inset-0 h-full w-full resize-none bg-transparent p-3 font-mono text-xs leading-6 text-zinc-300 outline-none"
          style={{ lineHeight: "1.5rem" }}
        />
      </div>
    </div>
  );
}
