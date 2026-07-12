import { useCallback, useRef } from "react";

export function useShiftSelect(
  selected: string[],
  setSelected: (files: string[]) => void,
) {
  const lastClickedIndex = useRef<number | null>(null);

  const handleToggle = useCallback((
    index: number,
    sortedNames: string[],
    shiftKey: boolean,
  ) => {
    const name = sortedNames[index];
    const wasSelected = selected.includes(name);
    const selecting = !wasSelected;

    if (shiftKey && lastClickedIndex.current !== null) {
      const from = Math.min(lastClickedIndex.current, index);
      const to = Math.max(lastClickedIndex.current, index);
      const rangeNames = sortedNames.slice(from, to + 1);

      let next: string[];
      if (selecting) {
        const asSet = new Set(selected);
        for (const n of rangeNames) asSet.add(n);
        next = Array.from(asSet);
      } else {
        const removeSet = new Set(rangeNames);
        next = selected.filter((n) => !removeSet.has(n));
      }
      setSelected(next);
    } else {
      if (selecting) {
        setSelected([...selected, name]);
      } else {
        setSelected(selected.filter((n) => n !== name));
      }
    }

    lastClickedIndex.current = index;
  }, [selected, setSelected]);

  const resetLastClicked = useCallback(() => {
    lastClickedIndex.current = null;
  }, []);

  return { handleToggle, resetLastClicked };
}
