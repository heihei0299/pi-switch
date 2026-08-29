import { useEffect, useRef } from "react";

export function JsonEditor({
  value,
  onChange,
  label,
  className = "h-80",
}: {
  value: string;
  onChange: (v: string) => void;
  label: string;
  className?: string;
}) {
  const lines = value.split("\n");
  const lineCount = lines.length;
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);

  // Sync scroll between textarea and line numbers
  useEffect(() => {
    const ta = textareaRef.current;
    const ln = lineNumbersRef.current;
    if (!ta || !ln) return;
    const onScroll = () => {
      ln.scrollTop = ta.scrollTop;
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
        {Array.from({ length: lineCount }, (_, i) => (
          <div key={i} className="leading-6">
            {i + 1}
          </div>
        ))}
      </div>
      {/* Editor */}
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
        className="flex-1 resize-none bg-zinc-950 p-3 font-mono text-xs leading-6 text-zinc-300 outline-none"
        style={{ lineHeight: "1.5rem" }}
      />
    </div>
  );
}
