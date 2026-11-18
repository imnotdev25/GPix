import { useEffect, useMemo, useRef, useState } from "react";
import type { TimelineMonth } from "../types";

const monthName = (m: TimelineMonth) =>
  new Date(m.year, m.month - 1, 1).toLocaleDateString(undefined, { month: "long", year: "numeric" });

/** Date scrubber along the right edge. Years are its stops; drag or click to
 *  jump to a month, and a bubble shows the month you're at while scrolling. */
export default function TimelineRail({
  months,
  total,
  position,
  onJump,
}: {
  months: TimelineMonth[];
  total: number;
  /** 0..1: how far down the whole library the top of the screen is */
  position: number;
  onJump: (m: TimelineMonth) => void;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const [peek, setPeek] = useState(false);
  const drag = useRef(false);
  const railRef = useRef<HTMLDivElement>(null);
  const peekTimer = useRef(0);

  // Where each month starts along the rail, by item count.
  const starts = useMemo(() => {
    const out: number[] = [];
    let acc = 0;
    for (const m of months) {
      out.push(total > 0 ? acc / total : 0);
      acc += m.count;
    }
    return out;
  }, [months, total]);

  const monthAt = (f: number) => {
    let i = 0;
    while (i + 1 < starts.length && starts[i + 1] <= f) i++;
    return i;
  };

  const pos = Math.min(Math.max(position, 0), 1);
  const current = monthAt(pos);

  // Show the month briefly whenever scrolling moves into another one.
  useEffect(() => {
    setPeek(true);
    window.clearTimeout(peekTimer.current);
    peekTimer.current = window.setTimeout(() => setPeek(false), 1200);
  }, [current]);
  useEffect(() => () => window.clearTimeout(peekTimer.current), []);

  // One label per year, skipping any that would overlap the previous one.
  const years = useMemo(() => {
    const out: { label: string; f: number; i: number }[] = [];
    let lastF = -1;
    months.forEach((m, i) => {
      if (i > 0 && months[i - 1].year === m.year) return;
      if (lastF >= 0 && starts[i] - lastF < 0.035) return;
      out.push({ label: String(m.year), f: starts[i], i });
      lastF = starts[i];
    });
    return out;
  }, [months, starts]);

  const fracAt = (y: number) => {
    const r = railRef.current?.getBoundingClientRect();
    return r && r.height > 0 ? Math.min(Math.max((y - r.top) / r.height, 0), 0.9999) : 0;
  };

  if (months.length === 0) return null;
  const shown = hover ?? (peek ? current : null);
  const thumbF = hover !== null ? starts[hover] : pos;

  return (
    <div
      ref={railRef}
      className={"rail" + (hover !== null ? " active" : "")}
      role="slider"
      tabIndex={0}
      aria-label="Jump to date"
      aria-valuemin={0}
      aria-valuemax={months.length - 1}
      aria-valuenow={current}
      aria-valuetext={monthName(months[current])}
      onPointerDown={(e) => {
        drag.current = true;
        try {
          e.currentTarget.setPointerCapture(e.pointerId);
        } catch {
          /* pointer already gone */
        }
        setHover(monthAt(fracAt(e.clientY)));
      }}
      onPointerMove={(e) => {
        if (drag.current || e.pointerType === "mouse") setHover(monthAt(fracAt(e.clientY)));
      }}
      onPointerUp={(e) => {
        if (!drag.current) return;
        drag.current = false;
        const i = monthAt(fracAt(e.clientY));
        setHover(null);
        onJump(months[i]);
      }}
      onPointerCancel={() => {
        drag.current = false;
        setHover(null);
      }}
      onPointerLeave={() => {
        if (!drag.current) setHover(null);
      }}
      onKeyDown={(e) => {
        let i = current;
        if (e.key === "ArrowDown" || e.key === "PageDown") i = Math.min(current + 1, months.length - 1);
        else if (e.key === "ArrowUp" || e.key === "PageUp") i = Math.max(current - 1, 0);
        else if (e.key === "Home") i = 0;
        else if (e.key === "End") i = months.length - 1;
        else return;
        e.preventDefault();
        onJump(months[i]);
      }}
    >
      {years.map((y) => (
        <span key={y.i} className="rail-year" style={{ top: `${y.f * 100}%` }}>
          {y.label}
        </span>
      ))}
      <span className="rail-thumb" style={{ top: `${thumbF * 100}%` }} />
      {shown !== null && (
        <span className="rail-bubble" style={{ top: `${thumbF * 100}%` }}>
          {monthName(months[shown])}
        </span>
      )}
    </div>
  );
}
